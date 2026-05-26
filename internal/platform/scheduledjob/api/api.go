// Package api wires HTTP routes for scheduled_job.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo       *scheduledjob.Repository
	CreateUC   *operations.CreateUseCase
	UpdateUC   *operations.UpdateUseCase
	PauseUC    *operations.PauseUseCase
	ResumeUC   *operations.ResumeUseCase
	ArchiveUC  *operations.ArchiveUseCase
	DeleteUC   *operations.DeleteUseCase
	FireNowUC  *operations.FireNowUseCase
}

// RegisterRoutes mounts the scheduled-job endpoints.
// TODO(wave-3e-followup): sync (bulk SDK upsert).
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/scheduled-jobs", func(r chi.Router) {
		r.Get("/", s.list)
		r.Post("/", s.create)
		r.Get("/{id}", s.getByID)
		r.Put("/{id}", s.update)
		r.Post("/{id}/pause", s.pause)
		r.Post("/{id}/resume", s.resume)
		r.Post("/{id}/archive", s.archive)
		r.Post("/{id}/fire-now", s.fireNow)
		r.Delete("/{id}", s.delete)
	})
}

func (s *State) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadScheduledJobs(ac); err != nil {
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
	for _, j := range rows {
		if j.ClientID == nil || ac.CanAccessClient(*j.ClientID) {
			out = append(out, j)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": out})
}

func (s *State) getByID(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadScheduledJobs(ac); err != nil {
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
		httperror.Write(w, httperror.NotFound("ScheduledJob", id))
		return
	}
	if j.ClientID != nil && !ac.CanAccessClient(*j.ClientID) {
		httperror.Write(w, httperror.Forbidden("No access to this scheduled job"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(j)
}

func (s *State) create(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
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
		httperror.Write(w, httperror.Forbidden("Only anchor users can create platform-scoped jobs"))
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
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.ScheduledJobID})
}

func (s *State) update(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
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

func runTransition[C any, E usecase.DomainEvent](w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, uc usecase.UseCase[C, E], cmd C) {
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), uc, cmd, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) pause(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	runTransition(w, r, ac, s.PauseUC, operations.PauseCommand{ID: chi.URLParam(r, "id")})
}

func (s *State) resume(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	runTransition(w, r, ac, s.ResumeUC, operations.ResumeCommand{ID: chi.URLParam(r, "id")})
}

func (s *State) archive(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	runTransition(w, r, ac, s.ArchiveUC, operations.ArchiveCommand{ID: chi.URLParam(r, "id")})
}

func (s *State) fireNow(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanFireScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.FireNowUC, operations.FireNowCommand{ID: id}, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"scheduledJobId": event.ScheduledJobID,
		"instanceId":     event.InstanceID,
	})
}

func (s *State) delete(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanDeleteScheduledJobs(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	runTransition(w, r, ac, s.DeleteUC, operations.DeleteCommand{ID: chi.URLParam(r, "id")})
}
