package oauthapi

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
)

// RegisterIntrospectRoutes mounts POST /oauth/introspect.
func (s *State) RegisterIntrospectRoutes(r chi.Router) {
	r.Post("/oauth/introspect", s.Introspect)
}

// RegisterRevokeRoutes mounts POST /oauth/revoke.
func (s *State) RegisterRevokeRoutes(r chi.Router) {
	r.Post("/oauth/revoke", s.Revoke)
}

// authenticateClientOrBearer authorizes a call to a protected OAuth
// endpoint (introspect/revoke) via either a valid Bearer access token or
// client credentials (Basic header / body). Mirrors Rust's
// authenticate_client_or_bearer; the resolved identity is unused by the
// callers, so only the error (nil = authorized) is returned.
func (s *State) authenticateClientOrBearer(r *http.Request, clientIDBody, clientSecretBody string) *oauthError {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if token := authservice.ExtractBearerToken(authHeader); token != "" {
			if _, err := s.Auth.ValidateToken(token); err != nil {
				return newOAuthError(http.StatusUnauthorized, "invalid_token", "Token is invalid or expired")
			}
			return nil
		}
		// A non-Bearer scheme (Basic ...) falls through to client auth.
	}
	if _, errResp := s.authenticateClient(r, clientIDBody, clientSecretBody); errResp != nil {
		return errResp
	}
	return nil
}

// introspectResponse is the RFC 7662 result. Every field except `active`
// is omitted when absent (Rust: skip_serializing_if = Option::is_none).
type introspectResponse struct {
	Active        bool    `json:"active"`
	Sub           *string `json:"sub,omitempty"`
	Scope         *string `json:"scope,omitempty"`
	Tier          *string `json:"tier,omitempty"`
	ClientID      *string `json:"client_id,omitempty"`
	Email         *string `json:"email,omitempty"`
	Name          *string `json:"name,omitempty"`
	PrincipalType *string `json:"type,omitempty"`
	Exp           *int64  `json:"exp,omitempty"`
	Iat           *int64  `json:"iat,omitempty"`
	Iss           *string `json:"iss,omitempty"`
	TokenType     *string `json:"token_type,omitempty"`
}

// Introspect is POST /oauth/introspect (RFC 7662). It authenticates the
// caller, then echoes the token's claims when valid or {active:false}
// otherwise — always 200.
func (s *State) Introspect(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Malformed form body")
		return
	}
	if errResp := s.authenticateClientOrBearer(r, r.PostFormValue("client_id"), r.PostFormValue("client_secret")); errResp != nil {
		errResp.write(w)
		return
	}

	claims, err := s.Auth.ValidateToken(r.PostFormValue("token"))
	if err != nil {
		// RFC 7662: an inactive/unknown token is reported, not errored.
		writeJSON(w, http.StatusOK, introspectResponse{Active: false})
		return
	}

	resp := introspectResponse{
		Active:        true,
		Sub:           ptr(claims.Subject),
		Tier:          ptr(claims.Tier),
		Email:         claims.Email,
		Name:          ptr(claims.Name),
		PrincipalType: ptr(claims.PrincipalType),
		Iss:           ptr(claims.Issuer),
		TokenType:     ptr("Bearer"),
	}
	// RFC 7662 `scope` carries the token's granted scopes (now permissions).
	if claims.Scope != "" {
		resp.Scope = ptr(claims.Scope)
	}
	if len(claims.Clients) > 0 {
		resp.ClientID = ptr(claims.Clients[0])
	}
	if claims.ExpiresAt != nil {
		resp.Exp = ptr(claims.ExpiresAt.Unix())
	}
	if claims.IssuedAt != nil {
		resp.Iat = ptr(claims.IssuedAt.Unix())
	}
	writeJSON(w, http.StatusOK, resp)
}

// Revoke is POST /oauth/revoke (RFC 7009). It authenticates the caller
// then best-effort revokes the token's refresh-token row (JWT access
// tokens are stateless and can't be revoked server-side). Always 200.
func (s *State) Revoke(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Malformed form body")
		return
	}
	if errResp := s.authenticateClientOrBearer(r, r.PostFormValue("client_id"), r.PostFormValue("client_secret")); errResp != nil {
		errResp.write(w)
		return
	}

	// Whether or not the hint says refresh_token, the only revocable
	// artifact is a refresh token (matched by hash). Best-effort; RFC 7009
	// mandates 200 even when the token is unknown.
	tokenHash := grantstore.HashToken(r.PostFormValue("token"))
	_, _ = s.RefreshTokens.RevokeByHash(r.Context(), tokenHash)
	w.WriteHeader(http.StatusOK)
}

func ptr[T any](v T) *T { return &v }

// writeJSON renders v with the given status and Content-Type, without the
// no-store cache headers the token endpoint adds (introspect/revoke
// responses are cacheable per their RFCs).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
