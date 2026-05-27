// Package api wires HTTP routes for email_domain_mapping via
// danielgtaylor/huma/v2. Anchor-only.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles the dependencies.
type State struct {
	Repo *emaildomainmapping.Repository
	UoW  *usecasepgx.UnitOfWork
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
	out := make([]MappingResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: MappingListResponse{Items: out}}, nil
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
	return &getOutput{Body: fromEntity(e)}, nil
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
	return &lookupOutput{Body: fromEntity(m)}, nil
}
