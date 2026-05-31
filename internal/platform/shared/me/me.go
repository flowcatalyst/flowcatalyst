// Package me serves /api/me — the canonical "who am I" lookup
// returned to agents, SDKs, and the dashboard.
package me

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State holds the deps the /api/me handlers reach into.
// accessible_application_ids isn't on the JWT so we resolve it from the
// principal row at request time; Applications backs /api/me/applications.
// Clients + AppConfigs back the /api/me/clients[...] endpoints.
type State struct {
	Principals   *principal.Repository
	Applications *application.Repository
	Clients      *client.Repository
	AppConfigs   *application.ClientConfigRepo
}

// RegisterRoutes mounts the /api/me routes at the supplied router.
func RegisterRoutes(r chi.Router, s *State) {
	r.Get("/api/me", s.whoami)
	r.Get("/api/me/applications", s.listMyApplications)
	r.Get("/api/me/clients", s.listMyClients)
	r.Get("/api/me/clients/{clientId}", s.getMyClient)
	r.Get("/api/me/clients/{clientId}/applications", s.listMyClientApplications)
}

// whoamiResponse mirrors Rust's WhoamiResponse exactly.
type whoamiResponse struct {
	PrincipalID              string   `json:"principalId"`
	PrincipalType            string   `json:"principalType"`
	Scope                    string   `json:"scope"`
	Name                     string   `json:"name"`
	Email                    *string  `json:"email,omitempty"`
	Active                   bool     `json:"active"`
	Roles                    []string `json:"roles"`
	AccessibleClientIDs      []string `json:"accessibleClientIds"`
	AccessibleApplicationIDs []string `json:"accessibleApplicationIds"`
}

func (s *State) whoami(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}

	out := whoamiResponse{
		PrincipalID:         ac.PrincipalID,
		Scope:               string(ac.Scope),
		Active:              true,
		Roles:               stringSliceOrEmpty(ac.Roles),
		AccessibleClientIDs: stringSliceOrEmpty(ac.Clients),
	}

	var email *string
	if ac.Email != "" {
		e := ac.Email
		email = &e
	}
	out.Email = email

	// accessible_application_ids isn't in the JWT claims, so resolve
	// it from the principal row. Anchors see every application
	// implicitly; we still surface their granted list for symmetry
	// with the Rust behaviour.
	p, err := s.Principals.FindByID(r.Context(), ac.PrincipalID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "principal lookup failed", err))
		return
	}
	if p != nil {
		out.AccessibleApplicationIDs = stringSliceOrEmpty(p.AccessibleApplicationIDs)
		out.PrincipalType = string(p.Type)
		if p.UserIdentity != nil {
			out.Name = p.UserIdentity.DisplayName()
		}
	} else {
		// Test-header principals (X-FC-Test-*) don't have a DB row.
		// Fall back to the context fields so the handler still
		// returns a useful response.
		out.AccessibleApplicationIDs = stringSliceOrEmpty(ac.Applications)
		out.PrincipalType = "USER"
		if out.Email != nil {
			out.Name = *out.Email
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// myApplicationResponse mirrors Rust's MyApplicationResponse (camelCase).
type myApplicationResponse struct {
	ID           string  `json:"id"`
	Code         string  `json:"code"`
	Name         string  `json:"name"`
	Description  *string `json:"description,omitempty"`
	IconURL      *string `json:"iconUrl,omitempty"`
	BaseURL      *string `json:"baseUrl,omitempty"`
	Website      *string `json:"website,omitempty"`
	LogoMimeType *string `json:"logoMimeType,omitempty"`
}

// myApplicationsListResponse mirrors Rust's MyApplicationsListResponse.
type myApplicationsListResponse struct {
	Applications []myApplicationResponse `json:"applications"`
	Total        int                     `json:"total"`
	// ClientID is empty for this principal-scoped variant (kept for
	// response-shape compatibility with the per-client endpoint). 1:1 with Rust.
	ClientID string `json:"clientId"`
}

// listMyApplications serves GET /api/me/applications: the applications the
// calling principal can access. Anchors see every application; others see the
// apps granted on their principal row (accessible_application_ids). Mirrors
// Rust me_api.rs list_my_applications.
func (s *State) listMyApplications(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}

	all, err := s.Applications.FindWithFilters(r.Context(), nil, nil) // all apps (active + inactive), 1:1 with Rust find_all
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "application lookup failed", err))
		return
	}

	// Resolve the accessible-app set for non-anchors (anchors see all).
	accessible := map[string]bool{}
	if !ac.IsAnchor() {
		if p, err := s.Principals.FindByID(r.Context(), ac.PrincipalID); err != nil {
			httperror.Write(w, usecase.Internal("REPO", "principal lookup failed", err))
			return
		} else if p != nil {
			for _, id := range p.AccessibleApplicationIDs {
				accessible[id] = true
			}
		} else {
			// Test-header principal (no DB row): fall back to context.
			for _, id := range ac.Applications {
				accessible[id] = true
			}
		}
	}

	apps := filterMyApplications(all, ac.IsAnchor(), accessible)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(myApplicationsListResponse{
		Applications: apps,
		Total:        len(apps),
		ClientID:     "",
	})
}

// filterMyApplications keeps the apps the caller can see (anchors see all,
// others only those in accessible) and maps each to the wire shape
// (DefaultBaseURL → baseUrl). Pure, so the access filter + field mapping are
// unit-testable without a DB. 1:1 with Rust's mapping.
func filterMyApplications(all []application.Application, isAnchor bool, accessible map[string]bool) []myApplicationResponse {
	apps := make([]myApplicationResponse, 0, len(all))
	for i := range all {
		a := &all[i]
		if isAnchor || accessible[a.ID] {
			apps = append(apps, myApplicationResponse{
				ID:           a.ID,
				Code:         a.Code,
				Name:         a.Name,
				Description:  a.Description,
				IconURL:      a.IconURL,
				BaseURL:      a.DefaultBaseURL,
				Website:      a.Website,
				LogoMimeType: a.LogoMimeType,
			})
		}
	}
	return apps
}

func stringSliceOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// myClientResponse mirrors Rust's MyClientResponse (camelCase).
type myClientResponse struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Identifier string  `json:"identifier"`
	Status     *string `json:"status,omitempty"`
	CreatedAt  string  `json:"createdAt"`
	UpdatedAt  string  `json:"updatedAt"`
}

// myClientsListResponse mirrors Rust's MyClientsListResponse.
type myClientsListResponse struct {
	Clients []myClientResponse `json:"clients"`
	Total   int                `json:"total"`
}

func clientToMyResponse(c *client.Client) myClientResponse {
	status := string(c.Status)
	return myClientResponse{
		ID:         c.ID,
		Name:       c.Name,
		Identifier: c.Identifier,
		Status:     &status,
		CreatedAt:  c.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:  c.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// listMyClients serves GET /api/me/clients: the clients the caller can access
// (anchors see all). Mirrors Rust me_api.rs list_my_clients.
func (s *State) listMyClients(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}
	all, err := s.Clients.FindAll(r.Context())
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "client find_all failed", err))
		return
	}
	out := make([]myClientResponse, 0, len(all))
	for i := range all {
		c := &all[i]
		if ac.IsAnchor() || ac.CanAccessClient(c.ID) {
			out = append(out, clientToMyResponse(c))
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(myClientsListResponse{Clients: out, Total: len(out)})
}

// getMyClient serves GET /api/me/clients/{clientId}. Mirrors Rust get_my_client.
func (s *State) getMyClient(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}
	id := chi.URLParam(r, "clientId")
	c, err := s.Clients.FindByID(r.Context(), id)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "client find_by_id failed", err))
		return
	}
	if c == nil {
		httperror.Write(w, httperror.NotFound("Client", id))
		return
	}
	if !ac.IsAnchor() && !ac.CanAccessClient(c.ID) {
		httperror.Write(w, httperror.Forbidden("No access to this client"))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(clientToMyResponse(c))
}

// listMyClientApplications serves GET /api/me/clients/{clientId}/applications:
// the applications enabled for a client the caller can access. Mirrors Rust
// list_my_client_applications.
func (s *State) listMyClientApplications(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}
	clientID := chi.URLParam(r, "clientId")
	c, err := s.Clients.FindByID(r.Context(), clientID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "client find_by_id failed", err))
		return
	}
	if c == nil {
		httperror.Write(w, httperror.NotFound("Client", clientID))
		return
	}
	if !ac.IsAnchor() && !ac.CanAccessClient(clientID) {
		httperror.Write(w, httperror.Forbidden("No access to this client"))
		return
	}
	configs, err := s.AppConfigs.FindByClient(r.Context(), clientID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "app config find_by_client failed", err))
		return
	}
	enabled := map[string]bool{}
	for i := range configs {
		if configs[i].Enabled {
			enabled[configs[i].ApplicationID] = true
		}
	}
	all, err := s.Applications.FindWithFilters(r.Context(), nil, nil)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "application lookup failed", err))
		return
	}
	apps := filterMyApplications(all, false, enabled) // enabled-only (not an anchor short-circuit)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(myApplicationsListResponse{
		Applications: apps,
		Total:        len(apps),
		ClientID:     clientID,
	})
}
