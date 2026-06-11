// Package api wires the event HTTP endpoints via huma.
package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apiroute"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo *event.Repository
}

const tag = "events"

// Register mounts the event endpoints.
func Register(api huma.API, s *State) {
	g := apiroute.New(api, tag)
	apiroute.Post(g, "batchIngestEvents", "/api/events/batch", "Ingest a batch of events (SDK)", http.StatusCreated, s.batchIngest)
	apiroute.Get(g, "eventFilterOptions", "/api/events/filter-options", "Distinct event types/sources/clients for filter UI", s.filterOptions)
	apiroute.Get(g, "listEventsRaw", "/api/events/list-raw", "List events with raw JSONB rows", s.listRaw)
	// SDK-compatibility alias: the Laravel/Rust client addresses the raw
	// collection as /api/events/raw; Go's canonical path is /list-raw. Same
	// handler.
	apiroute.Get(g, "listEventsRawAlias", "/api/events/raw", "List events raw (SDK alias of /list-raw)", s.listRaw)
	apiroute.Get(g, "listEvents", "/api/events", "List events with filters", s.list)
	apiroute.Get(g, "getEvent", "/api/events/{id}", "Get an event by id", s.getByID)

	// BFF tier — cookie-auth, SPA-facing. /bff/events mirrors the regular
	// list/detail handlers under cookie-auth. Mirrors Rust's events_router.
	registerBFF(api, s, "/bff/events", "Bff", "bff-events")

	// /bff/debug/events is a SEPARATE raw-event view (write-side
	// msg_events incl. context_data). The SPA's RawEventListPage binds
	// field="eventType"/field="deduplicationId" — a different item shape
	// from the regular list — so it gets its own handler returning a bare
	// array of RawEventResponse. Mirrors Rust's shared/debug_api.rs.
	gd := apiroute.New(api, "bff-debug-events")
	apiroute.Get(gd, "listDebugEvents", "/bff/debug/events", "List raw events (debug view of msg_events)", s.listDebugRaw)
}

// registerBFF mirrors Register under a different base path. Used so the
// SPA hits /bff/events with cookie-auth while SDK callers use /api/events
// with bearer-auth — the handlers are the same; the auth layer differs.
func registerBFF(api huma.API, s *State, base, opPrefix, tag string) {
	g := apiroute.New(api, tag)
	apiroute.Post(g, "batchIngestEvents"+opPrefix, base+"/batch", "Ingest a batch of events (SPA fan-out)", http.StatusCreated, s.batchIngest)
	apiroute.Get(g, "eventFilterOptions"+opPrefix, base+"/filter-options", "Distinct event types/sources/clients for filter UI", s.filterOptions)
	apiroute.Get(g, "listEventsRaw"+opPrefix, base+"/list-raw", "List events with raw JSONB rows", s.listRaw)
	apiroute.Get(g, "listEvents"+opPrefix, base, "List events with filters", s.list)
	apiroute.Get(g, "getEvent"+opPrefix, base+"/{id}", "Get an event by id", s.getByID)
}

// ── batch ingest ─────────────────────────────────────────────────────────

func (s *State) batchIngest(ctx context.Context, in *apicommon.In[BatchRequest]) (*apicommon.Out[BatchResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:batch:events-write"); err != nil {
		return nil, err
	}
	if len(in.Body.Items) > 1000 {
		return nil, httperror.BadRequest("BATCH_TOO_LARGE", "max 1000 items per batch")
	}
	events := make([]event.Event, 0, len(in.Body.Items))
	for _, it := range in.Body.Items {
		ev := event.New(it.Type, it.Source, it.Subject, it.Data)
		if it.ID != "" {
			ev.ID = it.ID
		}
		if it.SpecVersion != "" {
			ev.SpecVersion = it.SpecVersion
		}
		if it.DeduplicationID != "" {
			ev.DeduplicationID = it.DeduplicationID
		}
		ev.ClientID = it.ClientID
		ev.MessageGroup = it.MessageGroup
		ev.CorrelationID = it.CorrelationID
		ev.CausationID = it.CausationID
		events = append(events, *ev)
	}
	if _, err := s.Repo.InsertBatch(ctx, events); err != nil {
		return nil, usecase.Internal("REPO", "insert batch failed", err)
	}
	// Per-item result list — 1:1 with the outbox/SDK contract. Insert is
	// all-or-nothing here, so every persisted event reports SUCCESS.
	results := apicommon.MapSlice(events, func(e *event.Event) BatchResultItem {
		return BatchResultItem{ID: e.ID, Status: "SUCCESS"}
	})
	return &apicommon.Out[BatchResponse]{Body: BatchResponse{Results: results}}, nil
}

// ── list / detail ────────────────────────────────────────────────────────

type listInput struct {
	Type          string `query:"type"`
	Source        string `query:"source"`
	Subject       string `query:"subject"`
	ClientID      string `query:"clientId"`
	PrincipalID   string `query:"principalId"`
	CorrelationID string `query:"correlationId"`
	Since         string `query:"since" doc:"RFC3339 timestamp"`
	Until         string `query:"until" doc:"RFC3339 timestamp"`
	Limit         int    `query:"limit"`
	Offset        int    `query:"offset"`

	// SPA params (events.ts:50-61). `size` caps the row count; the
	// plural params are comma-separated multi-filters.
	Size         int    `query:"size" doc:"Max rows (default 50, max 1000)"`
	ClientIDs    string `query:"clientIds" doc:"CSV of client ids"`
	Applications string `query:"applications" doc:"CSV of application codes"`
	Subdomains   string `query:"subdomains" doc:"CSV of subdomains"`
	Aggregates   string `query:"aggregates" doc:"CSV of aggregates"`
	Types        string `query:"types" doc:"CSV of event types"`
}

// splitCSV mirrors Rust's split_csv (event/api.rs): trim, drop empties.
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

func (in *listInput) toFilters() event.FilterParams {
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
	return event.FilterParams{
		Type:          apicommon.OptStr(in.Type),
		Source:        apicommon.OptStr(in.Source),
		Subject:       apicommon.OptStr(in.Subject),
		ClientID:      apicommon.OptStr(in.ClientID),
		PrincipalID:   apicommon.OptStr(in.PrincipalID),
		CorrelationID: apicommon.OptStr(in.CorrelationID),
		Since:         ts(in.Since),
		Until:         ts(in.Until),
		Limit:         limit,
		Offset:        in.Offset,
		ClientIDs:     splitCSV(in.ClientIDs),
		Applications:  splitCSV(in.Applications),
		Subdomains:    splitCSV(in.Subdomains),
		Aggregates:    splitCSV(in.Aggregates),
		Types:         splitCSV(in.Types),
	}
}

// The list endpoints' Body is a bare JSON array — the SPA's EventListPage
// binds the returned array directly to its DataTable, so {items:[...]} would
// render zero rows. Mirrors Rust's list_events returning Vec<EventRead>.

// scopeFilters applies SQL-side tenant scoping (anchor sees all → no
// scoping). Without it a non-anchor holding event:view could read any
// tenant's events by passing arbitrary clientId/clientIds filters — the
// caller-controlled filters must only narrow within the principal's own
// tenants. Same pattern as scheduledjob's list.
func scopeFilters(ac *auth.AuthContext, f event.FilterParams) event.FilterParams {
	if !ac.IsAnchor() {
		clients := ac.Clients
		f.AccessibleClientIDs = &clients
	}
	return f
}

func (s *State) list(ctx context.Context, in *listInput) (*apicommon.Out[[]EventRead], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view"); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, scopeFilters(ac, in.toFilters()))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	out := apicommon.MapSlice(rows, readFromEntity)
	return &apicommon.Out[[]EventRead]{Body: out}, nil
}

func (s *State) listRaw(ctx context.Context, in *listInput) (*apicommon.Out[[]EventRead], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view-raw"); err != nil {
		return nil, err
	}
	rows, err := s.Repo.FindWithFilters(ctx, scopeFilters(ac, in.toFilters()))
	if err != nil {
		return nil, usecase.Internal("REPO", "find_raw failed", err)
	}
	out := apicommon.MapSlice(rows, readFromEntity)
	return &apicommon.Out[[]EventRead]{Body: out}, nil
}

// ── debug raw events ─────────────────────────────────────────────────────

type rawListInput struct {
	Size int `query:"size" doc:"Max rows (default 50, max 1000)"`
}

// listDebugRaw's Body is a bare array of RawEventResponse — the SPA's
// RawEventListPage binds it directly and its Type column reads
// field="eventType".
func (s *State) listDebugRaw(ctx context.Context, in *rawListInput) (*apicommon.Out[[]RawEventResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view-raw"); err != nil {
		return nil, err
	}
	limit := in.Size
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.Repo.FindRecentRaw(ctx, limit)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_recent_raw failed", err)
	}
	out := apicommon.MapSlice(rows, rawFromEntity)
	return &apicommon.Out[[]RawEventResponse]{Body: out}, nil
}

func (s *State) getByID(ctx context.Context, in *apicommon.IDInput) (*apicommon.Out[EventResponse], error) {
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view"); err != nil {
		return nil, err
	}
	ev, err := s.Repo.FindByID(ctx, in.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if ev == nil {
		return nil, httperror.NotFound("Event", in.ID)
	}
	// Per-resource tenant scoping, matching the list semantics: a
	// client-scoped event is only visible to principals with access to that
	// client; platform-scoped events (nil ClientID) stay visible to any
	// holder of event:view.
	if ev.ClientID != nil && !ac.CanAccessClient(*ev.ClientID) {
		return nil, httperror.Forbidden("No access to this event")
	}
	return &apicommon.Out[EventResponse]{Body: fromEntity(ev)}, nil
}

// ── filter-options ───────────────────────────────────────────────────────

func (s *State) filterOptions(ctx context.Context, _ *apicommon.Empty) (*apicommon.Out[EventFilterOptionsResponse], error) {
	// Same gate as list: the option values (application codes, subdomains,
	// event types) are derived from event rows — without this check the
	// endpoint leaked them to unauthenticated callers.
	ac := auth.FromContext(ctx)
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view"); err != nil {
		return nil, err
	}
	q := func(col string) []EventFilterOption {
		out, _ := s.Repo.DistinctValues(ctx, col, 200)
		return toFilterOptions(out)
	}
	return &apicommon.Out[EventFilterOptionsResponse]{Body: EventFilterOptionsResponse{
		Applications: q("application"),
		Subdomains:   q("subdomain"),
		EventTypes:   q("type"),
	}}, nil
}
