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
	huma.Register(api, huma.Operation{
		OperationID:   "listEmailDomainMappings",
		Method:        http.MethodGet,
		Path:          "/api/email-domain-mappings",
		Summary:       "List email-domain mappings",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createEmailDomainMapping",
		Method:        http.MethodPost,
		Path:          "/api/email-domain-mappings",
		Summary:       "Create an email-domain mapping",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "lookupEmailDomainMapping",
		Method:        http.MethodGet,
		Path:          "/api/email-domain-mappings/lookup",
		Summary:       "Resolve an email domain to its mapping",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.lookup)

	huma.Register(api, huma.Operation{
		OperationID:   "getEmailDomainMappingByDomain",
		Method:        http.MethodGet,
		Path:          "/api/email-domain-mappings/by-domain/{domain}",
		Summary:       "Resolve an email domain to its mapping (path param)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.byDomain)

	huma.Register(api, huma.Operation{
		OperationID:   "getEmailDomainMapping",
		Method:        http.MethodGet,
		Path:          "/api/email-domain-mappings/{id}",
		Summary:       "Get an email-domain mapping by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateEmailDomainMapping",
		Method:        http.MethodPut,
		Path:          "/api/email-domain-mappings/{id}",
		Summary:       "Update an email-domain mapping",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteEmailDomainMapping",
		Method:        http.MethodDelete,
		Path:          "/api/email-domain-mappings/{id}",
		Summary:       "Delete an email-domain mapping",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)
}

type emptyInput struct{}

type listOutput struct {
	Body MappingListResponse
}

func (s *State) list(ctx context.Context, _ *emptyInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	names := s.idpNames(ctx, rows)
	out := make([]MappingResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i], names[rows[i].IdentityProviderID]))
	}
	return &listOutput{Body: MappingListResponse{Mappings: out, Total: len(out)}}, nil
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

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body MappingResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(e, s.idpName(ctx, e.IdentityProviderID))}, nil
}

type createInput struct {
	Body CreateMappingRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateMapping(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().MappingID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateMappingRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateMapping(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type deleteInput struct {
	ID string `path:"id"`
}

func (s *State) delete(ctx context.Context, in *deleteInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteMapping(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
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
func (s *State) byDomain(ctx context.Context, in *byDomainInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(m, s.idpName(ctx, m.IdentityProviderID))}, nil
}
