package operations

import (
	"context"
	"strings"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	edmops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// CreateCommand is the input DTO. AllowedEmailDomains drives the email-domain
// mapping table (the single source of truth for domain → IdP routing): each
// listed domain is mapped to the new provider — created when unknown,
// re-pointed when it already exists. PrimaryClientID, when set, is linked on
// mappings that are new or have no primary client yet; an existing client
// link is never overwritten.
type CreateCommand struct {
	Code                string   `json:"code"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	OIDCIssuerURL       *string  `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string  `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef *string  `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     bool     `json:"oidcMultiTenant"`
	OIDCIssuerPattern   *string  `json:"oidcIssuerPattern,omitempty"`
	AllowedEmailDomains []string `json:"allowedEmailDomains,omitempty"`
	PrimaryClientID     *string  `json:"primaryClientId,omitempty"`
	SyncRolesFromIDP    bool     `json:"syncRolesFromIdp"`
	PortalClientID      *string  `json:"portalClientId,omitempty"`
	AllowedRoleIDs      []string `json:"allowedRoleIds,omitempty"`
}

// CreateResult summarises the orchestrated create.
type CreateResult struct {
	IdentityProviderID string   `json:"identityProviderId"`
	Code               string   `json:"code"`
	DomainsCreated     []string `json:"domainsCreated"`
	DomainsClaimed     []string `json:"domainsClaimed"`
}

// Deps bundles the repositories the identity-provider orchestration ops need.
type Deps struct {
	Repo     *identityprovider.Repository
	MoveDeps edmops.MoveDeps
}

// normalizeDomains lower-cases, trims, and dedupes ds, preserving order.
func normalizeDomains(ds []string) []string {
	seen := make(map[string]struct{}, len(ds))
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		n := strings.ToLower(strings.TrimSpace(d))
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out
}

// validateDomains applies the same DNS-name shape check the mapping create op
// uses.
func validateDomains(ds []string) error {
	for _, d := range normalizeDomains(ds) {
		if !strings.Contains(d, ".") || strings.ContainsAny(d, " /@") {
			return usecase.Validation("INVALID_EMAIL_DOMAIN",
				"Email domain '"+d+"' must be a valid DNS name (e.g. example.com)")
		}
	}
	return nil
}

// mapDomainTx routes one domain to ip inside the open transaction: creates a
// mapping when the domain is unknown, re-points the existing mapping (via the
// shared move behaviour) when it is. primaryClientID is linked only on new
// mappings or mappings with no primary client yet. Returns (created, claimed).
func mapDomainTx(
	ctx context.Context,
	s *usecasepgx.TxScopedUnitOfWork,
	deps Deps,
	ip *identityprovider.IdentityProvider,
	domain string,
	primaryClientID *string,
	ec usecase.ExecutionContext,
	auditCmd any,
) (bool, bool, error) {
	existing, err := deps.MoveDeps.Mappings.FindByEmailDomain(ctx, domain)
	if err != nil {
		return false, false, usecase.Internal("REPO", "find_by_email_domain failed", err)
	}
	if existing == nil {
		scope := emaildomainmapping.ScopeAnchor
		if primaryClientID != nil {
			scope = emaildomainmapping.ScopeClient
		}
		m := emaildomainmapping.New(domain, ip.ID, scope)
		m.PrimaryClientID = primaryClientID
		event := edmops.NewMappingCreatedEvent(ec, m.ID, m.EmailDomain)
		if r := usecasepgx.CommitScoped(ctx, s, m, deps.MoveDeps.Mappings, event, auditCmd); !usecase.IsSuccess(r) {
			_, e := usecase.Into(r)
			return false, false, e
		}
		return true, false, nil
	}
	if existing.IdentityProviderID == ip.ID {
		return false, false, nil // already routed here
	}
	// Claim the domain: link the client only when the mapping has none, then
	// re-point through the shared move behaviour (event + any side effects).
	if primaryClientID != nil && existing.PrimaryClientID == nil {
		existing.PrimaryClientID = primaryClientID
	}
	if _, err := edmops.MoveMappingTx(ctx, s, deps.MoveDeps, existing, ip, ec, auditCmd); err != nil {
		return false, false, err
	}
	return false, true, nil
}

// CreateIdentityProvider validates cmd, enforces code uniqueness, persists the
// IdP, and maps each listed email domain to it — all in one transaction. The
// coarse anchor-only write permission is enforced at the controller.
func CreateIdentityProvider(deps Deps) usecaseop.TxOperation[CreateCommand, CreateResult] {
	return usecaseop.TxOperation[CreateCommand, CreateResult]{
		Name: "CreateIdentityProvider",
		Validate: func(_ context.Context, cmd CreateCommand) error {
			if strings.TrimSpace(cmd.Code) == "" {
				return usecase.Validation("CODE_REQUIRED", "code is required")
			}
			if strings.TrimSpace(cmd.Name) == "" {
				return usecase.Validation("NAME_REQUIRED", "name is required")
			}
			if identityprovider.ParseType(cmd.Type) == identityprovider.TypeOIDC {
				if cmd.OIDCIssuerURL == nil || strings.TrimSpace(*cmd.OIDCIssuerURL) == "" {
					return usecase.Validation("OIDC_ISSUER_REQUIRED", "OIDC IDPs require oidcIssuerUrl")
				}
				if cmd.OIDCClientID == nil || strings.TrimSpace(*cmd.OIDCClientID) == "" {
					return usecase.Validation("OIDC_CLIENT_ID_REQUIRED", "OIDC IDPs require oidcClientId")
				}
			}
			return validateDomains(cmd.AllowedEmailDomains)
		},
		// The coarse "may write identity providers" permission (anchor-only) is
		// enforced at the controller; there is no per-resource authz dimension.
		Authorize: usecaseop.Public[CreateCommand],
		Execute: func(ctx context.Context, s *usecasepgx.TxScopedUnitOfWork, cmd CreateCommand, ec usecase.ExecutionContext) (CreateResult, error) {
			var zero CreateResult

			existing, err := deps.Repo.FindByCode(ctx, cmd.Code)
			if err != nil {
				return zero, usecase.Internal("REPO", "find_by_code failed", err)
			}
			if existing != nil {
				return zero, usecase.Conflict("CODE_EXISTS", "Identity provider with code '"+cmd.Code+"' already exists")
			}

			ip := identityprovider.New(cmd.Code, cmd.Name, identityprovider.ParseType(cmd.Type))
			ip.OIDCIssuerURL = cmd.OIDCIssuerURL
			ip.OIDCClientID = cmd.OIDCClientID
			ip.OIDCClientSecretRef = cmd.OIDCClientSecretRef
			ip.OIDCMultiTenant = cmd.OIDCMultiTenant
			ip.OIDCIssuerPattern = cmd.OIDCIssuerPattern
			ip.SyncRolesFromIDP = cmd.SyncRolesFromIDP
			ip.PortalClientID = cmd.PortalClientID
			if cmd.AllowedRoleIDs != nil {
				ip.AllowedRoleIDs = cmd.AllowedRoleIDs
			}

			event := IdentityProviderCreated{
				Metadata:           usecase.NewEventMetadata(ec, IdentityProviderCreatedType, Source, subjectFor(ip.ID)),
				IdentityProviderID: ip.ID,
				Code:               ip.Code,
			}
			if r := usecasepgx.CommitScoped(ctx, s, ip, deps.Repo, event, cmd); !usecase.IsSuccess(r) {
				_, e := usecase.Into(r)
				return zero, e
			}

			result := CreateResult{
				IdentityProviderID: ip.ID,
				Code:               ip.Code,
				DomainsCreated:     []string{},
				DomainsClaimed:     []string{},
			}
			for _, domain := range normalizeDomains(cmd.AllowedEmailDomains) {
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
			return result, nil
		},
	}
}
