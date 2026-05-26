// Package api wires the HTTP routes for the process subdomain.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo      *process.Repository
	CreateUC  *operations.CreateUseCase
	UpdateUC  *operations.UpdateUseCase
	ArchiveUC *operations.ArchiveUseCase
	DeleteUC  *operations.DeleteUseCase
}

// RegisterRoutes mounts the process endpoints. TODO(wave-3b): sync.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/processes", func(r chi.Router) {
		r.Get("/", s.list)
		r.Post("/", s.create)
		r.Get("/{id}", s.getByID)
		r.Put("/{id}", s.update)
		r.Post("/{id}/archive", s.archive)
		r.Delete("/{id}", s.delete)
	})
}

func (s *State) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadProcesses(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	q := r.URL.Query()
	var application, subdomain, status *string
	if v := q.Get("application"); v != "" {
		application = &v
	}
	if v := q.Get("subdomain"); v != "" {
		subdomain = &v
	}
	if v := q.Get("status"); v != "" {
		status = &v
	}
	rows, err := s.Repo.FindWithFilters(r.Context(), application, subdomain, status)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_with_filters failed", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": rows})
}

func (s *State) getByID(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadProcesses(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	p, err := s.Repo.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if p == nil {
		httperror.Write(w, httperror.NotFound("Process", id))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(p)
}

func (s *State) create(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteProcesses(ac); err != nil {
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
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.ProcessID})
}

func (s *State) update(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteProcesses(ac); err != nil {
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

func (s *State) archive(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteProcesses(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.ArchiveUC, operations.ArchiveCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) delete(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanDeleteProcesses(ac); err != nil {
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
