// Package api wires the HTTP routes for the event_type subdomain.
// Kept in a subpackage to avoid the import cycle that would form if it
// lived in the parent package (operations imports eventtype, api would
// need to import both).
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles the dependencies the HTTP handlers reach into. Wired
// once at startup in cmd/fc-platform-server; per-request handlers
// receive it via closure.
type State struct {
	Repo        *eventtype.Repository
	CreateUC    *operations.CreateUseCase
	UpdateUC    *operations.UpdateUseCase
	DeleteUC    *operations.DeleteUseCase
	AddSchemaUC *operations.AddSchemaUseCase
}

// RegisterRoutes mounts the event-type endpoints onto the supplied router.
//
// Routes match the Rust event_type/api.rs exactly (path, method, status code).
// JSON shapes match: CreatedResponse on POST, full EventType on GET, 204 on PUT/DELETE.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/event-types", func(r chi.Router) {
		r.Post("/", s.create)
		r.Get("/", s.list)
		r.Get("/{id}", s.getByID)
		r.Get("/by-code/{code}", s.getByCode)
		r.Put("/{id}", s.update)
		r.Delete("/{id}", s.delete)
		r.Post("/{id}/schemas", s.addSchema)
	})
}

// POST /api/event-types
func (s *State) create(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteEventTypes(ac); err != nil {
		httperror.Write(w, err)
		return
	}

	var body operations.CreateCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	// Resource-level access: anchor for anchor-level, client access for client-scoped.
	if body.ClientID != nil {
		if !ac.CanAccessClient(*body.ClientID) {
			httperror.Write(w, httperror.Forbidden("No access to client: "+*body.ClientID))
			return
		}
	} else if !ac.IsAnchor() {
		httperror.Write(w, httperror.Forbidden("Only anchor users can create anchor-level event types"))
		return
	}

	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.CreateUC, body, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.EventTypeID})
}

// GET /api/event-types/{id}
func (s *State) getByID(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadEventTypes(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	et, err := s.Repo.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if et == nil {
		httperror.Write(w, httperror.NotFound("EventType", id))
		return
	}
	if et.ClientID != nil && !ac.CanAccessClient(*et.ClientID) {
		httperror.Write(w, httperror.Forbidden("No access to this event type"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(et)
}

// GET /api/event-types/by-code/{code}
func (s *State) getByCode(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadEventTypes(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	code := chi.URLParam(r, "code")
	et, err := s.Repo.FindByCode(r.Context(), code)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_code failed", err))
		return
	}
	if et == nil {
		httperror.Write(w, httperror.NotFound("EventType", code))
		return
	}
	if et.ClientID != nil && !ac.CanAccessClient(*et.ClientID) {
		httperror.Write(w, httperror.Forbidden("No access to this event type"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(et)
}

// GET /api/event-types
func (s *State) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadEventTypes(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	q := r.URL.Query()
	var application, clientID, status, subdomain, aggregate *string
	if v := q.Get("application"); v != "" {
		application = &v
	}
	if v := q.Get("clientId"); v != "" {
		clientID = &v
	}
	if v := q.Get("status"); v != "" {
		status = &v
	}
	if v := q.Get("subdomain"); v != "" {
		subdomain = &v
	}
	if v := q.Get("aggregate"); v != "" {
		aggregate = &v
	}
	// Default to CURRENT when no filters supplied (matches Rust find_active default).
	if application == nil && clientID == nil && status == nil && subdomain == nil && aggregate == nil {
		def := "CURRENT"
		status = &def
	}

	rows, err := s.Repo.FindWithFilters(r.Context(), application, clientID, status, subdomain, aggregate)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_with_filters failed", err))
		return
	}
	// Filter by client access.
	out := rows[:0]
	for _, et := range rows {
		if et.ClientID == nil || ac.CanAccessClient(*et.ClientID) {
			out = append(out, et)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": out})
}

// PUT /api/event-types/{id}
func (s *State) update(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteEventTypes(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")

	var body operations.UpdateCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	body.ID = id // path is authoritative

	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.UpdateUC, body, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// DELETE /api/event-types/{id}
func (s *State) delete(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanDeleteEventTypes(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")

	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.DeleteUC, operations.DeleteCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/event-types/{id}/schemas
func (s *State) addSchema(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteEventTypes(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	var body operations.AddSchemaCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	body.EventTypeID = id // path is authoritative

	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.AddSchemaUC, body, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.EventTypeID})
}
