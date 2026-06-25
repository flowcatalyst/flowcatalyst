// Package api wires HTTP routes for dispatch_pool via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo *dispatchpool.Repository
	UoW  *usecasepgx.UnitOfWork
}

const tag = "dispatch-pools"

// Register mounts the dispatch-pool endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listDispatchPools", "/api/dispatch-pools", "List dispatch pools", s.list)
	apiroute.Post(g, "createDispatchPool", "/api/dispatch-pools", "Create a dispatch pool", http.StatusCreated, s.create)
	apiroute.Get(g, "getDispatchPool", "/api/dispatch-pools/{id}", "Get a dispatch pool by id", s.getByID)
	apiroute.Put(g, "updateDispatchPool", "/api/dispatch-pools/{id}", "Update a dispatch pool", http.StatusNoContent, s.update)
	apiroute.Post(g, "archiveDispatchPool", "/api/dispatch-pools/{id}/archive", "Archive a dispatch pool", http.StatusNoContent, s.archive)
	apiroute.Post(g, "suspendDispatchPool", "/api/dispatch-pools/{id}/suspend", "Suspend dispatch into a pool", http.StatusNoContent, s.suspend)
	apiroute.Post(g, "activateDispatchPool", "/api/dispatch-pools/{id}/activate", "Resume a suspended dispatch pool", http.StatusNoContent, s.activate)
	apiroute.Delete(g, "deleteDispatchPool", "/api/dispatch-pools/{id}", "Delete a dispatch pool", http.StatusNoContent, s.delete)
}

type listInput struct {
	Status   string `query:"status" doc:"Filter by status (ACTIVE, SUSPENDED, ARCHIVED)"`
	ClientID string `query:"clientId" doc:"Filter by client id"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[DispatchPoolListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadDispatchPools(ac); err != nil {
		return nil, err
	}
	status := apicommon.OptStr(in.Status)
	clientID := apicommon.OptStr(in.ClientID)
	rows, err := s.Repo.FindWithFilters(ctx, status, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	visible := auth.FilterClientScoped(ac, rows, func(p *dispatchpool.DispatchPool) *string { return p.ClientID })
	out := apicommon.MapSlice(visible, fromEntity)
	return &apicommon.Out[DispatchPoolListResponse]{Body: DispatchPoolListResponse{Pools: out, Total: len(out)}}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[DispatchPoolResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadDispatchPools(ac); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("DispatchPool", in.ID)
	}
	if p.ClientID != nil && !ac.CanAccessClient(*p.ClientID) {
		return nil, httperror.Forbidden("No access to this dispatch pool")
	}
	return &apicommon.Out[DispatchPoolResponse]{Body: fromEntity(p)}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateDispatchPoolRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	// Coarse permission at the controller; the use case enforces per-client
	// resource access (you may only bind a pool to a client you can access;
	// platform-wide requires anchor).
	if err := auth.CanWriteDispatchPools(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateDispatchPool(s.Repo), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.PoolID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateDispatchPoolRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteDispatchPools(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateDispatchPool(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) archive(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteDispatchPools(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ArchiveDispatchPool(s.Repo), operations.ArchiveCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) suspend(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteDispatchPools(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.SuspendDispatchPool(s.Repo), operations.SuspendCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) activate(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanWriteDispatchPools(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ActivateDispatchPool(s.Repo), operations.ActivateCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	if err := auth.CanDeleteDispatchPools(auth.FromContext(ctx)); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteDispatchPool(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}
