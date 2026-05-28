// Package api wires the HTTP routes for the identity_provider subdomain
// via danielgtaylor/huma/v2. Anchor-only.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps for the identity_provider handlers.
type State struct {
	Repo *identityprovider.Repository
	UoW  *usecasepgx.UnitOfWork
}

const tag = "identity-providers"

// Register mounts the IDP endpoints on the supplied huma API.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "listIdentityProviders",
		Method:        http.MethodGet,
		Path:          "/api/identity-providers",
		Summary:       "List identity providers",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createIdentityProvider",
		Method:        http.MethodPost,
		Path:          "/api/identity-providers",
		Summary:       "Create an identity provider",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "getIdentityProvider",
		Method:        http.MethodGet,
		Path:          "/api/identity-providers/{id}",
		Summary:       "Get an identity provider by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateIdentityProvider",
		Method:        http.MethodPut,
		Path:          "/api/identity-providers/{id}",
		Summary:       "Update an identity provider",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteIdentityProvider",
		Method:        http.MethodDelete,
		Path:          "/api/identity-providers/{id}",
		Summary:       "Delete an identity provider",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)
}

type emptyInput struct{}

type listOutput struct {
	Body IdentityProviderListResponse
}

func (s *State) list(ctx context.Context, _ *emptyInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadIdentityProviders(ac); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	out := make([]IdentityProviderResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: IdentityProviderListResponse{IdentityProviders: out, Total: len(out)}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body IdentityProviderResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadIdentityProviders(ac); err != nil {
		return nil, err
	}
	ip, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if ip == nil {
		return nil, httperror.NotFound("IdentityProvider", in.ID)
	}
	return &getOutput{Body: fromEntity(ip)}, nil
}

type createInput struct {
	Body CreateIdentityProviderRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteIdentityProviders(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateIdentityProvider(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().IdentityProviderID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateIdentityProviderRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteIdentityProviders(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateIdentityProvider(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type deleteInput struct {
	ID string `path:"id"`
}

func (s *State) delete(ctx context.Context, in *deleteInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteIdentityProviders(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteIdentityProvider(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}
