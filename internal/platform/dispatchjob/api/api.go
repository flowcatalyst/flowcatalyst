// Package api wires the dispatch-job read-only HTTP endpoints. All ops
// against msg_dispatch_jobs are scheduler-internal (poller + dispatcher
// in internal/platform/scheduler); these endpoints expose the data for
// admin / debug views.
//
// Routes:
//
//   GET /api/dispatch-jobs                — paginated list with filters
//   GET /api/dispatch-jobs/list-raw       — same with view-raw permission
//   GET /api/dispatch-jobs/filter-options — distinct facet values
//   GET /api/dispatch-jobs/{id}           — detail
//   GET /api/dispatch-jobs/{id}/raw       — raw row (view-raw permission)
//   GET /api/dispatch-jobs/{id}/attempts  — per-attempt history
//   GET /api/dispatch-jobs/event/{eventId} — jobs spawned by a single event
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo *dispatchjob.Repository
}

// RegisterRoutes mounts the endpoints.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/dispatch-jobs", func(r chi.Router) {
		r.Get("/", s.list)
		r.Get("/list-raw", s.listRaw)
		r.Get("/filter-options", s.filterOptions)
		r.Get("/event/{eventId}", s.byEvent)
		r.Get("/{id}", s.getByID)
		r.Get("/{id}/raw", s.getRaw)
		r.Get("/{id}/attempts", s.attempts)
	})
}

// ── list / detail ────────────────────────────────────────────────────────

func (s *State) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:messaging:dispatch-job:view"); err != nil {
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
	if err := auth.CanWritePermission(ac, "platform:messaging:dispatch-job:view-raw"); err != nil {
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
	if err := auth.CanWritePermission(ac, "platform:messaging:dispatch-job:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	j, err := s.Repo.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if j == nil {
		httperror.Write(w, httperror.NotFound("DispatchJob", id))
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *State) getRaw(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:messaging:dispatch-job:view-raw"); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	j, err := s.Repo.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_raw failed", err))
		return
	}
	if j == nil {
		httperror.Write(w, httperror.NotFound("DispatchJob", id))
		return
	}
	writeJSON(w, http.StatusOK, j)
}

func (s *State) attempts(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:messaging:dispatch-job:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	rows, err := s.Repo.AttemptsByJob(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "attempts failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) byEvent(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:messaging:dispatch-job:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	eventID := chi.URLParam(r, "eventId")
	rows, err := s.Repo.FindByEventID(r.Context(), eventID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "by_event failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) filterOptions(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:messaging:dispatch-job:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	q := func(col string) []string {
		out, _ := s.Repo.DistinctValues(r.Context(), col, 200)
		return out
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"statuses":         q("status"),
		"codes":            q("code"),
		"clientIds":        q("client_id"),
		"dispatchPoolIds":  q("dispatch_pool_id"),
		"subscriptionIds":  q("subscription_id"),
		"kinds":            q("kind"),
	})
}

// ── helpers ──────────────────────────────────────────────────────────────

func parseFilters(r *http.Request) dispatchjob.FilterParams {
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
	return dispatchjob.FilterParams{
		Status:         str("status"),
		ClientID:       str("clientId"),
		DispatchPoolID: str("dispatchPoolId"),
		SubscriptionID: str("subscriptionId"),
		Code:           str("code"),
		Since:          ts("since"),
		Until:          ts("until"),
		Limit:          limit,
		Offset:         offset,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
