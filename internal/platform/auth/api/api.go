// Package api wires admin HTTP routes for the auth subdomain. The
// runtime routes (/oauth/token, /oauth/authorize, /.well-known/*, OIDC
// login/callback) are registered by the provider and bridge packages
// in the auth-runtime follow-up.
package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State bundles deps.
type State struct {
	Repo *auth.Repository

	CreateOAuthClient     *operations.CreateOAuthClientUseCase
	UpdateOAuthClient     *operations.UpdateOAuthClientUseCase
	ActivateOAuthClient   *operations.ActivateOAuthClientUseCase
	DeactivateOAuthClient *operations.DeactivateOAuthClientUseCase
	DeleteOAuthClient     *operations.DeleteOAuthClientUseCase
	RotateSecret          *operations.RotateOAuthClientSecretUseCase

	CreateAnchorDomain *operations.CreateAnchorDomainUseCase
	UpdateAnchorDomain *operations.UpdateAnchorDomainUseCase
	DeleteAnchorDomain *operations.DeleteAnchorDomainUseCase

	CreateAuthConfig *operations.CreateAuthConfigUseCase
	UpdateAuthConfig *operations.UpdateAuthConfigUseCase
	DeleteAuthConfig *operations.DeleteAuthConfigUseCase

	CreateIdpRoleMapping *operations.CreateIdpRoleMappingUseCase
	DeleteIdpRoleMapping *operations.DeleteIdpRoleMappingUseCase
}

// RegisterRoutes mounts the auth admin endpoints. Anchor-only.
//
// TODO(auth-runtime): the public-facing OAuth/OIDC endpoints (token,
// authorize, .well-known, oidc-login, oidc-callback) are registered by
// the provider and bridge packages in the follow-up.
func RegisterRoutes(r chi.Router, s *State) {
	r.Route("/api/oauth-clients", func(r chi.Router) {
		r.Get("/", s.listOAuthClients)
		r.Post("/", s.createOAuthClient)
		r.Get("/{id}", s.getOAuthClient)
		r.Put("/{id}", s.updateOAuthClient)
		r.Post("/{id}/activate", s.activateOAuthClient)
		r.Post("/{id}/deactivate", s.deactivateOAuthClient)
		r.Post("/{id}/rotate-secret", s.rotateOAuthClientSecret)
		r.Delete("/{id}", s.deleteOAuthClient)
	})
	r.Route("/api/anchor-domains", func(r chi.Router) {
		r.Get("/", s.listAnchorDomains)
		r.Post("/", s.createAnchorDomain)
		r.Put("/{id}", s.updateAnchorDomain)
		r.Delete("/{id}", s.deleteAnchorDomain)
	})
	r.Route("/api/auth-configs", func(r chi.Router) {
		r.Get("/", s.listAuthConfigs)
		r.Post("/", s.createAuthConfig)
		r.Put("/{id}", s.updateAuthConfig)
		r.Delete("/{id}", s.deleteAuthConfig)
	})
	r.Route("/api/idp-role-mappings", func(r chi.Router) {
		r.Get("/", s.listIdpRoleMappings)
		r.Post("/", s.createIdpRoleMapping)
		r.Delete("/{id}", s.deleteIdpRoleMapping)
	})
}

// ── helpers ───────────────────────────────────────────────────────────────

func requireAnchor(w http.ResponseWriter, r *http.Request) (*platformauth.AuthContext, bool) {
	ac := platformauth.FromContext(r.Context())
	if err := platformauth.RequireAnchor(ac); err != nil {
		httperror.Write(w, err)
		return nil, false
	}
	return ac, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// ── OAuth client routes ───────────────────────────────────────────────────

func (s *State) listOAuthClients(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAnchor(w, r); !ok {
		return
	}
	rows, err := s.Repo.OAuthClients.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_all failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) getOAuthClient(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAnchor(w, r); !ok {
		return
	}
	id := chi.URLParam(r, "id")
	c, err := s.Repo.OAuthClients.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_by_id failed", err))
		return
	}
	if c == nil {
		httperror.Write(w, httperror.NotFound("OAuthClient", id))
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *State) createOAuthClient(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	var body operations.CreateOAuthClientCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.CreateOAuthClient, body, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	// CONFIDENTIAL clients get their plaintext secret returned ONCE.
	resp := map[string]any{
		"id":         event.OAuthClientID,
		"clientId":   event.ClientID,
		"clientName": event.ClientName,
	}
	if plaintext, ok := operations.PopStashedSecret(event.OAuthClientID); ok {
		resp["clientSecret"] = plaintext
	}
	writeJSON(w, http.StatusCreated, resp)
}

func (s *State) updateOAuthClient(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	var body operations.UpdateOAuthClientCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	body.ID = id
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.UpdateOAuthClient, body, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) activateOAuthClient(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.ActivateOAuthClient,
		operations.ActivateOAuthClientCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) deactivateOAuthClient(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.DeactivateOAuthClient,
		operations.DeactivateOAuthClientCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) rotateOAuthClientSecret(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.RotateSecret,
		operations.RotateOAuthClientSecretCommand{ID: id}, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	resp := map[string]any{"id": event.OAuthClientID}
	if plaintext, ok := operations.PopStashedSecret(event.OAuthClientID); ok {
		resp["clientSecret"] = plaintext
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *State) deleteOAuthClient(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.DeleteOAuthClient,
		operations.DeleteOAuthClientCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── AnchorDomain routes ───────────────────────────────────────────────────

func (s *State) listAnchorDomains(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAnchor(w, r); !ok {
		return
	}
	rows, err := s.Repo.AnchorDomains.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_all failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) createAnchorDomain(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	var body operations.CreateAnchorDomainCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.CreateAnchorDomain, body, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apicommon.CreatedResponse{ID: event.AnchorDomainID})
}

func (s *State) updateAnchorDomain(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	var body operations.UpdateAnchorDomainCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	body.ID = id
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.UpdateAnchorDomain, body, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) deleteAnchorDomain(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.DeleteAnchorDomain,
		operations.DeleteAnchorDomainCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── AuthConfig routes ─────────────────────────────────────────────────────

func (s *State) listAuthConfigs(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAnchor(w, r); !ok {
		return
	}
	rows, err := s.Repo.ClientAuthConfigs.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_all failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) createAuthConfig(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	var body operations.CreateAuthConfigCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.CreateAuthConfig, body, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apicommon.CreatedResponse{ID: event.AuthConfigID})
}

func (s *State) updateAuthConfig(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	var body operations.UpdateAuthConfigCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	body.ID = id
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.UpdateAuthConfig, body, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *State) deleteAuthConfig(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.DeleteAuthConfig,
		operations.DeleteAuthConfigCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── IdpRoleMapping routes ─────────────────────────────────────────────────

func (s *State) listIdpRoleMappings(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAnchor(w, r); !ok {
		return
	}
	rows, err := s.Repo.IdpRoleMappings.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "find_all failed", err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": rows})
}

func (s *State) createIdpRoleMapping(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	var body operations.CreateIdpRoleMappingCommand
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	event, err := usecase.Into(usecase.Run(r.Context(), s.CreateIdpRoleMapping, body, ec))
	if err != nil {
		httperror.Write(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, apicommon.CreatedResponse{ID: event.MappingID})
}

func (s *State) deleteIdpRoleMapping(w http.ResponseWriter, r *http.Request) {
	ac, ok := requireAnchor(w, r)
	if !ok {
		return
	}
	id := chi.URLParam(r, "id")
	ec := usecase.NewExecutionContext(ac.PrincipalID)
	if _, err := usecase.Into(usecase.Run(r.Context(), s.DeleteIdpRoleMapping,
		operations.DeleteIdpRoleMappingCommand{ID: id}, ec)); err != nil {
		httperror.Write(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
