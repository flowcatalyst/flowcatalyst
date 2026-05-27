// Package api wires the HTTP routes for the process subdomain via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
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
	huma.Register(api, huma.Operation{
		OperationID:   "list" + opSuffix + "Processes",
		Method:        http.MethodGet,
		Path:          base,
		Summary:       "List processes",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "create" + opSuffix + "Process",
		Method:        http.MethodPost,
		Path:          base,
		Summary:       "Create a process",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "get" + opSuffix + "ProcessByCode",
		Method:        http.MethodGet,
		Path:          base + "/by-code/{code}",
		Summary:       "Get a process by code",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByCode)

	huma.Register(api, huma.Operation{
		OperationID:   "get" + opSuffix + "Process",
		Method:        http.MethodGet,
		Path:          base + "/{id}",
		Summary:       "Get a process by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "update" + opSuffix + "Process",
		Method:        http.MethodPut,
		Path:          base + "/{id}",
		Summary:       "Update a process",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "archive" + opSuffix + "Process",
		Method:        http.MethodPost,
		Path:          base + "/{id}/archive",
		Summary:       "Archive a process",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.archive)

	huma.Register(api, huma.Operation{
		OperationID:   "delete" + opSuffix + "Process",
		Method:        http.MethodDelete,
		Path:          base + "/{id}",
		Summary:       "Delete a process",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)
}

type listInput struct {
	Application string `query:"application"`
	Subdomain   string `query:"subdomain"`
	Status      string `query:"status"`
}

type listOutput struct {
	Body ProcessListResponse
}

func (s *State) list(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadProcesses(ac); err != nil {
		return nil, err
	}
	var application, subdomain, status *string
	if in.Application != "" {
		application = &in.Application
	}
	if in.Subdomain != "" {
		subdomain = &in.Subdomain
	}
	if in.Status != "" {
		status = &in.Status
	}
	rows, err := s.Repo.FindWithFilters(ctx, application, subdomain, status)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := make([]ProcessResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: ProcessListResponse{Items: out}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body ProcessResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(p)}, nil
}

type getByCodeInput struct {
	Code string `path:"code"`
}

func (s *State) getByCode(ctx context.Context, in *getByCodeInput) (*getOutput, error) {
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
	return &getOutput{Body: fromEntity(p)}, nil
}

type createInput struct {
	Body CreateProcessRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteProcesses(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateProcess(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().ProcessID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateProcessRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteProcesses(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateProcess(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type archiveInput struct {
	ID string `path:"id"`
}

func (s *State) archive(ctx context.Context, in *archiveInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteProcesses(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ArchiveProcess(ctx, s.Repo, s.UoW, operations.ArchiveCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type deleteInput struct {
	ID string `path:"id"`
}

func (s *State) delete(ctx context.Context, in *deleteInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteProcesses(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteProcess(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}
