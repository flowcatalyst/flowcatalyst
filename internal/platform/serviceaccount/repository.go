package serviceaccount

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/encryption"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/repocommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo. Table: iam_service_accounts.
// Webhook credentials live as flat wh_* columns on the row; the
// entity's WebhookCredentials struct is reconstituted
// from those columns on read.
//
// Role assignments (RoleAssignment) live in iam_principal_roles, on the
// linked SERVICE principal, and are owned by the principal subdomain — this
// repo never WRITES them. It does READ them on the single-account loads, so
// GET /api/service-accounts/{id} and its /roles sub-route cannot disagree
// about what an account holds (they used to: the account object hardcoded an
// empty slice while the sub-route returned the real set).
//
// Inline SQL for that read, matching the principal repo's hydrateRoles — a
// junction read too trivial to be worth a sqlc regen.
type Repository struct {
	q    *dbq.Queries
	pool *pgxpool.Pool
	// enc encrypts the outbound webhook credentials at rest. Resolved once at
	// construction from FLOWCATALYST_APP_KEY; nil when unconfigured, which
	// makes a write that carries credentials fail rather than store plaintext.
	enc *encryption.Service
}

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	// Same lazy resolution the OAuth client-secret path uses; an unset key is
	// not an error here, only at the point a secret would have to be written.
	enc, _ := encryption.FromEnv()
	return &Repository{q: dbq.New(pool), pool: pool, enc: enc}
}

// FindByID loads by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*ServiceAccount, error) {
	res, err := r.q.ServiceAccountFindByID(ctx, id)
	row, err := repocommon.One(res, err, "service_account repo")
	if row == nil || err != nil {
		return nil, err
	}
	sa := rowToServiceAccount(*row, r.enc)
	if err := r.hydrateRoles(ctx, sa); err != nil {
		return nil, err
	}
	return sa, nil
}

// FindFirstByApplicationID loads the oldest active service account linked to
// an application — by convention the app's provisioned sync account. Nil when
// the application has no active SA.
func (r *Repository) FindFirstByApplicationID(ctx context.Context, applicationID string) (*ServiceAccount, error) {
	res, err := r.q.ServiceAccountFindFirstByApplicationID(ctx, &applicationID)
	row, err := repocommon.One(res, err, "service_account repo")
	if row == nil || err != nil {
		return nil, err
	}
	return rowToServiceAccount(*row, r.enc), nil
}

// FindByCode loads by unique code.
func (r *Repository) FindByCode(ctx context.Context, code string) (*ServiceAccount, error) {
	res, err := r.q.ServiceAccountFindByCode(ctx, code)
	row, err := repocommon.One(res, err, "service_account repo")
	if row == nil || err != nil {
		return nil, err
	}
	sa := rowToServiceAccount(*row, r.enc)
	if err := r.hydrateRoles(ctx, sa); err != nil {
		return nil, err
	}
	return sa, nil
}

// hydrateRoles populates sa.Roles from iam_principal_roles via the linked
// SERVICE principal — the same rows the /roles sub-route reads, so the two
// routes report one truth.
//
// Deliberately NOT called from FindAll: the list read omits per-row lookups,
// matching the existing principalId decision (api/dto.go). If the list is ever
// changed to hydrate, both fields should move together.
func (r *Repository) hydrateRoles(ctx context.Context, sa *ServiceAccount) error {
	if r.pool == nil || sa == nil {
		return nil
	}
	rows, err := r.pool.Query(ctx,
		`SELECT pr.role_name, pr.assignment_source, pr.assigned_at
		 FROM iam_principal_roles pr
		 JOIN iam_principals p ON p.id = pr.principal_id
		 WHERE p.service_account_id = $1
		 ORDER BY pr.role_name`, sa.ID)
	if err != nil {
		return fmt.Errorf("service account roles: %w", err)
	}
	defer rows.Close()
	out := make([]RoleAssignment, 0)
	for rows.Next() {
		var ra RoleAssignment
		if err := rows.Scan(&ra.Role, &ra.AssignmentSource, &ra.AssignedAt); err != nil {
			return fmt.Errorf("service account roles scan: %w", err)
		}
		out = append(out, ra)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("service account roles: %w", err)
	}
	sa.Roles = out
	return nil
}

// encryptCreds converts the entity's plaintext webhook credentials into their
// at-rest form, through the same helper (and the same "encrypted:" convention)
// that OAuth client secrets and identity-provider secrets already use.
//
// These are the only secrets the platform must be able to REPRODUCE rather
// than verify — the signing secret is the HMAC key a subscriber checks to know
// a delivery is genuinely ours — so they cannot be hashed. Encrypted at rest
// is the strongest available: a database dump then yields ciphertext that is
// useless without FLOWCATALYST_APP_KEY, which lives in the environment.
//
// Refuses to write a plaintext secret when no key is configured, matching
// EncryptSecretRef's contract. Values already carrying the prefix pass through
// unchanged, so re-persisting a loaded row is idempotent.
func (r *Repository) encryptCreds(creds WebhookCredentials) (token, secret *string, err error) {
	if token, err = encryption.EncryptSecretRef(r.enc, creds.Token); err != nil {
		return nil, nil, fmt.Errorf("encrypt webhook auth token: %w", err)
	}
	if secret, err = encryption.EncryptSecretRef(r.enc, creds.SigningSecret); err != nil {
		return nil, nil, fmt.Errorf("encrypt webhook signing secret: %w", err)
	}
	return token, secret, nil
}

// decryptSecretRef returns the plaintext behind an at-rest value.
//
// Only "encrypted:"-prefixed values are decrypted. Anything else is a legacy
// row written before this column was encrypted, and is returned as-is — so the
// prefix makes the two forms self-describing and no backfill is needed before
// the change can ship. A legacy row re-encrypts on its next write.
//
// A value that fails to decrypt is returned unchanged rather than dropped:
// silently blanking a credential would break deliveries with no signal, where
// passing it through fails loudly at the far end.
func decryptSecretRef(enc *encryption.Service, ref *string) *string {
	if ref == nil || *ref == "" || enc == nil {
		return ref
	}
	if !strings.HasPrefix(*ref, "encrypted:") {
		return ref // legacy plaintext
	}
	pt, err := enc.Decrypt(*ref)
	if err != nil {
		return ref
	}
	return &pt
}

// TouchLastUsed stamps last_used_at, best-effort: the caller is on a hot path
// (a token mint, a webhook delivery) and a failed stamp must never fail the
// thing that was actually being done, so the error is returned for logging but
// callers are expected to ignore it.
//
// "Used" means the account's CREDENTIALS were exercised — it authenticated, or
// its outbound secrets were handed to a delivery. That is the question the
// field exists to answer: is this account live, or is it a credential nobody
// deploys any more? An operator reads it before revoking.
func (r *Repository) TouchLastUsed(ctx context.Context, id string) error {
	if r.pool == nil || id == "" {
		return nil
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE iam_service_accounts SET last_used_at = NOW() WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("touch service account last_used_at: %w", err)
	}
	return nil
}

// FindAll returns every service account.
func (r *Repository) FindAll(ctx context.Context) ([]ServiceAccount, error) {
	rows, err := r.q.ServiceAccountFindAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ServiceAccount, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToServiceAccount(row, r.enc))
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[ServiceAccount]. Maps the
// WebhookCredentials struct onto the flat wh_* schema columns.
// wh_credentials_created_at / wh_credentials_regenerated_at are derived
// from sa.CreatedAt / sa.UpdatedAt today — when the SA gains an
// explicit credentials-rotation timestamp, plumb it through here.
func (r *Repository) Persist(ctx context.Context, sa *ServiceAccount, tx *usecasepgx.DbTx) error {
	creds := sa.WebhookCredentials
	// The entity carries plaintext (operations hand it back to the caller once,
	// at creation/rotation); the column carries ciphertext.
	tokenRef, secretRef, err := r.encryptCreds(creds)
	if err != nil {
		return err
	}
	return r.q.WithTx(tx.Inner()).ServiceAccountUpsert(ctx, dbq.ServiceAccountUpsertParams{
		ID:                         sa.ID,
		Code:                       sa.Code,
		Name:                       sa.Name,
		Description:                sa.Description,
		ApplicationID:              sa.ApplicationID,
		Scope:                      sa.Scope,
		ClientIds:                  sa.ClientIDs,
		Active:                     sa.Active,
		WhAuthType:                 stringPtrOrNil(string(creds.AuthType)),
		WhAuthTokenRef:             tokenRef,
		WhSigningSecretRef:         secretRef,
		WhSigningAlgorithm:         creds.SigningAlgorithm,
		WhCredentialsCreatedAt:     timePtr(sa.CreatedAt),
		WhCredentialsRegeneratedAt: nil,
		LastUsedAt:                 sa.LastUsedAt,
		CreatedAt:                  sa.CreatedAt,
		UpdatedAt:                  time.Now().UTC(),
	})
}

// Delete removes the row.
func (r *Repository) Delete(ctx context.Context, sa *ServiceAccount, tx *usecasepgx.DbTx) error {
	return r.q.WithTx(tx.Inner()).ServiceAccountDelete(ctx, sa.ID)
}

func rowToServiceAccount(row dbq.IamServiceAccount, enc *encryption.Service) *ServiceAccount {
	sa := &ServiceAccount{
		ID:            row.ID,
		Code:          row.Code,
		Name:          row.Name,
		Description:   row.Description,
		Active:        row.Active,
		ApplicationID: row.ApplicationID,
		Scope:         row.Scope,
		LastUsedAt:    row.LastUsedAt,
		CreatedAt:     row.CreatedAt,
		UpdatedAt:     row.UpdatedAt,
		ClientIDs:     append([]string{}, row.ClientIds...),
		Roles:         []RoleAssignment{},
		WebhookCredentials: WebhookCredentials{
			AuthType:         WebhookAuthType(stringDerefOrEmpty(row.WhAuthType)),
			Token:            decryptSecretRef(enc, row.WhAuthTokenRef),
			SigningSecret:    decryptSecretRef(enc, row.WhSigningSecretRef),
			SigningAlgorithm: row.WhSigningAlgorithm,
		},
	}
	if sa.WebhookCredentials.AuthType == "" {
		sa.WebhookCredentials = NoCredentials()
	}
	return sa
}

func stringPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func stringDerefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
