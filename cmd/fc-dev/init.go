package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/config"
	"github.com/flowcatalyst/flowcatalyst-go/internal/migrate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// newInitCmd bootstraps a fresh fc-dev environment.
//
// What it does — mirrors the Rust bin/fc-dev/src/init.rs:
//
//  1. Run migrations + the built-in seeds (idempotent).
//  2. Create the anchor admin if no anchor user exists yet (internal IDP +
//     email-domain mapping + Principal with hashed password + super-admin
//     role assignment).
//  3. Resolve (or create) a Default Client.
//  4. Create the Application (errors if the code is taken).
//  5. Mint the service account: a SERVICE Principal, a ServiceAccount row,
//     attach to the Application, plus a CONFIDENTIAL OAuth client with
//     `client_credentials` grant linked to the SA.
//  6. Write `.env` with FLOWCATALYST_BASE_URL/APP_CODE/CLIENT_ID/CLIENT_SECRET,
//     in-place for existing keys, appended otherwise.
//
// All writes go directly to the repositories (no UoW, no events) — this
// is platform-infrastructure bootstrap, exactly the exception class
// documented in docs/conventions.md §3.
func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a fresh local environment (admin user + default tenant + .env)",
		RunE:  runInit,
	}
	cmd.Flags().String("database-url", envStrDefault("FC_DATABASE_URL", ""), "Postgres URL (defaults to local embedded)")
	cmd.Flags().Bool("yes", false, "non-interactive — fail if any required value is missing from flags")
	cmd.Flags().String("root", ".", "project root for the .env write")

	// admin
	cmd.Flags().String("admin-email", envStrDefault("FC_BOOTSTRAP_ADMIN_EMAIL", ""), "anchor admin email")
	cmd.Flags().String("admin-password", envStrDefault("FC_BOOTSTRAP_ADMIN_PASSWORD", ""), "anchor admin password")

	// application
	cmd.Flags().String("code", "", "application code (URL-safe slug, e.g. \"orders\")")
	cmd.Flags().String("name", "", "application name")
	cmd.Flags().String("app-type", "APPLICATION", "application type: APPLICATION or INTEGRATION")
	cmd.Flags().String("description", "", "application description (optional)")
	cmd.Flags().String("default-base-url", "", "application's deployed base URL (optional)")

	// default client
	cmd.Flags().String("client-identifier", "default", "default client identifier")
	cmd.Flags().String("client-name", "Default Client", "default client display name")

	// .env target
	cmd.Flags().String("api-base-url", "http://localhost:8080", "API base URL written to FLOWCATALYST_BASE_URL")
	return cmd
}

func runInit(cmd *cobra.Command, _ []string) error {
	get := func(k string) string { v, _ := cmd.Flags().GetString(k); return v }
	getBool := func(k string) bool { v, _ := cmd.Flags().GetBool(k); return v }

	dbURL := get("database-url")
	if dbURL == "" {
		dbURL = "postgresql://postgres:postgres@localhost:15432/flowcatalyst?sslmode=disable"
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	pool, err := database.NewPool(ctx, config.DBConfig{URL: dbURL})
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer pool.Close()

	// Migrations + system seeds are idempotent — re-runnable.
	if err := migrate.Run(ctx, pool); err != nil {
		return fmt.Errorf("migrations: %w", err)
	}
	if err := seed.NewSeeder(pool).Run(ctx); err != nil {
		return fmt.Errorf("seed: %w", err)
	}

	fmt.Println("fc-dev init")

	// Reader for interactive prompts (one buffer keeps stdin state clean).
	stdin := bufio.NewReader(os.Stdin)

	// ── 1. Admin user ──────────────────────────────────────────────────
	var anchorExists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM iam_principals
		    WHERE type = 'USER' AND scope = 'ANCHOR')`).Scan(&anchorExists); err != nil {
		return fmt.Errorf("check anchor: %w", err)
	}

	if anchorExists {
		fmt.Println("→ admin user already present, skipping creation")
	} else {
		adminEmail, err := prompt(stdin, get("admin-email"), "Admin email", "", getBool("yes"))
		if err != nil {
			return err
		}
		adminPassword, err := promptSecret(stdin, get("admin-password"), "Admin password", getBool("yes"))
		if err != nil {
			return err
		}
		if err := createAdmin(ctx, pool, adminEmail, adminPassword); err != nil {
			return fmt.Errorf("create admin: %w", err)
		}
		fmt.Printf("  → admin %s created\n", adminEmail)
	}

	// ── 2. Default client ──────────────────────────────────────────────
	clientRepo := client.NewRepository(pool)
	clientIdent := get("client-identifier")
	existingClient, err := clientRepo.FindByIdentifier(ctx, clientIdent)
	if err != nil {
		return fmt.Errorf("find default client: %w", err)
	}
	var defaultClientID string
	if existingClient != nil {
		defaultClientID = existingClient.ID
		fmt.Printf("→ reusing default client \"%s\" (id=%s)\n", existingClient.Identifier, existingClient.ID)
	} else {
		c := client.New(get("client-name"), clientIdent)
		if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
			return clientRepo.Persist(ctx, c, tx)
		}); err != nil {
			return fmt.Errorf("insert default client: %w", err)
		}
		defaultClientID = c.ID
		fmt.Printf("  → default client \"%s\" created (id=%s)\n", c.Identifier, c.ID)
	}

	// ── 3. Application ─────────────────────────────────────────────────
	appCode, err := prompt(stdin, get("code"), "Application code (slug)", "", getBool("yes"))
	if err != nil {
		return err
	}
	appName, err := prompt(stdin, get("name"), "Application name", "", getBool("yes"))
	if err != nil {
		return err
	}
	rawType, err := prompt(stdin, get("app-type"), "Application type [APPLICATION|INTEGRATION]", "APPLICATION", getBool("yes"))
	if err != nil {
		return err
	}
	description := promptOptional(stdin, get("description"), "Description (optional)", getBool("yes"))
	appBaseURL := promptOptional(stdin, get("default-base-url"), "Application's deployed base URL (optional)", getBool("yes"))

	appRepo := application.NewRepository(pool)
	if existing, err := appRepo.FindByCode(ctx, appCode); err != nil {
		return fmt.Errorf("lookup application: %w", err)
	} else if existing != nil {
		return fmt.Errorf("application with code %q already exists (id=%s). Pick a different code or run `fc-dev fresh`", appCode, existing.ID)
	}

	app := application.New(appCode, appName)
	app.Type = application.ParseType(strings.ToUpper(rawType))
	if description != "" {
		app.Description = &description
	}
	if appBaseURL != "" {
		app.DefaultBaseURL = &appBaseURL
	}
	if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
		return appRepo.Persist(ctx, app, tx)
	}); err != nil {
		return fmt.Errorf("insert application: %w", err)
	}
	fmt.Printf("  → application \"%s\" created (id=%s)\n", appCode, app.ID)

	// ── 4. Service account + linked Principal ──────────────────────────
	saCode := "app:" + appCode
	saName := appName + " Service Account"
	sa := serviceaccount.New(saCode, saName)
	saDesc := "Service account for application: " + appName
	sa.Description = &saDesc
	sa.ApplicationID = &app.ID

	saPrincipal := principal.NewService(sa.ID, saName)
	saPrincipal.ApplicationID = &app.ID
	saPrincipal.ClientID = &defaultClientID
	saPrincipal.Scope = principal.ScopeAnchor

	// Attach the SA back to the application + persist all three in one tx.
	// NOTE: app_applications.service_account_id has a FK to iam_principals.id
	// (migration 028) — so we store the SA *principal* id, not the SA row id.
	// This diverges from internal/platform/application/operations/attach_service_account.go
	// which incorrectly stores sa.ID; that's a separate bug to fix (HANDOFF.md).
	app.ServiceAccountID = &saPrincipal.ID
	app.UpdatedAt = time.Now().UTC()

	principalRepo := principal.NewRepository(pool)
	saRepo := serviceaccount.NewRepository(pool)
	if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
		if err := principalRepo.Persist(ctx, saPrincipal, tx); err != nil {
			return fmt.Errorf("sa principal: %w", err)
		}
		if err := saRepo.Persist(ctx, sa, tx); err != nil {
			return fmt.Errorf("service account: %w", err)
		}
		return appRepo.Persist(ctx, app, tx)
	}); err != nil {
		return err
	}
	fmt.Printf("  → service account \"%s\" attached\n", saCode)

	// ── 5. OAuth client for the SA (client_credentials grant) ──────────
	clientSecretPlain, err := generateSecret()
	if err != nil {
		return err
	}
	publicClientID, err := generateSecret() // separate value for the public-facing client_id
	if err != nil {
		return err
	}
	// Client secrets are stored reversibly-encrypted (Rust parity:
	// client_secret_ref), so an app key must exist. Generate + persist one
	// when absent so a fresh dev box works without manual setup.
	appKey := os.Getenv("FLOWCATALYST_APP_KEY")
	if appKey == "" {
		appKey, err = encryption.GenerateKey()
		if err != nil {
			return fmt.Errorf("generate app key: %w", err)
		}
		_ = os.Setenv("FLOWCATALYST_APP_KEY", appKey)
	}
	enc, err := encryption.FromEnv()
	if err != nil {
		return fmt.Errorf("init encryption: %w", err)
	}
	secretRef, err := enc.Encrypt(clientSecretPlain)
	if err != nil {
		return fmt.Errorf("encrypt client secret: %w", err)
	}
	oauthClient := auth.NewOAuthClient(publicClientID, appName+" Service Account Client", auth.OAuthClientConfidential)
	oauthClient.SecretRef = &secretRef
	oauthClient.GrantTypes = []string{"client_credentials"}
	oauthClient.PrincipalID = &saPrincipal.ID

	authRepo := auth.NewRepository(pool)
	if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
		if err := authRepo.OAuthClients.Persist(ctx, oauthClient, tx); err != nil {
			return fmt.Errorf("oauth client: %w", err)
		}
		// oauth_client.Persist doesn't sync the application_ids junction
		// (see HANDOFF.md §7 step 5 OAuthClient follow-up). Write it
		// directly here so the SA's OAuth client is linked to the app.
		_, err := tx.Inner().Exec(ctx,
			`INSERT INTO oauth_client_application_ids (oauth_client_id, application_id)
			 VALUES ($1, $2) ON CONFLICT DO NOTHING`,
			oauthClient.ID, app.ID)
		return err
	}); err != nil {
		return err
	}
	fmt.Printf("  → OAuth client minted (id=%s)\n", oauthClient.ID)

	// ── 6. Write .env ──────────────────────────────────────────────────
	envPath := filepath.Join(get("root"), ".env")
	updates := [][2]string{
		{"FLOWCATALYST_BASE_URL", get("api-base-url")},
		{"FLOWCATALYST_APP_CODE", appCode},
		{"FLOWCATALYST_CLIENT_ID", publicClientID},
		{"FLOWCATALYST_CLIENT_SECRET", clientSecretPlain},
		// Persist the app key so the same encrypted client_secret_ref stays
		// decryptable across restarts (the /oauth/token verify path reads it).
		{"FLOWCATALYST_APP_KEY", appKey},
	}
	if err := writeEnvUpdates(envPath, updates); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}

	fmt.Println()
	fmt.Println("✓ Application scaffolded.")
	fmt.Printf("  Application:     %s (code=%s)\n", appName, appCode)
	fmt.Printf("  Service account: %s\n", sa.ID)
	fmt.Printf("  OAuth client:    %s (clientId=%s)\n", oauthClient.ID, publicClientID)
	fmt.Printf("  Default client:  %s\n", defaultClientID)
	fmt.Println()
	fmt.Printf("  Credentials written to %s. The clientSecret is shown ONLY in\n", envPath)
	fmt.Println("  the .env — the platform stores only the Argon2 hash and cannot")
	fmt.Println("  return it again. Rotate via the OAuth Clients page if needed.")
	return nil
}

// ─── Admin / IDP / EDM bootstrap ───────────────────────────────────────

func createAdmin(ctx context.Context, pool *pgxpool.Pool, email, password string) error {
	email = strings.TrimSpace(strings.ToLower(email))
	at := strings.IndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		return fmt.Errorf("invalid email: %s", email)
	}
	domain := email[at+1:]
	hash := hashSecret(password)

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // committed below on success

	dbtx := usecasepgx.WrapTxForBootstrap(tx)

	// Internal IDP (idempotent by code).
	var idpID string
	idpRow := tx.QueryRow(ctx,
		`SELECT id FROM oauth_identity_providers WHERE code = 'internal'`)
	switch err := idpRow.Scan(&idpID); {
	case err == nil:
		// reuse
	case errors.Is(err, pgx.ErrNoRows):
		idp := identityprovider.New("internal", "Internal Authentication", identityprovider.TypeInternal)
		if err := identityprovider.NewRepository(pool).Persist(ctx, idp, dbtx); err != nil {
			return fmt.Errorf("insert internal IDP: %w", err)
		}
		idpID = idp.ID
	default:
		return fmt.Errorf("look up internal IDP: %w", err)
	}

	// Anchor EDM for the admin's email domain (idempotent by domain).
	var edmExists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM tnt_email_domain_mappings WHERE email_domain = $1)`,
		domain).Scan(&edmExists); err != nil {
		return fmt.Errorf("lookup email domain mapping: %w", err)
	}
	if !edmExists {
		edm := emaildomainmapping.New(domain, idpID, emaildomainmapping.ScopeAnchor)
		if err := emaildomainmapping.NewRepository(pool).Persist(ctx, edm, dbtx); err != nil {
			return fmt.Errorf("insert email-domain mapping: %w", err)
		}
	}

	// Principal (USER, ANCHOR scope) with password hash + super-admin role.
	p := principal.NewUser(email, principal.ScopeAnchor)
	p.SetPasswordHash(hash)
	if err := principal.NewRepository(pool).Persist(ctx, p, dbtx); err != nil {
		return fmt.Errorf("insert admin principal: %w", err)
	}

	// principal.Persist doesn't sync iam_principal_roles (Phase 3c
	// deferral — see HANDOFF.md §4 #19). Write the super-admin grant
	// directly here so the admin can log in with full perms.
	if _, err := tx.Exec(ctx,
		`INSERT INTO iam_principal_roles
		     (principal_id, role_name, assignment_source, assigned_at)
		 VALUES ($1, 'platform:super-admin', 'BOOTSTRAP', NOW())
		 ON CONFLICT DO NOTHING`,
		p.ID); err != nil {
		return fmt.Errorf("assign super-admin role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit admin: %w", err)
	}
	slog.Info("created anchor admin", "email", email)
	return nil
}

// ─── infraPersist: run repo.Persist outside a use case ─────────────────

// infraPersist runs a single-tx bootstrap write through the existing
// sqlc-backed repositories. Use this instead of duplicating Persist's
// junction-handling code in init/seed flows.
func infraPersist(ctx context.Context, pool *pgxpool.Pool, fn func(*usecasepgx.DbTx) error) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // committed below on success
	if err := fn(usecasepgx.WrapTxForBootstrap(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ─── Secret + hash helpers ─────────────────────────────────────────────

func generateSecret() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// hashSecret applies Argon2id via the shared PHC-encoded scheme. The
// output is a `$argon2id$…$salt$hash` string; verifiers go through
// `passwordhash.Verify`.
func hashSecret(plaintext string) string {
	h, err := passwordhash.Hash(plaintext)
	if err != nil {
		// crypto/rand failure: extremely rare; the upstream errors are
		// only "read from /dev/urandom" style. Surface as panic — the
		// bootstrap can't recover from a broken RNG.
		panic("fc-dev init: passwordhash.Hash: " + err.Error())
	}
	return h
}

// ─── Interactive prompts ───────────────────────────────────────────────

func prompt(in *bufio.Reader, flagValue, question, def string, yes bool) (string, error) {
	if v := strings.TrimSpace(flagValue); v != "" {
		return v, nil
	}
	if yes {
		if strings.TrimSpace(def) == "" {
			return "", fmt.Errorf("--yes mode requires a flag value for: %s", question)
		}
		return def, nil
	}
	suffix := ""
	if strings.TrimSpace(def) != "" {
		suffix = fmt.Sprintf(" [%s]", strings.TrimSpace(def))
	}
	fmt.Printf("%s%s: ", question, suffix)
	line, err := in.ReadString('\n')
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimRight(line, "\r\n")
	if trimmed == "" {
		if def != "" {
			return def, nil
		}
		return "", fmt.Errorf("%s is required", question)
	}
	return trimmed, nil
}

// promptOptional is like prompt but returns "" without erroring when no
// value is provided. Use for fields the platform accepts as NULL/empty.
func promptOptional(in *bufio.Reader, flagValue, question string, yes bool) string {
	if v := strings.TrimSpace(flagValue); v != "" {
		return v
	}
	if yes {
		return ""
	}
	fmt.Printf("%s: ", question)
	line, err := in.ReadString('\n')
	if err != nil {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(line, "\r\n"))
}

func promptSecret(in *bufio.Reader, flagValue, question string, yes bool) (string, error) {
	if v := flagValue; v != "" {
		return v, nil
	}
	if yes {
		return "", fmt.Errorf("--yes mode requires --admin-password (or FC_BOOTSTRAP_ADMIN_PASSWORD)")
	}
	// Best-effort: we can't mask without a TTY-aware dep. With no TTY
	// (CI), this is just a normal line read.
	fmt.Printf("%s: ", question)
	line, err := in.ReadString('\n')
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimRight(line, "\r\n")
	if trimmed == "" {
		return "", fmt.Errorf("%s is required", question)
	}
	return trimmed, nil
}

// ─── .env writer ────────────────────────────────────────────────────────

// writeEnvUpdates idempotently merges `updates` into the file at `path`.
// Existing keys are rewritten in place; new keys are appended under a
// header comment. Values containing whitespace or shell-special
// characters are single-quoted with embedded quotes escaped.
func writeEnvUpdates(path string, updates [][2]string) error {
	original, _ := os.ReadFile(path) // missing → empty
	var lines []string
	if len(original) > 0 {
		lines = strings.Split(string(original), "\n")
	}
	seen := map[string]bool{}
	for i, line := range lines {
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		for _, kv := range updates {
			if kv[0] == key {
				lines[i] = fmt.Sprintf("%s=%s", kv[0], quoteEnvValue(kv[1]))
				seen[kv[0]] = true
			}
		}
	}
	var toAppend [][2]string
	for _, kv := range updates {
		if !seen[kv[0]] {
			toAppend = append(toAppend, kv)
		}
	}
	sort.SliceStable(toAppend, func(i, j int) bool { return toAppend[i][0] < toAppend[j][0] })
	if len(toAppend) > 0 {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "# FlowCatalyst (added by `fc-dev init`)")
		for _, kv := range toAppend {
			lines = append(lines, fmt.Sprintf("%s=%s", kv[0], quoteEnvValue(kv[1])))
		}
	}
	next := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	if next == string(original) {
		fmt.Printf("  → %s already current, no update needed\n", path)
		return nil
	}
	if parent := filepath.Dir(path); parent != "." && parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, []byte(next), 0o600); err != nil { //nolint:gosec // G703: path is the fc-dev-resolved config file path, not external input
		return err
	}
	action := "updated"
	if len(original) == 0 {
		action = "created"
	}
	fmt.Printf("  → %s %s\n", path, action)
	return nil
}

func quoteEnvValue(v string) string {
	if v == "" || strings.ContainsAny(v, " \t\n#'\"`$") {
		return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
	}
	return v
}
