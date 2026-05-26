package principal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed principal repo.
//
// Phase 3c scope: the principal row only. The junction tables
// (iam_principal_roles, iam_client_access_grants,
// iam_principal_application_access) are populated by deferred ops
// (assign_roles, grant_client_access, …); Persist does NOT sync them.
// Delete still cleans them to avoid orphans (only iam_principal_roles
// has FK ON DELETE CASCADE; the other two don't).
//
// User-identity fields are stored as flat columns on iam_principals
// (email, idp_type, external_idp_id, password_hash, last_login_at) —
// not as JSONB. The entity exposes UserIdentity{} as a struct for API
// shape; fields with no backing column (email_verified, first_name,
// last_name, picture_url, phone) are zero-valued on read and dropped
// on write. Mirrors the Rust impl.
type Repository struct{ q *dbq.Queries }

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{q: dbq.New(pool)}
}

// FindByID loads a principal by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*Principal, error) {
	row, err := r.q.PrincipalFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("principal repo: %w", err)
	}
	return rowToPrincipal(row), nil
}

// FindByEmail loads a user-type principal by email.
func (r *Repository) FindByEmail(ctx context.Context, email string) (*Principal, error) {
	row, err := r.q.PrincipalFindByEmail(ctx, &email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("principal repo: %w", err)
	}
	return rowToPrincipal(row), nil
}

// FindByServiceAccount loads the SERVICE-type principal linked to the
// given service-account row. Used by callers that need to translate a
// SA id into the principal id its FKs reference (e.g.
// `app_applications.service_account_id`, which has a FK to
// `iam_principals.id` per migration 028).
func (r *Repository) FindByServiceAccount(ctx context.Context, serviceAccountID string) (*Principal, error) {
	row, err := r.q.PrincipalFindByServiceAccount(ctx, &serviceAccountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("principal repo: %w", err)
	}
	return rowToPrincipal(row), nil
}

// FindAll lists every principal.
func (r *Repository) FindAll(ctx context.Context) ([]Principal, error) {
	rows, err := r.q.PrincipalFindAll(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Principal, 0, len(rows))
	for _, row := range rows {
		out = append(out, *rowToPrincipal(row))
	}
	return out, nil
}

// Persist implements usecasepgx.Persist[Principal].
func (r *Repository) Persist(ctx context.Context, p *Principal, tx *usecasepgx.DbTx) error {
	now := time.Now().UTC()

	var email, emailDomain, idpType, externalIdpID, passwordHash *string
	var lastLoginAt *time.Time

	if p.UserIdentity != nil {
		em := p.UserIdentity.Email
		email = &em
		if domain := domainOf(em); domain != "" {
			emailDomain = &domain
		}
		if p.UserIdentity.Provider != nil {
			idpType = p.UserIdentity.Provider
		}
		if p.UserIdentity.ExternalID != nil {
			externalIdpID = p.UserIdentity.ExternalID
		}
		if p.UserIdentity.PasswordHash != nil {
			passwordHash = p.UserIdentity.PasswordHash
		}
		if p.UserIdentity.LastLoginAt != nil {
			lastLoginAt = p.UserIdentity.LastLoginAt
		}
	}
	// USER without an explicit provider defaults to INTERNAL (matches Rust).
	if idpType == nil && p.Type == TypeUser {
		internal := "INTERNAL"
		idpType = &internal
	}
	// ExternalIdentity, when present, wins for the IDP columns.
	if p.ExternalIdentity != nil {
		provider := p.ExternalIdentity.ProviderID
		if provider != "" {
			idpType = &provider
		}
		ext := p.ExternalIdentity.ExternalID
		externalIdpID = &ext
	}

	scope := string(p.Scope)
	return r.q.WithTx(tx.Inner()).PrincipalUpsert(ctx, dbq.PrincipalUpsertParams{
		ID:               p.ID,
		Type:             string(p.Type),
		Scope:            &scope,
		ClientID:         p.ClientID,
		ApplicationID:    p.ApplicationID,
		Name:             p.Name,
		Active:           p.Active,
		Email:            email,
		EmailDomain:      emailDomain,
		IdpType:          idpType,
		ExternalIdpID:    externalIdpID,
		PasswordHash:     passwordHash,
		LastLoginAt:      lastLoginAt,
		ServiceAccountID: p.ServiceAccountID,
		CreatedAt:        p.CreatedAt,
		UpdatedAt:        now,
	})
}

// Delete removes the principal and the two non-FK-cascade junctions.
// iam_principal_roles has FK ON DELETE CASCADE so it goes via the main row.
func (r *Repository) Delete(ctx context.Context, p *Principal, tx *usecasepgx.DbTx) error {
	q := r.q.WithTx(tx.Inner())
	if err := q.PrincipalApplicationAccessClear(ctx, p.ID); err != nil {
		return err
	}
	if err := q.PrincipalClientAccessGrantsClear(ctx, p.ID); err != nil {
		return err
	}
	return q.PrincipalDelete(ctx, p.ID)
}

// rowToPrincipal projects the flat schema row onto the Principal aggregate.
// Mirrors the Rust From<PrincipalRow> for Principal mapping.
func rowToPrincipal(row dbq.IamPrincipal) *Principal {
	p := Principal{
		ID:                       row.ID,
		Type:                     ParseType(row.Type),
		ClientID:                 row.ClientID,
		ApplicationID:            row.ApplicationID,
		Name:                     row.Name,
		Active:                   row.Active,
		ServiceAccountID:         row.ServiceAccountID,
		CreatedAt:                row.CreatedAt,
		UpdatedAt:                row.UpdatedAt,
		Roles:                    []serviceaccount.RoleAssignment{},
		AssignedClients:          []string{},
		AccessibleApplicationIDs: []string{},
	}
	if row.Scope != nil {
		p.Scope = ParseScope(*row.Scope)
	} else {
		p.Scope = ScopeClient
	}
	if p.Type == TypeUser && row.Email != nil {
		p.UserIdentity = &UserIdentity{
			Email:        *row.Email,
			ExternalID:   row.ExternalIdpID,
			Provider:     row.IdpType,
			PasswordHash: row.PasswordHash,
			LastLoginAt:  row.LastLoginAt,
		}
	}
	if row.ExternalIdpID != nil {
		providerID := ""
		if row.IdpType != nil {
			providerID = *row.IdpType
		}
		p.ExternalIdentity = &ExternalIdentity{
			ProviderID: providerID,
			ExternalID: *row.ExternalIdpID,
		}
	}
	return &p
}

func domainOf(email string) string {
	at := strings.IndexByte(email, '@')
	if at < 0 || at == len(email)-1 {
		return ""
	}
	return email[at+1:]
}
