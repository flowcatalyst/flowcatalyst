package client

import (
	"context"

	"go.flowcatalyst.tech/internal/common/repository"
)

// instrumentedRepository wraps a Repository with metrics and logging
type instrumentedRepository struct {
	inner Repository
}

// newInstrumentedRepository creates an instrumented wrapper around a Repository
func newInstrumentedRepository(inner Repository) Repository {
	return &instrumentedRepository{inner: inner}
}

// === Client operations ===

func (r *instrumentedRepository) FindByID(ctx context.Context, id string) (*Client, error) {
	return repository.Instrument(ctx, collectionClients, "FindByID", func() (*Client, error) {
		return r.inner.FindByID(ctx, id)
	})
}

func (r *instrumentedRepository) FindByIdentifier(ctx context.Context, identifier string) (*Client, error) {
	return repository.Instrument(ctx, collectionClients, "FindByIdentifier", func() (*Client, error) {
		return r.inner.FindByIdentifier(ctx, identifier)
	})
}

func (r *instrumentedRepository) FindAll(ctx context.Context, skip, limit int64) ([]*Client, error) {
	return repository.Instrument(ctx, collectionClients, "FindAll", func() ([]*Client, error) {
		return r.inner.FindAll(ctx, skip, limit)
	})
}

func (r *instrumentedRepository) FindByStatus(ctx context.Context, status ClientStatus) ([]*Client, error) {
	return repository.Instrument(ctx, collectionClients, "FindByStatus", func() ([]*Client, error) {
		return r.inner.FindByStatus(ctx, status)
	})
}

func (r *instrumentedRepository) Search(ctx context.Context, query string) ([]*Client, error) {
	return repository.Instrument(ctx, collectionClients, "Search", func() ([]*Client, error) {
		return r.inner.Search(ctx, query)
	})
}

func (r *instrumentedRepository) Insert(ctx context.Context, client *Client) error {
	return repository.InstrumentVoid(ctx, collectionClients, "Insert", func() error {
		return r.inner.Insert(ctx, client)
	})
}

func (r *instrumentedRepository) Update(ctx context.Context, client *Client) error {
	return repository.InstrumentVoid(ctx, collectionClients, "Update", func() error {
		return r.inner.Update(ctx, client)
	})
}

func (r *instrumentedRepository) UpdateStatus(ctx context.Context, id string, status ClientStatus, reason string) error {
	return repository.InstrumentVoid(ctx, collectionClients, "UpdateStatus", func() error {
		return r.inner.UpdateStatus(ctx, id, status, reason)
	})
}

func (r *instrumentedRepository) AddNote(ctx context.Context, id string, note ClientNote) error {
	return repository.InstrumentVoid(ctx, collectionClients, "AddNote", func() error {
		return r.inner.AddNote(ctx, id, note)
	})
}

func (r *instrumentedRepository) Delete(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionClients, "Delete", func() error {
		return r.inner.Delete(ctx, id)
	})
}

// === Access Grant operations ===

func (r *instrumentedRepository) FindAccessGrantsByPrincipal(ctx context.Context, principalID string) ([]*ClientAccessGrant, error) {
	return repository.Instrument(ctx, collectionAccessGrants, "FindAccessGrantsByPrincipal", func() ([]*ClientAccessGrant, error) {
		return r.inner.FindAccessGrantsByPrincipal(ctx, principalID)
	})
}

func (r *instrumentedRepository) FindAccessGrantsByClient(ctx context.Context, clientID string) ([]*ClientAccessGrant, error) {
	return repository.Instrument(ctx, collectionAccessGrants, "FindAccessGrantsByClient", func() ([]*ClientAccessGrant, error) {
		return r.inner.FindAccessGrantsByClient(ctx, clientID)
	})
}

func (r *instrumentedRepository) GrantAccess(ctx context.Context, grant *ClientAccessGrant) error {
	return repository.InstrumentVoid(ctx, collectionAccessGrants, "GrantAccess", func() error {
		return r.inner.GrantAccess(ctx, grant)
	})
}

func (r *instrumentedRepository) RevokeAccess(ctx context.Context, principalID, clientID string) error {
	return repository.InstrumentVoid(ctx, collectionAccessGrants, "RevokeAccess", func() error {
		return r.inner.RevokeAccess(ctx, principalID, clientID)
	})
}

func (r *instrumentedRepository) HasAccess(ctx context.Context, principalID, clientID string) (bool, error) {
	return repository.Instrument(ctx, collectionAccessGrants, "HasAccess", func() (bool, error) {
		return r.inner.HasAccess(ctx, principalID, clientID)
	})
}

// === Anchor Domain operations ===

func (r *instrumentedRepository) FindAnchorDomains(ctx context.Context) ([]*AnchorDomain, error) {
	return repository.Instrument(ctx, collectionAnchorDomains, "FindAnchorDomains", func() ([]*AnchorDomain, error) {
		return r.inner.FindAnchorDomains(ctx)
	})
}

func (r *instrumentedRepository) IsAnchorDomain(ctx context.Context, domain string) (bool, error) {
	return repository.Instrument(ctx, collectionAnchorDomains, "IsAnchorDomain", func() (bool, error) {
		return r.inner.IsAnchorDomain(ctx, domain)
	})
}

func (r *instrumentedRepository) AddAnchorDomain(ctx context.Context, domain *AnchorDomain) error {
	return repository.InstrumentVoid(ctx, collectionAnchorDomains, "AddAnchorDomain", func() error {
		return r.inner.AddAnchorDomain(ctx, domain)
	})
}

func (r *instrumentedRepository) RemoveAnchorDomain(ctx context.Context, domain string) error {
	return repository.InstrumentVoid(ctx, collectionAnchorDomains, "RemoveAnchorDomain", func() error {
		return r.inner.RemoveAnchorDomain(ctx, domain)
	})
}

// === Auth Config operations ===

func (r *instrumentedRepository) FindAuthConfigByDomain(ctx context.Context, emailDomain string) (*ClientAuthConfig, error) {
	return repository.Instrument(ctx, collectionAuthConfigs, "FindAuthConfigByDomain", func() (*ClientAuthConfig, error) {
		return r.inner.FindAuthConfigByDomain(ctx, emailDomain)
	})
}

func (r *instrumentedRepository) FindAllAuthConfigs(ctx context.Context) ([]*ClientAuthConfig, error) {
	return repository.Instrument(ctx, collectionAuthConfigs, "FindAllAuthConfigs", func() ([]*ClientAuthConfig, error) {
		return r.inner.FindAllAuthConfigs(ctx)
	})
}

func (r *instrumentedRepository) InsertAuthConfig(ctx context.Context, config *ClientAuthConfig) error {
	return repository.InstrumentVoid(ctx, collectionAuthConfigs, "InsertAuthConfig", func() error {
		return r.inner.InsertAuthConfig(ctx, config)
	})
}

func (r *instrumentedRepository) UpdateAuthConfig(ctx context.Context, config *ClientAuthConfig) error {
	return repository.InstrumentVoid(ctx, collectionAuthConfigs, "UpdateAuthConfig", func() error {
		return r.inner.UpdateAuthConfig(ctx, config)
	})
}

func (r *instrumentedRepository) DeleteAuthConfig(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionAuthConfigs, "DeleteAuthConfig", func() error {
		return r.inner.DeleteAuthConfig(ctx, id)
	})
}

// === IDP Role Mapping operations ===

func (r *instrumentedRepository) FindIdpRoleMappingsByDomain(ctx context.Context, emailDomain string) ([]*IdpRoleMapping, error) {
	return repository.Instrument(ctx, collectionRoleMappings, "FindIdpRoleMappingsByDomain", func() ([]*IdpRoleMapping, error) {
		return r.inner.FindIdpRoleMappingsByDomain(ctx, emailDomain)
	})
}

func (r *instrumentedRepository) FindAllIdpRoleMappings(ctx context.Context) ([]*IdpRoleMapping, error) {
	return repository.Instrument(ctx, collectionRoleMappings, "FindAllIdpRoleMappings", func() ([]*IdpRoleMapping, error) {
		return r.inner.FindAllIdpRoleMappings(ctx)
	})
}

func (r *instrumentedRepository) InsertIdpRoleMapping(ctx context.Context, mapping *IdpRoleMapping) error {
	return repository.InstrumentVoid(ctx, collectionRoleMappings, "InsertIdpRoleMapping", func() error {
		return r.inner.InsertIdpRoleMapping(ctx, mapping)
	})
}

func (r *instrumentedRepository) DeleteIdpRoleMapping(ctx context.Context, id string) error {
	return repository.InstrumentVoid(ctx, collectionRoleMappings, "DeleteIdpRoleMapping", func() error {
		return r.inner.DeleteIdpRoleMapping(ctx, id)
	})
}

func (r *instrumentedRepository) DeleteIdpRoleMappingsByDomain(ctx context.Context, emailDomain string) error {
	return repository.InstrumentVoid(ctx, collectionRoleMappings, "DeleteIdpRoleMappingsByDomain", func() error {
		return r.inner.DeleteIdpRoleMappingsByDomain(ctx, emailDomain)
	})
}
