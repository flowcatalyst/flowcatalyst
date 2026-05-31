// Package clientselection serves /auth/client/* — the multi-tenant client
// context switcher used by the SPA in embedded auth mode. 1:1 with Rust
// shared/client_selection_api.rs (list accessible clients, switch, current).
package clientselection

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// State holds the deps the /auth/client handlers reach into.
type State struct {
	Principals *principal.Repository
	Clients    *client.Repository
	Roles      *role.Repository
	Grants     *principal.ClientAccessGrantRepo
	Auth       *authservice.AuthService
}

// RegisterRoutes mounts /auth/client/* on the supplied (authenticated) router.
func RegisterRoutes(r chi.Router, s *State) {
	r.Get("/auth/client/accessible", s.listAccessible)
	r.Post("/auth/client/switch", s.switchClient)
	r.Get("/auth/client/current", s.currentClient)
}

// clientInfo mirrors Rust ClientInfo.
type clientInfo struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Identifier string `json:"identifier"`
}

type accessibleClientsResponse struct {
	Clients         []clientInfo `json:"clients"`
	CurrentClientID *string      `json:"currentClientId,omitempty"`
	GlobalAccess    bool         `json:"globalAccess"`
}

type switchClientRequest struct {
	ClientID string `json:"clientId"`
}

type switchClientResponse struct {
	Token       string     `json:"token"`
	Client      clientInfo `json:"client"`
	Roles       []string   `json:"roles"`
	Permissions []string   `json:"permissions"`
}

type currentClientResponse struct {
	Client          *clientInfo `json:"client"`
	NoClientContext bool        `json:"noClientContext"`
}

func (s *State) loadPrincipal(r *http.Request, ac *auth.AuthContext) (*principal.Principal, error) {
	p, err := s.Principals.FindByID(r.Context(), ac.PrincipalID)
	if err != nil {
		return nil, usecase.Internal("REPO", "principal find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", ac.PrincipalID)
	}
	return p, nil
}

// accessibleClientIDs resolves the client IDs a principal may act inside,
// 1:1 with Rust get_accessible_client_ids: anchors → all active clients;
// client-scope → home client + active grants; partner → assigned clients +
// active grants.
func (s *State) accessibleClientIDs(r *http.Request, p *principal.Principal) ([]string, error) {
	switch p.Scope {
	case principal.ScopeAnchor:
		all, err := s.Clients.FindAll(r.Context())
		if err != nil {
			return nil, usecase.Internal("REPO", "client find_all failed", err)
		}
		ids := make([]string, 0, len(all))
		for i := range all {
			if all[i].Status == client.StatusActive {
				ids = append(ids, all[i].ID)
			}
		}
		return ids, nil
	case principal.ScopeClient:
		var ids []string
		if p.ClientID != nil {
			ids = append(ids, *p.ClientID)
		}
		return s.appendGrants(r, ids, p.ID)
	default: // ScopePartner
		ids := append([]string(nil), p.AssignedClients...)
		return s.appendGrants(r, ids, p.ID)
	}
}

func (s *State) appendGrants(r *http.Request, ids []string, principalID string) ([]string, error) {
	grants, err := s.Grants.FindByPrincipal(r.Context(), principalID)
	if err != nil {
		return nil, usecase.Internal("REPO", "grant find_by_principal failed", err)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for i := range grants {
		if !seen[grants[i].ClientID] {
			ids = append(ids, grants[i].ClientID)
			seen[grants[i].ClientID] = true
		}
	}
	return ids, nil
}

func (s *State) listAccessible(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}
	p, err := s.loadPrincipal(r, ac)
	if err != nil {
		httperror.Write(w, err)
		return
	}
	ids, err := s.accessibleClientIDs(r, p)
	if err != nil {
		httperror.Write(w, err)
		return
	}
	out := make([]clientInfo, 0, len(ids))
	for _, id := range ids {
		c, err := s.Clients.FindByID(r.Context(), id)
		if err != nil {
			httperror.Write(w, usecase.Internal("REPO", "client find_by_id failed", err))
			return
		}
		if c != nil && c.Status == client.StatusActive {
			out = append(out, clientInfo{ID: c.ID, Name: c.Name, Identifier: c.Identifier})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(accessibleClientsResponse{
		Clients:         out,
		CurrentClientID: p.ClientID,
		GlobalAccess:    p.Scope == principal.ScopeAnchor,
	})
}

func (s *State) switchClient(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}
	var req switchClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	p, err := s.loadPrincipal(r, ac)
	if err != nil {
		httperror.Write(w, err)
		return
	}
	// Access check (anchors may access any client).
	if p.Scope != principal.ScopeAnchor {
		ids, err := s.accessibleClientIDs(r, p)
		if err != nil {
			httperror.Write(w, err)
			return
		}
		if !contains(ids, req.ClientID) {
			httperror.Write(w, httperror.Forbidden("Access denied to client: "+req.ClientID))
			return
		}
	}
	c, err := s.Clients.FindByID(r.Context(), req.ClientID)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "client find_by_id failed", err))
		return
	}
	if c == nil {
		httperror.Write(w, httperror.NotFound("Client", req.ClientID))
		return
	}
	if c.Status != client.StatusActive {
		httperror.Write(w, httperror.Forbidden("Client is not active: "+c.Name))
		return
	}
	token, err := s.Auth.GenerateAccessToken(p)
	if err != nil {
		httperror.Write(w, usecase.Internal("TOKEN", "generate access token failed", err))
		return
	}
	roleCodes := make([]string, 0, len(p.Roles))
	for _, ra := range p.Roles {
		roleCodes = append(roleCodes, ra.Role)
	}
	perms, err := s.resolvePermissions(r, roleCodes)
	if err != nil {
		httperror.Write(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(switchClientResponse{
		Token:       token,
		Client:      clientInfo{ID: c.ID, Name: c.Name, Identifier: c.Identifier},
		Roles:       roleCodes,
		Permissions: perms,
	})
}

func (s *State) currentClient(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if ac == nil {
		httperror.Write(w, usecase.Authorization("UNAUTHENTICATED", "authentication required"))
		return
	}
	p, err := s.loadPrincipal(r, ac)
	if err != nil {
		httperror.Write(w, err)
		return
	}
	var info *clientInfo
	if p.ClientID != nil {
		c, err := s.Clients.FindByID(r.Context(), *p.ClientID)
		if err != nil {
			httperror.Write(w, usecase.Internal("REPO", "client find_by_id failed", err))
			return
		}
		if c != nil {
			info = &clientInfo{ID: c.ID, Name: c.Name, Identifier: c.Identifier}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(currentClientResponse{Client: info, NoClientContext: info == nil})
}

// resolvePermissions flattens the permission set of the named roles (deduped),
// 1:1 with Rust resolve_permissions.
func (s *State) resolvePermissions(r *http.Request, roleCodes []string) ([]string, error) {
	seen := map[string]bool{}
	out := []string{}
	for _, code := range roleCodes {
		role, err := s.Roles.FindByName(r.Context(), code)
		if err != nil {
			return nil, usecase.Internal("REPO", "role find_by_name failed", err)
		}
		if role == nil {
			continue
		}
		for _, perm := range role.Permissions {
			if !seen[perm] {
				seen[perm] = true
				out = append(out, perm)
			}
		}
	}
	return out, nil
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}
