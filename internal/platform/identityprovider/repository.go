package identityprovider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/sqlc/dbq"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// Repository is the Postgres-backed repo. Tables: oauth_identity_providers +
// oauth_identity_provider_allowed_roles (junction; one row per platform role
// the IdP may confer via role sync). AllowedEmailDomains is derived on read
// from tnt_email_domain_mappings — the mapping table owns domain → IdP
// routing; the legacy allowed_domains junction is dead (cleared on delete).
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
	ip, err := rowToIDP(row)
	if err != nil {
		return nil, err
	}
	return r.hydrateOne(ctx, ip)
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
	ip, err := rowToIDP(row)
	if err != nil {
		return nil, err
	}
	return r.hydrateOne(ctx, ip)
}

// FindAll returns every IDP with hydrated allowed domains.
func (r *Repository) FindAll(ctx context.Context) ([]IdentityProvider, error) {
	rows, err := r.q.IdentityProviderFindAll(ctx)
	if err != nil {
		return nil, err
	}
	// A corrupted type on any one row fails the WHOLE list read (X-06: "a
	// list containing the row fails too") rather than silently skipping or
	// coercing that row.
	bare := make([]IdentityProvider, 0, len(rows))
	for _, row := range rows {
		ip, err := rowToIDP(row)
		if err != nil {
			return nil, err
		}
		bare = append(bare, *ip)
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
		SyncRolesFromIdp:    ip.SyncRolesFromIDP,
		CreatedAt:           ip.CreatedAt,
		UpdatedAt:           time.Now().UTC(),

		OidcClientAuthMethod: string(ip.OIDCClientAuthMethod),
	}); err != nil {
		return fmt.Errorf("identity_provider persist: %w", err)
	}
	if err := q.IdentityProviderAllowedRolesClear(ctx, ip.ID); err != nil {
		return err
	}
	for _, roleID := range ip.AllowedRoleIDs {
		if err := q.IdentityProviderAllowedRoleInsert(ctx, dbq.IdentityProviderAllowedRoleInsertParams{
			IdentityProviderID: ip.ID,
			RoleID:             roleID,
		}); err != nil {
			return err
		}
	}
	// AllowedEmailDomains is intentionally NOT persisted — it is derived from
	// tnt_email_domain_mappings on read.
	return nil
}

// Delete removes the row and clears its junctions (allowed roles + the
// legacy allowed_domains rows, so migrated installs don't accumulate
// orphans).
func (r *Repository) Delete(ctx context.Context, ip *IdentityProvider, tx *usecasepgx.DbTx) error {
	q := r.q.WithTx(tx.Inner())
	if err := q.IdentityProviderDomainsClear(ctx, ip.ID); err != nil {
		return err
	}
	if err := q.IdentityProviderAllowedRolesClear(ctx, ip.ID); err != nil {
		return err
	}
	// The certificates carry no foreign key (docs/foreign-keys.md): they go
	// with their provider, in this same transaction.
	if err := q.IdentityProviderCertificatesDelete(ctx, ip.ID); err != nil {
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
	// Domains are derived from the mapping table (single source of truth for
	// domain → IdP routing); the allowed-roles junction is this repo's own.
	domainRows, err := r.q.IdentityProviderMappedDomainsForIDPs(ctx, ids)
	if err != nil {
		return nil, err
	}
	roleRows, err := r.q.IdentityProviderAllowedRolesForIDPs(ctx, ids)
	if err != nil {
		return nil, err
	}
	domainsByID := make(map[string][]string, len(idps))
	nonAnchor := make(map[string]bool, len(idps))
	for _, row := range domainRows {
		if row.ScopeType != "ANCHOR" {
			nonAnchor[row.IdentityProviderID] = true
		}
		domainsByID[row.IdentityProviderID] = append(domainsByID[row.IdentityProviderID], row.EmailDomain)
	}
	rolesByID := make(map[string][]string, len(idps))
	for _, row := range roleRows {
		rolesByID[row.IdentityProviderID] = append(rolesByID[row.IdentityProviderID], row.RoleID)
	}
	for i := range idps {
		domains := domainsByID[idps[i].ID]
		if domains == nil {
			domains = []string{}
		}
		idps[i].AllowedEmailDomains = domains
		idps[i].ServesNonAnchorDomain = nonAnchor[idps[i].ID]
		roles := rolesByID[idps[i].ID]
		if roles == nil {
			roles = []string{}
		}
		idps[i].AllowedRoleIDs = roles
	}
	return idps, nil
}

// rowToIDP hydrates the entity from its row. A type value that isn't one of
// the known Type constants (junk written before write-boundary validation
// existed, or a hand-edited row) is a loud read error — never
// round-tripped as-is and never coerced to INTERNAL, per the X-06 ruling.
// The row id is logged so the bad row can be found and fixed without a
// debugger.
func rowToIDP(row dbq.OauthIdentityProvider) (*IdentityProvider, error) {
	typ, ok := ParseType(row.Type)
	if !ok {
		slog.Error("identity provider row has unrecognised type", "id", row.ID, "type", row.Type)
		return nil, usecase.Internal("CORRUPT_IDENTITY_PROVIDER_TYPE",
			fmt.Sprintf("identity provider %s has an unrecognised type", row.ID), nil)
	}
	method, ok := ParseClientAuthMethod(row.OidcClientAuthMethod)
	if !ok {
		slog.Error("identity provider row has unrecognised client auth method", "id", row.ID, "method", row.OidcClientAuthMethod)
		return nil, usecase.Internal("CORRUPT_IDENTITY_PROVIDER_CLIENT_AUTH_METHOD",
			fmt.Sprintf("identity provider %s has an unrecognised client authentication method", row.ID), nil)
	}
	return &IdentityProvider{
		OIDCClientAuthMethod: method,
		ID:                   row.ID,
		Code:                 row.Code,
		Name:                 row.Name,
		Type:                 typ,
		OIDCIssuerURL:        row.OidcIssuerUrl,
		OIDCClientID:         row.OidcClientID,
		OIDCClientSecretRef:  row.OidcClientSecretRef,
		OIDCMultiTenant:      row.OidcMultiTenant,
		OIDCIssuerPattern:    row.OidcIssuerPattern,
		SyncRolesFromIDP:     row.SyncRolesFromIdp,
		CreatedAt:            row.CreatedAt,
		UpdatedAt:            row.UpdatedAt,
		AllowedEmailDomains:  []string{},
		AllowedRoleIDs:       []string{},
	}, nil
}

// ---- Client-credential certificates -------------------------------------

// FindCertificates returns the provider's certificates (ACTIVE and NEXT).
func (r *Repository) FindCertificates(ctx context.Context, idpID string) ([]Certificate, error) {
	rows, err := r.q.IdentityProviderCertificatesFor(ctx, idpID)
	if err != nil {
		return nil, fmt.Errorf("identity_provider certificates: %w", err)
	}
	out := make([]Certificate, 0, len(rows))
	for _, row := range rows {
		c, err := rowToCertificate(row)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, nil
}

// FindCertificate returns the provider's certificate in the given status, nil
// when there is none.
func (r *Repository) FindCertificate(ctx context.Context, idpID string, status CertificateStatus) (*Certificate, error) {
	row, err := r.q.IdentityProviderCertificateByStatus(ctx, dbq.IdentityProviderCertificateByStatusParams{
		IdentityProviderID: idpID, Status: string(status),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("identity_provider certificate: %w", err)
	}
	return rowToCertificate(row)
}

// FindCertificateByID loads one certificate, nil when there is none.
func (r *Repository) FindCertificateByID(ctx context.Context, id string) (*Certificate, error) {
	row, err := r.q.IdentityProviderCertificateByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("identity_provider certificate: %w", err)
	}
	return rowToCertificate(row)
}

// InsertCertificate stores c inside the open transaction.
func (r *Repository) InsertCertificate(ctx context.Context, tx pgx.Tx, c *Certificate) error {
	return r.q.WithTx(tx).IdentityProviderCertificateInsert(ctx, dbq.IdentityProviderCertificateInsertParams{
		ID: c.ID, IdentityProviderID: c.IdentityProviderID, Status: string(c.Status),
		PrivateKeyRef: c.PrivateKeyRef, CertificatePem: c.CertificatePEM,
		ThumbprintSha1: c.ThumbprintSHA1, ThumbprintSha256: c.ThumbprintSHA256,
		NotBefore: c.NotBefore, NotAfter: c.NotAfter, CreatedAt: c.CreatedAt,
	})
}

// DeleteCertificate removes one certificate inside the open transaction.
func (r *Repository) DeleteCertificate(ctx context.Context, tx pgx.Tx, id string) error {
	return r.q.WithTx(tx).IdentityProviderCertificateDelete(ctx, id)
}

// SetCertificateStatus moves one certificate to status inside the open
// transaction. The (provider, status) pair is unique, so the row now holding
// the target status must be gone first.
func (r *Repository) SetCertificateStatus(ctx context.Context, tx pgx.Tx, id string, status CertificateStatus) error {
	return r.q.WithTx(tx).IdentityProviderCertificateSetStatus(ctx, dbq.IdentityProviderCertificateSetStatusParams{
		ID: id, Status: string(status),
	})
}

func rowToCertificate(row dbq.OauthIdentityProviderCertificate) (*Certificate, error) {
	st := CertificateStatus(row.Status)
	if st != CertificateActive && st != CertificateNext {
		slog.Error("identity provider certificate row has unrecognised status", "id", row.ID, "status", row.Status)
		return nil, usecase.Internal("CORRUPT_IDENTITY_PROVIDER_CERTIFICATE_STATUS",
			fmt.Sprintf("identity provider certificate %s has an unrecognised status", row.ID), nil)
	}
	return &Certificate{
		ID: row.ID, IdentityProviderID: row.IdentityProviderID, Status: st,
		PrivateKeyRef: row.PrivateKeyRef, CertificatePEM: row.CertificatePem,
		ThumbprintSHA1: row.ThumbprintSha1, ThumbprintSHA256: row.ThumbprintSha256,
		NotBefore: row.NotBefore, NotAfter: row.NotAfter, CreatedAt: row.CreatedAt,
	}, nil
}
