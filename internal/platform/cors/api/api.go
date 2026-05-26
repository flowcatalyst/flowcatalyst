// Package api wires HTTP routes for the cors subdomain.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles the dependencies.
type State struct {
	Repo     *cors.Repository
	AddUC    *operations.AddUseCase
	DeleteUC *operations.DeleteUseCase
}

// RegisterRoutes mounts the CORS endpoints.
//
// The admin endpoints under /api/cors-origins are anchor-only; the
// platform/cors/allowed GET is public — browsers / SDKs hit it
// pre-flight to learn which origins are allowed.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/cors-origins", func(r chi.Router) {
		r.Get("/", s.list)
		r.Post("/", s.add)
		r.Delete("/{id}", s.delete)
	})
	r.Get("/api/platform/cors/allowed", s.publicAllowed)
}

func (s *State) publicAllowed(w http.ResponseWriter, r *http.Request) {
	rows, err := s.Repo.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_all failed", err))
		return
	}
	origins := make([]string, 0, len(rows))
	for _, o := range rows {
		origins = append(origins, o.Origin)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"allowedOrigins": origins})
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

func (s *State) add(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	var body operations.AddCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.AddUC, body, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.OriginID})
}

func (s *State) delete(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.DeleteUC, operations.DeleteCommand{OriginID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
