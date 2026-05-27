// Package api wires the dispatch-job read-only HTTP endpoints via huma.
package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo *dispatchjob.Repository
}

const (
	tag         = "dispatch-jobs"
	viewPerm    = "platform:messaging:dispatch-job:view"
	viewRawPerm = "platform:messaging:dispatch-job:view-raw"
)

// Register mounts the dispatch-job endpoints.
func Register(api huma.API, s *State) {
	huma.Register(api, huma.Operation{
		OperationID:   "listDispatchJobs",
		Method:        http.MethodGet,
		Path:          "/api/dispatch-jobs",
		Summary:       "List dispatch jobs with filters",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.list)

	huma.Register(api, huma.Operation{
		OperationID:   "listDispatchJobsRaw",
		Method:        http.MethodGet,
		Path:          "/api/dispatch-jobs/list-raw",
		Summary:       "List dispatch jobs (raw)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.listRaw)

	huma.Register(api, huma.Operation{
		OperationID:   "dispatchJobFilterOptions",
		Method:        http.MethodGet,
		Path:          "/api/dispatch-jobs/filter-options",
		Summary:       "Distinct facet values for dispatch jobs",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.filterOptions)

	huma.Register(api, huma.Operation{
		OperationID:   "dispatchJobsByEvent",
		Method:        http.MethodGet,
		Path:          "/api/dispatch-jobs/event/{eventId}",
		Summary:       "Dispatch jobs spawned by a specific event",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.byEvent)

	huma.Register(api, huma.Operation{
		OperationID:   "getDispatchJob",
		Method:        http.MethodGet,
		Path:          "/api/dispatch-jobs/{id}",
		Summary:       "Get a dispatch job by id",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getByID)

	huma.Register(api, huma.Operation{
		OperationID:   "getDispatchJobRaw",
		Method:        http.MethodGet,
		Path:          "/api/dispatch-jobs/{id}/raw",
		Summary:       "Get a dispatch job (raw)",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.getRaw)

	huma.Register(api, huma.Operation{
		OperationID:   "listDispatchJobAttempts",
		Method:        http.MethodGet,
		Path:          "/api/dispatch-jobs/{id}/attempts",
		Summary:       "List a dispatch job's attempt history",
		Tags:          []string{tag},
		DefaultStatus: http.StatusOK,
	}, s.attempts)
}

type listInput struct {
	Status         string `query:"status"`
	ClientID       string `query:"clientId"`
	DispatchPoolID string `query:"dispatchPoolId"`
	SubscriptionID string `query:"subscriptionId"`
	Code           string `query:"code"`
	Since          string `query:"since" doc:"RFC3339 timestamp"`
	Until          string `query:"until" doc:"RFC3339 timestamp"`
	Limit          int    `query:"limit"`
	Offset         int    `query:"offset"`
}

func (in *listInput) toFilters() dispatchjob.FilterParams {
	str := func(v string) *string {
		if v == "" {
			return nil
		}
		s := v
		return &s
	}
	ts := func(v string) *time.Time {
		if v == "" {
			return nil
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return &t
		}
		return nil
	}
	return dispatchjob.FilterParams{
		Status:         str(in.Status),
		ClientID:       str(in.ClientID),
		DispatchPoolID: str(in.DispatchPoolID),
		SubscriptionID: str(in.SubscriptionID),
		Code:           str(in.Code),
		Since:          ts(in.Since),
		Until:          ts(in.Until),
		Limit:          in.Limit,
		Offset:         in.Offset,
	}
}

type listOutput struct {
	Body DispatchJobListResponse
}

func (s *State) list(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, in.toFilters())
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := make([]DispatchJobResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: DispatchJobListResponse{Items: out}}, nil
}

func (s *State) listRaw(ctx context.Context, in *listInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewRawPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, in.toFilters())
	if err != nil {
		return nil, usecase.Internal("REPO", "find_raw failed", err)
	}
	out := make([]DispatchJobResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: DispatchJobListResponse{Items: out}}, nil
}

type getInput struct {
	ID string `path:"id"`
}

type getOutput struct {
	Body DispatchJobResponse
}

func (s *State) getByID(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	j, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("DispatchJob", in.ID)
	}
	return &getOutput{Body: fromEntity(j)}, nil
}

func (s *State) getRaw(ctx context.Context, in *getInput) (*getOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewRawPerm); err != nil {
		return nil, err
	}
	j, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_raw failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("DispatchJob", in.ID)
	}
	return &getOutput{Body: fromEntity(j)}, nil
}

type attemptsOutput struct {
	Body AttemptListResponse
}

func (s *State) attempts(ctx context.Context, in *getInput) (*attemptsOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.AttemptsByJob(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "attempts failed", err)
	}
	out := make([]AttemptDTO, 0, len(rows))
	for i := range rows {
		out = append(out, attemptFromEntity(&rows[i]))
	}
	return &attemptsOutput{Body: AttemptListResponse{Items: out}}, nil
}

type byEventInput struct {
	EventID string `path:"eventId"`
}

func (s *State) byEvent(ctx context.Context, in *byEventInput) (*listOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindByEventID(ctx, in.EventID)
	if err != nil {
		return nil, usecase.Internal("REPO", "by_event failed", err)
	}
	out := make([]DispatchJobResponse, 0, len(rows))
	for i := range rows {
		out = append(out, fromEntity(&rows[i]))
	}
	return &listOutput{Body: DispatchJobListResponse{Items: out}}, nil
}

type emptyInput struct{}

type filterOptionsOutput struct {
	Body DispatchJobFilterOptionsResponse
}

func (s *State) filterOptions(ctx context.Context, _ *emptyInput) (*filterOptionsOutput, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	q := func(col string) []string {
		out, _ := s.Repo.DistinctValues(ctx, col, 200)
		return out
	}
	return &filterOptionsOutput{Body: DispatchJobFilterOptionsResponse{
		Statuses:        q("status"),
		Codes:           q("code"),
		ClientIDs:       q("client_id"),
		DispatchPoolIDs: q("dispatch_pool_id"),
		SubscriptionIDs: q("subscription_id"),
		Kinds:           q("kind"),
	}}, nil
}
