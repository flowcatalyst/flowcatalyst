// Package api wires HTTP routes for email_domain_mapping via
// danielgtaylor/huma/v2. Anchor-only.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles the dependencies.
//
// IDPRepo is optional: when set, responses are enriched with the mapping's
// identityProviderName (the SPA reads this field). When nil, the name is
// omitted — the mapping is still returned without the join.
type State struct {
	Repo    *emaildomainmapping.Repository
	IDPRepo *identityprovider.Repository
	UoW     *usecasepgx.UnitOfWork
}

const tag = "email-domain-mappings"

// Register mounts the email-domain-mapping endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listEmailDomainMappings", "/api/email-domain-mappings", "List email-domain mappings", s.list)
	apiroute.Post(g, "createEmailDomainMapping", "/api/email-domain-mappings", "Create an email-domain mapping", http.StatusCreated, s.create)
	apiroute.Get(g, "lookupEmailDomainMapping", "/api/email-domain-mappings/lookup", "Resolve an email domain to its mapping", s.lookup)
	apiroute.Get(g, "getEmailDomainMappingByDomain", "/api/email-domain-mappings/by-domain/{domain}", "Resolve an email domain to its mapping (path param)", s.byDomain)
	apiroute.Get(g, "getEmailDomainMapping", "/api/email-domain-mappings/{id}", "Get an email-domain mapping by id", s.getByID)
	apiroute.Put(g, "updateEmailDomainMapping", "/api/email-domain-mappings/{id}", "Update an email-domain mapping", http.StatusNoContent, s.update)
	apiroute.Delete(g, "deleteEmailDomainMapping", "/api/email-domain-mappings/{id}", "Delete an email-domain mapping", http.StatusNoContent, s.delete)
}

func (s *State) list(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[MappingListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	names := s.idpNames(ctx, rows)
	out := apicommon.MapSlice(rows, func(e *emaildomainmapping.EmailDomainMapping) MappingResponse {
		return fromEntity(e, names[e.IdentityProviderID])
	})
	return &apicommon.Out[MappingListResponse]{Body: MappingListResponse{Mappings: out, Total: len(out)}}, nil
}

// idpNames batch-resolves identity-provider display names keyed by IDP id.
// Returns an empty map (no enrichment) when no IDP repo is wired.
func (s *State) idpNames(ctx context.Context, rows []emaildomainmapping.EmailDomainMapping) map[string]*string {
	out := make(map[string]*string)
	if s.IDPRepo == nil {
		return out
	}
	for i := range rows {
		id := rows[i].IdentityProviderID
		if _, seen := out[id]; seen {
			continue
		}
		idp, err := s.IDPRepo.FindByID(ctx, id)
		if err != nil || idp == nil {
			out[id] = nil
			continue
		}
		name := idp.Name
		out[id] = &name
	}
	return out
}

// idpName resolves a single identity-provider display name (nil when it
// cannot be looked up or no IDP repo is wired).
func (s *State) idpName(ctx context.Context, idpID string) *string {
	if s.IDPRepo == nil {
		return nil
	}
	idp, err := s.IDPRepo.FindByID(ctx, idpID)
	if err != nil || idp == nil {
		return nil
	}
	name := idp.Name
	return &name
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[MappingResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	e, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if e == nil {
		return nil, httperror.NotFound("EmailDomainMapping", in.ID)
	}
	return &apicommon.Out[MappingResponse]{Body: fromEntity(e, s.idpName(ctx, e.IdentityProviderID))}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateMappingRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateMapping(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: committed.Event().MappingID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateMappingRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateMapping(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteMapping(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type lookupInput struct {
	Domain string `query:"domain" doc:"Email domain to look up (e.g. example.com)"`
}

// lookupOutput uses huma's anonymous body so it can hold either a
// MappingResponse or a {found:false} envelope. The handler writes one of
// the two via the Body field.
type lookupOutput struct {
	Body any
}

func (s *State) lookup(ctx context.Context, in *lookupInput) (*lookupOutput, error) {
	if in.Domain == "" {
		return nil, httperror.BadRequest("DOMAIN_REQUIRED", "domain query param is required")
	}
	m, err := s.Repo.FindByEmailDomain(ctx, in.Domain)
	if err != nil {
		return nil, usecase.Internal("REPO", "lookup failed", err)
	}
	if m == nil {
		return &lookupOutput{Body: LookupNotFoundResponse{Found: false}}, nil
	}
	return &lookupOutput{Body: fromEntity(m, s.idpName(ctx, m.IdentityProviderID))}, nil
}

type byDomainInput struct {
	Domain string `path:"domain" doc:"Email domain to look up (e.g. example.com)"`
}

// byDomain resolves an email domain to its mapping via a path param. Mirrors
// the SPA's GET /api/email-domain-mappings/by-domain/{domain}. Returns 404
// when no mapping exists for the domain.
func (s *State) byDomain(ctx context.Context, in *byDomainInput) (*apicommon.Out[MappingResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	if in.Domain == "" {
		return nil, httperror.BadRequest("DOMAIN_REQUIRED", "domain path param is required")
	}
	m, err := s.Repo.FindByEmailDomain(ctx, in.Domain)
	if err != nil {
		return nil, usecase.Internal("REPO", "lookup failed", err)
	}
	if m == nil {
		return nil, httperror.NotFound("EmailDomainMapping", in.Domain)
	}
	return &apicommon.Out[MappingResponse]{Body: fromEntity(m, s.idpName(ctx, m.IdentityProviderID))}, nil
}
