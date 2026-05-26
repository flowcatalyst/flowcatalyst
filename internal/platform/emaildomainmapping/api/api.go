// Package api wires HTTP routes for email_domain_mapping.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles the dependencies.
type State struct {
	Repo     *emaildomainmapping.Repository
	CreateUC *operations.CreateUseCase
	UpdateUC *operations.UpdateUseCase
	DeleteUC *operations.DeleteUseCase
}

// RegisterRoutes mounts the endpoints. Anchor-only.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/email-domain-mappings", func(r chi.Router) {
		r.Get("/", s.list)
		r.Post("/", s.create)
		r.Get("/{id}", s.getByID)
		r.Put("/{id}", s.update)
		r.Delete("/{id}", s.delete)
	})
}

func (s *State) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	rows, err := s.Repo.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_all failed", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": rows})
}

func (s *State) getByID(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	e, err := s.Repo.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if e == nil {
		httperror.Write(w, httperror.NotFound("EmailDomainMapping", id))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(e)
}

func (s *State) create(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	var body operations.CreateCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
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
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.MappingID})
}

func (s *State) update(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
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
	if err := auth.RequireAnchor(ac); err != nil {
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

// lookup resolves an email domain to its mapping (which IDP, which scope).
// Used by the frontend's login page + the OIDC bridge's check-domain
// fallback. Public-readable so unauthenticated clients can probe
// available login methods.
func (s *State) lookup(w http.ResponseWriter, r *http.Request) {
	domain := r.URL.Query().Get("domain")
	if domain == "" {
		httperror.Write(w, httperror.BadRequest("DOMAIN_REQUIRED", "domain query param is required"))
		return
	}
	m, err := s.Repo.FindByEmailDomain(r.Context(), domain)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "lookup failed", err))
		return
	}
	if m == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"found": false})
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
