// Package api wires HTTP routes for scheduled_job via huma.
package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// State bundles deps. Instances is optional — leave nil if the instance
// surface isn't wired (the routes will then 501).
type State struct {
	Repo      *scheduledjob.Repository
	Instances *scheduledjob.InstanceRepository
	UoW       *usecasepgx.UnitOfWork
}

const tag = "scheduled-jobs"

// Register mounts the scheduled-job endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listScheduledJobs", "/api/scheduled-jobs", "List scheduled jobs", s.list)
	apiroute.Post(g, "createScheduledJob", "/api/scheduled-jobs", "Create a scheduled job", http.StatusCreated, s.create)
	apiroute.Get(g, "getScheduledJob", "/api/scheduled-jobs/{id}", "Get a scheduled job by id", s.getByID)
	apiroute.Put(g, "updateScheduledJob", "/api/scheduled-jobs/{id}", "Update a scheduled job", http.StatusNoContent, s.update)
	apiroute.Post(g, "pauseScheduledJob", "/api/scheduled-jobs/{id}/pause", "Pause a scheduled job", http.StatusNoContent, s.pause)
	apiroute.Post(g, "resumeScheduledJob", "/api/scheduled-jobs/{id}/resume", "Resume a scheduled job", http.StatusNoContent, s.resume)
	apiroute.Post(g, "archiveScheduledJob", "/api/scheduled-jobs/{id}/archive", "Archive a scheduled job", http.StatusNoContent, s.archive)
	apiroute.Post(g, "fireScheduledJobNow", "/api/scheduled-jobs/{id}/fire", "Fire a scheduled job immediately", http.StatusAccepted, s.fireNow)
	apiroute.Delete(g, "deleteScheduledJob", "/api/scheduled-jobs/{id}", "Delete a scheduled job", http.StatusNoContent, s.delete)
	apiroute.Get(g, "getScheduledJobByCode", "/api/scheduled-jobs/by-code/{code}", "Get a scheduled job by code", s.getByCode)
	apiroute.Get(g, "listScheduledJobInstances", "/api/scheduled-jobs/{id}/instances", "List firings for a scheduled job", s.listInstances)
	apiroute.Get(g, "getScheduledJobInstance", "/api/scheduled-jobs/instances/{instanceId}", "Get a single scheduled-job instance", s.getInstance)
	apiroute.Get(g, "listScheduledJobInstanceLogs", "/api/scheduled-jobs/instances/{instanceId}/logs", "List log entries for an instance", s.listInstanceLogs)
	apiroute.Post(g, "writeScheduledJobInstanceLog", "/api/scheduled-jobs/instances/{instanceId}/log", "Append a log entry to an instance", http.StatusNoContent, s.writeInstanceLog)
	apiroute.Post(g, "completeScheduledJobInstance", "/api/scheduled-jobs/instances/{instanceId}/complete", "Mark a scheduled-job instance as completed", http.StatusNoContent, s.completeInstance)
}

type listInput struct {
	Status   string `query:"status"`
	ClientID string `query:"clientId"`
	Search   string `query:"search"`
	apicommon.PageQuery
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[apicommon.OffsetPage[ScheduledJobResponse]], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		return nil, err
	}
	filters := scheduledjob.ListFilters{}
	filters.Status = apicommon.OptStr(in.Status)
	if in.ClientID != "" {
		// The literal "platform" selects platform-scoped jobs (client_id IS
		// NULL), which the repo expresses as a pointer-to-"". Mirrors the
		// Rust list handler's Some("platform") => Some(None) mapping.
		if in.ClientID == "platform" {
			empty := ""
			filters.ClientID = &empty
		} else {
			filters.ClientID = &in.ClientID
		}
	}
	filters.Search = apicommon.OptStr(in.Search)
	// Scope to accessible clients in SQL (anchor sees all → no scoping) so
	// COUNT and LIMIT/OFFSET stay consistent across pages.
	if !ac.IsAnchor() {
		clients := ac.Clients
		filters.AccessibleClientIDs = &clients
	}
	total, err := s.Repo.CountWithFilters(ctx, filters)
	if err != nil {
		return nil, usecase.Internal("REPO", "count_with_filters failed", err)
	}
	limit, offset := in.LimitVal(), in.OffsetVal()
	filters.Limit, filters.Offset = &limit, &offset
	rows, err := s.Repo.FindWithFilters(ctx, filters)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := make([]ScheduledJobResponse, 0, len(rows))
	for i := range rows {
		resp := fromEntity(&rows[i])
		if active, err := s.Instances.HasActiveInstance(ctx, rows[i].ID, rows[i].TracksCompletion); err == nil {
			resp.HasActiveInstance = active
		}
		out = append(out, resp)
	}
	page := apicommon.NewOffsetPage(out, in.PageIndex(), in.PageSizeVal(), total)
	return &apicommon.Out[apicommon.OffsetPage[ScheduledJobResponse]]{Body: page}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[ScheduledJobResponse], error) {
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
	resp := fromEntity(j)
	if active, err := s.Instances.HasActiveInstance(ctx, j.ID, j.TracksCompletion); err == nil {
		resp.HasActiveInstance = active
	}
	return &apicommon.Out[ScheduledJobResponse]{Body: resp}, nil
}

func (s *State) create(ctx context.Context, in *apicommon.In[CreateScheduledJobRequest]) (*apicommon.Out[apicommon.CreatedResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	// The client-scope check (client-scoped job → access that client; platform-
	// scoped → anchor) lives in CreateScheduledJob's Authorize phase, against
	// cmd.ClientID.
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecaseop.Run(ctx, s.UoW, operations.CreateScheduledJob(s.Repo), in.Body.toCommand(), ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[apicommon.CreatedResponse]{Body: apicommon.CreatedResponse{ID: event.ScheduledJobID}}, nil
}

// Per-resource scope (A2) — a non-anchor principal must not mutate another
// tenant's scheduled job by id — is enforced post-load inside each by-id use
// case's Execute phase (auth.CheckScopeAccess on the loaded job's client),
// mirroring Rust check_scope_access(auth, job.client_id). The handlers keep
// only the coarse permission check.

type updateInput struct {
	ID   string `path:"id"`
	Body UpdateScheduledJobRequest
}

func (s *State) update(ctx context.Context, in *updateInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.UpdateScheduledJob(s.Repo), in.Body.toCommand(in.ID), ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) pause(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.PauseScheduledJob(s.Repo), operations.PauseCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) resume(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ResumeScheduledJob(s.Repo), operations.ResumeCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

func (s *State) archive(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.ArchiveScheduledJob(s.Repo), operations.ArchiveCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

type fireNowInput struct {
	ID   string `path:"id"`
	Body *FireNowRequest
}

func (s *State) fireNow(ctx context.Context, in *fireNowInput) (*apicommon.Out[FireNowResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanFireScheduledJobs(ac); err != nil {
		return nil, err
	}
	var correlationID *string
	if in.Body != nil {
		correlationID = in.Body.CorrelationID
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecaseop.Run(ctx, s.UoW, operations.FireNow(s.Repo, s.Instances),
		operations.FireNowCommand{ID: in.ID, CorrelationID: correlationID}, ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[FireNowResponse]{Body: FireNowResponse{
		ID:             event.InstanceID,
		ScheduledJobID: event.ScheduledJobID,
		InstanceID:     event.InstanceID,
	}}, nil
}

func (s *State) delete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteScheduledJobs(ac); err != nil {
		return nil, err
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.DeleteScheduledJob(s.Repo), operations.DeleteCommand{ID: in.ID}, ec); err != nil {
		return nil, err
	}
	return &apicommon.Empty{}, nil
}

// ── by-code lookup ──────────────────────────────────────────────────────

type byCodeInput struct {
	Code     string `path:"code"`
	ClientID string `query:"clientId" doc:"Optional client scope; omit for platform-scoped lookup"`
}

func (s *State) getByCode(ctx context.Context, in *byCodeInput) (*apicommon.Out[ScheduledJobResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		return nil, err
	}
	clientID := apicommon.OptStr(in.ClientID)
	j, err := s.Repo.FindByCode(ctx, in.Code, clientID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_code failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("ScheduledJob", in.Code)
	}
	if j.ClientID != nil && !ac.CanAccessClient(*j.ClientID) {
		return nil, httperror.Forbidden("No access to this scheduled job")
	}
	resp := fromEntity(j)
	if active, err := s.Instances.HasActiveInstance(ctx, j.ID, j.TracksCompletion); err == nil {
		resp.HasActiveInstance = active
	}
	return &apicommon.Out[ScheduledJobResponse]{Body: resp}, nil
}

// ── instance endpoints ──────────────────────────────────────────────────

type listInstancesInput struct {
	ID     string `path:"id"`
	Status string `query:"status"`
	apicommon.PageQuery
}

func (s *State) listInstances(ctx context.Context, in *listInstancesInput) (*apicommon.Out[apicommon.OffsetPage[ScheduledJobInstanceResponse]], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		return nil, err
	}
	if s.Instances == nil {
		return nil, usecase.Internal("WIRING", "instances repo not configured", nil)
	}
	filters := scheduledjob.InstanceListFilters{ScheduledJobID: &in.ID}
	if in.Status != "" {
		st := scheduledjob.ParseInstanceStatus(in.Status)
		filters.Status = &st
	}
	total, err := s.Instances.Count(ctx, filters)
	if err != nil {
		return nil, usecase.Internal("REPO", "count_instances failed", err)
	}
	limit, offset := in.LimitVal(), in.OffsetVal()
	filters.Limit, filters.Offset = &limit, &offset
	rows, err := s.Instances.List(ctx, filters)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_instances failed", err)
	}
	out := apicommon.MapSlice(rows, instanceToResponse)
	page := apicommon.NewOffsetPage(out, in.PageIndex(), in.PageSizeVal(), total)
	return &apicommon.Out[apicommon.OffsetPage[ScheduledJobInstanceResponse]]{Body: page}, nil
}

type instanceInput struct {
	InstanceID string `path:"instanceId"`
}

func (s *State) getInstance(ctx context.Context, in *instanceInput) (*apicommon.Out[ScheduledJobInstanceResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		return nil, err
	}
	if s.Instances == nil {
		return nil, usecase.Internal("WIRING", "instances repo not configured", nil)
	}
	inst, err := s.Instances.FindByID(ctx, in.InstanceID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_instance failed", err)
	}
	if inst == nil {
		return nil, httperror.NotFound("ScheduledJobInstance", in.InstanceID)
	}
	if inst.ClientID != nil && !ac.CanAccessClient(*inst.ClientID) {
		return nil, httperror.Forbidden("No access to this instance")
	}
	return &apicommon.Out[ScheduledJobInstanceResponse]{Body: instanceToResponse(inst)}, nil
}

// listInstanceLogs's Out[[]…] Body is a bare JSON array — the Rust shape
// for GET /api/scheduled-jobs/instances/{instanceId}/logs.
func (s *State) listInstanceLogs(ctx context.Context, in *instanceInput) (*apicommon.Out[[]ScheduledJobInstanceLogResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		return nil, err
	}
	if s.Instances == nil {
		return nil, usecase.Internal("WIRING", "instances repo not configured", nil)
	}
	rows, err := s.Instances.ListLogs(ctx, in.InstanceID, 500)
	if err != nil {
		return nil, usecase.Internal("REPO", "list_logs failed", err)
	}
	out := apicommon.MapSlice(rows, instanceLogToResponse)
	return &apicommon.Out[[]ScheduledJobInstanceLogResponse]{Body: out}, nil
}

type writeLogInput struct {
	InstanceID string `path:"instanceId"`
	Body       WriteInstanceLogRequest
}

func (s *State) writeInstanceLog(ctx context.Context, in *writeLogInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	if s.Instances == nil {
		return nil, usecase.Internal("WIRING", "instances repo not configured", nil)
	}
	inst, err := s.Instances.FindByID(ctx, in.InstanceID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_instance failed", err)
	}
	if inst == nil {
		return nil, httperror.NotFound("ScheduledJobInstance", in.InstanceID)
	}
	if err := auth.CheckScopeAccess(ac, inst.ClientID); err != nil { // A2: per-instance client scope
		return nil, err
	}
	log := &scheduledjob.ScheduledJobInstanceLog{
		ID:             tsid.Generate(tsid.ScheduledJobInstanceLog),
		InstanceID:     in.InstanceID,
		ScheduledJobID: &inst.ScheduledJobID,
		ClientID:       inst.ClientID,
		Level:          in.Body.Level,
		Message:        in.Body.Message,
		Metadata:       in.Body.Metadata,
	}
	if err := s.Instances.WriteLog(ctx, log); err != nil {
		return nil, usecase.Internal("REPO", "write_log failed", err)
	}
	return &apicommon.Empty{}, nil
}

type completeInstanceInput struct {
	InstanceID string `path:"instanceId"`
	Body       CompleteInstanceRequest
}

func (s *State) completeInstance(ctx context.Context, in *completeInstanceInput) (*apicommon.Empty, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		return nil, err
	}
	if s.Instances == nil {
		return nil, usecase.Internal("WIRING", "instances repo not configured", nil)
	}
	inst, err := s.Instances.FindByID(ctx, in.InstanceID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_instance failed", err)
	}
	if inst == nil {
		return nil, httperror.NotFound("ScheduledJobInstance", in.InstanceID)
	}
	if err := auth.CheckScopeAccess(ac, inst.ClientID); err != nil { // A2: per-instance client scope
		return nil, err
	}
	status, completion := resolveInstanceCompletion(in.Body.Status, in.Body.CompletionStatus)
	var compStatus *string
	if completion != "" {
		compStatus = &completion
	}
	// The SDK sends the payload as `result`; the SPA sends `completionResult`.
	// Prefer the explicit completionResult, fall back to result.
	result := in.Body.CompletionResult
	if len(result) == 0 {
		result = in.Body.Result
	}
	if err := s.Instances.MarkComplete(ctx, in.InstanceID, status, compStatus, result); err != nil {
		return nil, usecase.Internal("REPO", "mark_complete failed", err)
	}
	return &apicommon.Empty{}, nil
}

// resolveInstanceCompletion disambiguates the two complete-instance request
// dialects (see CompleteInstanceRequest) into an instance lifecycle status and
// a completion outcome ("" when none):
//
//   - SDK (Laravel/Rust): {status:"SUCCESS"|"FAILURE"} — `status` carries the
//     completion OUTCOME and the instance becomes COMPLETED.
//   - SPA/internal: {status:"<instance-status>", completionStatus} — `status`
//     is the instance lifecycle status.
//
// SUCCESS/FAILURE never collide with the instance statuses
// (QUEUED/IN_FLIGHT/DELIVERED/COMPLETED/FAILED/DELIVERY_FAILED), so the value
// alone disambiguates. An explicit completionStatus always wins.
func resolveInstanceCompletion(status, completionStatus string) (scheduledjob.InstanceStatus, string) {
	completion := completionStatus
	switch strings.ToUpper(status) {
	case "":
		return scheduledjob.InstanceStatusCompleted, completion
	case "SUCCESS", "FAILURE":
		if completion == "" {
			completion = strings.ToUpper(status)
		}
		return scheduledjob.InstanceStatusCompleted, completion
	default:
		return scheduledjob.ParseInstanceStatus(status), completion
	}
}
