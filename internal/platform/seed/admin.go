package seed

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Bootstrap admin env vars — match the Rust contract so existing
// deployment configs keep working.
const (
	EnvBootstrapEmail    = "FLOWCATALYST_BOOTSTRAP_ADMIN_EMAIL"
	EnvBootstrapPassword = "FLOWCATALYST_BOOTSTRAP_ADMIN_PASSWORD"
	EnvBootstrapName     = "FLOWCATALYST_BOOTSTRAP_ADMIN_NAME"

	bootstrapRoleSuperAdmin = "platform:super-admin"
	bootstrapRoleSource     = "BOOTSTRAP"
	bootstrapDefaultName    = "Bootstrap Admin"
)

// BootstrapAdmin creates the initial super-admin USER + ANCHOR principal
// when no anchor user exists yet. Idempotent: if any anchor user is
// already present (or the named user has been created externally), the
// function returns nil after logging.
//
// Reads credentials from the env vars above. When email/password are
// unset on a fresh install the function logs a warning and returns
// nil — production deployments must opt in explicitly so we don't bake
// a known password into prod. fc-dev pre-sets the env to
// admin@flowcatalyst.local / DevPassword123! so the local workflow
// "just works".
//
// Mirrors crates/fc-platform/src/shared/bootstrap_admin.rs.
func (s *Seeder) seedBootstrapAdmin(ctx context.Context) error {
	var anchorCount int
	if err := s.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM iam_principals WHERE type = 'USER' AND scope = 'ANCHOR'`,
	).Scan(&anchorCount); err != nil {
		return fmt.Errorf("count anchor users: %w", err)
	}
	if anchorCount > 0 {
		return nil
	}

	email := strings.TrimSpace(os.Getenv(EnvBootstrapEmail))
	password := os.Getenv(EnvBootstrapPassword)
	if email == "" || password == "" {
		slog.Warn("no bootstrap admin configured — set "+EnvBootstrapEmail+" + "+EnvBootstrapPassword+" to create one",
			"email_set", email != "", "password_set", password != "")
		return nil
	}
	at := strings.IndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		slog.Warn("invalid bootstrap email format; skipping", "email", email)
		return nil
	}
	name := os.Getenv(EnvBootstrapName)
	if name == "" {
		name = bootstrapDefaultName
	}

	hash, err := passwordhash.Hash(password)
	if err != nil {
		return fmt.Errorf("hash bootstrap password: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // committed on success below
	dbtx := usecasepgx.WrapTxForBootstrap(tx)

	// Idempotency: someone may have created this user via SQL before
	// bootstrap ran. Skip with a log if so.
	var existing string
	switch err := tx.QueryRow(ctx,
		`SELECT id FROM iam_principals WHERE email = $1`, email,
	).Scan(&existing); {
	case err == nil:
		slog.Info("bootstrap admin already present; skipping", "email", email)
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		// proceed
	default:
		return fmt.Errorf("look up bootstrap admin: %w", err)
	}

	// Ensure the 'internal' identity provider row exists. Idempotent.
	var idpID string
	switch err := tx.QueryRow(ctx,
		`SELECT id FROM oauth_identity_providers WHERE code = 'internal'`,
	).Scan(&idpID); {
	case err == nil:
		// reuse
	case errors.Is(err, pgx.ErrNoRows):
		idp := identityprovider.New("internal", "Internal Authentication", identityprovider.TypeInternal)
		if err := identityprovider.NewRepository(s.pool).Persist(ctx, idp, dbtx); err != nil {
			return fmt.Errorf("insert internal IDP: %w", err)
		}
		idpID = idp.ID
	default:
		return fmt.Errorf("look up internal IDP: %w", err)
	}

	// Ensure an ANCHOR email-domain mapping for the admin's domain
	// exists so subsequent logins resolve to the internal IDP.
	domain := email[at+1:]
	var edmExists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM tnt_email_domain_mappings WHERE email_domain = $1)`,
		domain,
	).Scan(&edmExists); err != nil {
		return fmt.Errorf("lookup email domain mapping: %w", err)
	}
	if !edmExists {
		edm := emaildomainmapping.New(domain, idpID, emaildomainmapping.ScopeAnchor)
		if err := emaildomainmapping.NewRepository(s.pool).Persist(ctx, edm, dbtx); err != nil {
			return fmt.Errorf("insert email-domain mapping: %w", err)
		}
	}

	p := principal.NewUser(email, principal.ScopeAnchor)
	p.Name = name
	p.SetPasswordHash(hash)
	if err := principal.NewRepository(s.pool).Persist(ctx, p, dbtx); err != nil {
		return fmt.Errorf("insert admin principal: %w", err)
	}

	// principal.Persist does NOT sync iam_principal_roles — junction
	// writes are explicit. Add the super-admin grant directly so the
	// new admin can sign in with full permissions.
	if _, err := tx.Exec(ctx,
		`INSERT INTO iam_principal_roles
		     (principal_id, role_name, assignment_source, assigned_at)
		 VALUES ($1, $2, $3, NOW())
		 ON CONFLICT DO NOTHING`,
		p.ID, bootstrapRoleSuperAdmin, bootstrapRoleSource,
	); err != nil {
		return fmt.Errorf("assign super-admin role: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit bootstrap admin: %w", err)
	}
	slog.Info("bootstrap admin created",
		"email", email,
		"role", bootstrapRoleSuperAdmin,
		"scope", "ANCHOR")
	return nil
}
