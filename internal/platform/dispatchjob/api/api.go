// Package api wires the dispatch-job read-only HTTP endpoints via huma.
package api

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
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
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listDispatchJobs", "/api/dispatch-jobs", "List dispatch jobs with filters", s.list)
	apiroute.Get(g, "listDispatchJobsRaw", "/api/dispatch-jobs/list-raw", "List dispatch jobs (raw)", s.listRaw)
	apiroute.Get(g, "dispatchJobFilterOptions", "/api/dispatch-jobs/filter-options", "Distinct facet values for dispatch jobs", s.filterOptions)
	apiroute.Get(g, "dispatchJobsByEvent", "/api/dispatch-jobs/event/{eventId}", "Dispatch jobs spawned by a specific event", s.byEvent)
	apiroute.Get(g, "getDispatchJob", "/api/dispatch-jobs/{id}", "Get a dispatch job by id", s.getByID)
	apiroute.Get(g, "getDispatchJobRaw", "/api/dispatch-jobs/{id}/raw", "Get a dispatch job (raw)", s.getRaw)
	apiroute.Get(g, "listDispatchJobAttempts", "/api/dispatch-jobs/{id}/attempts", "List a dispatch job's attempt history", s.attempts)

	// SDK-compatibility aliases. The Laravel/Rust client addresses these as
	// /api/dispatch-jobs/by-event/{eventId} and the collection-level
	// /api/dispatch-jobs/raw; Go's canonical paths are /event/{eventId} and
	// /list-raw. Same handlers — keeps the existing SDK working unmodified.
	apiroute.Get(g, "dispatchJobsByEventAlias", "/api/dispatch-jobs/by-event/{eventId}", "Dispatch jobs spawned by an event (SDK alias of /event/{eventId})", s.byEvent)
	apiroute.Get(g, "listDispatchJobsRawAlias", "/api/dispatch-jobs/raw", "List dispatch jobs raw (SDK alias of /list-raw)", s.listRaw)

	// BFF tier — /bff/dispatch-jobs mirrors the regular handlers under
	// cookie-auth. Mirrors Rust.
	registerBFF(api, s, "/bff/dispatch-jobs", "Bff", "bff-dispatch-jobs")

	// /bff/debug/dispatch-jobs is a SEPARATE raw-job view (write-side
	// msg_dispatch_jobs). The SPA's RawDispatchJobListPage binds a bare
	// array of the raw envelope shape, so it gets its own handler.
	// Mirrors Rust's shared/debug_api.rs.
	gd := apiroute.New(api, "bff-debug-dispatch-jobs")
	apiroute.Get(gd, "listDebugDispatchJobs", "/bff/debug/dispatch-jobs", "List raw dispatch jobs (debug view of msg_dispatch_jobs)", s.listDebugRaw)
}

// registerBFF dual-mounts the dispatch-job handlers under an alternate
// base path so the SPA can hit /bff/dispatch-jobs with cookie-auth.
func registerBFF(api huma.API, s *State, base, opPrefix, tag string) {
	g := apiroute.New(api, tag)
	apiroute.Get(g, "listDispatchJobs"+opPrefix, base, "List dispatch jobs", s.list)
	apiroute.Get(g, "listDispatchJobsRaw"+opPrefix, base+"/list-raw", "List dispatch jobs with raw rows", s.listRaw)
	apiroute.Get(g, "dispatchJobFilterOptions"+opPrefix, base+"/filter-options", "Distinct filter values for dispatch jobs", s.filterOptions)
	apiroute.Get(g, "listDispatchJobsByEvent"+opPrefix, base+"/event/{eventId}", "List dispatch jobs created by an event", s.byEvent)
	apiroute.Get(g, "getDispatchJob"+opPrefix, base+"/{id}", "Get a dispatch job by id", s.getByID)
	apiroute.Get(g, "getDispatchJobRaw"+opPrefix, base+"/{id}/raw", "Get a dispatch job with raw row", s.getRaw)
	apiroute.Get(g, "listDispatchJobAttempts"+opPrefix, base+"/{id}/attempts", "List a dispatch job's attempt history", s.attempts)
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

	// SPA params (dispatch-jobs.ts:35-44). `size` caps rows; the plural
	// params are comma-separated multi-filters.
	Size         int    `query:"size" doc:"Max rows (default 50, max 1000)"`
	ClientIDs    string `query:"clientIds" doc:"CSV of client ids"`
	Statuses     string `query:"statuses" doc:"CSV of statuses"`
	Applications string `query:"applications" doc:"CSV of application codes"`
	Subdomains   string `query:"subdomains" doc:"CSV of subdomains"`
	Aggregates   string `query:"aggregates" doc:"CSV of aggregates"`
	Codes        string `query:"codes" doc:"CSV of codes"`
	Source       string `query:"source" doc:"Free-text source filter"`
}

// splitCSV mirrors Rust's split_csv (dispatch_job/api.rs): trim, drop empties.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func (in *listInput) toFilters() dispatchjob.FilterParams {
	ts := func(v string) *time.Time {
		if v == "" {
			return nil
		}
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return &t
		}
		return nil
	}
	// `size` (SPA) and `limit` (SDK) both cap rows; size wins when set.
	limit := in.Limit
	if in.Size > 0 {
		limit = in.Size
	}
	// `source` free-text reuses the singular Source filter.
	src := apicommon.OptStr(in.Source)
	return dispatchjob.FilterParams{
		Status:         apicommon.OptStr(in.Status),
		ClientID:       apicommon.OptStr(in.ClientID),
		DispatchPoolID: apicommon.OptStr(in.DispatchPoolID),
		SubscriptionID: apicommon.OptStr(in.SubscriptionID),
		Code:           apicommon.OptStr(in.Code),
		Source:         src,
		Since:          ts(in.Since),
		Until:          ts(in.Until),
		Limit:          limit,
		Offset:         in.Offset,
		ClientIDs:      splitCSV(in.ClientIDs),
		Statuses:       splitCSV(in.Statuses),
		Applications:   splitCSV(in.Applications),
		Subdomains:     splitCSV(in.Subdomains),
		Aggregates:     splitCSV(in.Aggregates),
		Codes:          splitCSV(in.Codes),
	}
}

// scopeFilters applies SQL-side tenant scoping (anchor sees all → no
// scoping). Without it a non-anchor holding dispatch-job:view could read any
// tenant's jobs by passing arbitrary clientId/clientIds filters — the
// caller-controlled filters must only narrow within the principal's own
// tenants. Same pattern as scheduledjob's and event's list.
func scopeFilters(ac *auth.AuthContext, f dispatchjob.FilterParams) dispatchjob.FilterParams {
	if !ac.IsAnchor() {
		clients := ac.Clients
		f.AccessibleClientIDs = &clients
	}
	return f
}

// list's Body is a bare JSON array — the SPA's DispatchJobListPage binds
// the returned array directly to its DataTable, so {items:[...]} would
// render zero rows. Mirrors Rust's list_dispatch_jobs returning
// Vec<DispatchJobReadResponse>. (Shared by listRaw + byEvent.)
func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[[]DispatchJobRead], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, scopeFilters(ac, in.toFilters()))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := apicommon.MapSlice(rows, readFromEntity)
	return &apicommon.Out[[]DispatchJobRead]{Body: out}, nil
}

func (s *State) listRaw(ctx context.Context, in *listInput) (*apicommon.Out[[]DispatchJobRead], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewRawPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, scopeFilters(ac, in.toFilters()))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_raw failed", err)
	}
	out := apicommon.MapSlice(rows, readFromEntity)
	return &apicommon.Out[[]DispatchJobRead]{Body: out}, nil
}

// ── debug raw dispatch jobs ──────────────────────────────────────────────

type rawListInput struct {
	Size int `query:"size" doc:"Max rows (default 50, max 1000)"`
}

// listDebugRaw's Body is a bare array of RawDispatchJobResponse — the
// SPA's RawDispatchJobListPage binds it directly.
func (s *State) listDebugRaw(ctx context.Context, in *rawListInput) (*apicommon.Out[[]RawDispatchJobResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewRawPerm); err != nil {
		return nil, err
	}
	limit := in.Size
	if limit <= 0 {
		limit = 50
	}
	// FindRecentRaw reads the write-side msg_dispatch_jobs table, which carries
	// the un-projected envelope (payload/metadata) the debug view needs — the
	// read projection used by the regular list drops it. Returns the most-recent
	// N jobs.
	rows, err := s.Repo.FindRecentRaw(ctx, limit)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_recent_raw failed", err)
	}
	out := apicommon.MapSlice(rows, rawFromEntity)
	return &apicommon.Out[[]RawDispatchJobResponse]{Body: out}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[DispatchJobResponse], error) {
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
	if err := auth.CheckScopeAccess(ac, j.ClientID); err != nil { // A2: per-resource client scope
		return nil, err
	}
	return &apicommon.Out[DispatchJobResponse]{Body: fromEntity(j)}, nil
}

func (s *State) getRaw(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[DispatchJobResponse], error) {
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
	if err := auth.CheckScopeAccess(ac, j.ClientID); err != nil { // A2: per-resource client scope
		return nil, err
	}
	return &apicommon.Out[DispatchJobResponse]{Body: fromEntity(j)}, nil
}

// attempts' Body is a bare JSON array — the Rust shape for
// GET /api/dispatch-jobs/{id}/attempts.
func (s *State) attempts(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[[]AttemptDTO], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	// A2: load the job to enforce per-resource client scope before exposing
	// its attempts (the attempts query itself isn't client-scoped).
	j, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("DispatchJob", in.ID)
	}
	if err := auth.CheckScopeAccess(ac, j.ClientID); err != nil {
		return nil, err
	}
	rows, err := s.Repo.AttemptsByJob(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "attempts failed", err)
	}
	out := apicommon.MapSlice(rows, attemptFromEntity)
	return &apicommon.Out[[]AttemptDTO]{Body: out}, nil
}

type byEventInput struct {
	EventID string `path:"eventId"`
}

func (s *State) byEvent(ctx context.Context, in *byEventInput) (*apicommon.Out[[]DispatchJobRead], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindByEventID(ctx, in.EventID)
	if err != nil {
		return nil, usecase.Internal("REPO", "by_event failed", err)
	}
	out := make([]DispatchJobRead, 0, len(rows))
	for i := range rows {
		// A2: a non-anchor caller only sees jobs for clients it can access.
		// NOTE: CanAccessScope (not FilterClientScoped) — platform-scoped
		// jobs (nil client) are visible to anchors/super-admins only here.
		if !auth.CanAccessScope(ac, rows[i].ClientID) {
			continue
		}
		out = append(out, readFromEntity(&rows[i]))
	}
	return &apicommon.Out[[]DispatchJobRead]{Body: out}, nil
}

func (s *State) filterOptions(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[DispatchJobFilterOptionsResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	q := func(col string) []string {
		out, _ := s.Repo.DistinctValues(ctx, col, 200)
		return out
	}
	return &apicommon.Out[DispatchJobFilterOptionsResponse]{Body: DispatchJobFilterOptionsResponse{
		Statuses:        q("status"),
		Codes:           q("code"),
		ClientIDs:       q("client_id"),
		DispatchPoolIDs: q("dispatch_pool_id"),
		SubscriptionIDs: q("subscription_id"),
		Kinds:           q("kind"),
	}}, nil
}
