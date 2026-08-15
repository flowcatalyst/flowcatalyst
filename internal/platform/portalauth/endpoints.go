package portalauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/grantstore"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/oauthapi"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordhash"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// PasswordResetSender mints and emails a portal forgot-password link.
// Implemented by the password-reset principalEmailer.
type PasswordResetSender interface {
	SendPortalReset(ctx context.Context, identityID, email string, redirectURI *string) error
}

// State bundles the portal auth surface's dependencies.
type State struct {
	OAuthClients oauthapi.OAuthClientFinder
	Identities   *portalidentity.Repository
	IdPs         *identityprovider.Repository
	Flows        *FlowRepo
	AuthCodes    *grantstore.AuthorizationCodeRepository
	// PasswordReset backs POST /portal/auth/password-reset (optional — nil
	// silently accepts requests without sending, like an unconfigured
	// mailer).
	PasswordReset PasswordResetSender
	// LoginPagePath is the SPA route /portal/authorize bounces to
	// (default "/portal/login").
	LoginPagePath string
	// RateLimit throttles the password endpoint per (client, email) — the
	// portal plane has no per-principal login-backoff table, so this is the
	// brute-force ceiling. Optional (nil disables).
	RateLimit         ratelimit.Store
	RateLimitPolicies ratelimit.Policies
}

// RegisterRoutes mounts the portal plane. Mounted OUTSIDE the platform auth
// middleware: these endpoints authenticate portal identities themselves and
// never read fc_session.
func (s *State) RegisterRoutes(r chi.Router) {
	r.Get("/portal/authorize", s.Authorize)
	r.Post("/portal/auth/check-domain", s.CheckDomain)
	r.Post("/portal/auth/login", s.PasswordLogin)
	r.Post("/portal/auth/password-reset", s.RequestPasswordReset)
}

// Authorize is GET /portal/authorize — the portal plane's authorization
// entry. It mirrors /oauth/authorize's client/redirect/PKCE validation but
// NEVER consults a session: it parks the chain in a login flow and bounces
// to the SPA portal login page, where the user authenticates fresh.
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

	if strings.TrimSpace(stateParam) == "" {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_request", "`state` parameter is required for CSRF protection")
		return
	}

	// Client + redirect_uri validation BEFORE any redirect (RFC 6749
	// §4.1.2.1 — same fail-direct posture as /oauth/authorize).
	client, err := s.OAuthClients.FindByClientID(r.Context(), clientID)
	switch {
	case err != nil:
		writeOAuthErr(w, http.StatusInternalServerError, "server_error", "Internal error")
		return
	case client == nil, !client.Active:
		writeOAuthErr(w, http.StatusBadRequest, "unauthorized_client", "Unknown or inactive client")
		return
	case client.PortalClientID == nil || *client.PortalClientID == "":
		// The portal plane serves ONLY portal-flagged clients; everything
		// else belongs on /oauth/authorize.
		writeOAuthErr(w, http.StatusBadRequest, "unauthorized_client", "Client is not a portal client")
		return
	}
	if !oauthapi.MatchRedirectURI(redirectURI, client.RedirectURIs) {
		writeOAuthErr(w, http.StatusBadRequest, "invalid_request", "Invalid redirect_uri")
		return
	}

	// redirect_uri validated — errors may bounce back with OAuth params.
	if responseType != "code" {
		errRedirect(w, r, redirectURI, "unsupported_response_type", "Only 'code' response type is supported", stateParam)
		return
	}
	if client.PKCERequired && codeChallenge == "" {
		errRedirect(w, r, redirectURI, "invalid_request", "PKCE code_challenge is required", stateParam)
		return
	}
	if codeChallengeMethod != "" && codeChallengeMethod != "S256" {
		errRedirect(w, r, redirectURI, "invalid_request", "Only the S256 code_challenge_method is supported", stateParam)
		return
	}

	flow, err := NewLoginFlow(clientID, *client.PortalClientID, redirectURI, stateParam)
	if err != nil {
		errRedirect(w, r, redirectURI, "server_error", "Could not start the login flow", stateParam)
		return
	}
	flow.Scope = optStr(scope)
	flow.Nonce = optStr(nonce)
	flow.CodeChallenge = optStr(codeChallenge)
	if codeChallenge != "" {
		method := codeChallengeMethod
		if method == "" {
			method = "S256" // never store a challenge without its method (see /oauth/authorize)
		}
		flow.CodeChallengeMethod = &method
	}
	if err := s.Flows.Insert(r.Context(), flow); err != nil {
		errRedirect(w, r, redirectURI, "server_error", "Could not start the login flow", stateParam)
		return
	}

	page := s.LoginPagePath
	if page == "" {
		page = "/portal/login"
	}
	http.Redirect(w, r, page+"?flow="+url.QueryEscape(flow.ID), http.StatusTemporaryRedirect)
}

// CheckDomain is POST /portal/auth/check-domain {flowId, email}: routes the
// email within the flow's client context. A domain owned by an IdP BOUND to
// that client's portal → SSO (with the redirect to start it); anything else
// → password. Deliberately does not reveal whether the identity exists.
func (s *State) CheckDomain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FlowID string `json:"flowId"`
		Email  string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_BODY", "malformed request body"))
		return
	}
	flow, err := s.Flows.Find(r.Context(), body.FlowID)
	if err != nil {
		httperror.Write(w, usecase.Internal("FLOW", "flow lookup failed", err))
		return
	}
	if flow == nil {
		httperror.Write(w, httperror.BadRequest("FLOW_EXPIRED", "The login flow has expired — return to the portal and try again"))
		return
	}
	domain := emailDomainOf(body.Email)
	if domain == "" {
		httperror.Write(w, httperror.BadRequest("EMAIL_INVALID", "email is not valid"))
		return
	}

	idp, err := s.portalIdPForDomain(r, flow.PortalClientID, domain)
	if err != nil {
		httperror.Write(w, usecase.Internal("IDP", "identity provider lookup failed", err))
		return
	}
	if idp != nil {
		writeJSON(w, map[string]string{
			"method": "SSO",
			"redirectUrl": "/portal/auth/oidc/login?flow=" + url.QueryEscape(flow.ID) +
				"&provider_id=" + url.QueryEscape(idp.ID),
		})
		return
	}
	writeJSON(w, map[string]string{"method": "PASSWORD"})
}

// portalIdPForDomain finds an IdP bound to the client's portal plane that
// owns the email domain (see identityprovider.PortalIdPForDomain).
func (s *State) portalIdPForDomain(r *http.Request, portalClientID, domain string) (*identityprovider.IdentityProvider, error) {
	return identityprovider.PortalIdPForDomain(r.Context(), s.IdPs, portalClientID, domain)
}

// PasswordLogin is POST /portal/auth/login {flowId, email, password}. On
// success it consumes the flow, mints the authorization code with the
// portal-identity subject, and answers with the code redirect. No cookie is
// ever set. Failures are uniform INVALID_CREDENTIALS (no account/status
// enumeration) and do NOT consume the flow (the user may retry until TTL).
func (s *State) PasswordLogin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FlowID   string `json:"flowId"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_BODY", "malformed request body"))
		return
	}
	flow, err := s.Flows.Find(r.Context(), body.FlowID)
	if err != nil {
		httperror.Write(w, usecase.Internal("FLOW", "flow lookup failed", err))
		return
	}
	if flow == nil {
		httperror.Write(w, httperror.BadRequest("FLOW_EXPIRED", "The login flow has expired — return to the portal and try again"))
		return
	}

	// Brute-force ceiling: cluster-wide throttle per (client, email). The
	// portal plane deliberately has no lockout table; the limiter is the
	// control.
	limitKey := flow.PortalClientID + ":" + strings.ToLower(strings.TrimSpace(body.Email))
	if rej := ratelimit.Enforce(r.Context(), s.RateLimit, ratelimit.BucketPortalLogin, limitKey, s.RateLimitPolicies.PortalLogin); rej != nil {
		ratelimit.WriteTooManyRequests(w, rej.RetryAfterSecs, "too many attempts")
		return
	}

	// SSO enforcement: a domain owned by this portal's IdP never
	// authenticates by password — even one set via a stray invite. The org's
	// IdP is the authority (suspend there = suspended here); allowing the
	// password would be an SSO bypass.
	if idp, ierr := s.portalIdPForDomain(r, flow.PortalClientID, emailDomainOf(body.Email)); ierr == nil && idp != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "SSO_REQUIRED",
			"message": "Sign in with your organisation account",
		})
		return
	}

	ident, err := s.Identities.FindByClientAndEmail(r.Context(), flow.PortalClientID, body.Email)
	if err != nil {
		httperror.Write(w, usecase.Internal("IDENTITY", "identity lookup failed", err))
		return
	}
	if ident == nil || !ident.CanSignInWithPassword() {
		// Unknown, suspended, and password-less identities are
		// indistinguishable — burn comparable time and refuse.
		passwordhash.EqualizeTiming(body.Password)
		writeInvalidCredentials(w)
		return
	}
	if err := passwordhash.Verify(body.Password, *ident.PasswordHash); err != nil {
		writeInvalidCredentials(w)
		return
	}

	// Success: single-use the flow, then mint the code.
	consumed, err := s.Flows.Consume(r.Context(), flow.ID)
	if err != nil || consumed == nil {
		httperror.Write(w, httperror.BadRequest("FLOW_EXPIRED", "The login flow has expired — return to the portal and try again"))
		return
	}
	redirectURL, err := s.IssueCode(r, consumed, ident.ID)
	if err != nil {
		httperror.Write(w, usecase.Internal("CODE", "could not issue the authorization code", err))
		return
	}
	_ = s.Identities.TouchLastLogin(r.Context(), ident.ID)
	writeJSON(w, map[string]string{"redirectUrl": redirectURL})
}

// RequestPasswordReset is POST /portal/auth/password-reset {flowId, email}:
// the portal forgot-password flow. Silent-success anti-enumeration — the
// response never reveals whether the identity exists; only an existing
// ACTIVE identity in the flow's client context is actually mailed. The
// reset link's post-completion redirect defaults to the portal's origin
// (derived from the flow's validated redirect URI) so the user lands back
// at the portal to sign in.
func (s *State) RequestPasswordReset(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FlowID string `json:"flowId"`
		Email  string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_BODY", "malformed request body"))
		return
	}
	flow, err := s.Flows.Find(r.Context(), body.FlowID)
	if err != nil {
		httperror.Write(w, usecase.Internal("FLOW", "flow lookup failed", err))
		return
	}
	if flow == nil {
		httperror.Write(w, httperror.BadRequest("FLOW_EXPIRED", "The login flow has expired — return to the portal and try again"))
		return
	}
	emailAddr := strings.ToLower(strings.TrimSpace(body.Email))
	if emailDomainOf(emailAddr) == "" {
		httperror.Write(w, httperror.BadRequest("EMAIL_INVALID", "email is not valid"))
		return
	}

	// Same brute-force ceiling as the login endpoint, keyed per
	// (client, email) with a distinct sub-key so resets and logins don't
	// share a budget.
	limitKey := "reset:" + flow.PortalClientID + ":" + emailAddr
	if rej := ratelimit.Enforce(r.Context(), s.RateLimit, ratelimit.BucketPortalLogin, limitKey, s.RateLimitPolicies.PortalLogin); rej != nil {
		ratelimit.WriteTooManyRequests(w, rej.RetryAfterSecs, "too many attempts")
		return
	}

	// SSO-owned domains never get password resets (nothing to reset that
	// should be usable). Silent success — same anti-enumeration shape.
	if idp, ierr := s.portalIdPForDomain(r, flow.PortalClientID, emailDomainOf(emailAddr)); ierr == nil && idp != nil {
		writeJSON(w, map[string]string{
			"message": "If an account exists, a reset email has been sent.",
		})
		return
	}

	if s.PasswordReset != nil {
		ident, ierr := s.Identities.FindByClientAndEmail(r.Context(), flow.PortalClientID, emailAddr)
		if ierr == nil && ident != nil && ident.Status == portalidentity.StatusActive {
			redirect := portalOriginOf(flow.RedirectURI)
			if serr := s.PasswordReset.SendPortalReset(r.Context(), ident.ID, ident.Email, redirect); serr != nil {
				// Suppressed (anti-enumeration): log-worthy but never surfaced.
				_ = serr
			}
		}
	}
	writeJSON(w, map[string]string{
		"message": "If an account exists, a reset email has been sent.",
	})
}

// portalOriginOf derives "scheme://host/" from the flow's validated
// redirect URI — admin-registered OAuth config, never caller input.
func portalOriginOf(redirectURI string) *string {
	u, err := url.Parse(redirectURI)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil
	}
	origin := u.Scheme + "://" + u.Host + "/"
	return &origin
}

// IssueCode mints the authorization code for a consumed flow and returns the
// full redirect URL. Shared by the password handler and the bridge's portal
// SSO sink.
func (s *State) IssueCode(r *http.Request, flow *LoginFlow, subjectID string) (string, error) {
	raw, err := randomToken(48)
	if err != nil {
		return "", err
	}
	code := grantstore.NewAuthorizationCode(raw, flow.OAuthClientID, subjectID, flow.RedirectURI)
	code.Scope = flow.Scope
	code.Nonce = flow.Nonce
	code.State = &flow.State
	if flow.CodeChallenge != nil {
		code.CodeChallenge = flow.CodeChallenge
		code.CodeChallengeMethod = flow.CodeChallengeMethod
	}
	if err := s.AuthCodes.Insert(r.Context(), code); err != nil {
		return "", err
	}
	sep := "?"
	if strings.Contains(flow.RedirectURI, "?") {
		sep = "&"
	}
	return flow.RedirectURI + sep + "code=" + url.QueryEscape(code.Code) +
		"&state=" + url.QueryEscape(flow.State), nil
}

// ─── helpers ─────────────────────────────────────────────────────────────

func optStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func emailDomainOf(email string) string {
	email = strings.TrimSpace(email)
	at := strings.LastIndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return ""
	}
	return strings.ToLower(email[at+1:])
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// writeOAuthErr emits the RFC-6749 {error, error_description} body (matching
// /oauth/authorize's direct-failure shape).
func writeOAuthErr(w http.ResponseWriter, status int, code, desc string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": desc})
}

// writeInvalidCredentials emits the uniform 401 for the password endpoint —
// unknown, suspended, and wrong-password cases are indistinguishable.
func writeInvalidCredentials(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    "INVALID_CREDENTIALS",
		"message": "Invalid email or password",
	})
}

// errRedirect bounces back to the (validated) redirect_uri with OAuth error
// params.
func errRedirect(w http.ResponseWriter, r *http.Request, redirectURI, code, desc, state string) {
	sep := "?"
	if strings.Contains(redirectURI, "?") {
		sep = "&"
	}
	u := redirectURI + sep + "error=" + url.QueryEscape(code) +
		"&error_description=" + url.QueryEscape(desc) + "&state=" + url.QueryEscape(state)
	http.Redirect(w, r, u, http.StatusTemporaryRedirect) //nolint:gosec // G710: redirectURI was validated against the client's registered URIs before any errRedirect call
}
