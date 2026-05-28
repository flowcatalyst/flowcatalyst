// Package api wires HTTP routes for the cors subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

type State struct {
	Repo *cors.Repository
	UoW  *usecasepgx.UnitOfWork
}

const tag = "cors-origins"

func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "publicAllowedOrigins",
		Method:        http.MethodGet,
		Path:          "/api/platform/cors/allowed",
		Summary:       "List allowed CORS origins (public)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.publicAllowed)

	huma.Register(api, huma.Operation{
		OperationID:   "listCorsOrigins",
		Method:        http.MethodGet,
		Path:          "/api/platform/cors",
		Summary:       "List CORS origins (anchor)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "addCorsOrigin",
		Method:        http.MethodPost,
		Path:          "/api/platform/cors",
		Summary:       "Add a CORS origin",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.add)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteCorsOrigin",
		Method:        http.MethodDelete,
		Path:          "/api/platform/cors/{id}",
		Summary:       "Remove a CORS origin",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)
}

type emptyInput struct{}

type publicOutput struct {
	Body PublicAllowedResponse
}

func (s *State) publicAllowed(ctx context.Context, _ *emptyInput) (*publicOutput, error) {
	rows, err := s.Repo.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_all failed", err)
	}
	origins := make([]string, 0, len(rows))
	for _, o := range rows {
		origins = append(origins, o.Origin)
	}
	return &publicOutput{Body: PublicAllowedResponse{AllowedOrigins: origins}}, nil
}

type listOutput struct {
	Body CorsOriginListResponse
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
	out := make([]AllowedOriginResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: CorsOriginListResponse{CorsOrigins: out, Total: len(out)}}, nil
}

type addInput struct {
	Body AddOriginRequest
}

type addOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) add(ctx context.Context, in *addInput) (*addOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.AddOrigin(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &addOutput{Body: apicommon.CreatedResponse{ID: committed.Event().OriginID}}, nil
}

type deleteInput struct {
	ID string `path:"id"`
}

type emptyOutput struct{}

func (s *State) delete(ctx context.Context, in *deleteInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.RequireAnchor(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteOrigin(ctx, s.Repo, s.UoW, operations.DeleteCommand{OriginID: in.ID}, ec); err != nil {
		if httperror.IsNotFound(err) {
			return nil, err
		}
		return nil, err
	}
	return &emptyOutput{}, nil
}
