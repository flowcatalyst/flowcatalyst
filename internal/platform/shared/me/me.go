// Package me serves /api/me — the canonical "who am I" lookup
// returned to agents, SDKs, and the dashboard.
package me

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State holds the deps the /api/me handler reaches into. Only the
// principal repo is needed today; accessible_application_ids isn't on
// the JWT so we resolve it from the row at request time. Other /me
// sub-routes (clients, applications) can grow this state without
// re-threading callers.
type State struct {
	Principals *principal.Repository
}

// RegisterRoutes mounts GET /api/me at the supplied router.
func RegisterRoutes(r chi.Router, s *State) {
	r.Get("/api/me", s.whoami)
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

func stringSliceOrEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
