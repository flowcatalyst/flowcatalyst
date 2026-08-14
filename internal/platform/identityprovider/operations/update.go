package operations

import (
	"context"
	"strings"

	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// internalIdPCode is the seeded internal (password-auth) provider's code.
// Domains removed from an OIDC provider fall back to it.
const internalIdPCode = "internal"

// UpdateCommand is the input DTO. Nil slices mean "do not change";
// AllowedEmailDomains, when supplied, is the desired set of domains routed to
// this provider — additions are mapped (created or claimed, like create) and
// removals fall back to the internal provider (converting the domain's
// OIDC-provisioned users back to internal auth).
type UpdateCommand struct {
	ID                  string   `json:"id"`
	Name                *string  `json:"name,omitempty"`
	OIDCIssuerURL       *string  `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string  `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef *string  `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     *bool    `json:"oidcMultiTenant,omitempty"`
	OIDCIssuerPattern   *string  `json:"oidcIssuerPattern,omitempty"`
	AllowedEmailDomains []string `json:"allowedEmailDomains,omitempty"`
	PrimaryClientID     *string  `json:"primaryClientId,omitempty"`
	SyncRolesFromIDP    *bool    `json:"syncRolesFromIdp,omitempty"`
	PortalClientID      *string  `json:"portalClientId,omitempty"`
	AllowedRoleIDs      []string `json:"allowedRoleIds,omitempty"`
}

// UpdateResult summarises the orchestrated update.
type UpdateResult struct {
	IdentityProviderID string   `json:"identityProviderId"`
	Code               string   `json:"code"`
	DomainsCreated     []string `json:"domainsCreated"`
	DomainsClaimed     []string `json:"domainsClaimed"`
	DomainsReleased    []string `json:"domainsReleased"`
	// UsersReset counts OIDC-provisioned users converted back to internal
	// auth because their domain was removed from this provider.
	UsersReset int `json:"usersReset"`
}

// UpdateIdentityProvider mutates an existing IdP and reconciles the mapping
// table with the supplied domain set, all in one transaction.
func UpdateIdentityProvider(deps Deps) usecaseop.TxOperation[UpdateCommand, UpdateResult] {
	return usecaseop.TxOperation[UpdateCommand, UpdateResult]{
		Name: "UpdateIdentityProvider",
		Validate: func(_ context.Context, cmd UpdateCommand) error {
			if strings.TrimSpace(cmd.ID) == "" {
				return usecase.Validation("ID_REQUIRED", "id is required")
			}
			if cmd.Name != nil && strings.TrimSpace(*cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name cannot be empty")
			}
			return validateDomains(cmd.AllowedEmailDomains)
		},
		// The coarse "may write identity providers" permission (anchor-only) is
		// enforced at the controller; there is no per-resource authz dimension.
		Authorize: usecaseop.Public[UpdateCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd UpdateCommand, ec usecase.ExecutionContext) (UpdateResult, error) {
			var zero UpdateResult

			ip, err := deps.Repo.FindByID(ctx, cmd.ID)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_id failed", err)
			}
			if ip == nil {
				return zero, httperror.NotFound("IdentityProvider", cmd.ID)
			}
			if cmd.Name != nil {
				ip.Name = strings.TrimSpace(*cmd.Name)
			}
			if cmd.OIDCIssuerURL != nil {
				ip.OIDCIssuerURL = cmd.OIDCIssuerURL
			}
			if cmd.OIDCClientID != nil {
				ip.OIDCClientID = cmd.OIDCClientID
			}
			if cmd.OIDCClientSecretRef != nil {
				ip.OIDCClientSecretRef = cmd.OIDCClientSecretRef
			}
			if cmd.OIDCMultiTenant != nil {
				ip.OIDCMultiTenant = *cmd.OIDCMultiTenant
			}
			if cmd.OIDCIssuerPattern != nil {
				ip.OIDCIssuerPattern = cmd.OIDCIssuerPattern
			}
			if cmd.SyncRolesFromIDP != nil {
				ip.SyncRolesFromIDP = *cmd.SyncRolesFromIDP
			}
			if cmd.PortalClientID != nil {
				// Empty string clears the portal binding; a value sets it.
				if trimmed := strings.TrimSpace(*cmd.PortalClientID); trimmed == "" {
					ip.PortalClientID = nil
				} else {
					ip.PortalClientID = &trimmed
				}
			}
			if cmd.AllowedRoleIDs != nil {
				ip.AllowedRoleIDs = cmd.AllowedRoleIDs
			}

			event := IdentityProviderUpdated{
				Metadata:           usecase.NewEventMetadata(ec, IdentityProviderUpdatedType, Source, subjectFor(ip.ID)),
				IdentityProviderID: ip.ID,
				Code:               ip.Code,
			}
			if r := usecasepgx.CommitScoped(ctx, s, ip, deps.Repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			result := UpdateResult{
				IdentityProviderID: ip.ID,
				Code:               ip.Code,
				DomainsCreated:     []string{},
				DomainsClaimed:     []string{},
				DomainsReleased:    []string{},
			}
			if cmd.AllowedEmailDomains == nil {
				return result, nil // domain set untouched
			}

			desired := normalizeDomains(cmd.AllowedEmailDomains)
			desiredSet := make(map[string]struct{}, len(desired))
			for _, d := range desired {
				desiredSet[d] = struct{}{}
			}
			current, err := deps.MoveDeps.Mappings.FindByIdentityProvider(ctx, ip.ID)
			if err != nil {
				return zero, usecase.Internal("REPO", "mappings by identity provider failed", err)
			}

			// Additions: map like create does.
			for _, domain := range desired {
				created, claimed, err := mapDomainTx(ctx, s, deps, ip, domain, cmd.PrimaryClientID, ec, cmd)
				if err != nil {
					return zero, err
				}
				if created {
					result.DomainsCreated = append(result.DomainsCreated, domain)
				}
				if claimed {
					result.DomainsClaimed = append(result.DomainsClaimed, domain)
				}
			}

			// Removals: fall back to the internal provider. The mapping (and
			// its client/2FA config) survives; only the routing — and, for an
			// OIDC source, the affected users' provider marker + IDP_SYNC
			// roles — changes. Deleting a mapping outright stays an explicit
			// act on the email-domain page.
			if ip.Code != internalIdPCode {
				var internal *identityprovider.IdentityProvider
				for i := range current {
					m := &current[i]
					if _, keep := desiredSet[m.EmailDomain]; keep {
						continue
					}
					if internal == nil {
						internal, err = deps.Repo.FindByCode(ctx, internalIdPCode)
						if err != nil {
							return zero, usecase.Internal("REPO", "internal idp lookup failed", err)
						}
						if internal == nil {
							return zero, usecase.Internal("SEED",
								"internal identity provider missing; cannot release domain '"+m.EmailDomain+"'", nil)
						}
					}
					reset, err := edmops.MoveMappingTx(ctx, s, deps.MoveDeps, m, internal, ec, cmd)
					if err != nil {
						return zero, err
					}
					result.DomainsReleased = append(result.DomainsReleased, m.EmailDomain)
					result.UsersReset += reset
				}
			}
			return result, nil
		},
	}
}
