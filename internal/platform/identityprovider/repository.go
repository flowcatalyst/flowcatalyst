package identityprovider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// Repository is the Postgres-backed repo. Tables:
// oauth_identity_providers + oauth_identity_provider_allowed_domains
// (junction; one row per allowed email domain).
type Repository struct{ q *dbq.Queries }

// NewRepository wires a repo.
func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{q: dbq.New(pool)}
}

// FindByID loads by id.
func (r *Repository) FindByID(ctx context.Context, id string) (*IdentityProvider, error) {
	row, err := r.q.IdentityProviderFindByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("identity_provider repo: %w", err)
	}
	return r.hydrateOne(ctx, rowToIDP(row))
}

// FindByCode loads by unique code.
func (r *Repository) FindByCode(ctx context.Context, code string) (*IdentityProvider, error) {
	row, err := r.q.IdentityProviderFindByCode(ctx, code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("identity_provider repo: %w", err)
	}
	return r.hydrateOne(ctx, rowToIDP(row))
}

// FindAll returns every IDP with hydrated allowed domains.
func (r *Repository) FindAll(ctx context.Context) ([]IdentityProvider, error) {
	rows, err := r.q.IdentityProviderFindAll(ctx)
	if err != nil {
		return nil, err
	}
	bare := make([]IdentityProvider, 0, len(rows))
	for _, row := range rows {
		bare = append(bare, *rowToIDP(row))
	}
	return r.hydrateAll(ctx, bare)
}

// Persist implements usecasepgx.Persist[IdentityProvider]. Replaces the
// junction rows wholesale.
func (r *Repository) Persist(ctx context.Context, ip *IdentityProvider, tx *usecasepgx.DbTx) error {
	q := r.q.WithTx(tx.Inner())
	if err := q.IdentityProviderUpsert(ctx, dbq.IdentityProviderUpsertParams{
		ID:                  ip.ID,
		Code:                ip.Code,
		Name:                ip.Name,
		Type:                string(ip.Type),
		OidcIssuerUrl:       ip.OIDCIssuerURL,
		OidcClientID:        ip.OIDCClientID,
		OidcClientSecretRef: ip.OIDCClientSecretRef,
		OidcMultiTenant:     ip.OIDCMultiTenant,
		OidcIssuerPattern:   ip.OIDCIssuerPattern,
		CreatedAt:           ip.CreatedAt,
		UpdatedAt:           time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("identity_provider persist: %w", err)
	}
	if err := q.IdentityProviderDomainsClear(ctx, ip.ID); err != nil {
		return err
	}
	for _, d := range ip.AllowedEmailDomains {
		if err := q.IdentityProviderDomainInsert(ctx, dbq.IdentityProviderDomainInsertParams{
			IdentityProviderID: ip.ID,
			EmailDomain:        d,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Delete removes the row; the junction table cascades (or is cleared
// explicitly above; either way the orphans are bounded).
func (r *Repository) Delete(ctx context.Context, ip *IdentityProvider, tx *usecasepgx.DbTx) error {
	q := r.q.WithTx(tx.Inner())
	if err := q.IdentityProviderDomainsClear(ctx, ip.ID); err != nil {
		return err
	}
	return q.IdentityProviderDelete(ctx, ip.ID)
}

func (r *Repository) hydrateOne(ctx context.Context, ip *IdentityProvider) (*IdentityProvider, error) {
	out, err := r.hydrateAll(ctx, []IdentityProvider{*ip})
	if err != nil {
		return nil, err
	}
	return &out[0], nil
}

func (r *Repository) hydrateAll(ctx context.Context, idps []IdentityProvider) ([]IdentityProvider, error) {
	if len(idps) == 0 {
		return idps, nil
	}
	ids := make([]string, len(idps))
	for i, ip := range idps {
		ids[i] = ip.ID
	}
	rows, err := r.q.IdentityProviderDomainsForIDPs(ctx, ids)
	if err != nil {
		return nil, err
	}
	byID := make(map[string][]string, len(idps))
	for _, row := range rows {
		byID[row.IdentityProviderID] = append(byID[row.IdentityProviderID], row.EmailDomain)
	}
	for i := range idps {
		domains := byID[idps[i].ID]
		if domains == nil {
			domains = []string{}
		}
		idps[i].AllowedEmailDomains = domains
	}
	return idps, nil
}

func rowToIDP(row dbq.OauthIdentityProvider) *IdentityProvider {
	return &IdentityProvider{
		ID:                  row.ID,
		Code:                row.Code,
		Name:                row.Name,
		Type:                ParseType(row.Type),
		OIDCIssuerURL:       row.OidcIssuerUrl,
		OIDCClientID:        row.OidcClientID,
		OIDCClientSecretRef: row.OidcClientSecretRef,
		OIDCMultiTenant:     row.OidcMultiTenant,
		OIDCIssuerPattern:   row.OidcIssuerPattern,
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
		AllowedEmailDomains: []string{},
	}
}
