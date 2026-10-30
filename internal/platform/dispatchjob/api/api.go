// Package api wires the dispatch-job read-only HTTP endpoints via huma.
package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/operations"
	dispatchprocessing "github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob/processing"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/sdk"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// State bundles deps. UoW is required by the Cancel/Complete/Resend
// use-case-envelope handlers (T3/A-01) — see operations/. The wiring site
// (internal/server/wire_routes.go, dispatchjobapi.State{...}) must be
// updated to pass it; see this tranche's report for the exact line.
type State struct {
	Repo *dispatchjob.Repository
	UoW  *usecasepgx.UnitOfWork
	// ClientIdentifier resolves a job's client_id to the client's identifier
	// slug for the list rows (the grid shows a client, not an id). nil
	// leaves clientIdentifier empty. Wired to client.NewCachedIdentifierResolver.
	ClientIdentifier func(ctx context.Context, clientID string) (string, bool)
	// Plan builds a job's delivery — signing account, headers, a real
	// signature over the real body — WITHOUT sending it, for the "sign"
	// action (owner, 2026-09-22). nil disables the route's body (501).
	// Wired to processing.Handler.Plan.
	Plan func(ctx context.Context, job *dispatchjob.DispatchJob) (*dispatchprocessing.DeliveryPlan, error)
	// Apps resolves a caller's bound application ids to codes, to confine a
	// caller admitted only by the application-service dispatch-job:view to its
	// own applications' jobs (F20). Optional only in tests: nil refuses such a
	// caller.
	Apps *application.Repository
	// MaxWindowDays is the widest created_at window a list-shaped read may ask
	// for (FC_DISPATCH_JOB_MAX_WINDOW_DAYS); < 1 means
	// dispatchjob.DefaultMaxWindowDays.
	MaxWindowDays int

	// filterOptions keeps the filter-options answers for a short time.
	foCache filterOptionsCache
}

// window holds f to the time window every list-shaped read carries and reports
// it (see dispatchjob.EnforceWindow).
func (s *State) window(f dispatchjob.FilterParams) (dispatchjob.FilterParams, dispatchjob.Window, error) {
	return dispatchjob.EnforceWindow(f, time.Now(), s.MaxWindowDays)
}

// withClientIdentifier fills each list row's clientIdentifier through the
// resolver's cache — one lookup per distinct client per TTL, not per row.
func (s *State) withClientIdentifier(ctx context.Context, rows []DispatchJobRead) []DispatchJobRead {
	if s.ClientIdentifier == nil {
		return rows
	}
	for i := range rows {
		if rows[i].ClientID == nil || *rows[i].ClientID == "" {
			continue
		}
		if ident, ok := s.ClientIdentifier(ctx, *rows[i].ClientID); ok {
			rows[i].ClientIdentifier = &ident
		}
	}
	return rows
}

const (
	tag         = "dispatch-jobs"
	viewPerm    = "platform:messaging:dispatch-job:view"
	viewRawPerm = "platform:messaging:dispatch-job:view-raw"
)

// requireRequeue gates requeue by ids and by filter. A request that asks to
// redrive COMPLETED jobs (redriveCompleted) additionally needs
// dispatch-job:redrive-completed; asking without it is a 403, not a quiet skip.
func requireRequeue(ac *auth.AuthContext, redriveCompleted bool) error {
	if err := auth.CanWritePermission(ac, auth.DispatchJobRequeuePerm); err != nil {
		return err
	}
	if redriveCompleted {
		return auth.CanWritePermission(ac, auth.DispatchJobRedriveCompletedPerm)
	}
	return nil
}

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
	apiroute.Post(g, "requeueDispatchJobs", "/api/dispatch-jobs/requeue", "Reset dispatch jobs to PENDING for re-dispatch", http.StatusOK, s.requeue)
	apiroute.Post(g, "previewRequeueDispatchJobsByFilter", "/api/dispatch-jobs/requeue-by-filter/preview", "Count the dispatch jobs a filter matches and how many a requeue would reset", http.StatusOK, s.requeueByFilterPreview)
	apiroute.Post(g, "requeueDispatchJobsByFilter", "/api/dispatch-jobs/requeue-by-filter", "Reset every dispatch job a filter matches to PENDING (confirmed count, capped)", http.StatusOK, s.requeueByFilter)
	apiroute.Post(g, "cancelDispatchJob", "/api/dispatch-jobs/{id}/cancel", "Cancel a PENDING or FAILED dispatch job", http.StatusOK, s.cancel)
	apiroute.Post(g, "completeDispatchJob", "/api/dispatch-jobs/{id}/complete", "Mark a FAILED dispatch job COMPLETED (delivered out of band)", http.StatusOK, s.complete)
	apiroute.Put(g, "editDispatchJobPayload", "/api/dispatch-jobs/{id}/payload", "Replace the payload of a PENDING or FAILED dispatch job (status and attempts are unchanged)", http.StatusOK, s.editPayload)
	apiroute.Post(g, "signDispatchJob", "/api/dispatch-jobs/{id}/sign", "Build the job's delivery (signing account, headers, signature) without sending it", http.StatusOK, s.sign)

	// SDK-compatibility aliases. The Laravel SDK client addresses these as
	// /api/dispatch-jobs/by-event/{eventId} and the collection-level
	// /api/dispatch-jobs/raw; Go's canonical paths are /event/{eventId} and
	// /list-raw. Same handlers — keeps the existing SDK working unmodified.
	apiroute.Get(g, "dispatchJobsByEventAlias", "/api/dispatch-jobs/by-event/{eventId}", "Dispatch jobs spawned by an event (SDK alias of /event/{eventId})", s.byEvent)
	apiroute.Get(g, "listDispatchJobsRawAlias", "/api/dispatch-jobs/raw", "List dispatch jobs raw (SDK alias of /list-raw)", s.listRaw)

	// BFF tier — /bff/dispatch-jobs mirrors the regular handlers under
	// cookie-auth.
	registerBFF(api, s, "/bff/dispatch-jobs", "Bff", "bff-dispatch-jobs")

	// /bff/debug/dispatch-jobs is a SEPARATE raw-job view (write-side
	// msg_dispatch_jobs). The SPA's RawDispatchJobListPage binds a bare
	// array of the raw envelope shape, so it gets its own handler.
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
	apiroute.Post(g, "requeueDispatchJobs"+opPrefix, base+"/requeue", "Reset dispatch jobs to PENDING for re-dispatch", http.StatusOK, s.requeue)
	apiroute.Post(g, "previewRequeueDispatchJobsByFilter"+opPrefix, base+"/requeue-by-filter/preview", "Count the dispatch jobs a filter matches and how many a requeue would reset", http.StatusOK, s.requeueByFilterPreview)
	apiroute.Post(g, "requeueDispatchJobsByFilter"+opPrefix, base+"/requeue-by-filter", "Reset every dispatch job a filter matches to PENDING (confirmed count, capped)", http.StatusOK, s.requeueByFilter)
	apiroute.Post(g, "cancelDispatchJob"+opPrefix, base+"/{id}/cancel", "Cancel a PENDING or FAILED dispatch job", http.StatusOK, s.cancel)
	apiroute.Post(g, "completeDispatchJob"+opPrefix, base+"/{id}/complete", "Mark a FAILED dispatch job COMPLETED (delivered out of band)", http.StatusOK, s.complete)
	apiroute.Put(g, "editDispatchJobPayload"+opPrefix, base+"/{id}/payload", "Replace the payload of a PENDING or FAILED dispatch job (status and attempts are unchanged)", http.StatusOK, s.editPayload)
	apiroute.Post(g, "signDispatchJob"+opPrefix, base+"/{id}/sign", "Build the job's delivery (signing account, headers, signature) without sending it", http.StatusOK, s.sign)
}

// sign is the dry run: what a delivery of this job would send right now.
// The signature it returns is real — verifiable by the subscriber's own
// verify command against the same body and timestamp — which is the point:
// it lets an operator prove which side holds the wrong secret without a
// delivery. Gated on the raw-view permission, like the payload it returns.
func (s *State) sign(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[dispatchprocessing.DeliveryPlan], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewRawPerm); err != nil {
		return nil, err
	}
	if s.Plan == nil {
		return nil, huma.Error501NotImplemented("delivery planning is not wired on this server")
	}
	j, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("DispatchJob", in.ID)
	}
	if err := auth.CheckScopeAccess(ac, ids.StringPtr(j.ClientID)); err != nil {
		return nil, err
	}
	plan, err := s.Plan(ctx, j)
	if err != nil {
		return nil, usecase.Internal("PLAN", "build delivery failed", err)
	}
	return &apicommon.Out[dispatchprocessing.DeliveryPlan]{Body: *plan}, nil
}

// DispatchJobFilter is the dispatch-job filter: the list's query parameters
// and, under the same names, the requeue-by-filter request bodies. One struct
// and one conversion (toParams) so the list and a bulk requeue of "what the
// list shows" cannot select different jobs. The plural fields are
// comma-separated multi-filters in both places.
type DispatchJobFilter struct {
	Status         string `query:"status" json:"status,omitempty"`
	ClientID       string `query:"clientId" json:"clientId,omitempty"`
	DispatchPoolID string `query:"dispatchPoolId" json:"dispatchPoolId,omitempty"`
	SubscriptionID string `query:"subscriptionId" json:"subscriptionId,omitempty"`
	Code           string `query:"code" json:"code,omitempty"`
	Since          string `query:"since" json:"since,omitempty" doc:"RFC3339 timestamp"`
	Until          string `query:"until" json:"until,omitempty" doc:"RFC3339 timestamp"`
	MessageGroup   string `query:"messageGroup" json:"messageGroup,omitempty" doc:"Exact message group"`
	ClientIDs      string `query:"clientIds" json:"clientIds,omitempty" doc:"CSV of client ids"`
	Statuses       string `query:"statuses" json:"statuses,omitempty" doc:"CSV of statuses"`
	Applications   string `query:"applications" json:"applications,omitempty" doc:"CSV of application codes"`
	Subdomains     string `query:"subdomains" json:"subdomains,omitempty" doc:"CSV of subdomains"`
	Aggregates     string `query:"aggregates" json:"aggregates,omitempty" doc:"CSV of aggregates"`
	Codes          string `query:"codes" json:"codes,omitempty" doc:"CSV of codes"`
	Source         string `query:"source" json:"source,omitempty" doc:"Free-text source filter"`

	EventType        string `query:"eventType" json:"eventType,omitempty" doc:"Exact event type. A job's code is its event type for a job raised by an event (and the caller's own job code for an SDK or function-dispatched job), so this filters the same column as code, by an index that also orders by createdAt. Combines with eventTypes and codes (all must hold)."`
	EventTypes       string `query:"eventTypes" json:"eventTypes,omitempty" doc:"CSV of event types (at most 10; more is 400 TOO_MANY_EVENT_TYPES)"`
	SubscriptionIDs  string `query:"subscriptionIds" json:"subscriptionIds,omitempty" doc:"CSV of subscription ids (at most 50; more is 400 TOO_MANY_SUBSCRIPTION_IDS). Combines with subscriptionId."`
	SubscriptionCode string `query:"subscriptionCode" json:"subscriptionCode,omitempty" doc:"Subscription code, exact; a trailing * makes it a prefix. Resolved to subscription ids first (400 TOO_MANY_MATCHES past 200); a code that matches nothing lists no jobs."`
	SubscriptionName string `query:"subscriptionName" json:"subscriptionName,omitempty" doc:"Subscription name, exact and case-insensitive; a trailing * makes it a prefix. Resolved like subscriptionCode."`
	ConnectionID     string `query:"connectionId" json:"connectionId,omitempty" doc:"Jobs delivered through this connection: the jobs of its subscriptions and the jobs that name the connection themselves (function dispatch)."`
	ConnectionCode   string `query:"connectionCode" json:"connectionCode,omitempty" doc:"Connection code, exact; a trailing * makes it a prefix. Resolved to connection ids first (400 TOO_MANY_MATCHES past 50); a code that matches nothing lists no jobs."`
}

// Caps on the multi-value filters, so one request cannot ship an unbounded ANY list.
const (
	maxEventTypes      = 10
	maxSubscriptionIDs = 50
)

type listInput struct {
	DispatchJobFilter
	Limit  int `query:"limit"`
	Offset int `query:"offset"`
	// `size` (SPA) caps rows like `limit` (SDK).
	Size int    `query:"size" doc:"Max rows (default 50, max 1000)"`
	Sort string `query:"sort" doc:"createdAt.asc | createdAt.desc (default)"`
}

// splitCSV splits a comma-separated query value: trim, drop empties.
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

// toParams converts the filter. With strict, an unparseable since/until is a
// 400: the list ignores one (and shows more rows), but a bulk requeue that
// silently dropped a bound would reset more jobs than the caller asked for.
func (in *DispatchJobFilter) toParams(strict bool) (dispatchjob.FilterParams, error) {
	eventTypes := splitCSV(in.EventTypes)
	if in.EventType != "" && !slices.Contains(eventTypes, in.EventType) {
		eventTypes = append(eventTypes, in.EventType)
	}
	if len(eventTypes) > maxEventTypes {
		return dispatchjob.FilterParams{}, usecase.Validation("TOO_MANY_EVENT_TYPES",
			"at most "+strconv.Itoa(maxEventTypes)+" event types may be filtered at once")
	}
	subIDs := splitCSV(in.SubscriptionIDs)
	if len(subIDs) > maxSubscriptionIDs {
		return dispatchjob.FilterParams{}, usecase.Validation("TOO_MANY_SUBSCRIPTION_IDS",
			"at most "+strconv.Itoa(maxSubscriptionIDs)+" subscription ids may be filtered at once")
	}
	var bad error
	ts := func(name, v string) *time.Time {
		if v == "" {
			return nil
		}
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			if bad == nil {
				bad = usecase.Validation("INVALID_TIMESTAMP", name+" must be an RFC3339 timestamp")
			}
			return nil
		}
		return &t
	}
	p := dispatchjob.FilterParams{
		Status:         apicommon.OptStr(in.Status),
		ClientID:       apicommon.OptStr(in.ClientID),
		DispatchPoolID: apicommon.OptStr(in.DispatchPoolID),
		SubscriptionID: apicommon.OptStr(in.SubscriptionID),
		Code:           apicommon.OptStr(in.Code),
		Source:         apicommon.OptStr(in.Source),
		MessageGroup:   apicommon.OptStr(in.MessageGroup),
		Since:          ts("since", in.Since),
		Until:          ts("until", in.Until),
		ClientIDs:      splitCSV(in.ClientIDs),
		Statuses:       splitCSV(in.Statuses),
		Applications:   splitCSV(in.Applications),
		Subdomains:     splitCSV(in.Subdomains),
		Aggregates:     splitCSV(in.Aggregates),
		Codes:          splitCSV(in.Codes),

		EventTypes:       eventTypes,
		SubscriptionIDs:  subIDs,
		SubscriptionCode: apicommon.OptStr(strings.TrimSpace(in.SubscriptionCode)),
		SubscriptionName: apicommon.OptStr(strings.TrimSpace(in.SubscriptionName)),
		ConnectionID:     apicommon.OptStr(in.ConnectionID),
		ConnectionCode:   apicommon.OptStr(strings.TrimSpace(in.ConnectionCode)),
	}
	if strict && bad != nil {
		return dispatchjob.FilterParams{}, bad
	}
	return p, nil
}

func (in *listInput) toFilters() (dispatchjob.FilterParams, error) {
	p, err := in.toParams(false)
	if err != nil {
		return p, err
	}
	// `size` (SPA) and `limit` (SDK) both cap rows; size wins when set.
	p.Limit = in.Limit
	if in.Size > 0 {
		p.Limit = in.Size
	}
	p.Offset = in.Offset
	p.SortAscending = in.Sort == "createdAt.asc"
	return p, nil
}

// listFilters is the list's filter for ac: the query's filter, tenant-scoped, with its
// by-name references resolved (a lookup scoped to the caller's own tenants) and held to
// the time window.
func (s *State) listFilters(ctx context.Context, ac *auth.AuthContext, in *listInput) (dispatchjob.FilterParams, dispatchjob.Window, error) {
	f, err := in.toFilters()
	if err != nil {
		return f, dispatchjob.Window{}, err
	}
	return s.scoped(ctx, ac, f)
}

// scoped scopes f to ac's tenants, resolves its by-name references and holds it to the
// time window — the order every list-shaped read takes.
func (s *State) scoped(ctx context.Context, ac *auth.AuthContext, f dispatchjob.FilterParams) (dispatchjob.FilterParams, dispatchjob.Window, error) {
	f, win, err := s.window(scopeFilters(ac, f))
	if err != nil {
		return f, win, err
	}
	f, err = s.Repo.ResolveRefs(ctx, f)
	return f, win, err
}

// scopeFilters applies SQL-side tenant scoping (anchor sees all → no
// scoping). Without it a non-anchor holding dispatch-job:view could read any
// tenant's jobs by passing arbitrary clientId/clientIds filters — the
// caller-controlled filters must only narrow within the principal's own
// tenants. Same pattern as scheduledjob's and event's list.
func scopeFilters(ac *auth.AuthContext, f dispatchjob.FilterParams) dispatchjob.FilterParams {
	return operations.VisibleTo(ac, f)
}

// list's Body is a bare JSON array — the SPA's DispatchJobListPage binds
// the returned array directly to its DataTable, so {items:[...]} would
// render zero rows. (Shared by listRaw + byEvent.)
// readConfinement answers the application codes a dispatch-job read is
// confined to for ac (F20): none for a caller holding the messaging view
// permission (as before) or reaching every application; for a caller admitted
// only by the application-service dispatch-job:view, the codes of the
// applications it is bound to (403 for a caller bound to none).
func (s *State) readConfinement(ctx context.Context, ac *auth.AuthContext) (*[]string, error) {
	if auth.CanReadAllDispatchJobs(ac) {
		return nil, nil
	}
	codes, confined, err := sdk.NewAppScope(s.Apps).ConfinedCodes(ctx, ac)
	if err != nil || !confined {
		return nil, err
	}
	return &codes, nil
}

// ownedBy reports whether a job's application (the first segment of its code,
// the projection's application column) is one of apps; always true when the
// read is not confined (apps nil).
func ownedBy(apps *[]string, code string) bool {
	return apps == nil || slices.Contains(*apps, sdk.FirstSegment(code))
}

// DispatchJobListOut is a list answer: a bare JSON array (the SPA and the SDKs
// bind it directly) with the applied time window in response headers.
type DispatchJobListOut struct {
	WindowFrom      string `header:"X-Window-From" doc:"Start of the created_at window the list was held to (RFC3339, UTC)"`
	WindowTo        string `header:"X-Window-To" doc:"End of the window (RFC3339, UTC); now when the request gave no until"`
	WindowDefaulted string `header:"X-Window-Defaulted" doc:"true when the server supplied a bound the request did not (no since/until: the last 24 hours)"`
	Body            []DispatchJobRead
}

func windowedList(w dispatchjob.Window, rows []DispatchJobRead) *DispatchJobListOut {
	return &DispatchJobListOut{
		WindowFrom:      w.From.Format(time.RFC3339),
		WindowTo:        w.To.Format(time.RFC3339),
		WindowDefaulted: strconv.FormatBool(w.Defaulted),
		Body:            rows,
	}
}

func (s *State) list(ctx context.Context, in *listInput) (*DispatchJobListOut, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadDispatchJobs(ac); err != nil {
		return nil, err
	}
	apps, err := s.readConfinement(ctx, ac)
	if err != nil {
		return nil, err
	}
	f, win, err := s.listFilters(ctx, ac, in)
	if err != nil {
		return nil, err
	}
	f.AccessibleApplications = apps
	rows, err := s.Repo.FindWithFilters(ctx, f)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := s.withClientIdentifier(ctx, apicommon.MapSlice(rows, readFromEntity))
	return windowedList(win, out), nil
}

func (s *State) listRaw(ctx context.Context, in *listInput) (*DispatchJobListOut, error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewRawPerm); err != nil {
		return nil, err
	}
	f, win, err := s.listFilters(ctx, ac, in)
	if err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, f)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_raw failed", err)
	}
	out := s.withClientIdentifier(ctx, apicommon.MapSlice(rows, readFromEntity))
	return windowedList(win, out), nil
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
	if err := auth.CanReadDispatchJobs(ac); err != nil {
		return nil, err
	}
	apps, err := s.readConfinement(ctx, ac)
	if err != nil {
		return nil, err
	}
	j, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	// A job of another application answers the same not-found a missing id
	// would (F20).
	if j == nil || !ownedBy(apps, j.Code) {
		return nil, httperror.NotFound("DispatchJob", in.ID)
	}
	if err := auth.CheckScopeAccess(ac, ids.StringPtr(j.ClientID)); err != nil { // A2: per-resource client scope
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
	if err := auth.CheckScopeAccess(ac, ids.StringPtr(j.ClientID)); err != nil { // A2: per-resource client scope
		return nil, err
	}
	return &apicommon.Out[DispatchJobResponse]{Body: fromEntity(j)}, nil
}

// attempts' Body is a bare JSON array — the wire shape for
// GET /api/dispatch-jobs/{id}/attempts.
func (s *State) attempts(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[[]AttemptDTO], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanReadDispatchJobs(ac); err != nil {
		return nil, err
	}
	apps, err := s.readConfinement(ctx, ac)
	if err != nil {
		return nil, err
	}
	// A2: load the job to enforce per-resource client scope before exposing
	// its attempts (the attempts query itself isn't client-scoped); F20: and
	// the application confinement.
	j, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if j == nil || !ownedBy(apps, j.Code) {
		return nil, httperror.NotFound("DispatchJob", in.ID)
	}
	if err := auth.CheckScopeAccess(ac, ids.StringPtr(j.ClientID)); err != nil {
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
	if err := auth.CanReadDispatchJobs(ac); err != nil {
		return nil, err
	}
	apps, err := s.readConfinement(ctx, ac)
	if err != nil {
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
		if !auth.CanAccessScope(ac, ids.StringPtr(rows[i].ClientID)) {
			continue
		}
		// F20: only the caller's own applications' jobs.
		if !ownedBy(apps, rows[i].Code) {
			continue
		}
		out = append(out, readFromEntity(&rows[i]))
	}
	return &apicommon.Out[[]DispatchJobRead]{Body: out}, nil
}

// RequeueRequest is the body of POST /dispatch-jobs/requeue.
type RequeueRequest struct {
	IDs                     []string `json:"ids" doc:"Dispatch job ids to reset to PENDING for re-dispatch. QUEUED and PROCESSING jobs are never reset; COMPLETED jobs only with redriveCompleted and confirmRedriveCompleted. Skipped jobs are reported, not an error."`
	RedriveCompleted        bool     `json:"redriveCompleted,omitempty" doc:"Also re-deliver COMPLETED jobs. Needs confirmRedriveCompleted=REDRIVE and the dispatch-job:redrive-completed permission (403 without it)."`
	ConfirmRedriveCompleted string   `json:"confirmRedriveCompleted,omitempty" doc:"The literal REDRIVE, confirming redriveCompleted. Without both fields, COMPLETED jobs are skipped."`
}

// SkippedJobResponse is a job a requeue found and left alone.
type SkippedJobResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Reason string `json:"reason" enum:"IN_FLIGHT,COMPLETED_REDRIVE_NOT_CONFIRMED" doc:"IN_FLIGHT: QUEUED or PROCESSING, never reset. COMPLETED_REDRIVE_NOT_CONFIRMED: completed jobs are redriven only with redriveCompleted and confirmRedriveCompleted."`
}

// RequeueResponse reports how many jobs were reset.
type RequeueResponse struct {
	Requeued         int64                `json:"requeued" doc:"Jobs reset to PENDING (redriven COMPLETED jobs included)"`
	Redriven         int64                `json:"redriven" doc:"How many of them were COMPLETED jobs re-delivered (0 unless the request redrove)"`
	SkippedInFlight  int64                `json:"skippedInFlight" doc:"QUEUED or PROCESSING jobs left alone"`
	SkippedCompleted int64                `json:"skippedCompleted" doc:"COMPLETED jobs left alone because the redrive was not requested and confirmed"`
	Skipped          []SkippedJobResponse `json:"skipped" doc:"The skipped jobs, with the reason"`
}

// requeue resets the given jobs to PENDING so the scheduler re-dispatches
// them (clears scheduled_for + attempt_count + terminal stamps). An
// operator recovery action. Runs through the ResendDispatchJobs use case
// (T3/A-01). A job in flight (QUEUED, PROCESSING) is never reset and a
// COMPLETED job only by a confirmed redrive; the rest are skipped and reported.
//
// Gated on dispatch-job:requeue (and dispatch-job:redrive-completed for a
// redrive request). Per-tenant scoping (a requeue grant can't be used to
// requeue another tenant's jobs) is enforced inside the use case's Execute,
// not in SQL here — see ResendDispatchJobs.
func (s *State) requeue(ctx context.Context, in *apicommon.In[RequeueRequest]) (*apicommon.Out[RequeueResponse], error) {
	ac := auth.FromContext(ctx)
	if err := requireRequeue(ac, in.Body.RedriveCompleted); err != nil {
		return nil, err
	}
	if len(in.Body.IDs) == 0 {
		return &apicommon.Out[RequeueResponse]{Body: RequeueResponse{Skipped: []SkippedJobResponse{}}}, nil
	}
	ec := auth.NewExecutionContext(ctx)
	cmd := operations.ResendCommand{
		IDs: in.Body.IDs, RedriveCompleted: in.Body.RedriveCompleted, ConfirmRedriveCompleted: in.Body.ConfirmRedriveCompleted,
	}
	event, err := usecaseop.Run(ctx, s.UoW, operations.ResendDispatchJobs(s.Repo), cmd, ec)
	if err != nil {
		return nil, err
	}
	out := RequeueResponse{
		Requeued: int64(len(event.IDs)), Redriven: int64(len(event.Redriven)),
		Skipped: make([]SkippedJobResponse, 0, len(event.Skipped)),
	}
	for _, sk := range event.Skipped {
		out.Skipped = append(out.Skipped, SkippedJobResponse{ID: sk.ID, Status: sk.Status, Reason: sk.Reason})
		if sk.Reason == dispatchjob.SkipInFlight {
			out.SkippedInFlight++
		} else {
			out.SkippedCompleted++
		}
	}
	return &apicommon.Out[RequeueResponse]{Body: out}, nil
}

// WindowResponse is the created_at window a bulk read was held to.
type WindowResponse struct {
	From      time.Time `json:"from" doc:"Start of the window (inclusive)"`
	To        time.Time `json:"to" doc:"End of the window; now when the request gave no until"`
	Defaulted bool      `json:"defaulted" doc:"true when the server supplied a bound the request did not (no since/until: the last 24 hours)"`
}

func windowResponse(w dispatchjob.Window) WindowResponse {
	return WindowResponse{From: w.From, To: w.To, Defaulted: w.Defaulted}
}

// RequeueByFilterPreviewRequest is the body of POST
// /dispatch-jobs/requeue-by-filter/preview: the list's filter. With no
// since/until the last 24 hours are counted; a window beyond
// FC_DISPATCH_JOB_MAX_WINDOW_DAYS (default 31) is 400 WINDOW_TOO_LARGE.
type RequeueByFilterPreviewRequest struct {
	DispatchJobFilter
}

// RequeueByFilterPreviewResponse describes the jobs a filter matches.
type RequeueByFilterPreviewResponse struct {
	Count                int64            `json:"count" doc:"Jobs the filter matches in the window that the caller can see"`
	ByStatus             map[string]int64 `json:"byStatus" doc:"count broken down by status"`
	EligibleCount        int64            `json:"eligibleCount" doc:"How many of them a requeue would reset for this caller (not COMPLETED, not in flight); send it back as confirmCount"`
	CompletedCount       int64            `json:"completedCount" doc:"COMPLETED jobs the filter matches that the caller can see. A requeue skips them; they are redriven only with redriveCompleted and confirmRedriveCompleted"`
	InFlightCount        int64            `json:"inFlightCount" doc:"QUEUED or PROCESSING jobs the filter matches; a requeue never resets them"`
	RedriveEligibleCount int64            `json:"redriveEligibleCount" doc:"eligibleCount + completedCount: the confirmCount of a request that also redrives COMPLETED jobs"`
	Window               WindowResponse   `json:"window" doc:"The created_at window the counts cover"`
}

// RequeueByFilterRequest is the body of POST /dispatch-jobs/requeue-by-filter:
// the list's filter, the count the caller confirmed, and a cap. The same window
// rules as the preview apply (default last 24 hours, maximum
// FC_DISPATCH_JOB_MAX_WINDOW_DAYS).
type RequeueByFilterRequest struct {
	DispatchJobFilter
	ConfirmCount            int64  `json:"confirmCount" minimum:"0" doc:"The eligibleCount the caller was shown (redriveEligibleCount when redriveCompleted). If more jobs would be reset now, nothing is reset and the response is 409 FILTER_MATCH_CHANGED."`
	Limit                   int    `json:"limit,omitempty" minimum:"0" maximum:"10000" doc:"Most jobs to reset in this request (default and maximum 10000)"`
	RedriveCompleted        bool   `json:"redriveCompleted,omitempty" doc:"Also re-deliver the COMPLETED jobs the filter matches. Needs confirmRedriveCompleted=REDRIVE and the dispatch-job:redrive-completed permission (403 without it). Without both fields COMPLETED jobs are skipped and counted in skippedCompleted."`
	ConfirmRedriveCompleted string `json:"confirmRedriveCompleted,omitempty" doc:"The literal REDRIVE, confirming redriveCompleted."`
}

// RequeueByFilterResponse reports a requeue-by-filter run.
type RequeueByFilterResponse struct {
	Requeued         int64          `json:"requeued" doc:"Jobs reset to PENDING (redriven COMPLETED jobs included)"`
	Redriven         int64          `json:"redriven" doc:"How many of them were COMPLETED jobs re-delivered"`
	SkippedInFlight  int64          `json:"skippedInFlight" doc:"QUEUED or PROCESSING matches left alone (never reset)"`
	SkippedCompleted int64          `json:"skippedCompleted" doc:"COMPLETED matches left alone because the redrive was not requested and confirmed"`
	Truncated        bool           `json:"truncated" doc:"The run stopped at its limit (or at confirmCount) with eligible jobs left"`
	Remaining        int64          `json:"remaining" doc:"Eligible jobs still matching after the run; run again to continue"`
	Window           WindowResponse `json:"window" doc:"The created_at window the run was held to"`
}

// requeueByFilterPreview counts what a requeue-by-filter would touch, within
// the request's window. Callable with dispatch-job:requeue or dispatch-job:view
// (the counts are no more than the list shows).
func (s *State) requeueByFilterPreview(ctx context.Context, in *apicommon.In[RequeueByFilterPreviewRequest]) (*apicommon.Out[RequeueByFilterPreviewResponse], error) {
	ac := auth.FromContext(ctx)
	if auth.CanWritePermission(ac, auth.DispatchJobRequeuePerm) != nil {
		if err := auth.CanWritePermission(ac, viewPerm); err != nil {
			return nil, err
		}
	}
	f, err := in.Body.toParams(true)
	if err != nil {
		return nil, err
	}
	f, win, err := s.scoped(ctx, ac, f)
	if err != nil {
		return nil, err
	}
	sum, err := operations.SummarizeMatches(ctx, s.Repo, f)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[RequeueByFilterPreviewResponse]{Body: RequeueByFilterPreviewResponse{
		Count: sum.Count, ByStatus: sum.ByStatus, EligibleCount: sum.Eligible,
		CompletedCount: sum.Completed, InFlightCount: sum.InFlight, RedriveEligibleCount: sum.RedriveEligible(),
		Window: windowResponse(win),
	}}, nil
}

// requeueByFilter resets every job the filter matches in its window — up to
// limit and never more than confirmCount — through ResendDispatchJobs,
// ResendBatchSize ids per transaction. Gated like requeue; tenant scoping
// stays in the use case.
func (s *State) requeueByFilter(ctx context.Context, in *apicommon.In[RequeueByFilterRequest]) (*apicommon.Out[RequeueByFilterResponse], error) {
	ac := auth.FromContext(ctx)
	if err := requireRequeue(ac, in.Body.RedriveCompleted); err != nil {
		return nil, err
	}
	f, err := in.Body.toParams(true)
	if err != nil {
		return nil, err
	}
	f, win, err := s.scoped(ctx, ac, f)
	if err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	opts := operations.ResendByFilterOptions{
		RedriveCompleted: in.Body.RedriveCompleted, ConfirmRedriveCompleted: in.Body.ConfirmRedriveCompleted,
	}
	res, err := operations.ResendByFilter(ctx, s.UoW, s.Repo, f, in.Body.ConfirmCount, in.Body.Limit, opts, ec)
	if err != nil {
		return nil, err
	}
	return &apicommon.Out[RequeueByFilterResponse]{Body: RequeueByFilterResponse{
		Requeued: res.Requeued, Redriven: res.Redriven,
		SkippedInFlight: res.SkippedInFlight, SkippedCompleted: res.SkippedCompleted,
		Truncated: res.Remaining > 0, Remaining: res.Remaining, Window: windowResponse(win),
	}}, nil
}

// cancel overrides a FAILED job to CANCELLED. Gated on dispatch-job:cancel;
// per-resource client scope is enforced inside the use case.
// Returns the updated job so the SPA can refresh its row without a second
// GET.
func (s *State) cancel(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[DispatchJobResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, auth.DispatchJobCancelPerm); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.CancelDispatchJob(s.Repo), operations.CancelCommand{ID: in.ID}, ec); err != nil {
		return nil, transitionConflict(err)
	}
	return s.reload(ctx, in.ID)
}

// complete overrides a FAILED job to COMPLETED (delivered out of band).
// Same gating and reload shape as cancel.
func (s *State) complete(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[DispatchJobResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, auth.DispatchJobCompletePerm); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	if _, err := usecaseop.Run(ctx, s.UoW, operations.CompleteDispatchJob(s.Repo), operations.CompleteCommand{ID: in.ID}, ec); err != nil {
		return nil, transitionConflict(err)
	}
	return s.reload(ctx, in.ID)
}

// EditPayloadRequest is the body of PUT /dispatch-jobs/{id}/payload.
type EditPayloadRequest struct {
	Payload            string  `json:"payload" minLength:"1" doc:"The new payload text (UTF-8, at most 256 KiB; valid JSON when the content type is JSON)"`
	PayloadContentType *string `json:"payloadContentType,omitempty" doc:"New media type; omit to keep the job's"`
}

type editPayloadInput struct {
	ID   string `path:"id"`
	Body EditPayloadRequest
}

// editPayload rewrites the payload of a PENDING or FAILED job so an operator
// can fix it and then requeue. Gated on dispatch-job:edit-payload;
// per-resource client scope and the status rule are in the operation. Status
// and attempts are not touched.
func (s *State) editPayload(ctx context.Context, in *editPayloadInput) (*apicommon.Out[DispatchJobResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, auth.DispatchJobEditPayloadPerm); err != nil {
		return nil, err
	}
	ec := auth.NewExecutionContext(ctx)
	cmd := operations.EditPayloadCommand{ID: in.ID, Payload: in.Body.Payload, PayloadContentType: in.Body.PayloadContentType}
	if _, err := usecaseop.Run(ctx, s.UoW, operations.EditDispatchJobPayload(s.Repo), cmd, ec); err != nil {
		return nil, transitionConflict(err)
	}
	return s.reload(ctx, in.ID)
}

// transitionConflict turns the SQL status guard's refusal (the job left the
// allowed status between the operation's read and its write) into a 409, the
// same answer the operation gives when it sees the wrong status itself. Any
// other error passes through.
func transitionConflict(err error) error {
	if errors.Is(err, dispatchjob.ErrTransitionRefused) {
		return usecase.Conflict("STATUS_CHANGED", "the job changed status while the action ran; reload it and try again")
	}
	return err
}

// reload re-fetches a job so cancel/complete can return the updated
// DispatchJobResponse.
func (s *State) reload(ctx context.Context, id string) (*apicommon.Out[DispatchJobResponse], error) {
	j, err := s.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if j == nil {
		return nil, httperror.NotFound("DispatchJob", id)
	}
	return &apicommon.Out[DispatchJobResponse]{Body: fromEntity(j)}, nil
}

type filterOptionsInput struct {
	Since string `query:"since" doc:"RFC3339; start of the created_at window the values are read from (default: 24 hours before until/now)"`
	Until string `query:"until" doc:"RFC3339; end of the window (default: now)"`
}

func (s *State) filterOptions(ctx context.Context, in *filterOptionsInput) (*apicommon.Out[DispatchJobFilterOptionsResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, viewPerm); err != nil {
		return nil, err
	}
	f, err := (&DispatchJobFilter{Since: in.Since, Until: in.Until}).toParams(true)
	if err != nil {
		return nil, err
	}
	f, win, err := s.window(f)
	if err != nil {
		return nil, err
	}
	// Scoped as the list is: a client user sees the values of platform jobs
	// and its own tenants' jobs only.
	scope := operations.VisibleTo(ac, dispatchjob.FilterParams{}).AccessibleClientIDs
	now := time.Now()
	key := filterOptionsKey(scope, win.From, win.To)
	if cached, ok := s.foCache.get(key, now); ok {
		return &apicommon.Out[DispatchJobFilterOptionsResponse]{Body: cached}, nil
	}
	failed := false
	q := func(col string) []string {
		out, err := s.Repo.DistinctValues(ctx, col, 200, scope, f.Since, f.Until)
		if err != nil {
			failed = true
		}
		return out
	}
	// Event types come from the jobs of the window, not from the event-type catalogue:
	// an event may be emitted without its type being registered, and a direct job's
	// code is not an event type at all, so a catalogue-backed list would miss values
	// the list can filter on.
	codes := q("code")
	resp := DispatchJobFilterOptionsResponse{
		Statuses:        q("status"),
		Codes:           codes,
		EventTypes:      codes,
		ClientIDs:       q("client_id"),
		DispatchPoolIDs: q("dispatch_pool_id"),
		SubscriptionIDs: q("subscription_id"),
		Kinds:           q("kind"),
		Window:          windowResponse(win),
	}
	if !failed { // never keep a partial answer
		s.foCache.put(key, now, resp)
	}
	return &apicommon.Out[DispatchJobFilterOptionsResponse]{Body: resp}, nil
}
