// Package api wires the audit log read-only HTTP endpoints.
//
// Routes:
//
//   GET /api/audit-logs                       — paginated list with filters
//   GET /api/audit-logs/recent                — alias for list with default ORDER + LIMIT
//   GET /api/audit-logs/entity-types          — facet: distinct entity types
//   GET /api/audit-logs/operations            — facet: distinct operations
//   GET /api/audit-logs/application-ids       — facet: distinct application IDs
//   GET /api/audit-logs/client-ids            — facet: distinct client IDs
//   GET /api/audit-logs/{id}                  — single audit row
//   GET /api/audit-logs/entity/{entityId}     — rows for a specific entity
//   GET /api/audit-logs/principal/{principalId} — rows actor-keyed to a principal
package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo *audit.Repository
}

// RegisterRoutes mounts the audit endpoints.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/audit-logs", func(r chi.Router) {
		r.Get("/", s.list)
		r.Get("/recent", s.list)
		r.Get("/entity-types", s.entityTypes)
		r.Get("/operations", s.operations)
		r.Get("/application-ids", s.applicationIDs)
		r.Get("/client-ids", s.clientIDs)
		r.Get("/{id}", s.getByID)
		r.Get("/entity/{entityId}", s.byEntity)
		r.Get("/principal/{principalId}", s.byPrincipal)
	})
}

// ── list / detail ───────────────────────────────────────────────────────

func (s *State) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:admin:audit-log:view"); err != nil {
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

func (s *State) getByID(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:admin:audit-log:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	log, err := s.Repo.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if log == nil {
		httperror.Write(w, httperror.NotFound("AuditLog", id))
		return
	}
	writeJSON(w, http.StatusOK, log)
}

func (s *State) byEntity(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:admin:audit-log:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	entityID := chi.URLParam(r, "entityId")
	rows, err := s.Repo.FindWithFilters(r.Context(), audit.FilterParams{EntityID: &entityID, Limit: 500})
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "by_entity failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) byPrincipal(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:admin:audit-log:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	principalID := chi.URLParam(r, "principalId")
	rows, err := s.Repo.FindWithFilters(r.Context(), audit.FilterParams{PrincipalID: &principalID, Limit: 500})
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "by_principal failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

// ── facets ──────────────────────────────────────────────────────────────

func (s *State) entityTypes(w http.ResponseWriter, r *http.Request) {
	s.distinct(w, r, "entity_type")
}
func (s *State) operations(w http.ResponseWriter, r *http.Request) {
	s.distinct(w, r, "operation")
}
func (s *State) applicationIDs(w http.ResponseWriter, r *http.Request) {
	s.distinct(w, r, "application_id")
}
func (s *State) clientIDs(w http.ResponseWriter, r *http.Request) {
	s.distinct(w, r, "client_id")
}

func (s *State) distinct(w http.ResponseWriter, r *http.Request, column string) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWritePermission(ac, "platform:admin:audit-log:view"); err != nil {
		httperror.Write(w, err)
		return
	}
	out, err := s.Repo.DistinctValues(r.Context(), column, 500)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "distinct failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// ── helpers ──────────────────────────────────────────────────────────────

func parseFilters(r *http.Request) audit.FilterParams {
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
	return audit.FilterParams{
		EntityType:  str("entityType"),
		EntityID:    str("entityId"),
		PrincipalID: str("principalId"),
		ClientID:    str("clientId"),
		Since:       ts("since"),
		Until:       ts("until"),
		Limit:       limit,
		Offset:      offset,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
