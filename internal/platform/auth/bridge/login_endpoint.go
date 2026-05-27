package bridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// LoginEndpoint serves the OIDC bridge HTTP surface:
//
//   POST /oauth/check-domain
//   GET  /oauth/oidc/login
//   GET  /oauth/oidc/callback
//
// The handlers do the redirect-and-exchange dance; the per-request
// session-cookie write (and any consent UI) is owned by the
// SessionWriter callback the platform server installs.
type LoginEndpoint struct {
	bridge     *Bridge
	states     *LoginStateRepo
	principals *principal.Repository
	mappings   *emaildomainmapping.Repository
	uow        *usecasepgx.UnitOfWork

	// SessionWriter is called after the callback exchange completes
	// successfully. It receives the resolved principal ID + the
	// redirect-back URL the front-channel should land on next, and must
	// either set a session cookie + redirect, or render a server-rendered
	// response. The default implementation just emits a 200 with the
	// principal ID — replace at startup.
	SessionWriter func(w http.ResponseWriter, r *http.Request, principalID string, returnURL string)
}

// NewLoginEndpoint wires the bridge HTTP handlers. The mappings repo
// and UoW power the auto-provisioning path in handleCallback — when a
// successful OIDC handshake yields an email that doesn't match a
// FlowCatalyst principal, the bridge creates one using the scope and
// primary-client-id carried by the matching email-domain mapping
// (drop-in parity with the Rust impl).
func NewLoginEndpoint(
	b *Bridge,
	states *LoginStateRepo,
	principals *principal.Repository,
	mappings *emaildomainmapping.Repository,
	uow *usecasepgx.UnitOfWork,
) *LoginEndpoint {
	return &LoginEndpoint{
		bridge:     b,
		states:     states,
		principals: principals,
		mappings:   mappings,
		uow:        uow,
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

// RegisterRoutes mounts the OIDC bridge endpoints.
func (e *LoginEndpoint) RegisterRoutes(r chi.Router) {
	r.Post("/oauth/check-domain", e.handleCheckDomain)
	r.Get("/oauth/oidc/login", e.handleLogin)
	r.Get("/oauth/oidc/callback", e.handleCallback)
}

// handleCheckDomain answers "is this email's domain configured for OIDC?".
// Used by the frontend's login page to decide whether to show a password
// field (INTERNAL) or kick off an OIDC redirect.
func (e *LoginEndpoint) handleCheckDomain(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httperror.Write(w, httperror.BadRequest("INVALID_JSON", err.Error()))
		return
	}
	if body.Email == "" {
		httperror.Write(w, httperror.BadRequest("EMAIL_REQUIRED", "email is required"))
		return
	}
	_, cfg, err := e.bridge.ResolveForEmail(r.Context(), body.Email)
	if err != nil {
		// Resolution failures (not configured / IDP unreachable) collapse
		// into "internal" so the UI doesn't leak whether a domain has OIDC.
		writeJSON(w, http.StatusOK, map[string]any{"provider": "INTERNAL"})
		return
	}
	resp := map[string]any{"provider": string(cfg.AuthProvider)}
	if cfg.OIDCIssuerURL != nil {
		resp["issuerUrl"] = *cfg.OIDCIssuerURL
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleLogin starts an OIDC login. Takes ?email=X (required) and
// optional ?returnUrl=Y. Generates state/nonce/PKCE verifier, persists
// the state row, then 302-redirects to the IDP's authorize URL.
func (e *LoginEndpoint) handleLogin(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	returnURL := r.URL.Query().Get("returnUrl")
	if email == "" {
		httperror.Write(w, httperror.BadRequest("EMAIL_REQUIRED", "email query param is required"))
		return
	}

	resolved, cfg, err := e.bridge.ResolveForEmail(r.Context(), email)
	if err != nil || resolved == nil {
		httperror.Write(w, httperror.BadRequest("OIDC_NOT_CONFIGURED",
			"OIDC is not configured for this email's domain"))
		return
	}

	state := randString(32)
	nonce := randString(32)
	verifier := randString(64)
	challenge := pkceChallenge(verifier)
	loginState := NewLoginState(state, emailDomain(email),
		"", // identityProviderID — populated once the IDP table is wired in
		cfg.ID, nonce, verifier)
	if returnURL != "" {
		loginState.ReturnURL = &returnURL
	}
	if err := e.states.Insert(r.Context(), loginState); err != nil {
		httperror.Write(w, usecase.Internal("OIDC_STATE", "persist state failed", err))
		return
	}

	redirectURI := absoluteCallbackURL(r)
	authURL := resolved.AuthCodeURL(state, redirectURI) +
		"&nonce=" + url.QueryEscape(nonce) +
		"&code_challenge=" + url.QueryEscape(challenge) +
		"&code_challenge_method=S256"
	http.Redirect(w, r, authURL, http.StatusFound)
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

	loginState, err := e.states.FindByState(r.Context(), state)
	if err != nil {
		httperror.Write(w, usecase.Internal("OIDC_STATE", "lookup state failed", err))
		return
	}
	if loginState == nil {
		httperror.Write(w, httperror.BadRequest("INVALID_STATE", "unknown state"))
		return
	}
	if loginState.IsExpired() {
		_ = e.states.Delete(r.Context(), state)
		httperror.Write(w, httperror.BadRequest("STATE_EXPIRED", "login state expired"))
		return
	}

	resolved, _, err := e.bridge.ResolveForEmail(r.Context(),
		"x@"+loginState.EmailDomain)
	if err != nil || resolved == nil {
		httperror.Write(w, httperror.BadRequest("OIDC_NOT_CONFIGURED", "OIDC config lost"))
		return
	}

	redirectURI := absoluteCallbackURL(r)
	tok, err := resolved.Exchange(r.Context(), code, redirectURI)
	if err != nil {
		httperror.Write(w, usecase.Internal("OIDC_EXCHANGE", "code exchange failed", err))
		return
	}
	rawIDToken, ok := tok.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		httperror.Write(w, httperror.BadRequest("NO_ID_TOKEN", "IDP did not return id_token"))
		return
	}
	idToken, err := resolved.VerifyIDToken(r.Context(), rawIDToken)
	if err != nil {
		httperror.Write(w, usecase.Authorization("OIDC_VERIFY", "id_token verification failed: "+err.Error()))
		return
	}
	var claims struct {
		Email string `json:"email"`
		Nonce string `json:"nonce"`
	}
	if err := idToken.Claims(&claims); err != nil {
		httperror.Write(w, httperror.BadRequest("OIDC_CLAIMS", "id_token claims malformed"))
		return
	}
	if claims.Nonce != loginState.Nonce {
		httperror.Write(w, usecase.Authorization("NONCE_MISMATCH", "nonce did not match"))
		return
	}

	// Resolve or create the FlowCatalyst principal. Drop-in parity with
	// the Rust impl: lookup by email; if missing, auto-provision using
	// the scope + primary-client-id carried by the email-domain mapping
	// that drove this login. Role-mapping from IDP claims is a separate
	// follow-up (Rust calls sync_oidc_login_with_allowed_roles; Go
	// today plumbs no roles into the new principal).
	p, err := e.principals.FindByEmail(r.Context(), claims.Email)
	if err != nil {
		httperror.Write(w, usecase.Internal("REPO", "principal lookup failed", err))
		return
	}
	if p == nil {
		p, err = e.autoProvision(r.Context(), claims.Email, loginState.EmailDomainMappingID)
		if err != nil {
			httperror.Write(w, err)
			return
		}
	}

	// Best-effort cleanup of the state row.
	_ = e.states.Delete(r.Context(), state)

	returnURL := ""
	if loginState.ReturnURL != nil {
		returnURL = *loginState.ReturnURL
	}
	e.SessionWriter(w, r, p.ID, returnURL)
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
	committed, err := principalops.CreateUser(ctx, e.principals, e.uow, cmd, ec)
	if err != nil {
		return nil, err
	}
	created, err := e.principals.FindByID(ctx, committed.Event().UserID)
	if err != nil {
		return nil, usecase.Internal("REPO", "post-create principal lookup failed", err)
	}
	if created == nil {
		// Shouldn't happen — Persist just succeeded.
		return nil, usecase.Internal("REPO", "post-create principal missing", errors.New("not found"))
	}
	return created, nil
}

// ── helpers ──────────────────────────────────────────────────────────────

// absoluteCallbackURL derives the public-facing /oauth/oidc/callback
// URL the IDP will redirect to. Prefers the X-Forwarded-* headers when
// the platform is behind a load balancer.
func absoluteCallbackURL(r *http.Request) string {
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
	return scheme + "://" + host + "/oauth/oidc/callback"
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

// writeJSON writes a JSON response.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Compile-time guard: ensure the context import stays live as the
// callback expands.
var _ = context.Background
var _ = errors.New
