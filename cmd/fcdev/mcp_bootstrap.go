package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/mcp"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// mcpLocalClientID is the fixed client_id for the local-dev MCP OAuth client.
// Stable across environments so a shared MCP client config works.
const mcpLocalClientID = "flowcatalyst-mcp-local"

// mcpBootstrapRole is the role the local MCP account holds. It stays
// platform:super-admin because no single seeded role covers every read the MCP
// tools and resources make: platform:viewer covers all of them except
// platform:developer:application-openapi:view (get_openapi and
// get_application_capabilities, GET /bff/developer/applications/{id}/openapi/current),
// which only platform:admin and platform:developer hold, and those lack the
// subscription and role views. TestMCPBootstrapRole pins this; when a seeded
// role covers the set, it fails and this should become that role.
const mcpBootstrapRole = "platform:super-admin"

// bootstrapMCPCredentials idempotently provisions a local MCP OAuth client so
// `fcdev mcp` (and `fcdev start --mcp`) authenticate with zero config.
// If the client already exists it is a
// no-op; otherwise it creates a SERVICE principal (super-admin, anchor scope) +
// service account + a CONFIDENTIAL client_credentials OAuth client, then writes
// the plaintext secret to mcp-credentials.json in the OS cache dir (macOS
// ~/Library/Caches, Linux ~/.cache; see mcp.CredentialsPath), mode 0600.
//
// Dev-only: it is called only from `fcdev start`, which is the dev monolith.
// Requires a stable FLOWCATALYST_APP_KEY (ensured by start.go) so the stored
// encrypted secret stays decryptable at /oauth/token across restarts.
func bootstrapMCPCredentials(ctx context.Context, pool *pgxpool.Pool, baseURL string) error {
	authRepo := auth.NewRepository(pool)

	existing, err := authRepo.OAuthClients.FindByClientID(ctx, mcpLocalClientID)
	if err != nil {
		return fmt.Errorf("look up MCP client: %w", err)
	}
	enc, err := encryption.FromEnv()
	if err != nil {
		return fmt.Errorf("init encryption: %w", err)
	}
	if enc == nil {
		return fmt.Errorf("FLOWCATALYST_APP_KEY not set; cannot encrypt MCP client secret")
	}

	if existing != nil {
		// Already provisioned on a prior boot. The credentials file normally
		// persists from the first run, but it can go missing or be overwritten
		// (another fcdev instance on this machine writes the same path unless
		// FC_MCP_CREDENTIALS_PATH is set), so make it match this database.
		return healMCPCredentialsFile(enc, existing.SecretRef, baseURL)
	}

	secret, err := generateSecret()
	if err != nil {
		return err
	}
	secretRef, err := enc.Encrypt(secret)
	if err != nil {
		return fmt.Errorf("encrypt MCP client secret: %w", err)
	}

	sa := serviceaccount.New("mcp:local", "fc-mcp local")
	saDesc := "Local MCP service account (provisioned by fcdev)"
	sa.Description = &saDesc

	saPrincipal := principal.NewService(sa.ID, "fc-mcp local")
	saPrincipal.Scope = principal.ScopeAnchor

	oauthClient := auth.NewOAuthClient(mcpLocalClientID, "FlowCatalyst MCP (local dev)", auth.OAuthClientConfidential)
	oauthClient.SecretRef = &secretRef
	oauthClient.GrantTypes = []string{"client_credentials"}
	oauthClient.PrincipalID = ids.StringPtr(&saPrincipal.ID)

	principalRepo := principal.NewRepository(pool)
	saRepo := serviceaccount.NewRepository(pool)
	if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
		if err := principalRepo.Persist(ctx, saPrincipal, tx); err != nil {
			return fmt.Errorf("mcp principal: %w", err)
		}
		if err := saRepo.Persist(ctx, sa, tx); err != nil {
			return fmt.Errorf("mcp service account: %w", err)
		}
		if err := authRepo.OAuthClients.Persist(ctx, oauthClient, tx); err != nil {
			return fmt.Errorf("mcp oauth client: %w", err)
		}
		// principal.Persist doesn't sync iam_principal_roles; grant the role
		// directly (see mcpBootstrapRole).
		_, err := tx.Inner().Exec(ctx,
			`INSERT INTO iam_principal_roles
			     (principal_id, role_name, assignment_source, assigned_at)
			 VALUES ($1, $2, 'BOOTSTRAP', NOW())
			 ON CONFLICT DO NOTHING`,
			saPrincipal.ID, mcpBootstrapRole)
		return err
	}); err != nil {
		return err
	}

	if err := mcp.WriteCredentialsFile(mcpLocalClientID, secret, baseURL); err != nil {
		return fmt.Errorf("write MCP credentials file: %w", err)
	}
	path, _ := mcp.CredentialsPath()
	slog.Info("bootstrapped local MCP credentials", "client_id", mcpLocalClientID, "credentials", path)
	return nil
}

// ensureAppKeyFile reads, or generates and persists (0600), the field-encryption
// key (FLOWCATALYST_APP_KEY) so OAuth client secrets — including the bootstrapped
// MCP client — stay decryptable across restarts. Analogous to the JWT signing
// key file. fc-server requires operators to supply the key via env instead.
func ensureAppKeyFile(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil {
		if k := strings.TrimSpace(string(b)); k != "" {
			return k, nil
		}
	}
	key, err := encryption.GenerateKey()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(key), 0o600); err != nil {
		return "", err
	}
	return key, nil
}

// appKeyPath is where fcdev persists the field-encryption key: beside the
// embedded Postgres data directory, whichever database that instance
// ultimately connects to.
func appKeyPath(embeddedDBPath string) string {
	return filepath.Join(filepath.Dir(embeddedDBPath), "app-key")
}

// resolveAppKey is the single resolution every fcdev command that encrypts or
// decrypts client secrets must use, so `fcdev init` agrees with the running
// instance: FLOWCATALYST_APP_KEY in the environment wins; else the persisted
// key file; else a new key is generated and persisted (0600). The result is
// exported to the process environment for encryption.FromEnv.
func resolveAppKey(embeddedDBPath string) (string, error) {
	if k := strings.TrimSpace(os.Getenv("FLOWCATALYST_APP_KEY")); k != "" {
		return k, nil
	}
	key, err := ensureAppKeyFile(appKeyPath(embeddedDBPath))
	if err != nil {
		return "", err
	}
	if err := os.Setenv("FLOWCATALYST_APP_KEY", key); err != nil {
		return "", err
	}
	return key, nil
}

// healMCPCredentialsFile rewrites mcp-credentials.json from the stored client
// secret when the file is missing or does not match this database's client
// (other client id, secret or base URL). It changes nothing when they match.
func healMCPCredentialsFile(enc *encryption.Service, secretRef *string, baseURL string) error {
	if secretRef == nil || *secretRef == "" {
		return fmt.Errorf("MCP client %s has no stored secret", mcpLocalClientID)
	}
	secret, err := enc.Decrypt(*secretRef)
	if err != nil {
		return fmt.Errorf("decrypt MCP client secret: %w", err)
	}
	if id, s, u, ok := mcp.ReadCredentialsFile(); ok && id == mcpLocalClientID && s == secret && u == baseURL {
		return nil
	}
	if err := mcp.WriteCredentialsFile(mcpLocalClientID, secret, baseURL); err != nil {
		return fmt.Errorf("rewrite MCP credentials file: %w", err)
	}
	path, _ := mcp.CredentialsPath()
	slog.Info("rewrote local MCP credentials to match this database", "client_id", mcpLocalClientID, "credentials", path)
	return nil
}
