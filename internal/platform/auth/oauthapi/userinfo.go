package oauthapi

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
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

// Userinfo is GET/POST /oauth/userinfo (OIDC). It validates the bearer access
// token and answers with the caller's identity plus the authority that token's
// relying party is entitled to know about — recomputed from the principal, not
// echoed back off the token.
//
// Echoing the token was the bug: an interactive login receives an
// IDENTITY-ONLY access token (no roles, applications or scope by design), so
// userinfo answered every such call with empty arrays — asserting "this user
// holds nothing", which is false. It is the one endpoint an identity token
// exists to be used against, and it said nothing.
//
// The principal is now loaded fresh and passed through the same confinement
// the client's id_token gets (confineToClient), keyed on the token's azp. So
// userinfo is "the id_token's claims, as of now" — useful for picking up a
// role change without re-authenticating, and incapable of telling a relying
// party more than its id_token already did.
//
// Falls back to the token's own claims when the principal can't be resolved,
// or when the token carries no azp (a session token, or client_credentials,
// where the client is the principal and there is nothing to confine to).
func (s *State) Userinfo(w http.ResponseWriter, r *http.Request) {
	claims, errResp := s.validateBearer(r)
	if errResp != nil {
		errResp.write(w)
		return
	}

	roles, apps, clients := nonNil(claims.Roles), nonNil(claims.Applications), nonNil(claims.Clients)
	scope := claims.Scope

	if p := s.userinfoPrincipal(r, claims.Subject); p != nil {
		roles, apps, clients = roleNamesOf(p), authservice.ApplicationsClaim(p), authservice.ClientsClaim(p)
		if client := s.userinfoClient(r, claims.AZP); client != nil && len(client.ApplicationIDs) > 0 {
			scoped, narrowed, err := s.confineToClient(r.Context(), p, client)
			if err == nil {
				roles, apps = narrowed, authservice.ApplicationsClaim(scoped)
			}
		}
		// The scope claim is the token's granted permission set; it is a
		// property of the credential, not of the principal, so it is never
		// recomputed here — an identity token legitimately has none.
	}

	writeJSON(w, http.StatusOK, userInfoResponse{
		Sub:           claims.Subject,
		Email:         claims.Email,
		Name:          claims.Name,
		Tier:          claims.Tier,
		Scope:         scope,
		PrincipalType: claims.PrincipalType,
		ClientID:      userinfoClientID(clients),
		Clients:       clients,
		Roles:         roles,
		Applications:  apps,
	})
}

// userinfoPrincipal loads the token subject, returning nil (rather than an
// error) when it cannot — userinfo then answers from the token alone rather
// than failing a request that has a perfectly valid credential.
func (s *State) userinfoPrincipal(r *http.Request, subject string) *principal.Principal {
	if s.Principals == nil || subject == "" {
		return nil
	}
	p, err := s.Principals.FindByID(r.Context(), subject)
	if err != nil || p == nil || !p.Active {
		return nil
	}
	return p
}

// userinfoClient resolves the token's azp to its OAuth client, or nil when the
// token carries no azp or the client has since been removed.
func (s *State) userinfoClient(r *http.Request, azp *string) *auth.OAuthClient {
	if azp == nil || *azp == "" || s.OAuthClients == nil {
		return nil
	}
	c, err := s.OAuthClients.FindByClientID(r.Context(), *azp)
	if err != nil {
		return nil
	}
	return c
}

// validateBearer extracts and validates the Authorization: Bearer access
// token.
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
// wildcard.
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
