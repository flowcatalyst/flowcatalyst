// Package api wires the HTTP routes for the connection subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles the dependencies.
type State struct {
	Repo *connection.Repository
	UoW  *usecasepgx.UnitOfWork
}

const tag = "connections"

// Register mounts the connection endpoints.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "listConnections",
		Method:        http.MethodGet,
		Path:          "/api/connections",
		Summary:       "List connections",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createConnection",
		Method:        http.MethodPost,
		Path:          "/api/connections",
		Summary:       "Create a connection",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "getConnection",
		Method:        http.MethodGet,
		Path:          "/api/connections/{id}",
		Summary:       "Get a connection by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateConnection",
		Method:        http.MethodPut,
		Path:          "/api/connections/{id}",
		Summary:       "Update a connection",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteConnection",
		Method:        http.MethodDelete,
		Path:          "/api/connections/{id}",
		Summary:       "Delete a connection",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)
}

type listInput struct {
	Status   string `query:"status"`
	ClientID string `query:"clientId"`
}

type listOutput struct {
	Body ConnectionListResponse
}

func (s *State) list(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadConnections(ac); err != nil {
		return nil, err
	}
	var status, clientID *string
	if in.Status != "" {
		status = &in.Status
	}
	if in.ClientID != "" {
		clientID = &in.ClientID
	}
	rows, err := s.Repo.FindWithFilters(ctx, status, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := make([]ConnectionResponse, 0, len(rows))
	for i := range rows {
		c := &rows[i]
		if c.ClientID == nil || ac.CanAccessClient(*c.ClientID) {
			out = append(out, fromEntity(c))
		}
	}
	return &listOutput{Body: ConnectionListResponse{Items: out}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body ConnectionResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	c, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Connection", in.ID)
	}
	if c.ClientID != nil && !ac.CanAccessClient(*c.ClientID) {
		return nil, httperror.Forbidden("No access to this connection")
	}
	return &getOutput{Body: fromEntity(c)}, nil
}

type createInput struct {
	Body CreateConnectionRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if in.Body.ClientID != nil && !ac.CanAccessClient(*in.Body.ClientID) {
		return nil, httperror.Forbidden("No access to client: " + *in.Body.ClientID)
	}
	if in.Body.ClientID == nil && !ac.IsAnchor() {
		return nil, httperror.Forbidden("Only anchor users can create anchor-level connections")
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateConnection(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().ConnectionID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateConnectionRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateConnection(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type deleteInput struct {
	ID string `path:"id"`
}

func (s *State) delete(ctx context.Context, in *deleteInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteConnection(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}
