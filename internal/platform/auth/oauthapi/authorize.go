package oauthapi

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
)

// RegisterAuthorizeRoutes mounts GET /oauth/authorize. It MUST be mounted
// OUTSIDE the session-auth middleware: an absent/expired session is a
// redirect-to-login case here, not a 401.
func (s *State) RegisterAuthorizeRoutes(r chi.Router) {
	r.Get("/oauth/authorize", s.Authorize)
}

// Authorize is GET /oauth/authorize (OAuth2 authorization-code flow with
// PKCE), a 1:1 port of oauth_api.rs::authorize. When the caller already
// has a valid session it issues a code and redirects to redirect_uri;
// otherwise it stashes the request and redirects to the SPA login page.
func (s *State) Authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	responseType := q.Get("response_type")
	clientID := q.Get("client_id")
	redirectURI := q.Get("redirect_uri")
	scope := q.Get("scope")
	stateParam := q.Get("state")
	nonce := q.Get("nonce")
	codeChallenge := q.Get("code_challenge")
	codeChallengeMethod := q.Get("code_challenge_method")
	providerID := q.Get("provider")
	prompt := q.Get("prompt")
	maxAge := q.Get("max_age")

	// `state` is mandatory for CSRF protection on the callback. Reject with
	// 400 (not a redirect) — we can't safely bounce the UA without it.
	if strings.TrimSpace(stateParam) == "" {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "`state` parameter is required for CSRF protection")
		return
	}

	// Cluster-wide per-client_id throttle, before the DB lookup.
	if rej := ratelimit.Enforce(r.Context(), s.RateLimit, ratelimit.BucketOAuthAuthorizeClient, clientID, s.RateLimitPolicies.OAuthAuthorizeClient); rej != nil {
		ratelimit.WriteTooManyRequests(w, rej.RetryAfterSecs, "rate limit exceeded")
		return
	}

	// Resolve and validate the client + redirect_uri BEFORE any errorRedirect.
	// RFC 6749 §4.1.2.1: when the client is unknown/invalid or the redirect_uri
	// is unregistered, we MUST NOT auto-redirect the user-agent to that URI
	// (open-redirect / phishing vector). Until redirect_uri is matched against
	// the client's registered set, every failure is a direct 4xx, not a bounce.
	client, err := s.OAuthClients.FindByClientID(r.Context(), clientID)
	switch {
	case err != nil:
		writeOAuthError(w, http.StatusInternalServerError, "server_error", "Internal error")
		return
	case client == nil:
		writeOAuthError(w, http.StatusBadRequest, "unauthorized_client", "Unknown client")
		return
	case !client.Active:
		writeOAuthError(w, http.StatusBadRequest, "unauthorized_client", "Client is not active")
		return
	}
	if !MatchRedirectURI(redirectURI, client.RedirectURIs) {
		writeOAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid redirect_uri")
		return
	}

	// redirect_uri is now validated against the client — from here on, errors
	// may safely bounce the user-agent back to it with OAuth error params.
	if responseType != "code" {
		errorRedirect(w, r, redirectURI, "unsupported_response_type", "Only 'code' response type is supported", stateParam)
		return
	}
	// The authorization-code flow starts here, so the client must be
	// permitted the authorization_code grant.
	if !grantAllowed(client, "authorization_code") {
		errorRedirect(w, r, redirectURI, "unauthorized_client", "Client is not permitted to use the authorization_code grant", stateParam)
		return
	}
	if client.PKCERequired && codeChallenge == "" {
		errorRedirect(w, r, redirectURI, "invalid_request", "PKCE code_challenge is required", stateParam)
		return
	}
	// Only S256 is offered (discovery advertises S256 only). "plain" is refused
	// per the OAuth 2.0 Security BCP — it offers no protection against a code
	// interceptor who also sees the challenge.
	if codeChallengeMethod != "" && codeChallengeMethod != "S256" {
		errorRedirect(w, r, redirectURI, "invalid_request", "Only the S256 code_challenge_method is supported", stateParam)
		return
	}
	if scope != "" {
		if invalid := invalidScopes(scope, client.Scopes); len(invalid) > 0 {
			errorRedirect(w, r, redirectURI, "invalid_scope", "Invalid scope(s): "+strings.Join(invalid, ", "), stateParam)
			return
		}
	}

	// Resolve the session once. A session older than max_age must
	// re-authenticate (OIDC Core §3.1.2.1), so treat it as stale below.
	sessTok := s.sessionToken(r)
	var sessSubject string
	var sessIssuedAt time.Time
	sessOK := false
	if sessTok != "" && s.ValidateSession != nil {
		sessSubject, sessIssuedAt, sessOK = s.ValidateSession(sessTok)
	}
	sessionStale := sessOK && maxAgeExceeded(maxAge, sessIssuedAt)

	// prompt handling (OIDC Core §3.1.2.1).
	forceLogin := false
	switch prompt {
	case "none":
		// prompt=none forbids any UI — a missing or stale session is an error.
		if !sessOK || sessionStale {
			errorRedirect(w, r, redirectURI, "login_required", "User is not authenticated", stateParam)
			return
		}
	case "login":
		forceLogin = true
	}

	// Authenticated, fresh session → issue the code immediately.
	if !forceLogin && sessOK && !sessionStale {
		code := grantstore.NewAuthorizationCode(randomString(64), clientID, sessSubject, redirectURI)
		code.Scope = strPtrOrNil(scope)
		code.Nonce = strPtrOrNil(nonce)
		code.State = strPtrOrNil(stateParam)
		if codeChallenge != "" {
			// Persist the challenge binding unconditionally so PKCE can never be
			// silently stripped by omitting the method — which would otherwise
			// pass the PKCERequired gate above yet store nothing, letting an
			// intercepted code be redeemed without a verifier. An absent method
			// defaults to S256 (not RFC 7636's "plain", which we no longer
			// offer), so a challenge without an explicit method is treated as the
			// secure variant every modern client already uses.
			method := codeChallengeMethod
			if method == "" {
				method = "S256"
			}
			code.CodeChallenge = &codeChallenge
			code.CodeChallengeMethod = &method
		}
		if err := s.AuthCodes.Insert(r.Context(), code); err != nil {
			errorRedirect(w, r, redirectURI, "server_error", "Failed to create authorization code", stateParam)
			return
		}
		redirectURL := redirectURI + querySep(redirectURI) + "code=" + pctEncode(code.Code) + "&state=" + pctEncode(stateParam)
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect) //nolint:gosec // G710: redirectURL is built from a redirect_uri already validated against the client's registered URIs
		return
	}

	if providerID != "" {
		// Direct-IdP entry (docs/portal-identity-plan.md): the downstream app
		// pre-selected an upstream IdP (e.g. a portal's "Login with Acme SSO"
		// button), so skip the SPA login page and chain straight into the OIDC
		// bridge. The bridge stashes these oauth_* params in its login state
		// and, after the IdP callback establishes the session, resumes this
		// /oauth/authorize request so the app receives its code — the same
		// chaining contract the SPA uses. An unknown/misconfigured provider is
		// rejected by the bridge at login start (fail closed, no IdP redirect).
		bridgeURL := "/auth/oidc/login?provider_id=" + pctEncode(providerID) +
			"&oauth_client_id=" + pctEncode(clientID) +
			"&oauth_redirect_uri=" + pctEncode(redirectURI) +
			"&oauth_state=" + pctEncode(stateParam)
		if scope != "" {
			bridgeURL += "&oauth_scope=" + pctEncode(scope)
		}
		if codeChallenge != "" {
			bridgeURL += "&oauth_code_challenge=" + pctEncode(codeChallenge)
		}
		if codeChallengeMethod != "" {
			bridgeURL += "&oauth_code_challenge_method=" + pctEncode(codeChallengeMethod)
		}
		if nonce != "" {
			bridgeURL += "&oauth_nonce=" + pctEncode(nonce)
		}
		http.Redirect(w, r, bridgeURL, http.StatusTemporaryRedirect)
		return
	}

	// Not authenticated → stash the request and bounce to the SPA login page.
	// (The provider-direct branch above skips the stash: the bridge's login
	// state carries the whole OAuth chain, and nothing consumes pending-auth
	// rows — the stash exists for wire parity with Rust.)
	pending := &grantstore.PendingAuth{
		ClientID:            clientID,
		RedirectURI:         redirectURI,
		Scope:               strPtrOrNil(scope),
		CodeChallenge:       strPtrOrNil(codeChallenge),
		CodeChallengeMethod: strPtrOrNil(codeChallengeMethod),
		Nonce:               strPtrOrNil(nonce),
		CreatedAt:           time.Now().UTC(),
	}
	if err := s.PendingAuth.Insert(r.Context(), stateParam, pending); err != nil {
		errorRedirect(w, r, redirectURI, "server_error", "Internal error", stateParam)
		return
	}

	// Redirect to the SPA login page with the OAuth params so it can rebuild
	// the authorize URL after the user signs in.
	loginURL := "/auth/login?oauth=true&response_type=code" +
		"&client_id=" + pctEncode(clientID) +
		"&redirect_uri=" + pctEncode(redirectURI) +
		"&state=" + pctEncode(stateParam)
	if scope != "" {
		loginURL += "&scope=" + pctEncode(scope)
	}
	if codeChallenge != "" {
		loginURL += "&code_challenge=" + pctEncode(codeChallenge)
	}
	if codeChallengeMethod != "" {
		loginURL += "&code_challenge_method=" + pctEncode(codeChallengeMethod)
	}
	if nonce != "" {
		loginURL += "&nonce=" + pctEncode(nonce)
	}
	http.Redirect(w, r, loginURL, http.StatusTemporaryRedirect)
}

// sessionToken pulls the session JWT from the fc_session cookie, falling
// back to the Authorization: Bearer header (cookie takes precedence, as
// in Rust).
func (s *State) sessionToken(r *http.Request) string {
	if c, err := r.Cookie("fc_session"); err == nil && c.Value != "" {
		return c.Value
	}
	return authservice.ExtractBearerToken(r.Header.Get("Authorization"))
}

// maxAgeExceeded reports whether the OIDC max_age (seconds) has elapsed
// since the session token was issued. An absent/invalid max_age, or an
// unknown issue time, is treated as "not exceeded" (lenient — max_age is
// optional and we never want to gratuitously force re-login).
func maxAgeExceeded(maxAge string, issuedAt time.Time) bool {
	if maxAge == "" || issuedAt.IsZero() {
		return false
	}
	secs, err := strconv.Atoi(maxAge)
	if err != nil || secs < 0 {
		return false
	}
	return time.Since(issuedAt) > time.Duration(secs)*time.Second
}

// ─── helpers ─────────────────────────────────────────────────────────────

// errorRedirect bounces the user-agent back to redirect_uri with the OAuth
// error params (302-equivalent temporary redirect, matching Rust).
func errorRedirect(w http.ResponseWriter, r *http.Request, redirectURI, errCode, desc, state string) {
	url := redirectURI + querySep(redirectURI) + "error=" + pctEncode(errCode) + "&error_description=" + pctEncode(desc)
	if state != "" {
		url += "&state=" + pctEncode(state)
	}
	http.Redirect(w, r, url, http.StatusTemporaryRedirect) //nolint:gosec // G710: error redirect to the client's validated redirect_uri
}

// querySep returns the separator to append query params to redirectURI: "&"
// when the URI already carries a query component (RFC 6749 §3.1.2 permits a
// registered redirect endpoint to include its own query, which the AS must
// preserve rather than clobber with a second "?"), otherwise "?".
func querySep(redirectURI string) string {
	if strings.Contains(redirectURI, "?") {
		return "&"
	}
	return "?"
}

func invalidScopes(scope string, clientScopes []string) []string {
	standard := map[string]bool{"openid": true, "profile": true, "email": true, "offline_access": true}
	var invalid []string
	for _, sc := range strings.Fields(scope) {
		if standard[sc] {
			continue
		}
		found := false
		for _, ds := range clientScopes {
			if ds == sc {
				found = true
				break
			}
		}
		if !found {
			invalid = append(invalid, sc)
		}
	}
	return invalid
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// randomString returns a URL-safe random string built from n bytes of
// crypto entropy (base64url, no padding — length ≈ 4n/3). The previous
// charset[b%62] mapping was modulo-biased (256 % 62 ≠ 0 over-weights the
// first 8 charset entries) and silently discarded rand.Read errors; raw
// bytes through base64 are unbiased, and the panic matches the bridge's
// randString posture — a failed CSPRNG is not a condition to limp past
// while minting authorization codes.
func randomString(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// pctEncode percent-encodes per RFC 3986 (unreserved set preserved),
// matching Rust's urlencoding::encode (space → %20, not '+').
func pctEncode(s string) string {
	const hex = "0123456789ABCDEF"
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'A' && c <= 'Z', c >= 'a' && c <= 'z', c >= '0' && c <= '9',
			c == '-', c == '_', c == '.', c == '~':
			b.WriteByte(c)
		default:
			b.WriteByte('%')
			b.WriteByte(hex[c>>4])
			b.WriteByte(hex[c&0x0F])
		}
	}
	return b.String()
}
