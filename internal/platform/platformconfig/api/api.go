// Package api wires HTTP routes for platform_config.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo           *platformconfig.Repository
	SetPropertyUC  *operations.SetPropertyUseCase
	GrantAccessUC  *operations.GrantAccessUseCase
	RevokeAccessUC *operations.RevokeAccessUseCase
}

// RegisterRoutes mounts the platform_config endpoints. Anchor-only by
// default; per-app/per-role access is enforced by HasAccess against
// app_platform_config_access for non-anchor callers.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/platform-config", func(r chi.Router) {
		r.Get("/{app}", s.listProperties)
		r.Put("/{app}/{section}/{property}", s.setProperty)
		r.Get("/{app}/access", s.listAccess)
		r.Post("/{app}/access", s.grantAccess)
		r.Delete("/access/{id}", s.revokeAccess)
	})
}

func (s *State) listProperties(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	app := chi.URLParam(r, "app")

	// Check access: anchor always allowed; otherwise any of the principal's
	// roles must have can_read for this app.
	if !ac.IsAnchor() {
		ok, err := s.Repo.HasAccess(r.Context(), app, ac.Roles, false)
		if err != nil {
			httperror.Write(w, usecase.Internal("REPO", "has_access failed", err))
			return
		}
		if !ok {
			httperror.Write(w, httperror.Forbidden("No read access to platform config for "+app))
			return
		}
	}

	rows, err := s.Repo.FindConfigsByApplication(r.Context(), app)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_configs_by_application failed", err))
		return
	}
	// Mask SECRET values for non-anchor callers.
	if !ac.IsAnchor() {
		for i := range rows {
			if rows[i].ValueType == platformconfig.ValueSecret {
				rows[i].Value = "***"
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": rows})
}

func (s *State) setProperty(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	app := chi.URLParam(r, "app")
	section := chi.URLParam(r, "section")
	property := chi.URLParam(r, "property")

	if !ac.IsAnchor() {
		ok, err := s.Repo.HasAccess(r.Context(), app, ac.Roles, true)
		if err != nil {
			httperror.Write(w, usecase.Internal("REPO", "has_access failed", err))
			return
		}
		if !ok {
			httperror.Write(w, httperror.Forbidden("No write access to platform config for "+app))
			return
		}
	}

	var body operations.SetPropertyCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	body.ApplicationCode = app
	body.Section = section
	body.Property = property

	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.SetPropertyUC, body, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.ConfigID})
}

func (s *State) listAccess(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	app := chi.URLParam(r, "app")
	rows, err := s.Repo.FindAccessByApplication(r.Context(), app)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_access_by_application failed", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"items": rows})
}

func (s *State) grantAccess(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	app := chi.URLParam(r, "app")
	var body operations.GrantAccessCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	body.ApplicationCode = app
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.GrantAccessUC, body, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(apicommon.CreatedResponse{ID: event.AccessID})
}

func (s *State) revokeAccess(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.RevokeAccessUC, operations.RevokeAccessCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
