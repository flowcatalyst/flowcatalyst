// Package api wires HTTP routes for scheduled_job via huma.
package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps.
type State struct {
	Repo *scheduledjob.Repository
	UoW  *usecasepgx.UnitOfWork
}

const tag = "scheduled-jobs"

// Register mounts the scheduled-job endpoints.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "listScheduledJobs",
		Method:        http.MethodGet,
		Path:          "/api/scheduled-jobs",
		Summary:       "List scheduled jobs",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "createScheduledJob",
		Method:        http.MethodPost,
		Path:          "/api/scheduled-jobs",
		Summary:       "Create a scheduled job",
		Tags:          []string{tag},
		DefaultStatus: http.StatusCreated,
	}, s.create)

	huma.Register(api, huma.Operation{
		OperationID:   "getScheduledJob",
		Method:        http.MethodGet,
		Path:          "/api/scheduled-jobs/{id}",
		Summary:       "Get a scheduled job by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "updateScheduledJob",
		Method:        http.MethodPut,
		Path:          "/api/scheduled-jobs/{id}",
		Summary:       "Update a scheduled job",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.update)

	huma.Register(api, huma.Operation{
		OperationID:   "pauseScheduledJob",
		Method:        http.MethodPost,
		Path:          "/api/scheduled-jobs/{id}/pause",
		Summary:       "Pause a scheduled job",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.pause)

	huma.Register(api, huma.Operation{
		OperationID:   "resumeScheduledJob",
		Method:        http.MethodPost,
		Path:          "/api/scheduled-jobs/{id}/resume",
		Summary:       "Resume a scheduled job",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.resume)

	huma.Register(api, huma.Operation{
		OperationID:   "archiveScheduledJob",
		Method:        http.MethodPost,
		Path:          "/api/scheduled-jobs/{id}/archive",
		Summary:       "Archive a scheduled job",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.archive)

	huma.Register(api, huma.Operation{
		OperationID:   "fireScheduledJobNow",
		Method:        http.MethodPost,
		Path:          "/api/scheduled-jobs/{id}/fire-now",
		Summary:       "Fire a scheduled job immediately",
		Tags:          []string{tag},
		DefaultStatus: http.StatusAccepted,
	}, s.fireNow)

	huma.Register(api, huma.Operation{
		OperationID:   "deleteScheduledJob",
		Method:        http.MethodDelete,
		Path:          "/api/scheduled-jobs/{id}",
		Summary:       "Delete a scheduled job",
		Tags:          []string{tag},
		DefaultStatus: http.StatusNoContent,
	}, s.delete)
}

type listInput struct {
	Status   string `query:"status"`
	ClientID string `query:"clientId"`
}

type listOutput struct {
	Body ScheduledJobListResponse
}

func (s *State) list(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
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
	out := make([]ScheduledJobResponse, 0, len(rows))
	for i := range rows {
		j := &rows[i]
		if j.ClientID == nil || ac.CanAccessClient(*j.ClientID) {
			out = append(out, fromEntity(j))
		}
	}
	return &listOutput{Body: ScheduledJobListResponse{Items: out}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body ScheduledJobResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		return nil, err
	}
	j, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("ScheduledJob", in.ID)
	}
	if j.ClientID != nil && !ac.CanAccessClient(*j.ClientID) {
		return nil, httperror.Forbidden("No access to this scheduled job")
	}
	return &getOutput{Body: fromEntity(j)}, nil
}

type createInput struct {
	Body CreateScheduledJobRequest
}

type createOutput struct {
	Body apicommon.CreatedResponse
}

func (s *State) create(ctx context.Context, in *createInput) (*createOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	if in.Body.ClientID != nil && !ac.CanAccessClient(*in.Body.ClientID) {
		return nil, httperror.Forbidden("No access to client: " + *in.Body.ClientID)
	}
	if in.Body.ClientID == nil && !ac.IsAnchor() {
		return nil, httperror.Forbidden("Only anchor users can create platform-scoped jobs")
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.CreateScheduledJob(ctx, s.Repo, s.UoW, in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &createOutput{Body: apicommon.CreatedResponse{ID: committed.Event().ScheduledJobID}}, nil
}

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateScheduledJobRequest
}

type emptyOutput struct{}

func (s *State) update(ctx context.Context, in *updateInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.UpdateScheduledJob(ctx, s.Repo, s.UoW, in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type idInput struct {
	ID string `path:"id"`
}

func (s *State) pause(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.PauseScheduledJob(ctx, s.Repo, s.UoW, operations.PauseCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) resume(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ResumeScheduledJob(ctx, s.Repo, s.UoW, operations.ResumeCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

func (s *State) archive(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.ArchiveScheduledJob(ctx, s.Repo, s.UoW, operations.ArchiveCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}

type fireNowOutput struct {
	Body FireNowResponse
}

func (s *State) fireNow(ctx context.Context, in *idInput) (*fireNowOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanFireScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	committed, err := operations.FireNow(ctx, s.Repo, s.UoW, operations.FireNowCommand{ID: in.ID}, ec)
	if err != nil {
		return nil, err
	}
	return &fireNowOutput{Body: FireNowResponse{
		ScheduledJobID: committed.Event().ScheduledJobID,
		InstanceID:     committed.Event().InstanceID,
	}}, nil
}

func (s *State) delete(ctx context.Context, in *idInput) (*emptyOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := operations.DeleteScheduledJob(ctx, s.Repo, s.UoW, operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &emptyOutput{}, nil
}
