// Package api wires the HTTP routes for the connection subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
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
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listConnections", "/api/connections", "List connections", s.list)
	apiroute.Post(g, "createConnection", "/api/connections", "Create a connection", http.StatusCreated, s.create)
	apiroute.Get(g, "getConnection", "/api/connections/{id}", "Get a connection by id", s.getByID)
	apiroute.Put(g, "updateConnection", "/api/connections/{id}", "Update a connection", http.StatusNoContent, s.update)
	apiroute.Delete(g, "deleteConnection", "/api/connections/{id}", "Delete a connection", http.StatusNoContent, s.delete)
	apiroute.Post(g, "pauseConnection", "/api/connections/{id}/pause", "Pause a connection", http.StatusOK, s.pause)
	apiroute.Post(g, "activateConnection", "/api/connections/{id}/activate", "Activate a connection", http.StatusOK, s.activate)
}

type listInput struct {
	Status   string `query:"status"`
	ClientID string `query:"clientId"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[ConnectionListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadConnections(ac); err != nil {
		return nil, err
	}
	status := apicommon.OptStr(in.Status)
	clientID := apicommon.OptStr(in.ClientID)
	rows, err := s.Repo.FindWithFilters(ctx, status, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	visible := auth.FilterClientScoped(ac, rows, func(c *connection.Connection) *string { return c.ClientID })
	out := apicommon.MapSlice(visible, fromEntity)
	return &apicommon.Out[ConnectionListResponse]{Body: ConnectionListResponse{Connections: out, Total: len(out)}}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ConnectionResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadConnections(ac); err != nil {
		return nil, err
	}
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
	return &apicommon.Out[ConnectionResponse]{Body: fromEntity(c)}, nil
}

// create returns the full connection (201), not just `{id}`: the SPA's
// SubscriptionCreatePage pushes the returned connection straight into a
// Select, where a bare id renders with a blank label until reload.
func (s *State) create(ctx context.Context, in *apicommon.In[CreateConnectionRequest]) (*apicommon.Out[ConnectionResponse], error) {
	// Coarse permission at the controller; the use case enforces per-client
	// resource access (you may only bind a connection to a client you can
	// access; platform-wide requires anchor).
	if err := auth.CanCreateConnections(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateConnection(s.Repo), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	id := event.ConnectionID
	c, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "post-create reload failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Connection", id)
	}
	return &apicommon.Out[ConnectionResponse]{Body: fromEntity(c)}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateConnectionRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	if err := auth.CanUpdateConnections(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateConnection(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanDeleteConnections(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteConnection(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) pause(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ConnectionResponse], error) {
	if err := auth.CanUpdateConnections(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.PauseConnection(s.Repo), operations.PauseCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return s.reload(ctx, in.ID)
}

func (s *State) activate(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ConnectionResponse], error) {
	if err := auth.CanUpdateConnections(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ActivateConnection(s.Repo), operations.ActivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return s.reload(ctx, in.ID)
}

// reload re-fetches the connection so the status-flip handlers can return the
// updated ConnectionResponse (matching the Rust reference shape).
func (s *State) reload(ctx context.Context, id string) (*apicommon.Out[ConnectionResponse], error) {
	c, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Connection", id)
	}
	return &apicommon.Out[ConnectionResponse]{Body: fromEntity(c)}, nil
}
