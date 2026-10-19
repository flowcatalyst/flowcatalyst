package oauthapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
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
// client credentials (Basic header / body). It returns the authenticated OAuth
// client, or nil when the caller authenticated with a Bearer token (a bearer
// carries no OAuth client identity), plus the error (nil = authorized).
// bearerAllowed=false refuses the Bearer path (client credentials only).
func (s *State) authenticateClientOrBearer(r *http.Request, clientIDBody, clientSecretBody string, bearerAllowed bool) (*auth.OAuthClient, *oauthError) {
	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if token := authservice.ExtractBearerToken(authHeader); token != "" {
			if !bearerAllowed {
				return nil, newOAuthError(http.StatusUnauthorized, "invalid_client", "Client authentication required")
			}
			if _, err := s.Auth.ValidateToken(token); err != nil {
				return nil, newOAuthError(http.StatusUnauthorized, "invalid_token", "Token is invalid or expired")
			}
			return nil, nil
		}
		// A non-Bearer scheme (Basic ...) falls through to client auth.
	}
	client, errResp := s.authenticateClient(r, clientIDBody, clientSecretBody)
	if errResp != nil {
		return nil, errResp
	}
	return client, nil
}

// IntrospectResponse is the RFC 7662 result. Every field except `active`
// is omitted when absent.
type IntrospectResponse struct {
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
	if _, errResp := s.authenticateClientOrBearer(r, r.PostFormValue("client_id"), r.PostFormValue("client_secret"), !s.IntrospectClientAuthOnly); errResp != nil {
		errResp.write(w)
		return
	}

	claims, err := s.Auth.ValidateToken(r.PostFormValue("token"))
	if err != nil {
		// RFC 7662: an inactive/unknown token is reported, not errored.
		writeJSON(w, http.StatusOK, IntrospectResponse{Active: false})
		return
	}

	resp := IntrospectResponse{
		Active:        true,
		Sub:           new(claims.Subject),
		Tier:          new(claims.Tier),
		Email:         claims.Email,
		Name:          new(claims.Name),
		PrincipalType: new(claims.PrincipalType),
		Iss:           new(claims.Issuer),
		TokenType:     new("Bearer"),
	}
	// RFC 7662 `scope` carries the token's granted scopes (now permissions).
	if claims.Scope != "" {
		resp.Scope = new(claims.Scope)
	}
	if len(claims.Clients) > 0 {
		resp.ClientID = new(claims.Clients[0])
	}
	if claims.ExpiresAt != nil {
		resp.Exp = new(claims.ExpiresAt.Unix())
	}
	if claims.IssuedAt != nil {
		resp.Iat = new(claims.IssuedAt.Unix())
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
	client, errResp := s.authenticateClientOrBearer(r, r.PostFormValue("client_id"), r.PostFormValue("client_secret"), true)
	if errResp != nil {
		errResp.write(w)
		return
	}

	// Whether or not the hint says refresh_token, the only revocable
	// artifact is a refresh token (matched by hash). Best-effort; RFC 7009
	// mandates 200 even when the token is unknown.
	tokenHash := grantstore.HashToken(r.PostFormValue("token"))
	// RFC 7009 §2.1: a client may only revoke tokens issued to it. A token
	// bound to another client is left alone (still 200, so the response
	// reveals nothing about the token). A Bearer caller has no client
	// identity and is not checked; tokens with no client binding (session /
	// legacy) are not checked.
	if client != nil && !s.RevokeAnyClient {
		if stored, err := s.RefreshTokens.FindByHash(r.Context(), tokenHash); err == nil &&
			stored != nil && stored.OAuthClientID != nil && *stored.OAuthClientID != client.ClientID {
			slog.Warn("oauth revoke: token belongs to another client; not revoked",
				"caller_client", client.ClientID, "token_client", *stored.OAuthClientID)
			w.WriteHeader(http.StatusOK)
			return
		}
	}
	_, _ = s.RefreshTokens.RevokeByHash(r.Context(), tokenHash)
	w.WriteHeader(http.StatusOK)
}

// writeJSON renders v with the given status and Content-Type, without the
// no-store cache headers the token endpoint adds (introspect/revoke
// responses are cacheable per their RFCs).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
