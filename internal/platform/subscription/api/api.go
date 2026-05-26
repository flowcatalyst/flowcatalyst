// Package api wires HTTP routes for the subscription subdomain.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription/operations"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles the dependencies.
type State struct {
	Repo     *subscription.Repository
	CreateUC *operations.CreateUseCase
	UpdateUC *operations.UpdateUseCase
	DeleteUC *operations.DeleteUseCase
	PauseUC  *operations.PauseUseCase
	ResumeUC *operations.ResumeUseCase
}

// RegisterRoutes mounts subscription endpoints.
//
// TODO(wave-3b): port sync.go (bulk SDK upsert). The Rust SDK pushes a
// declarative set of subscriptions; the platform diffs against current
// state and emits SubscriptionsSynced.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/subscriptions", func(r chi.Router) {
		r.Get("/", s.list)
		r.Post("/", s.create)
		r.Get("/{id}", s.getByID)
		r.Put("/{id}", s.update)
		r.Delete("/{id}", s.delete)
		r.Post("/{id}/pause", s.pause)
		r.Post("/{id}/resume", s.resume)
	})
}

func (s *State) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadSubscriptions(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	q := r.URL.Query()
	var status, clientID *string
	if v := q.Get("status"); v != "" {
		status = &v
	}
	if v := q.Get("clientId"); v != "" {
		clientID = &v
	}
	rows, err := s.Repo.FindWithFilters(r.Context(), status, clientID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_with_filters failed", err))
		return
	}
	out := rows[:0]
	for _, sub := range rows {
		if sub.ClientID == nil || ac.CanAccessClient(*sub.ClientID) {
			out = append(out, sub)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": out})
}

func (s *State) getByID(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadSubscriptions(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	sub, err := s.Repo.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if sub == nil {
		httperror.Write(w, httperror.NotFound("Subscription", id))
		return
	}
	if sub.ClientID != nil && !ac.CanAccessClient(*sub.ClientID) {
		httperror.Write(w, httperror.Forbidden("No access to this subscription"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(sub)
}

func (s *State) create(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteSubscriptions(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	var body operations.CreateCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	if body.ClientID != nil && !ac.CanAccessClient(*body.ClientID) {
		httperror.Write(w, httperror.Forbidden("No access to client: "+*body.ClientID))
		return
	}
	if body.ClientID == nil && !ac.IsAnchor() {
		httperror.Write(w, httperror.Forbidden("Only anchor users can create anchor-level subscriptions"))
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
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.SubscriptionID})
}

func (s *State) update(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteSubscriptions(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	var body operations.UpdateCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	body.ID = id
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.UpdateUC, body, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) delete(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanDeleteSubscriptions(ac); err != nil {
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

func (s *State) pause(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteSubscriptions(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.PauseUC, operations.PauseCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) resume(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteSubscriptions(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.ResumeUC, operations.ResumeCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
