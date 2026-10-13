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
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/migrate"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// newInitCmd bootstraps a fresh fcdev environment.
//
// What it does:
//
//  1. Run migrations + the built-in seeds (idempotent).
//  2. Create the anchor admin if no anchor user exists yet (internal IDP +
//     email-domain mapping + Principal with hashed password + super-admin
//     role assignment).
//  3. Resolve (or create) a Default Client.
//  4. Create the Application (errors if the code is taken).
//  5. Provision the application's service account via the shared
//     ProvisionServiceAccount use case: SERVICE principal with the
//     platform:application-service role, confined to this one application
//     (not all-applications), attached to the Application, plus a
//     CONFIDENTIAL OAuth client with the `client_credentials` grant.
//  6. Write `.env` with FLOWCATALYST_BASE_URL/APP_CODE/CLIENT_ID/CLIENT_SECRET,
//     in-place for existing keys, appended otherwise.
//
// Admin/client/application writes go directly to the repositories (no UoW,
// no events) — platform-infrastructure bootstrap, the exception class
// documented in docs/conventions.md §3. The service account goes through the
// real use case so it matches a normally provisioned one.
func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Bootstrap a fresh local environment (admin user + default tenant + .env)",
		Long: `Bootstrap a fresh local environment: admin user, default client, one
application, and that application's service-account credentials (.env).

The credentials are provisioned the same way as an application service account
created in the admin UI: the platform:application-service role, confined to
the one application. Event types you publish must start with
"<application-code>:"; register event types through the SDK sync endpoint
(POST /api/applications/{appCode}/event-types/sync).`,
		RunE: runInit,
	}
	cmd.Flags().String("database-url", envStrDefault("FC_DATABASE_URL", ""), "Postgres URL (defaults to local embedded)")
	cmd.Flags().String("embedded-db-path", envStrDefault("FC_EMBEDDED_DB_PATH", defaultEmbeddedPath()), "embedded Postgres data directory (the app key is read from / persisted beside it, as `fcdev start` does)")
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

	pool, err := database.NewPool(ctx, database.Config{URL: dbURL})
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

	fmt.Println("fcdev init")

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
		defaultClientID = string(existingClient.ID)
		fmt.Printf("→ reusing default client \"%s\" (id=%s)\n", existingClient.Identifier, existingClient.ID)
	} else {
		c := client.New(get("client-name"), clientIdent)
		if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
			return clientRepo.Persist(ctx, c, tx)
		}); err != nil {
			return fmt.Errorf("insert default client: %w", err)
		}
		defaultClientID = string(c.ID)
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
		return fmt.Errorf("application with code %q already exists (id=%s). Pick a different code or run `fcdev fresh`", appCode, existing.ID)
	}

	app := application.New(appCode, appName)
	appType, ok := application.ParseType(strings.ToUpper(rawType))
	if !ok {
		return fmt.Errorf("invalid application type %q: must be APPLICATION or INTEGRATION", rawType)
	}
	app.Type = appType
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

	// ── 4. Service account + linked Principal + OAuth client ───────────
	// Provisioned through the same use case as the admin UI / API
	// (POST /api/applications/{id}/provision-service-account) so the two
	// cannot drift: the SERVICE principal gets the platform:application-service
	// role (PROVISIONED), AllApplications=false and access to exactly this one
	// application, plus a confidential client_credentials OAuth client limited
	// to it.
	//
	// The client secret is stored as a keyed hash, so an app key must exist.
	// Same resolution as the running fcdev (env, else the persisted key file,
	// else generate + persist), so the credentials minted here verify there.
	appKey, err := resolveAppKey(get("embedded-db-path"))
	if err != nil {
		return fmt.Errorf("resolve app key: %w", err)
	}
	provisioned, err := provisionAppCredentials(ctx, pool, app.ID)
	if err != nil {
		return fmt.Errorf("provision service account: %w", err)
	}
	fmt.Printf("  → service account \"%s\" provisioned (role platform:application-service, confined to \"%s\")\n",
		provisioned.ServiceAccountCode, appCode)
	fmt.Printf("  → OAuth client minted (id=%s)\n", provisioned.OAuthClientRowID)
	publicClientID := provisioned.OAuthClientID
	clientSecretPlain := provisioned.OAuthClientSecret

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
	fmt.Printf("  Service account: %s\n", provisioned.ServiceAccountID)
	fmt.Printf("  OAuth client:    %s (clientId=%s)\n", provisioned.OAuthClientRowID, publicClientID)
	fmt.Printf("  Default client:  %s\n", defaultClientID)
	fmt.Println()
	fmt.Printf("  Credentials written to %s. The clientSecret is shown ONLY in\n", envPath)
	fmt.Println("  the .env — the platform stores only the Argon2 hash and cannot")
	fmt.Println("  return it again. Rotate via the OAuth Clients page if needed.")
	fmt.Println()
	fmt.Printf("  These credentials are confined to the %q application:\n", appCode)
	fmt.Printf("    - event types you publish must start with \"%s:\" (e.g. \"%s:order:created\");\n", appCode, appCode)
	fmt.Println("      anything else is rejected with 400 (it belongs to another application).")
	fmt.Printf("    - register event types through the SDK sync endpoint\n")
	fmt.Printf("      (POST /api/applications/%s/event-types/sync), not POST /api/event-types:\n", appCode)
	fmt.Println("      the application-service role does not carry the event-type admin permissions.")
	return nil
}

// provisionAppCredentials runs the shared ProvisionServiceAccount use case
// against the pool (the same operation the API runs), acting as the
// bootstrap admin so the audit trail has a real principal.
func provisionAppCredentials(ctx context.Context, pool *pgxpool.Pool, appID ids.ApplicationID) (appops.ProvisionServiceAccountResult, error) {
	var actor string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM iam_principals WHERE type = 'USER' AND scope = 'ANCHOR'
		  ORDER BY created_at LIMIT 1`).Scan(&actor); err != nil {
		return appops.ProvisionServiceAccountResult{}, fmt.Errorf("find bootstrap admin: %w", err)
	}
	uow := usecasepgx.New(pool, platformsink.New())
	return usecaseop.RunTx(ctx, uow,
		appops.ProvisionServiceAccount(
			application.NewRepository(pool),
			serviceaccount.NewRepository(pool),
			principal.NewRepository(pool),
			auth.NewRepository(pool).OAuthClients),
		appops.ProvisionServiceAccountCommand{ApplicationID: string(appID)},
		usecase.NewExecutionContext(actor))
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
	// deferral — see docs/wire-contract.md §4 #19). Write the super-admin grant
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
		panic("fcdev init: passwordhash.Hash: " + err.Error())
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
		before, _, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key := strings.TrimSpace(before)
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
	slices.SortStableFunc(toAppend, func(a, b [2]string) int { return strings.Compare(a[0], b[0]) })
	if len(toAppend) > 0 {
		if len(lines) > 0 && lines[len(lines)-1] != "" {
			lines = append(lines, "")
		}
		lines = append(lines, "# FlowCatalyst (added by `fcdev init`)")
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
	if err := os.WriteFile(path, []byte(next), 0o600); err != nil { //nolint:gosec // G703: path is the fcdev-resolved config file path, not external input
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
