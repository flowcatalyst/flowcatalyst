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
// re-pointed when it already exists. MappingScope is required whenever this
// request would create a brand-new mapping (see Validate/Execute); it has no
// effect on a domain's existing scope. PrimaryClientID, when set, is linked
// on mappings that are new or have no primary client yet; an existing client
// link is never overwritten.
type CreateCommand struct {
	Code                string   `json:"code"`
	Name                string   `json:"name"`
	Type                string   `json:"type"`
	OIDCIssuerURL       *string  `json:"oidcIssuerUrl,omitempty"`
	OIDCClientID        *string  `json:"oidcClientId,omitempty"`
	OIDCClientSecretRef *string  `json:"oidcClientSecretRef,omitempty"`
	OIDCMultiTenant     *bool    `json:"oidcMultiTenant,omitempty"`
	OIDCIssuerPattern   *string  `json:"oidcIssuerPattern,omitempty"`
	AllowedEmailDomains []string `json:"allowedEmailDomains,omitempty"`
	MappingScope        *string  `json:"mappingScope,omitempty"`
	PrimaryClientID     *string  `json:"primaryClientId,omitempty"`
	SyncRolesFromIDP    bool     `json:"syncRolesFromIdp"`
	AllowedRoleIDs      []string `json:"allowedRoleIds,omitempty"`
}

// CreateResult summarises the orchestrated create.
type CreateResult struct {
	IdentityProviderID string   `json:"identityProviderId"`
	Code               string   `json:"code"`
	DomainsCreated     []string `json:"domainsCreated"`
	DomainsClaimed     []string `json:"domainsClaimed"`
	// DomainsLinked lists domains whose mapping gained a primary client link
	// from this request (a brand-new mapping created with CLIENT scope, or an
	// existing mapping — claimed or already routed here — that had no client
	// yet). A claimed-and-linked domain appears in both DomainsClaimed and
	// DomainsLinked.
	DomainsLinked []string `json:"domainsLinked"`
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

// validateMappingScope enforces the mapping-scope contract shared by create
// and update:
//
//  1. mappingScope, when set, must parse to ANCHOR or CLIENT — PARTNER
//     mappings are managed on the email-domain page, not here.
//  2. CLIENT requires a non-blank primaryClientId.
//  3. ANCHOR forbids a primaryClientId.
//  4. A primaryClientId without a mappingScope is rejected outright — the
//     caller must say which scope the client is being linked under.
//
// Returns the parsed scope (nil when the command set neither field — legal
// only when every domain in the request already has a mapping; Execute
// enforces that) and the normalized client id (nil when blank or absent).
func validateMappingScope(mappingScope, primaryClientID *string) (*emaildomainmapping.ScopeType, *string, error) {
	var clientID *string
	if primaryClientID != nil {
		if trimmed := strings.TrimSpace(*primaryClientID); trimmed != "" {
			clientID = &trimmed
		}
	}
	if mappingScope == nil {
		if clientID != nil {
			return nil, nil, usecase.Validation("MAPPING_SCOPE_REQUIRED",
				"mappingScope is required when primaryClientId is set")
		}
		return nil, nil, nil
	}
	scope, ok := emaildomainmapping.ParseScopeType(*mappingScope)
	if !ok || scope == emaildomainmapping.ScopePartner {
		return nil, nil, usecase.Validation("INVALID_MAPPING_SCOPE",
			"mappingScope must be ANCHOR or CLIENT; partner mappings are managed on the email-domain page")
	}
	switch scope {
	case emaildomainmapping.ScopeClient:
		if clientID == nil {
			return nil, nil, usecase.Validation("PRIMARY_CLIENT_REQUIRED",
				"primaryClientId is required when mappingScope is CLIENT")
		}
	case emaildomainmapping.ScopeAnchor:
		if clientID != nil {
			return nil, nil, usecase.Validation("PRIMARY_CLIENT_NOT_ALLOWED",
				"primaryClientId is not allowed when mappingScope is ANCHOR")
		}
	}
	return &scope, clientID, nil
}

// requireScopeForNewDomains fails fast — before any row in this request is
// written — when scope is nil and any of domains has no existing mapping.
// mappingScope is required whenever the request would create a brand-new
// mapping; a nil scope is only legal when every domain already routes
// somewhere (claims and no-op links need no scope choice).
func requireScopeForNewDomains(ctx context.Context, deps Deps, domains []string, scope *emaildomainmapping.ScopeType) error {
	if scope != nil {
		return nil
	}
	for _, d := range domains {
		existing, err := deps.MoveDeps.Mappings.FindByEmailDomain(ctx, d)
		if err != nil {
			return usecase.Internal("REPO", "find_by_email_domain failed", err)
		}
		if existing == nil {
			return usecase.Validation("MAPPING_SCOPE_REQUIRED",
				"mappingScope is required: domain '"+d+"' has no mapping yet; choose ANCHOR or CLIENT")
		}
	}
	return nil
}

// mapDomainResult reports what mapDomainTx did to one domain's mapping.
type mapDomainResult struct {
	created bool
	claimed bool
	// linked reports the mapping gained a primary client from this call —
	// either a brand-new CLIENT-scoped mapping, or an existing mapping
	// (claimed or already routed here) that had no client yet.
	linked bool
}

// mapDomainTx routes one domain to ip inside the open transaction: creates a
// mapping when the domain is unknown, re-points the existing mapping (via the
// shared move behaviour) when it is routed elsewhere, or leaves it in place
// when it is already routed here. scope is the caller-validated mapping
// scope; it is only read when a new mapping is created (the caller —
// requireScopeForNewDomains — guarantees it is non-nil whenever this call
// would create one). primaryClientID is linked only on new mappings or
// mappings with no primary client yet — an existing client link is never
// overwritten, and a mapping's existing scope is never changed.
func mapDomainTx(
	ctx context.Context,
	s *usecasepgx.TxScopedUnitOfWork,
	deps Deps,
	ip *identityprovider.IdentityProvider,
	domain string,
	scope *emaildomainmapping.ScopeType,
	primaryClientID *string,
	ec usecase.ExecutionContext,
	auditCmd any,
) (mapDomainResult, error) {
	existing, err := deps.MoveDeps.Mappings.FindByEmailDomain(ctx, domain)
	if err != nil {
		return mapDomainResult{}, usecase.Internal("REPO", "find_by_email_domain failed", err)
	}
	if existing == nil {
		if scope == nil {
			// Invariant: requireScopeForNewDomains must have already rejected
			// this request before any write happened.
			return mapDomainResult{}, usecase.Internal("INVARIANT_MAPPING_SCOPE",
				"mapDomainTx reached a new mapping with no resolved scope for domain '"+domain+"'", nil)
		}
		m := emaildomainmapping.New(domain, ip.ID, *scope)
		if *scope == emaildomainmapping.ScopeClient {
			m.PrimaryClientID = primaryClientID
		}
		event := edmops.NewMappingCreatedEvent(ec, m.ID, m.EmailDomain)
		if r := usecasepgx.CommitScoped(ctx, s, m, deps.MoveDeps.Mappings, event, auditCmd); !usecase.IsSuccess(r) {
			_, e := usecase.Into(r)
			return mapDomainResult{}, e
		}
		return mapDomainResult{created: true, linked: *scope == emaildomainmapping.ScopeClient}, nil
	}
	if existing.IdentityProviderID == ip.ID {
		// Already routed here: scope untouched; link the client only if it is
		// missing.
		if primaryClientID == nil || existing.PrimaryClientID != nil {
			return mapDomainResult{}, nil
		}
		existing.PrimaryClientID = primaryClientID
		event := edmops.NewMappingUpdatedEvent(ec, existing.ID, existing.EmailDomain)
		if r := usecasepgx.CommitScoped(ctx, s, existing, deps.MoveDeps.Mappings, event, auditCmd); !usecase.IsSuccess(r) {
			_, e := usecase.Into(r)
			return mapDomainResult{}, e
		}
		return mapDomainResult{linked: true}, nil
	}
	// Claim the domain: link the client only when the mapping has none, then
	// re-point through the shared move behaviour (event + any side effects).
	linked := false
	if primaryClientID != nil && existing.PrimaryClientID == nil {
		existing.PrimaryClientID = primaryClientID
		linked = true
	}
	if _, err := edmops.MoveMappingTx(ctx, s, deps.MoveDeps, existing, ip, ec, auditCmd); err != nil {
		return mapDomainResult{}, err
	}
	return mapDomainResult{claimed: true, linked: linked}, nil
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
			typ, ok := identityprovider.ParseType(cmd.Type)
			if !ok {
				return usecase.Validation("INVALID_TYPE", "type must be INTERNAL or OIDC")
			}
			if typ == identityprovider.TypeOIDC {
				if cmd.OIDCIssuerURL == nil || strings.TrimSpace(*cmd.OIDCIssuerURL) == "" {
					return usecase.Validation("OIDC_ISSUER_REQUIRED", "OIDC IDPs require oidcIssuerUrl")
				}
				if cmd.OIDCClientID == nil || strings.TrimSpace(*cmd.OIDCClientID) == "" {
					return usecase.Validation("OIDC_CLIENT_ID_REQUIRED", "OIDC IDPs require oidcClientId")
				}
			}
			if err := validateDomains(cmd.AllowedEmailDomains); err != nil {
				return err
			}
			_, _, err := validateMappingScope(cmd.MappingScope, cmd.PrimaryClientID)
			return err
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

			// Already restricted to legal values in Validate above.
			scope, primaryClientID, err := validateMappingScope(cmd.MappingScope, cmd.PrimaryClientID)
			if err != nil {
				return zero, usecase.Internal("INVARIANT_MAPPING_SCOPE", "validated mapping scope failed to resolve", err)
			}
			domains := normalizeDomains(cmd.AllowedEmailDomains)
			if err := requireScopeForNewDomains(ctx, deps, domains, scope); err != nil {
				return zero, err
			}

			// Already restricted to a known value in Validate above.
			typ, ok := identityprovider.ParseType(cmd.Type)
			if !ok {
				return zero, usecase.Internal("INVARIANT_TYPE", "validated type failed to parse", nil)
			}
			ip := identityprovider.New(cmd.Code, cmd.Name, typ)
			ip.OIDCIssuerURL = cmd.OIDCIssuerURL
			ip.OIDCClientID = cmd.OIDCClientID
			ip.OIDCClientSecretRef = cmd.OIDCClientSecretRef
			if cmd.OIDCMultiTenant != nil {
				ip.OIDCMultiTenant = *cmd.OIDCMultiTenant
			}
			ip.OIDCIssuerPattern = cmd.OIDCIssuerPattern
			ip.SyncRolesFromIDP = cmd.SyncRolesFromIDP
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
				DomainsLinked:      []string{},
			}
			for _, domain := range domains {
				mr, err := mapDomainTx(ctx, s, deps, ip, domain, scope, primaryClientID, ec, cmd)
				if err != nil {
					return zero, err
				}
				if mr.created {
					result.DomainsCreated = append(result.DomainsCreated, domain)
				}
				if mr.claimed {
					result.DomainsClaimed = append(result.DomainsClaimed, domain)
				}
				if mr.linked {
					result.DomainsLinked = append(result.DomainsLinked, domain)
				}
			}
			return result, nil
		},
	}
}
