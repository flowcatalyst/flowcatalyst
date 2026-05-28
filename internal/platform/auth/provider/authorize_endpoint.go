package provider

import (
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/ory/fosite"
)

// AuthorizeEndpoint serves GET/POST /oauth/authorize. It implements the
// authorization-code flow's front channel:
//
//  1. The client redirects the user-agent here with response_type=code +
//     client_id + redirect_uri + scope + code_challenge (PKCE).
//  2. We validate the request via fosite, then check whether the user is
//     already authenticated (session cookie / OIDC callback redirect from
//     /oauth/oidc/login).
//  3. If authenticated, populate the session with our Claims and ask
//     fosite to mint an authorize_code + redirect the user-agent back to
//     redirect_uri.
//
// The interactive consent step is owned by the front-end — this endpoint
// auto-grants the requested scopes if they're a subset of the client's
// allowed scopes (matching the Rust impl). Users without a session are
// redirected to /oauth/oidc/login (or /login for INTERNAL flows) with a
// returnUrl that lands back here once authentication completes.
type AuthorizeEndpoint struct{ provider *Provider }

// NewAuthorizeEndpoint wires the handler.
func NewAuthorizeEndpoint(p *Provider) *AuthorizeEndpoint { return &AuthorizeEndpoint{provider: p} }

// RegisterRoutes mounts GET/POST /oauth/authorize.
func (e *AuthorizeEndpoint) RegisterRoutes(r chi.Router) {
	r.Get("/oauth/authorize", e.handle)
	r.Post("/oauth/authorize", e.handle)
}

// SessionPrincipalResolver returns the authenticated principal ID for
// the request, or "" if the user isn't signed in. The platform wires
// this against its session-cookie + bearer-token middleware so the
// authorize endpoint can look up an established session without
// duplicating the auth-middleware logic.
type SessionPrincipalResolver func(r *http.Request) string

func (e *AuthorizeEndpoint) handle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	session := NewSession()

	authorizeRequest, err := e.provider.OAuth2.NewAuthorizeRequest(ctx, r)
	if err != nil {
		e.provider.OAuth2.WriteAuthorizeError(ctx, w, authorizeRequest, err)
		return
	}

	// Resolve the authenticated principal. If unknown, bounce to the
	// login page with the original authorize-request URL preserved so we
	// can resume after auth completes.
	principalID := e.provider.SessionResolver(r)
	if principalID == "" {
		// Defer to the OIDC bridge's login start endpoint. Path + param
		// name match Rust's fc-platform /auth/oidc/login?return_url=…
		login := "/auth/oidc/login?return_url=" + url.QueryEscape(r.URL.String())
		http.Redirect(w, r, login, http.StatusFound)
		return
	}

	claims, err := BuildClaims(ctx, e.provider.cfg, e.provider.principals, e.provider.roles, principalID)
	if err != nil {
		e.provider.OAuth2.WriteAuthorizeError(ctx, w, authorizeRequest,
			fosite.ErrServerError.WithWrap(err).WithDescription("Principal resolution failed."))
		return
	}
	// Hydrate OIDC ID-token-only claims from the authorize request.
	// nonce echoes back what the client supplied; azp is the client_id
	// when there's a single audience (RFC 7519 / OIDC Core 2). auth_time
	// is now — Rust uses the same: the moment we mint the token is the
	// canonical auth_time for a fresh session.
	if nonce := authorizeRequest.GetRequestForm().Get("nonce"); nonce != "" {
		claims.Nonce = nonce
	}
	if c := authorizeRequest.GetClient(); c != nil {
		claims.AuthorizedParty = c.GetID()
	}
	claims.AuthTime = time.Now().Unix()
	session.applyClaims(claims)

	// Auto-grant requested scopes that are within the client's allowed
	// set — matches the Rust impl, which doesn't interpose a consent
	// screen for first-party clients. If you need a consent screen,
	// short-circuit here and redirect to /consent with the authorize
	// state stashed in oauth_oidc_login_states.
	for _, s := range authorizeRequest.GetRequestedScopes() {
		authorizeRequest.GrantScope(s)
	}
	for _, a := range authorizeRequest.GetRequestedAudience() {
		authorizeRequest.GrantAudience(a)
	}

	response, err := e.provider.OAuth2.NewAuthorizeResponse(ctx, authorizeRequest, session)
	if err != nil {
		e.provider.OAuth2.WriteAuthorizeError(ctx, w, authorizeRequest, err)
		return
	}
	e.provider.OAuth2.WriteAuthorizeResponse(ctx, w, authorizeRequest, response)
}
