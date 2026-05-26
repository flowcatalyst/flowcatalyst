// Package api wires the HTTP routes for the application subdomain.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo                 *application.Repository
	ClientConfigRepo     *application.ClientConfigRepo
	CreateUC             *operations.CreateUseCase
	UpdateUC             *operations.UpdateUseCase
	ActivateUC           *operations.ActivateUseCase
	DeactivateUC         *operations.DeactivateUseCase
	DeleteUC             *operations.DeleteUseCase
	AttachServiceAccount *operations.AttachServiceAccountUseCase
	EnableForClient      *operations.EnableForClientUseCase
	DisableForClient     *operations.DisableForClientUseCase
}

// RegisterRoutes mounts the application endpoints.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/applications", func(r chi.Router) {
		r.Get("/", s.list)
		r.Post("/", s.create)
		r.Get("/by-code/{code}", s.getByCode)
		r.Get("/{id}", s.getByID)
		r.Put("/{id}", s.update)
		r.Post("/{id}/activate", s.activate)
		r.Post("/{id}/deactivate", s.deactivate)
		r.Delete("/{id}", s.delete)

		// Provisioning
		r.Post("/{id}/service-account", s.attachServiceAccount)

		// Per-client config sub-aggregate
		r.Get("/{id}/clients", s.listClientConfigs)
		r.Post("/{id}/clients/{clientId}/enable", s.enableForClient)
		r.Post("/{id}/clients/{clientId}/disable", s.disableForClient)
	})
}

// ── client_config + provisioning handlers ──────────────────────────────────

func (s *State) getByCode(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadApplications(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	code := chi.URLParam(r, "code")
	app, err := s.Repo.FindByCode(r.Context(), code)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_code failed", err))
		return
	}
	if app == nil {
		httperror.Write(w, httperror.NotFound("Application", code))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(app)
}

func (s *State) attachServiceAccount(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	var body struct {
		ServiceAccountID   string `json:"serviceAccountId"`
		ServiceAccountCode string `json:"serviceAccountCode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.AttachServiceAccount,
		operations.AttachServiceAccountCommand{
			ApplicationID:      id,
			ServiceAccountID:   body.ServiceAccountID,
			ServiceAccountCode: body.ServiceAccountCode,
		}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) listClientConfigs(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadApplications(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	rows, err := s.ClientConfigRepo.FindByApplication(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "list client configs failed", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": rows})
}

func (s *State) enableForClient(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	clientID := chi.URLParam(r, "clientId")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.EnableForClient,
		operations.EnableForClientCommand{ApplicationID: id, ClientID: clientID}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) disableForClient(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	clientID := chi.URLParam(r, "clientId")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.DisableForClient,
		operations.DisableForClientCommand{ApplicationID: id, ClientID: clientID}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// keep apicommon import live for create handler
var _ = apicommon.CreatedResponse{}

func (s *State) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadApplications(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	q := r.URL.Query()
	var appType, active *string
	if v := q.Get("type"); v != "" {
		appType = &v
	}
	if v := q.Get("active"); v != "" {
		active = &v
	}
	rows, err := s.Repo.FindWithFilters(r.Context(), appType, active)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_with_filters failed", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": rows})
}

func (s *State) getByID(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanReadApplications(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	a, err := s.Repo.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if a == nil {
		httperror.Write(w, httperror.NotFound("Application", id))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(a)
}

func (s *State) create(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteApplications(ac); err != nil {
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
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.ApplicationID})
}

func (s *State) update(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteApplications(ac); err != nil {
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

func (s *State) activate(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteApplications(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.ActivateUC, operations.ActivateCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) deactivate(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanWriteApplications(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.DeactivateUC, operations.DeactivateCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) delete(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanDeleteApplications(ac); err != nil {
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
