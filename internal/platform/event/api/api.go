// Package api wires the event HTTP endpoints for both the admin
// (frontend list/detail) and SDK (batch ingest) consumers.
//
// Routes:
//
//   POST /api/events/batch        — SDK consumer outbox ingest (infrastructure path)
//   GET  /api/events              — list with filters (admin)
//   GET  /api/events/{id}         — detail (admin)
//   GET  /api/events/list-raw     — raw JSONB rows (admin, view-raw permission)
//   GET  /api/events/filter-options — distinct types/sources for the UI's filter dropdowns
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo *event.Repository
}

// RegisterRoutes mounts the event endpoints.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/events", func(r chi.Router) {
		r.Post("/batch", s.batchIngest)
		r.Get("/filter-options", s.filterOptions)
		r.Get("/list-raw", s.listRaw)
		r.Get("/", s.list)
		r.Get("/{id}", s.getByID)
	})
}

// ── batch ingest (SDK) ──────────────────────────────────────────────────

type batchItem struct {
	ID              string          `json:"id,omitempty"`
	Type            string          `json:"type"`
	Source          string          `json:"source"`
	Subject         string          `json:"subject"`
	Data            json.RawMessage `json:"data"`
	DeduplicationID string          `json:"deduplicationId,omitempty"`
	ClientID        *string         `json:"clientId,omitempty"`
	CorrelationID   *string         `json:"correlationId,omitempty"`
	CausationID     *string         `json:"causationId,omitempty"`
}

type batchRequest struct {
	Items []batchItem `json:"items"`
}

type batchResponse struct {
	Accepted int `json:"accepted"`
}

func (s *State) batchIngest(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:messaging:batch:events-write"); err != nil {
		httperror.Write(w, err)
		return
	}
	var body batchRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	if len(body.Items) > 1000 {
		httperror.Write(w, httperror.BadRequest("BATCH_TOO_LARGE", "max 1000 items per batch"))
		return
	}
	events := make([]event.Event, 0, len(body.Items))
	for _, it := range body.Items {
		ev := event.New(it.Type, it.Source, it.Subject, it.Data)
		if it.ID != "" {
			ev.ID = it.ID
		}
		if it.DeduplicationID != "" {
			ev.DeduplicationID = it.DeduplicationID
		}
		ev.ClientID = it.ClientID
		ev.CorrelationID = it.CorrelationID
		ev.CausationID = it.CausationID
		events = append(events, *ev)
	}
	n, err := s.Repo.InsertBatch(r.Context(), events)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "insert batch failed", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(batchResponse{Accepted: n})
}

// ── list / detail (admin) ───────────────────────────────────────────────

func (s *State) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	rows, err := s.Repo.FindWithFilters(r.Context(), parseFilters(r))
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_with_filters failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) listRaw(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view-raw"); err != nil {
		httperror.Write(w, err)
		return
	}
	rows, err := s.Repo.FindWithFilters(r.Context(), parseFilters(r))
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_raw failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) getByID(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:messaging:event:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	ev, err := s.Repo.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if ev == nil {
		httperror.Write(w, httperror.NotFound("Event", id))
		return
	}
	writeJSON(w, http.StatusOK, ev)
}

// ── filter-options (admin) ──────────────────────────────────────────────

// filterOptions returns distinct types + sources + client_ids for the
// frontend's filter dropdowns. The Rust impl returns up to 200 of each
// — same cap here.
func (s *State) filterOptions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := func(col string) []string {
		out, _ := s.Repo.DistinctValues(ctx, col, 200)
		return out
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"types":     q("type"),
		"sources":   q("source"),
		"clientIds": q("client_id"),
	})
}

// ── helpers ──────────────────────────────────────────────────────────────

func parseFilters(r *http.Request) event.FilterParams {
	q := r.URL.Query()
	str := func(k string) *string {
		if v := q.Get(k); v != "" {
			return &v
		}
		return nil
	}
	ts := func(k string) *time.Time {
		if v := q.Get(k); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return &t
			}
		}
		return nil
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	offset, _ := strconv.Atoi(q.Get("offset"))
	return event.FilterParams{
		Type:          str("type"),
		Source:        str("source"),
		Subject:       str("subject"),
		ClientID:      str("clientId"),
		PrincipalID:   str("principalId"),
		CorrelationID: str("correlationId"),
		Since:         ts("since"),
		Until:         ts("until"),
		Limit:         limit,
		Offset:        offset,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
