package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/credentialpublish"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/database"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
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
//     the account's webhook credentials (SIGNING_SECRET, WEBHOOK_AUTH_TOKEN)
//     and APP_KEY, in place for existing keys, appended otherwise.
//
// An application that already exists is refused, as before; with
// --regenerate-signing-secret its service account's signing secret is
// replaced instead and only that is written.
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
(POST /api/applications/{appCode}/event-types/sync).

.env gets FLOWCATALYST_BASE_URL, FLOWCATALYST_APP_CODE, FLOWCATALYST_CLIENT_ID,
FLOWCATALYST_CLIENT_SECRET, FLOWCATALYST_SIGNING_SECRET (the HMAC secret the
platform signs its deliveries to the application with),
FLOWCATALYST_WEBHOOK_AUTH_TOKEN (the bearer token it sends with them) and
FLOWCATALYST_APP_KEY. A key already in the file is rewritten in place, others
are appended. A secret that had a different value keeps its old value in a
comment line above it ("# replaced by fcdev init: KEY=old"), so a value you
set yourself is never lost.

The platform shows the client secret, signing secret and token only when it
creates them, so init refuses an application code that already exists and
writes nothing. For an application that exists without its signing secret in
.env, --regenerate-signing-secret replaces the service account's signing
secret and writes the new one: any running copy of the application that
verifies with the old secret refuses every delivery until it is restarted
with the new one.`,
		RunE: runInit,
	}
	cmd.Flags().String("database-url", envStrDefault("FC_DATABASE_URL", ""), "Postgres URL (defaults to local embedded)")
	cmd.Flags().String("embedded-db-path", envStrDefault("FC_EMBEDDED_DB_PATH", defaultEmbeddedPath()), "embedded Postgres data directory (the app key is read from / persisted beside it, as `fcdev start` does)")
	addEmbeddedDBPasswordFlag(cmd)
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
	cmd.Flags().Bool("regenerate-signing-secret", false, "for an application --code that already exists: replace its service account's signing secret and write the new one to .env (a running copy of the application verifying with the old secret refuses deliveries until restarted with the new one)")

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
	out := cmd.OutOrStdout()

	dbURL := get("database-url")
	if dbURL == "" {
		dbURL = embeddedDBURL(resolveEmbeddedDBPassword(cmd), 15432)
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

	_, _ = fmt.Fprintln(out, "fcdev init")

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
		_, _ = fmt.Fprintln(out, "→ admin user already present, skipping creation")
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
		_, _ = fmt.Fprintf(out, "  → admin %s created\n", adminEmail)
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
		_, _ = fmt.Fprintf(out, "→ reusing default client \"%s\" (id=%s)\n", existingClient.Identifier, existingClient.ID)
	} else {
		c := client.New(get("client-name"), clientIdent)
		if err := infraPersist(ctx, pool, func(tx *usecasepgx.DbTx) error {
			return clientRepo.Persist(ctx, c, tx)
		}); err != nil {
			return fmt.Errorf("insert default client: %w", err)
		}
		defaultClientID = string(c.ID)
		_, _ = fmt.Fprintf(out, "  → default client \"%s\" created (id=%s)\n", c.Identifier, c.ID)
	}

	// ── 3. Application ─────────────────────────────────────────────────
	envPath := filepath.Join(get("root"), ".env")
	appCode, err := prompt(stdin, get("code"), "Application code (slug)", "", getBool("yes"))
	if err != nil {
		return err
	}
	appRepo := application.NewRepository(pool)
	existing, err := appRepo.FindByCode(ctx, appCode)
	if err != nil {
		return fmt.Errorf("lookup application: %w", err)
	}
	if existing != nil {
		if !getBool("regenerate-signing-secret") {
			return existingApplicationError(existing, envPath)
		}
		return regenerateAppSigningSecret(ctx, out, pool, existing, get("embedded-db-path"), envPath)
	}
	if getBool("regenerate-signing-secret") {
		_, _ = fmt.Fprintf(out, "→ application %q is new: --regenerate-signing-secret has nothing to replace, its signing secret is minted below\n", appCode)
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
	_, _ = fmt.Fprintf(out, "  → application \"%s\" created (id=%s)\n", appCode, app.ID)

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
	_, _ = fmt.Fprintf(out, "  → service account \"%s\" provisioned (role platform:application-service, confined to \"%s\")\n",
		provisioned.ServiceAccountCode, appCode)
	_, _ = fmt.Fprintf(out, "  → OAuth client minted (id=%s)\n", provisioned.OAuthClientRowID)
	publicClientID := provisioned.OAuthClientID

	// ── 6. Write .env ──────────────────────────────────────────────────
	updates := [][2]string{
		{"FLOWCATALYST_BASE_URL", get("api-base-url")},
		{"FLOWCATALYST_APP_CODE", appCode},
		{"FLOWCATALYST_CLIENT_ID", publicClientID},
		{envClientSecret, provisioned.OAuthClientSecret},
		{envSigningSecret, provisioned.SigningSecret},
		{envWebhookAuthToken, provisioned.AuthToken},
		// Persist the app key so the same encrypted client_secret_ref stays
		// decryptable across restarts (the /oauth/token verify path reads it).
		{"FLOWCATALYST_APP_KEY", appKey},
	}
	if err := writeEnvUpdates(out, envPath, updates); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}

	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, "✓ Application scaffolded.")
	_, _ = fmt.Fprintf(out, "  Application:     %s (code=%s)\n", appName, appCode)
	_, _ = fmt.Fprintf(out, "  Service account: %s\n", provisioned.ServiceAccountID)
	_, _ = fmt.Fprintf(out, "  OAuth client:    %s (clientId=%s)\n", provisioned.OAuthClientRowID, publicClientID)
	_, _ = fmt.Fprintf(out, "  Default client:  %s\n", defaultClientID)
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "  Credentials written to %s: the client secret, the signing secret\n", envPath)
	_, _ = fmt.Fprintln(out, "  (FLOWCATALYST_SIGNING_SECRET, which the platform signs its deliveries to")
	_, _ = fmt.Fprintln(out, "  the application with) and the webhook token are shown ONLY in the .env;")
	_, _ = fmt.Fprintln(out, "  the platform cannot return them again. Rotate the client secret on the")
	_, _ = fmt.Fprintln(out, "  OAuth Clients page, the signing secret with `fcdev init --code "+appCode+" --regenerate-signing-secret`.")
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "  These credentials are confined to the %q application:\n", appCode)
	_, _ = fmt.Fprintf(out, "    - event types you publish must start with \"%s:\" (e.g. \"%s:order:created\");\n", appCode, appCode)
	_, _ = fmt.Fprintln(out, "      anything else is rejected with 400 (it belongs to another application).")
	_, _ = fmt.Fprintf(out, "    - register event types through the SDK sync endpoint\n")
	_, _ = fmt.Fprintf(out, "      (POST /api/applications/%s/event-types/sync), not POST /api/event-types:\n", appCode)
	_, _ = fmt.Fprintln(out, "      the application-service role does not carry the event-type admin permissions.")
	return nil
}

// The .env names of the credentials init writes. They are the SDKs' names
// (pkg/fcsdk/webhook.SigningSecretEnvVar, the Laravel and Rust SDKs, the
// TypeScript SDK's examples), the family FLOWCATALYST_APP_CODE belongs to.
const (
	envClientSecret     = "FLOWCATALYST_CLIENT_SECRET"
	envSigningSecret    = "FLOWCATALYST_SIGNING_SECRET"
	envWebhookAuthToken = "FLOWCATALYST_WEBHOOK_AUTH_TOKEN"
)

// envSecretKeys are the keys whose previous value writeEnvUpdates keeps in a
// comment when it replaces it: a secret cannot be read back from the
// platform, and the old one may be one the developer set themselves.
var envSecretKeys = map[string]bool{
	envClientSecret:     true,
	envSigningSecret:    true,
	envWebhookAuthToken: true,
}

// existingApplicationError is init's answer for a code that is taken. Nothing
// is written: the platform shows a service account's secrets only when it
// creates or regenerates them, and a second provisioning is refused
// (ALREADY_PROVISIONED), so there is no signing secret to write.
func existingApplicationError(app *application.Application, envPath string) error {
	if app.ServiceAccountID == nil || *app.ServiceAccountID == "" {
		return fmt.Errorf("application with code %q already exists (id=%s) and has no service account; nothing was written to %s. "+
			"Pick a different code or run `fcdev fresh`, or provision its service account on the application's page in the console",
			app.Code, app.ID, envPath)
	}
	return fmt.Errorf(`application with code %q already exists (id=%s) and its service account is provisioned; nothing was written to %s.
%s was not written: the platform shows a signing secret only when it creates or regenerates it.
To get one, regenerate it and write it to %s:
    fcdev init --code %s --regenerate-signing-secret
This replaces the service account's signing secret: a running copy of the application that verifies with the old one refuses every delivery until it is restarted with the new one.
To start over instead, pick a different code or run `+"`fcdev fresh`",
		app.Code, app.ID, envPath, envSigningSecret, envPath, app.Code)
}

// regenerateAppSigningSecret replaces the signing secret of the application's
// service account through the same use case as the console's "Regenerate
// secret" (RegenerateSigningSecret) and writes the new value to .env. Only
// the signing secret changes: the OAuth client and the webhook token stay as
// they are.
func regenerateAppSigningSecret(ctx context.Context, out io.Writer, pool *pgxpool.Pool, app *application.Application, embeddedDBPath, envPath string) error {
	// The signing secret is stored encrypted under the app key the running
	// fcdev uses; resolve it the same way, before the repository reads it.
	if _, err := resolveAppKey(embeddedDBPath); err != nil {
		return fmt.Errorf("resolve app key: %w", err)
	}
	saRepo := serviceaccount.NewRepository(pool)
	sa, err := appServiceAccount(ctx, pool, saRepo, app)
	if err != nil {
		return err
	}
	if sa == nil {
		return fmt.Errorf("application %q has no service account to regenerate the signing secret of (its service principal %v has none); nothing was written to %s",
			app.Code, deref(app.ServiceAccountID), envPath)
	}
	actor, err := bootstrapActor(ctx, pool)
	if err != nil {
		return err
	}
	var secret string
	if _, err := usecaseop.Run(ctx, usecasepgx.New(pool, platformsink.New()),
		saops.RegenerateSigningSecret(saRepo),
		saops.RegenerateSigningSecretCommand{
			ServiceAccountID: sa.ID,
			Disclose:         func(plaintext string) { secret = plaintext },
		},
		usecase.NewExecutionContext(actor)); err != nil {
		return fmt.Errorf("regenerate signing secret: %w", err)
	}
	_, _ = fmt.Fprintf(out, "  → signing secret of service account \"%s\" regenerated; the old one no longer verifies\n", sa.Code)
	if err := writeEnvUpdates(out, envPath, [][2]string{{envSigningSecret, secret}}); err != nil {
		return fmt.Errorf("write .env: %w", err)
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintf(out, "✓ %s written to %s for application %q.\n", envSigningSecret, envPath, app.Code)
	_, _ = fmt.Fprintln(out, "  Restart any running copy of the application so it verifies with the new secret.")
	_, _ = fmt.Fprintln(out, "  The client id and secret and the webhook token were not changed or written.")
	return nil
}

// appServiceAccount finds the service account behind the application's
// service principal (the application stores the principal's prn_ id).
func appServiceAccount(ctx context.Context, pool *pgxpool.Pool, saRepo *serviceaccount.Repository, app *application.Application) (*serviceaccount.ServiceAccount, error) {
	if app.ServiceAccountID == nil || *app.ServiceAccountID == "" {
		return nil, nil
	}
	p, err := principal.NewRepository(pool).FindByID(ctx, ids.PrincipalID(*app.ServiceAccountID))
	if err != nil {
		return nil, fmt.Errorf("find service principal: %w", err)
	}
	if p == nil || p.ServiceAccountID == nil {
		return nil, nil
	}
	sa, err := saRepo.FindByID(ctx, *p.ServiceAccountID)
	if err != nil {
		return nil, fmt.Errorf("find service account: %w", err)
	}
	return sa, nil
}

// bootstrapActor is the principal init's use cases run as, so the audit
// trail has a real one: the first anchor user.
func bootstrapActor(ctx context.Context, pool *pgxpool.Pool) (string, error) {
	var actor string
	if err := pool.QueryRow(ctx,
		`SELECT id FROM iam_principals WHERE type = 'USER' AND scope = 'ANCHOR'
		  ORDER BY created_at LIMIT 1`).Scan(&actor); err != nil {
		return "", fmt.Errorf("find bootstrap admin: %w", err)
	}
	return actor, nil
}

// provisionAppCredentials runs the shared ProvisionServiceAccount use case
// against the pool (the same operation the API runs), acting as the
// bootstrap admin so the audit trail has a real principal.
func provisionAppCredentials(ctx context.Context, pool *pgxpool.Pool, appID ids.ApplicationID) (appops.ProvisionServiceAccountResult, error) {
	actor, err := bootstrapActor(ctx, pool)
	if err != nil {
		return appops.ProvisionServiceAccountResult{}, err
	}
	uow := usecasepgx.New(pool, platformsink.New())
	return usecaseop.RunTx(ctx, uow,
		appops.ProvisionServiceAccount(
			application.NewRepository(pool),
			serviceaccount.NewRepository(pool),
			principal.NewRepository(pool),
			auth.NewRepository(pool).OAuthClients,
			// fcdev's local bootstrap never publishes credentials.
			credentialpublish.Config{}),
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
//
// A key of envSecretKeys whose old, non-empty value differs keeps that value
// in a comment line above it (envReplacedPrefix), in place of an earlier such
// comment for the key, so a secret the developer set is never lost: the
// platform cannot show it again.
func writeEnvUpdates(out io.Writer, path string, updates [][2]string) error {
	original, _ := os.ReadFile(path) // missing → empty
	var lines []string
	if len(original) > 0 {
		lines = strings.Split(string(original), "\n")
	}
	want := map[string]string{}
	for _, kv := range updates {
		want[kv[0]] = fmt.Sprintf("%s=%s", kv[0], quoteEnvValue(kv[1]))
	}
	envKey := func(line string) (key, value string, ok bool) {
		before, value, ok := strings.Cut(line, "=")
		key = strings.TrimSpace(before)
		return key, value, ok && !strings.HasPrefix(key, "#")
	}

	// Pass 1: which secrets lose a value that is set and different.
	replacing := map[string]bool{}
	for _, line := range lines {
		if key, value, ok := envKey(line); ok && envSecretKeys[key] && want[key] != "" &&
			line != want[key] && unquoteEnvValue(value) != "" {
			replacing[key] = true
		}
	}

	// Pass 2: rewrite in place.
	seen := map[string]bool{}
	next := make([]string, 0, len(lines)+len(updates)+2)
	for _, line := range lines {
		if key, ok := replacedCommentKey(line); ok && replacing[key] {
			continue // superseded by the comment written below
		}
		key, _, ok := envKey(line)
		if !ok || want[key] == "" {
			next = append(next, line)
			continue
		}
		if replacing[key] {
			next = append(next, envReplacedPrefix+line)
		}
		next = append(next, want[key])
		seen[key] = true
	}
	lines = next

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
			lines = append(lines, want[kv[0]])
		}
	}
	content := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	if content == string(original) {
		_, _ = fmt.Fprintf(out, "  → %s already current, no update needed\n", path)
		return nil
	}
	if parent := filepath.Dir(path); parent != "." && parent != "" {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil { //nolint:gosec // G703: path is the fcdev-resolved config file path, not external input
		return err
	}
	action := "updated"
	if len(original) == 0 {
		action = "created"
	}
	_, _ = fmt.Fprintf(out, "  → %s %s\n", path, action)
	for _, kv := range updates {
		if replacing[kv[0]] {
			_, _ = fmt.Fprintf(out, "  → %s replaced; its previous value is kept in the comment line above it\n", kv[0])
		}
	}
	return nil
}

// envReplacedPrefix starts the comment line that keeps a replaced secret.
const envReplacedPrefix = "# replaced by fcdev init: "

// replacedCommentKey is the key of a line written with envReplacedPrefix.
func replacedCommentKey(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, envReplacedPrefix)
	if !ok {
		return "", false
	}
	before, _, ok := strings.Cut(rest, "=")
	return strings.TrimSpace(before), ok
}

func quoteEnvValue(v string) string {
	if v == "" || strings.ContainsAny(v, " \t\n#'\"`$") {
		return "'" + strings.ReplaceAll(v, "'", `'\''`) + "'"
	}
	return v
}

// unquoteEnvValue undoes quoteEnvValue well enough to tell an empty value,
// bare or quoted, from a set one.
func unquoteEnvValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && (v[0] == '\'' || v[0] == '"') && v[len(v)-1] == v[0] {
		return v[1 : len(v)-1]
	}
	return v
}
