// Package api wires the HTTP routes for the process subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo *process.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Register mounts the process endpoints under both /api/processes and
// /bff/processes. The two prefixes serve the same handlers — the BFF
// alias exists so the cookie-session frontend can hit the same surface
// without the bearer-token expectations of /api consumers. Mirrors
// Rust's router.rs which nests processes_router under both paths.
func Register(api huma.API, s *State) {
	registerAt(api, s, "/api/processes", "", "processes")
	registerAt(api, s, "/bff/processes", "Bff", "bff-processes")
}

func registerAt(api huma.API, s *State, base, opSuffix, tag string) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "list"+opSuffix+"Processes", base, "List processes", s.list)
	apiroute.Post(g, "create"+opSuffix+"Process", base, "Create a process", http.StatusCreated, s.create)
	apiroute.Get(g, "get"+opSuffix+"ProcessByCode", base+"/by-code/{code}", "Get a process by code", s.getByCode)
	apiroute.Get(g, "get"+opSuffix+"Process", base+"/{id}", "Get a process by id", s.getByID)
	apiroute.Put(g, "update"+opSuffix+"Process", base+"/{id}", "Update a process", http.StatusNoContent, s.update)
	apiroute.Post(g, "archive"+opSuffix+"Process", base+"/{id}/archive", "Archive a process", http.StatusNoContent, s.archive)
	apiroute.Delete(g, "delete"+opSuffix+"Process", base+"/{id}", "Delete a process", http.StatusNoContent, s.delete)
}

type listInput struct {
	Application string `query:"application"`
	Subdomain   string `query:"subdomain"`
	Status      string `query:"status"`
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[ProcessListResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadProcesses(ac); err != nil {
		return nil, err
	}
	application := apicommon.OptStr(in.Application)
	subdomain := apicommon.OptStr(in.Subdomain)
	status := apicommon.OptStr(in.Status)
	rows, err := s.Repo.FindWithFilters(ctx, application, subdomain, status)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := apicommon.MapSlice(rows, fromEntity)
	return &apicommon.Out[ProcessListResponse]{Body: ProcessListResponse{Items: out}}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ProcessResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadProcesses(ac); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Process", in.ID)
	}
	return &apicommon.Out[ProcessResponse]{Body: fromEntity(p)}, nil
}

type getByCodeInput struct {
	Code string `path:"code"`
}

func (s *State) getByCode(ctx context.Context, in *getByCodeInput) (*apicommon.Out[ProcessResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadProcesses(ac); err != nil {
		return nil, err
	}
	p, err := s.Repo.FindByCode(ctx, in.Code)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Process", in.Code)
	}
	return &apicommon.Out[ProcessResponse]{Body: fromEntity(p)}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateProcessRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteProcesses(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateProcess(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: committed.Event().ProcessID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateProcessRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteProcesses(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateProcess(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) archive(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteProcesses(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ArchiveProcess(ctx, s.Repo, s.UoW, operations.ArchiveCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteProcesses(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteProcess(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}
