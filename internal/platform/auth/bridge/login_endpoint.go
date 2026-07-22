package bridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/oauthapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// LoginEndpoint serves the OIDC bridge HTTP surface:
//
//	GET  /auth/oidc/login
//	GET  /auth/oidc/callback
//
// The paths match Rust fc-platform (`/auth/oidc/*`) so the frontend and
// Rust clients work against either backend without per-implementation
// hacks.
//
// /auth/check-domain is owned by the login package, not the bridge —
// it does the email-domain-mapping lookup and only needs the bridge for
// the redirect URL it returns.
//
// The handlers do the redirect-and-exchange dance; the per-request
// session-cookie write (and any consent UI) is owned by the
// SessionWriter callback the platform server installs.
type LoginEndpoint struct {
	bridge       *Bridge
	states       *LoginStateRepo
	principals   *principal.Repository
	mappings     *emaildomainmapping.Repository
	roles        *role.Repository
	idpMappings  *auth.IdpRoleMappingRepo
	uow          *usecasepgx.UnitOfWork
	oauthClients *auth.OAuthClientRepo

	// SessionWriter is called after the callback exchange completes
	// successfully. It receives the resolved principal ID + the
	// redirect-back URL the front-channel should land on next, and must
	// either set a session cookie + redirect, or render a server-rendered
	// response. The default implementation just emits a 200 with the
	// principal ID — replace at startup.
	SessionWriter func(w http.ResponseWriter, r *http.Request, principalID string, returnURL string)

	// ExternalBaseURL, when set (wire.go supplies the configured public
	// base), pins the OIDC callback URL the IdP redirects to. Without it
	// the URL falls back to being derived from X-Forwarded-Proto/Host —
	// forwardable, client-controllable headers that don't belong in a
	// security-relevant URL. Set at startup alongside SessionWriter.
	ExternalBaseURL string

	// CookieSecure mirrors the flag the SessionWriter sets on the session
	// cookie. The logout clear must carry the same attributes as the set —
	// some browsers treat a Secure and a non-Secure cookie of the same name
	// as distinct, leaving the real session cookie standing after "logout".
	CookieSecure bool
}

// NewLoginEndpoint wires the bridge HTTP handlers. The mappings repo
// and UoW power the auto-provisioning path in handleCallback — when a
// successful OIDC handshake yields an email that doesn't match a
// FlowCatalyst principal, the bridge creates one using the scope and
// primary-client-id carried by the matching email-domain mapping. The
// idpMappings repo + roles repo power the IDP role-sync that runs on
// every callback (both new and existing users): the roles claim is
// translated through oauth_idp_role_mappings, filtered through the
// mapping's allowed_role_ids, and applied with source=IDP_SYNC so
// admin-assigned roles aren't trampled. Drop-in parity with Rust's
// sync_oidc_login_with_allowed_roles.
func NewLoginEndpoint(
	b *Bridge,
	states *LoginStateRepo,
	principals *principal.Repository,
	mappings *emaildomainmapping.Repository,
	roles *role.Repository,
	idpMappings *auth.IdpRoleMappingRepo,
	uow *usecasepgx.UnitOfWork,
	oauthClients *auth.OAuthClientRepo,
) *LoginEndpoint {
	return &LoginEndpoint{
		bridge:       b,
		states:       states,
		principals:   principals,
		mappings:     mappings,
		roles:        roles,
		idpMappings:  idpMappings,
		uow:          uow,
		oauthClients: oauthClients,
		SessionWriter: func(w http.ResponseWriter, _ *http.Request, principalID string, returnURL string) {
			if returnURL != "" {
				http.Redirect(w, nil, returnURL, http.StatusFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"principalId": principalID})
		},
	}
}

// RegisterRoutes mounts the OIDC bridge endpoints. /auth/check-domain
// is intentionally NOT registered here — the login package owns that
// path and uses identity_provider / email_domain_mapping to compute
// the redirect URL the SPA follows back into this bridge.
func (e *LoginEndpoint) RegisterRoutes(r chi.Router) {
	r.Get("/auth/oidc/login", e.handleLogin)
	r.Get("/auth/oidc/callback", e.handleCallback)
	r.Get("/auth/oidc/session/end", e.handleSessionEnd)
}

// handleSessionEnd implements OIDC RP-Initiated Logout 1.0. It always clears
// the fc_session cookie, and when a post_logout_redirect_uri is supplied it
// verifies the URI is registered for the requesting client before redirecting.
// The client is identified via the `aud` claim of id_token_hint, or via an
// explicit client_id parameter (the spec-sanctioned alternative) when no hint
// is sent. The registered whitelist — matched by MatchRedirectURI,
// which is URL-component-aware and blocks host-confusion — is the security
// boundary (not the token signature, which is intentionally not verified), so a
// URI we cannot tie to a client's whitelist is refused rather than redirected
// to (CWE-601 open-redirect defence).
func (e *LoginEndpoint) handleSessionEnd(w http.ResponseWriter, r *http.Request) {
	// Always clear the session cookie — with the same attributes the
	// SessionWriter set it with (notably Secure), or the clear may not
	// match the original cookie.
	http.SetCookie(w, &http.Cookie{
		Name:     platformmw.SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   e.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})

	q := r.URL.Query()
	redirectURI := q.Get("post_logout_redirect_uri")
	idTokenHint := q.Get("id_token_hint")
	clientIDParam := q.Get("client_id")
	state := q.Get("state")

	// No redirect requested — session ended.
	if redirectURI == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "Session ended"})
		return
	}

	reject := func(reason string) {
		slog.Warn("rejected post_logout_redirect_uri", "redirect_uri", redirectURI, "reason", reason)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error":             "invalid_request",
			"error_description": "Invalid post_logout_redirect_uri: " + reason,
		})
	}

	// Identify the requesting client to scope the post_logout_redirect_uri
	// whitelist check. Prefer the id_token_hint's aud (OIDC RP-Initiated
	// Logout), and accept an explicit client_id as the spec-sanctioned
	// alternative when no hint is supplied. The registered whitelist — not the
	// hint — is the security boundary, and the hint is not signature-verified,
	// so either identifier is equally acceptable here.
	clientID := ""
	if idTokenHint != "" {
		clientID = extractAudFromIDTokenHint(idTokenHint)
	}
	if clientID == "" {
		clientID = clientIDParam
	}
	if clientID == "" {
		reject("id_token_hint or client_id is required to verify post_logout_redirect_uri")
		return
	}
	client, err := e.oauthClients.FindByClientID(r.Context(), clientID)
	if err != nil {
		reject("internal error verifying client")
		return
	}
	if client == nil {
		reject("id_token_hint audience does not match any registered client")
		return
	}
	if !oauthapi.MatchRedirectURI(redirectURI, client.PostLogoutRedirectURIs) {
		reject("not in the client's registered post_logout_redirect_uris")
		return
	}

	target := redirectURI
	if state != "" {
		sep := "?"
		if strings.Contains(target, "?") {
			sep = "&"
		}
		target += sep + "state=" + url.QueryEscape(state)
	}
	http.Redirect(w, r, target, http.StatusSeeOther) //nolint:gosec // G710: post-login redirect to the validated redirect target
}

// extractAudFromIDTokenHint pulls the `aud` (client_id) claim from an
// id_token_hint WITHOUT verifying its signature — the registered
// post_logout_redirect_uris whitelist is the security boundary, not the
// hint. Returns "" on any structural malformation. 1:1 with Rust
// extract_aud_from_id_token_hint. `aud` may be a string or an array of
// strings (OIDC Core §2); for an array the first entry is used.
func extractAudFromIDTokenHint(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Aud json.RawMessage `json:"aud"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	var s string
	if err := json.Unmarshal(claims.Aud, &s); err == nil {
		return s
	}
	var arr []string
	if err := json.Unmarshal(claims.Aud, &arr); err == nil && len(arr) > 0 {
		return arr[0]
	}
	return ""
}

// handleLogin starts an OIDC login. Two entry modes:
//
//   - ?domain=X — home-realm discovery via email-domain mapping (the SPA
//     login path for client-employee SSO). Matches Rust's signature
//     (snake_case `return_url`, `domain` over `email` so users don't leak
//     the local part).
//   - ?provider_id=idp_… — provider-direct (docs/portal-identity-plan.md): a
//     portal app names the upstream IdP explicitly; no email-domain mapping
//     is consulted. The login state carries an EMPTY mapping id, which is
//     what routes the callback down the portal (null-client JIT) path.
//
// Both modes generate state/nonce/PKCE verifier, persist the state row, then
// 302-redirect to the IDP's authorize URL.
func (e *LoginEndpoint) handleLogin(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	providerID := q.Get("provider_id")
	domain := q.Get("domain")
	if domain == "" {
		// Back-compat: also accept ?email= and derive the domain.
		if email := q.Get("email"); email != "" {
			domain = emailDomain(email)
		}
	}
	returnURL := q.Get("return_url")
	if returnURL == "" {
		returnURL = q.Get("returnUrl") // tolerate camelCase legacy callers
	}
	if domain == "" && providerID == "" {
		httperror.Write(w, httperror.BadRequest("DOMAIN_REQUIRED", "domain or provider_id query param is required"))
		return
	}

	var (
		oidcClient *resolved
		idpID      string
		mappingID  string
	)
	if providerID != "" {
		res, idp, err := e.bridge.ResolveByProviderID(r.Context(), providerID)
		if err != nil {
			httperror.Write(w, usecase.Internal("OIDC_RESOLVE_FAILED",
				"OIDC could not be initialised for this provider", err))
			return
		}
		oidcClient, idpID = res, idp.ID
		// mappingID stays "" — the provider-direct marker the callback keys on.
	} else {
		// Resolve uses email; synthesise one with a throwaway local-part.
		res, idp, mapping, err := e.bridge.ResolveForEmail(r.Context(), "x@"+domain)
		if err != nil {
			// A real failure (client-secret decrypt, issuer discovery, lookup) —
			// surface it as an init error with the cause logged, not a misleading
			// "not configured". A domain genuinely lacking an OIDC provider is the
			// resolved==nil case below.
			httperror.Write(w, usecase.Internal("OIDC_RESOLVE_FAILED",
				"OIDC could not be initialised for this domain", err))
			return
		}
		if res == nil {
			httperror.Write(w, httperror.BadRequest("OIDC_NOT_CONFIGURED",
				"OIDC is not configured for this domain"))
			return
		}
		oidcClient, idpID, mappingID = res, idp.ID, mapping.ID
	}

	state := randString(32)
	nonce := randString(32)
	verifier := randString(64)
	challenge := pkceChallenge(verifier)
	loginState := NewLoginState(state, domain, idpID, mappingID, nonce, verifier)
	if returnURL != "" {
		loginState.ReturnURL = &returnURL
	}
	// OAuth chaining: when this SSO login was started inside an /oauth/authorize
	// flow (a downstream app using FlowCatalyst as its IdP), the SPA forwards the
	// OAuth request params here so the callback can resume /oauth/authorize and
	// issue the code back to the app. 1:1 with Rust's OidcLoginParams.
	loginState.OAuthClientID = optParam(q, "oauth_client_id")
	loginState.OAuthRedirectURI = optParam(q, "oauth_redirect_uri")
	loginState.OAuthScope = optParam(q, "oauth_scope")
	loginState.OAuthState = optParam(q, "oauth_state")
	loginState.OAuthCodeChallenge = optParam(q, "oauth_code_challenge")
	loginState.OAuthCodeChallengeMethod = optParam(q, "oauth_code_challenge_method")
	loginState.OAuthNonce = optParam(q, "oauth_nonce")
	if err := e.states.Insert(r.Context(), loginState); err != nil {
		httperror.Write(w, usecase.Internal("OIDC_STATE", "persist state failed", err))
		return
	}

	redirectURI := e.absoluteCallbackURL(r)
	authURL := oidcClient.AuthCodeURL(state, redirectURI) +
		"&nonce=" + url.QueryEscape(nonce) +
		"&code_challenge=" + url.QueryEscape(challenge) +
		"&code_challenge_method=S256"
	http.Redirect(w, r, authURL, http.StatusFound) //nolint:gosec // G710: authURL is the upstream IdP authorize URL resolved from server config, not user input
}

// handleCallback completes the login: validate state, exchange code,
// verify ID token, resolve/create the FlowCatalyst principal, and hand
// off to SessionWriter.
func (e *LoginEndpoint) handleCallback(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	code := r.URL.Query().Get("code")
	if state == "" || code == "" {
		httperror.Write(w, httperror.BadRequest("MISSING_PARAM", "state and code are required"))
		return
	}

	// Atomically consume the state: single-use, replay-proof, and rejects an
	// expired row — all in one DELETE … RETURNING. Consuming up-front (rather
	// than deleting at the end) means a failed callback can't be retried with
	// the same state, matching Rust.
	loginState, err := e.states.Consume(r.Context(), state)
	if err != nil {
		httperror.Write(w, usecase.Internal("OIDC_STATE", "lookup state failed", err))
		return
	}
	if loginState == nil {
		// Unknown, already-consumed (replay), or expired — indistinguishable by
		// design, all rejected.
		httperror.Write(w, httperror.BadRequest("INVALID_STATE", "unknown or expired login session"))
		return
	}

	// Re-resolve the OIDC client the same way the login started: an empty
	// mapping id marks a provider-direct (portal) login, which never consults
	// the email-domain mapping table. providerDirect drives the trust-binding
	// and provisioning branches below.
	providerDirect := loginState.EmailDomainMappingID == ""
	var (
		oidcClient *resolved
		idp        *identityprovider.IdentityProvider
		mapping    *emaildomainmapping.EmailDomainMapping
	)
	if providerDirect {
		res, resolvedIdp, err := e.bridge.ResolveByProviderID(r.Context(), loginState.IdentityProviderID)
		if err != nil {
			httperror.Write(w, usecase.Internal("OIDC_RESOLVE_FAILED",
				"OIDC could not be initialised for this provider", err))
			return
		}
		oidcClient, idp = res, resolvedIdp
	} else {
		res, _, resolvedMapping, err := e.bridge.ResolveForEmail(r.Context(),
			"x@"+loginState.EmailDomain)
		if err != nil {
			httperror.Write(w, usecase.Internal("OIDC_RESOLVE_FAILED",
				"OIDC could not be initialised for this domain", err))
			return
		}
		if res == nil || resolvedMapping == nil {
			httperror.Write(w, httperror.BadRequest("OIDC_NOT_CONFIGURED",
				"OIDC is not configured for this domain"))
			return
		}
		oidcClient, mapping = res, resolvedMapping
	}

	redirectURI := e.absoluteCallbackURL(r)
	tok, err := oidcClient.Exchange(r.Context(), code, redirectURI, loginState.CodeVerifier)
	if err != nil {
		httperror.Write(w, usecase.Internal("OIDC_EXCHANGE", "code exchange failed", err))
		return
	}
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		httperror.Write(w, httperror.BadRequest("NO_ID_TOKEN", "IDP did not return id_token"))
		return
	}
	idToken, err := oidcClient.VerifyIDToken(r.Context(), rawIDToken)
	if err != nil {
		httperror.Write(w, usecase.Authorization("OIDC_VERIFY", "id_token verification failed: "+err.Error()))
		return
	}
	var claims struct {
		Email             string   `json:"email"`
		PreferredUsername string   `json:"preferred_username"`
		Tid               string   `json:"tid"`
		Nonce             string   `json:"nonce"`
		Roles             []string `json:"roles"`
	}
	if err := idToken.Claims(&claims); err != nil {
		httperror.Write(w, httperror.BadRequest("OIDC_CLAIMS", "id_token claims malformed"))
		return
	}
	if claims.Nonce != loginState.Nonce {
		httperror.Write(w, usecase.Authorization("NONCE_MISMATCH", "nonce did not match"))
		return
	}

	// Resolve the user's email — some IdPs only populate preferred_username.
	email := claims.Email
	if email == "" {
		email = claims.PreferredUsername
	}
	if email == "" {
		httperror.Write(w, usecase.Authorization("NO_EMAIL", "id_token has no email / preferred_username claim"))
		return
	}

	// Reject Entra external/guest accounts (#EXT# UPNs): their identity is owned
	// by another organisation and falls outside this domain's trust boundary.
	// 1:1 with Rust.
	if strings.Contains(strings.ToLower(email), "#ext#") {
		httperror.Write(w, usecase.Authorization("EXTERNAL_GUEST", "external guest accounts are not supported"))
		return
	}

	// Tenant binding — which check applies depends on how the login started:
	//
	// Mapping-based (SPA home-realm) logins, 1:1 with Rust: a multi-tenant
	// IdP's shared keys sign tokens from ANY tenant, so pin the token two ways:
	//   1. The token's email domain MUST equal the login domain — an email
	//      domain is verified inside its owning tenant.
	//   2. If the mapping pins an explicit tenant (required_oidc_tenant_id), the
	//      token's `tid` claim MUST match it exactly.
	//
	// Provider-direct (portal) logins have no login domain and no mapping; the
	// binding is the IdP's own allowed_email_domains list. ResolveByProviderID
	// already guarantees the list is non-empty for multi-tenant IdPs; for a
	// single-tenant IdP an empty list means "any account at this IdP", which
	// the IdP itself bounds.
	if providerDirect {
		if len(idp.AllowedEmailDomains) > 0 && !domainAllowed(emailDomain(email), idp.AllowedEmailDomains) {
			httperror.Write(w, usecase.Authorization("EMAIL_DOMAIN_MISMATCH",
				"the token's email domain is not allowed for this identity provider"))
			return
		}
	} else {
		if !strings.EqualFold(emailDomain(email), loginState.EmailDomain) {
			httperror.Write(w, usecase.Authorization("EMAIL_DOMAIN_MISMATCH",
				"the token's email domain does not match the login domain"))
			return
		}
		if mapping.RequiredOIDCTenantID != nil && *mapping.RequiredOIDCTenantID != "" {
			if claims.Tid == "" {
				httperror.Write(w, usecase.Authorization("TENANT_MISMATCH", "id_token has no tenant id (tid) claim"))
				return
			}
			if claims.Tid != *mapping.RequiredOIDCTenantID {
				httperror.Write(w, usecase.Authorization("TENANT_MISMATCH", "id_token tenant does not match the configured tenant"))
				return
			}
		}
	}

	// Resolve or create the FlowCatalyst principal. Drop-in parity with
	// Rust's sync_oidc_login_with_allowed_roles: lookup by email; if
	// missing, auto-provision using the scope + primary-client-id from
	// the email-domain mapping. Then translate IDP roles → platform
	// roles (filtered by the mapping's allowed_role_ids) and apply via
	// SyncIdpRoles. Existing users get the same role sync — if HR
	// removed someone from a group upstream, their next login drops the
	// corresponding platform role.
	p, err := e.principals.FindByEmail(r.Context(), email)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "principal lookup failed", err))
		return
	}
	if p == nil {
		if providerDirect {
			// Portal JIT provisioning: an INERT identity (null-client, no
			// roles, no app access — see CreatePortalUser). What this user may
			// do is entirely the consuming portal app's membership data; the
			// platform only records that the person authenticated.
			p, err = e.autoProvisionPortal(r.Context(), email)
		} else {
			p, err = e.autoProvision(r.Context(), email, loginState.EmailDomainMappingID)
		}
		if err != nil {
			httperror.Write(w, err)
			return
		}
	} else if herr := e.principals.LowercaseEmail(r.Context(), p); herr != nil {
		// Self-heal a legacy mixed-case row in place. Non-fatal: the user is
		// already authenticated; if the write fails we just retry next login.
		slog.Warn("OIDC login: email lowercase self-heal failed; continuing",
			"principalId", p.ID, "err", herr)
	}
	// IDP→platform role sync is a mapping-flow feature: provider-direct logins
	// have no mapping (and portal roles are portal-side data), so skipping it
	// also guarantees a portal IdP can never grant platform roles.
	if !providerDirect {
		if err := e.syncIdpRoles(r.Context(), p, claims.Roles, loginState.EmailDomainMappingID); err != nil {
			// Role sync failure shouldn't block login — the principal is
			// already valid. Log and continue with whatever role set is in
			// place. Mirrors Rust's behaviour where the role sync is a
			// best-effort step after auth.
			slog.Warn("OIDC role sync failed; continuing without role update",
				"principalId", p.ID, "err", err)
		}
	}

	// The state row was already consumed atomically up-front, so no cleanup here.

	// Decide where to send the now-authenticated user (the SessionWriter sets the
	// session cookie first, then redirects here):
	//   1. a chained OAuth login (started inside /oauth/authorize) resumes that
	//      flow so the downstream app receives its code;
	//   2. otherwise a safe same-site relative return_url (absolute / "//host" /
	//      backslash forms are open-redirect vectors and are dropped);
	//   3. otherwise the dashboard, so a plain SSO login lands somewhere useful.
	target := "/dashboard"
	switch {
	case loginState.OAuthClientID != nil && *loginState.OAuthClientID != "":
		target = buildAuthorizeRedirect(loginState)
	case loginState.ReturnURL != nil:
		if safe := safeRelativeReturnURL(*loginState.ReturnURL); safe != "" {
			target = safe
		}
	}
	e.SessionWriter(w, r, p.ID, target)
}

// buildAuthorizeRedirect resumes a chained OAuth flow: a relative /oauth/authorize
// URL carrying the stored OAuth request params, so the downstream app's code is
// issued once the session cookie is in place. 1:1 with Rust determine_redirect_url.
func buildAuthorizeRedirect(s *OIDCLoginState) string {
	u := "/oauth/authorize?response_type=code&client_id=" + url.QueryEscape(*s.OAuthClientID)
	add := func(key string, val *string) {
		if val != nil && *val != "" {
			u += "&" + key + "=" + url.QueryEscape(*val)
		}
	}
	add("redirect_uri", s.OAuthRedirectURI)
	add("scope", s.OAuthScope)
	add("state", s.OAuthState)
	add("code_challenge", s.OAuthCodeChallenge)
	add("code_challenge_method", s.OAuthCodeChallengeMethod)
	add("nonce", s.OAuthNonce)
	return u
}

// optParam returns a pointer to q[key], or nil when the param is absent/empty.
func optParam(q url.Values, key string) *string {
	if v := q.Get(key); v != "" {
		return &v
	}
	return nil
}

// safeRelativeReturnURL returns u only when it is a safe same-site relative
// path — it must start with a single "/" and not "//" or "/\" (which browsers
// treat as host-relative and would redirect off-site). Anything else → "".
func safeRelativeReturnURL(u string) string {
	if u == "" || u[0] != '/' || strings.HasPrefix(u, "//") || strings.HasPrefix(u, "/\\") {
		return ""
	}
	return u
}

// autoProvision creates a Principal for `email` using the scope +
// primary-client-id carried by the EmailDomainMapping that drove this
// login. Returns the newly-created Principal, or an error suitable for
// surfacing to the user. The mapping ID is the same one the bridge
// resolved at login-time and persisted in the login_state row.
//
// Roles are intentionally NOT assigned here. Rust calls
// sync_oidc_login_with_allowed_roles to apply IDP-claim-derived role
// mappings; that step is a follow-up. The provisioned user has no
// roles and will only be useful for flows that don't depend on
// platform-permission gating (typical first-login UX).
func (e *LoginEndpoint) autoProvision(ctx context.Context, email, mappingID string) (*principal.Principal, error) {
	mapping, err := e.mappings.FindByID(ctx, mappingID)
	if err != nil {
		return nil, usecase.Internal("REPO", "email_domain_mapping lookup failed", err)
	}
	if mapping == nil {
		return nil, usecase.Authorization("MAPPING_GONE",
			"The email-domain mapping that drove this login no longer exists; cannot auto-provision")
	}

	idpType := "OIDC"
	cmd := principalops.CreateCommand{
		Email:    email,
		Scope:    string(mapping.ScopeType),
		ClientID: mapping.PrimaryClientID,
		IDPType:  &idpType,
	}
	// The execution context's PrincipalID is empty — the new user is
	// being created by the system in response to a self-service login,
	// not by an authenticated actor. Audit rows will record an empty
	// principal, matching the Rust convention for self-provisioning.
	ec := usecase.NewExecutionContext("")
	event, err := usecaseop.Run(ctx, e.uow, principalops.CreateUser(e.principals), cmd, ec)
	if err != nil {
		return nil, err
	}
	created, err := e.principals.FindByID(ctx, event.UserID)
	if err != nil {
		return nil, usecase.Internal("REPO", "post-create principal lookup failed", err)
	}
	if created == nil {
		// Shouldn't happen — Persist just succeeded.
		return nil, usecase.Internal("REPO", "post-create principal missing", errors.New("not found"))
	}
	return created, nil
}

// autoProvisionPortal creates the inert portal identity for a
// provider-direct login (CreatePortalUser: scope CLIENT, client_id NULL, no
// roles, AllApplications=false). Same system-actor convention as
// autoProvision: an empty ExecutionContext principal id.
func (e *LoginEndpoint) autoProvisionPortal(ctx context.Context, email string) (*principal.Principal, error) {
	provider := "OIDC"
	cmd := principalops.CreatePortalUserCommand{
		Email:    email,
		Provider: &provider,
	}
	ec := usecase.NewExecutionContext("")
	event, err := usecaseop.Run(ctx, e.uow, principalops.CreatePortalUser(e.principals), cmd, ec)
	if err != nil {
		return nil, err
	}
	created, err := e.principals.FindByID(ctx, event.UserID)
	if err != nil {
		return nil, usecase.Internal("REPO", "post-create principal lookup failed", err)
	}
	if created == nil {
		return nil, usecase.Internal("REPO", "post-create principal missing", errors.New("not found"))
	}
	return created, nil
}

// domainAllowed reports whether the email domain is in the IdP's
// allowed_email_domains list (case-insensitive).
func domainAllowed(domain string, allowed []string) bool {
	for _, a := range allowed {
		if strings.EqualFold(domain, a) {
			return true
		}
	}
	return false
}

// syncIdpRoles translates the IDP `roles` claim through
// oauth_idp_role_mappings, filters by the EmailDomainMapping's
// allowed_role_ids (when non-empty), and applies the resulting
// platform-role set with source=IDP_SYNC. Preserves admin-assigned
// roles untouched.
//
// An empty claim is a valid input: the user lost every group
// upstream, so all their IDP-sourced platform roles should drop. The
// caller treats any error here as non-fatal — the principal is
// already authenticated; we just log and continue.
func (e *LoginEndpoint) syncIdpRoles(ctx context.Context, p *principal.Principal, idpRoles []string, mappingID string) error {
	mapping, err := e.mappings.FindByID(ctx, mappingID)
	if err != nil {
		return usecase.Internal("REPO", "email_domain_mapping lookup failed", err)
	}
	if mapping == nil {
		// Mapping vanished between login and callback. Drop all
		// IDP-sourced roles defensively — we can't validate the claim
		// without a mapping.
		return e.applySyncIdpRoles(ctx, p, nil)
	}

	// Load every IDP role mapping; in-memory filter. Mirrors Rust's
	// find_idp_role_mapping which doesn't filter by IDP type either.
	allMappings, err := e.idpMappings.FindAll(ctx)
	if err != nil {
		return usecase.Internal("REPO", "idp_role_mappings list failed", err)
	}
	byIdpRoleName := make(map[string]string, len(allMappings))
	for _, m := range allMappings {
		byIdpRoleName[m.IdpRoleName] = m.PlatformRoleName
	}

	allowed := mapping.AllowedRoleIDs
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, n := range allowed {
		allowedSet[n] = struct{}{}
	}
	hasAllowList := len(allowed) > 0

	authorized := make(map[string]struct{}, len(idpRoles))
	for _, idpRole := range idpRoles {
		platformRole, ok := byIdpRoleName[idpRole]
		if !ok {
			// Unknown role — Rust logs this at warn as a security
			// rejection. Match that (minus the email: the principal ID
			// already identifies the user without putting PII in the logs).
			slog.Warn("REJECTED unauthorized IDP role: not found in idp_role_mappings",
				"principalId", p.ID, "idpRole", idpRole)
			continue
		}
		if hasAllowList {
			if _, ok := allowedSet[platformRole]; !ok {
				slog.Debug("skipped IDP role: not in email_domain_mapping allowed_role_ids",
					"principalId", p.ID, "idpRole", idpRole, "platformRole", platformRole)
				continue
			}
		}
		authorized[platformRole] = struct{}{}
	}
	platformRoles := make([]string, 0, len(authorized))
	for r := range authorized {
		platformRoles = append(platformRoles, r)
	}
	return e.applySyncIdpRoles(ctx, p, platformRoles)
}

func (e *LoginEndpoint) applySyncIdpRoles(ctx context.Context, p *principal.Principal, platformRoles []string) error {
	cmd := principalops.SyncIdpRolesCommand{
		UserID:        p.ID,
		PlatformRoles: platformRoles,
	}
	// The actor here is the system, not an authenticated user — match
	// the auto-provision pattern of an empty principal id.
	ec := usecase.NewExecutionContext("")
	if _, err := usecaseop.Run(ctx, e.uow, principalops.SyncIdpRoles(e.principals, e.roles), cmd, ec); err != nil {
		return err
	}
	return nil
}

// ── helpers ──────────────────────────────────────────────────────────────

// absoluteCallbackURL derives the public-facing /auth/oidc/callback URL the
// IDP will redirect to. Prefers the configured ExternalBaseURL — request
// headers (X-Forwarded-Proto/Host, Host) are forwardable and client-
// controllable, so deriving a security-relevant URL from them is a last
// resort kept only for unconfigured dev setups. (The blast radius of a
// spoofed host is bounded — IdPs only redirect to registered URIs — but the
// header trust is gratuitous when the public base is known.)
func (e *LoginEndpoint) absoluteCallbackURL(r *http.Request) string {
	if e.ExternalBaseURL != "" {
		return strings.TrimRight(e.ExternalBaseURL, "/") + "/auth/oidc/callback"
	}
	scheme := "https"
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		scheme = fwd
	} else if r.TLS == nil {
		scheme = "http"
	}
	host := r.Host
	if fwd := r.Header.Get("X-Forwarded-Host"); fwd != "" {
		host = fwd
	}
	return scheme + "://" + host + "/auth/oidc/callback"
}

// randString returns a URL-safe base64 string with at least n bytes of
// entropy. Crypto-grade.
func randString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// pkceChallenge returns the S256 PKCE challenge for the given verifier.
func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
