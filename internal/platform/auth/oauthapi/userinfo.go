package oauthapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
)

// RegisterUserinfoRoutes mounts GET+POST /oauth/userinfo. Closes the
// parity gap where the endpoint was advertised in discovery but 404'd.
func (s *State) RegisterUserinfoRoutes(r chi.Router) {
	r.Get("/oauth/userinfo", s.Userinfo)
	r.Post("/oauth/userinfo", s.Userinfo)
}

// userInfoResponse is the OIDC UserInfo body. sub/name/tier/type and the
// array claims are always present; scope carries the token's granted
// permissions (empty for tokens without a scope claim); email and client_id
// are omitted when absent.
type userInfoResponse struct {
	Sub           string   `json:"sub"`
	Email         *string  `json:"email,omitempty"`
	Name          string   `json:"name"`
	Tier          string   `json:"tier"`
	Scope         string   `json:"scope"`
	PrincipalType string   `json:"type"`
	ClientID      *string  `json:"client_id,omitempty"`
	Clients       []string `json:"clients"`
	Roles         []string `json:"roles"`
	Applications  []string `json:"applications"`
}

// Userinfo is GET/POST /oauth/userinfo (OIDC). It validates the bearer
// access token and returns the identity claims subset.
func (s *State) Userinfo(w http.ResponseWriter, r *http.Request) {
	claims, errResp := s.validateBearer(r)
	if errResp != nil {
		errResp.write(w)
		return
	}
	writeJSON(w, http.StatusOK, userInfoResponse{
		Sub:           claims.Subject,
		Email:         claims.Email,
		Name:          claims.Name,
		Tier:          claims.Tier,
		Scope:         claims.Scope,
		PrincipalType: claims.PrincipalType,
		ClientID:      userinfoClientID(claims.Clients),
		Clients:       nonNil(claims.Clients),
		Roles:         nonNil(claims.Roles),
		Applications:  nonNil(claims.Applications),
	})
}

// validateBearer extracts and validates the Authorization: Bearer access
// token, mirroring Rust's extract_and_validate_token.
func (s *State) validateBearer(r *http.Request) (*authservice.AccessTokenClaims, *oauthError) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, newOAuthError(http.StatusUnauthorized, "invalid_request", "Missing Authorization header")
	}
	token := authservice.ExtractBearerToken(authHeader)
	if token == "" {
		return nil, newOAuthError(http.StatusUnauthorized, "invalid_request", "Invalid Authorization header format")
	}
	claims, err := s.Auth.ValidateToken(token)
	if err != nil {
		return nil, newOAuthError(http.StatusUnauthorized, "invalid_token", "Token is invalid or expired")
	}
	return claims, nil
}

// userinfoClientID derives the client_id from the first client entry,
// stripping the ":identifier" suffix. Returns nil for the anchor "*"
// wildcard. Mirrors Rust's userinfo client_id logic.
func userinfoClientID(clients []string) *string {
	if len(clients) == 0 {
		return nil
	}
	first := clients[0]
	if first == "*" {
		return nil
	}
	if i := strings.IndexByte(first, ':'); i >= 0 {
		first = first[:i]
	}
	return &first
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
