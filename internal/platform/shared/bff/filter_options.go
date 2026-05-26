package bff

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// FilterOption is the canonical {value,label} pair every filter dropdown
// expects. Matches `fc_platform::shared::filter_options_api::FilterOption`.
type FilterOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FilterOptionsState bundles the repos the BFF filter endpoints reach
// into. Held by reference so future endpoints can grow the state
// without re-threading every caller.
type FilterOptionsState struct {
	Clients    *client.Repository
	EventTypes *eventtype.Repository
}

// RegisterFilterOptions mounts the most-called BFF dropdown endpoints:
//
//	GET /bff/filter-options/clients          — every page with a client filter
//	GET /bff/event-types/filters/applications — event-types page filter
//
// Both pull all rows from the underlying repo (matches Rust's
// `find_active` + in-memory filter pattern — cheap given low row
// counts; revisit if these endpoints ever become hot). Output is
// alphabetically sorted on label so the dropdown is stable across
// reloads.
//
// Other BFF filter endpoints (subdomains, aggregates, dispatch-jobs,
// events) are not yet ported — they follow the same shape. See
// HANDOFF.md for the prioritised list.
func RegisterFilterOptions(r chi.Router, s *FilterOptionsState) {
	r.Get("/bff/filter-options/clients", s.clientOptions)
	r.Get("/bff/event-types/filters/applications", s.eventTypeApplications)
}

// GET /bff/filter-options/clients
//
// Rust shape: `{"clients": [{"value": "<id>", "label": "<name>"}, ...]}`
// Filtered by the caller's auth context — anchor sees all, non-anchor
// sees only clients in their access set.
func (s *FilterOptionsState) clientOptions(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	rows, err := s.Clients.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list clients failed", err))
		return
	}
	out := make([]FilterOption, 0, len(rows))
	for _, c := range rows {
		if c.Status != client.StatusActive {
			continue
		}
		if !ac.IsAnchor() && !ac.CanAccessClient(c.ID) {
			continue
		}
		out = append(out, FilterOption{Value: c.ID, Label: c.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	writeJSON(w, http.StatusOK, map[string]any{"clients": out})
}

// GET /bff/event-types/filters/applications
//
// Rust shape: `{"applications": [{"value": "<app>", "label": "<app>"}, ...]}`
// Extracted from the `application:subdomain:aggregate:event` event-type
// codes by splitting on `:` and taking the first segment.
func (s *FilterOptionsState) eventTypeApplications(w http.ResponseWriter, r *http.Request) {
	// Empty filters → all event types. Status is intentionally
	// unconstrained: the Rust handler uses `find_active_shallow` but
	// the goal is "give me every application that has at least one
	// event type," for which CURRENT+ARCHIVED is fine.
	rows, err := s.EventTypes.FindWithFilters(r.Context(), nil, nil, nil, nil, nil)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list event types failed", err))
		return
	}
	seen := map[string]struct{}{}
	for _, et := range rows {
		// Code format: `application:subdomain:aggregate:event`. The
		// `Application` field on the entity already holds the parsed
		// app segment; fall back to splitting the code if absent so
		// rows missing the denormalised field still contribute.
		app := et.Application
		if app == "" {
			if parts := strings.SplitN(et.Code, ":", 2); len(parts) > 0 {
				app = parts[0]
			}
		}
		if app == "" {
			continue
		}
		seen[app] = struct{}{}
	}
	out := make([]FilterOption, 0, len(seen))
	for app := range seen {
		out = append(out, FilterOption{Value: app, Label: app})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	writeJSON(w, http.StatusOK, map[string]any{"applications": out})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
