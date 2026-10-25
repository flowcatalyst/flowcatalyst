package pages

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"maps"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// SignIn serves the new sign-in pages under /ui/auth: the email step with SSO
// discovery, the password step and second factor, the landing after SSO,
// forgot and reset password, logout, and the portal login (auth_signin_portal.go,
// auth_reset.go). They are ADDITIVE: the SPA's /auth/login and every existing
// auth endpoint are unchanged and stay the default; these pages are reached
// only by their /ui/auth/* URL (docs/ui/README.md, "Sign-in pages").
//
// The pages do not re-implement the sign-in rules. Every step is one call to
// the platform's own public endpoint (/auth/check-domain, /auth/login,
// /auth/2fa/verify, /auth/password-reset/*, /portal/auth/*, /api/public/login-theme)
// dispatched in-process through Upstream, the platform's router, so the
// per-(email, IP) backoff and lockout, the SSO enforcement, the Argon2id timing
// equalisation, the login CSRF guard, the rate limiters and the cookie builders
// are the endpoints' own. The page's job is the HTML and the redirect after.
type SignIn struct {
	// Upstream serves the platform's public auth endpoints (the platform's
	// root router).
	Upstream http.Handler
	// Authenticate attaches the caller's AuthContext from the fc_session
	// cookie (used by the SSO landing only).
	Authenticate func(http.Handler) http.Handler
}

// Mount registers the sign-in pages on r (the /ui group, outside the session
// guards: these pages are what a caller without a session reaches).
func (s *SignIn) Mount(r chi.Router) {
	r.Group(func(r chi.Router) {
		r.Use(signInHeaders)
		r.Get("/auth/login", s.loginPage)
		r.Post("/auth/login/email", s.checkEmail)
		r.Post("/auth/login/password", s.password)
		r.Post("/auth/login/2fa", s.twoFactor)
		r.Post("/auth/login/setup", s.requestSetup)
		r.With(s.Authenticate).Get("/auth/callback", s.callback)
		r.Get("/auth/logout", s.logoutPage)
		r.Post("/auth/logout", s.logout)
		s.mountReset(r)
		s.mountPortal(r)
	})
}

// signInHeaders widens the CSP's img-src for the pages that show a tenant's
// logo from its own https host (the SPA's login page does the same); every
// other directive and header stays as guard.SecurityHeaders set it.
func signInHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if csp := h.Get("Content-Security-Policy"); csp != "" {
			h.Set("Content-Security-Policy", strings.Replace(csp, "img-src 'self' data:", "img-src 'self' data: https:", 1))
		}
		next.ServeHTTP(w, r)
	})
}

// ── Upstream ─────────────────────────────────────────────────────────────

// upstreamResponse is what an upstream endpoint answered.
type upstreamResponse struct {
	Status int
	Header http.Header
	Body   []byte
}

// upstreamWriter buffers a handler's response.
type upstreamWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (u *upstreamWriter) Header() http.Header { return u.header }

func (u *upstreamWriter) Write(b []byte) (int, error) {
	if u.status == 0 {
		u.status = http.StatusOK
	}
	return u.body.Write(b)
}

func (u *upstreamWriter) WriteHeader(code int) {
	if u.status == 0 {
		u.status = code
	}
}

// forwardedHeaders are the browser's request headers an upstream endpoint
// reads: the provenance the login CSRF guard checks, the client identity the
// backoff and rate limiters key on, and the cookies (the trusted-device cookie
// of the second factor). Nothing else is passed on: no Accept-Encoding (the
// answer is parsed, not shown), no Datastar header.
var forwardedHeaders = []string{
	"Cookie", "Origin", "Sec-Fetch-Site", "User-Agent", "Accept-Language",
	"X-Forwarded-For", "X-Forwarded-Proto", "X-Forwarded-Host", "X-Real-Ip",
}

// call runs one upstream endpoint in-process as the browser's request (same
// host, remote address, provenance headers and cookies) with a JSON body, and
// returns what it answered. It starts from a fresh context: the router stores
// its routing state there, and reusing this request's would misroute.
func (s *SignIn) call(r *http.Request, method, target string, body any) upstreamResponse {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return upstreamResponse{Status: http.StatusInternalServerError}
		}
		rd = bytes.NewReader(b)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var req *http.Request
	var err error
	if rd != nil {
		req, err = http.NewRequestWithContext(ctx, method, target, rd)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, target, http.NoBody)
	}
	if err != nil {
		return upstreamResponse{Status: http.StatusInternalServerError}
	}
	for _, h := range forwardedHeaders {
		for _, v := range r.Header.Values(h) {
			req.Header.Add(h, v)
		}
	}
	req.Host, req.RemoteAddr, req.TLS = r.Host, r.RemoteAddr, r.TLS
	req.Header.Set("Accept", "application/json")
	if rd != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	uw := &upstreamWriter{header: http.Header{}}
	s.Upstream.ServeHTTP(uw, req)
	if uw.status == 0 {
		uw.status = http.StatusOK
	}
	return upstreamResponse{Status: uw.status, Header: uw.header, Body: uw.body.Bytes()}
}

// decode reads the JSON body into v.
func (u upstreamResponse) decode(v any) bool { return json.Unmarshal(u.Body, v) == nil }

// message is the platform error envelope's message, or fallback.
func (u upstreamResponse) message(fallback string) string {
	var e struct {
		Message string `json:"message"`
	}
	if u.decode(&e) && e.Message != "" {
		return e.Message
	}
	return fallback
}

// code is the platform error envelope's code.
func (u upstreamResponse) code() string {
	var e struct {
		Code string `json:"code"`
	}
	_ = u.decode(&e)
	return e.Code
}

// retryAfter is the Retry-After seconds of a throttled answer (0: none).
func (u upstreamResponse) retryAfter() int {
	n, err := strconv.Atoi(u.Header.Get("Retry-After"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// cookies passes the upstream's Set-Cookie headers (the session cookie, the
// trusted-device cookie) to the browser unchanged: the endpoint's own builders
// decided their attributes.
func (u upstreamResponse) cookies(w http.ResponseWriter) {
	for _, c := range u.Header.Values("Set-Cookie") {
		w.Header().Add("Set-Cookie", c)
	}
}

// ── The sign-in context ──────────────────────────────────────────────────

// oauthForwardFields is the /oauth/authorize parameter set a sign-in carries
// through an OAuth round trip (frontend/src/api/auth.ts OAUTH_FORWARD_FIELDS).
var oauthForwardFields = []string{
	"response_type", "client_id", "redirect_uri", "scope", "state",
	"code_challenge", "code_challenge_method", "nonce",
}

// ssoBridgeFields are the subset the OIDC bridge reads, as oauth_<field>.
var ssoBridgeFields = []string{
	"client_id", "redirect_uri", "scope", "state", "code_challenge", "code_challenge_method", "nonce",
}

var interactionUID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

// signinContext is what a sign-in carries from page to page: where to go after
// (?redirect=, an OIDC interaction, an OAuth round trip), the branding client
// and the reset banner. Values are read from the query or the posted form and
// capped; they are written back as hidden fields or query parameters, and the
// destinations are built from them only through landing and ssoURL, which
// validate.
type signinContext struct {
	vals url.Values
}

// signinContextKeys lists the keys kept, in the order they are written.
var signinContextKeys = append([]string{"redirect", "client", "oauth", "interaction", "reset"}, oauthForwardFields...)

func signinContextFrom(r *http.Request) signinContext {
	_ = r.ParseForm()
	c := signinContext{vals: url.Values{}}
	for _, k := range signinContextKeys {
		if v := strings.TrimSpace(r.Form.Get(k)); v != "" && len(v) <= 2048 {
			c.vals.Set(k, v)
		}
	}
	return c
}

func (c signinContext) get(k string) string { return c.vals.Get(k) }

// client is the branding identifier (?client=).
func (c signinContext) client() string { return c.get("client") }

// query is the context as a query string (links), plus extra pairs.
func (c signinContext) query(extra ...string) string {
	q := url.Values{}
	maps.Copy(q, c.vals)
	for i := 0; i+1 < len(extra); i += 2 {
		if extra[i+1] == "" {
			q.Del(extra[i])
		} else {
			q.Set(extra[i], extra[i+1])
		}
	}
	return q.Encode()
}

// hidden lists the context as hidden form fields, in a fixed order. The reset
// banner is a one-page display and is not carried.
func (c signinContext) hidden() [][2]string {
	var out [][2]string
	for _, k := range signinContextKeys {
		if v := c.get(k); v != "" && k != "reset" {
			out = append(out, [2]string{k, v})
		}
	}
	return out
}

// authorizeURL rebuilds the /oauth/authorize URL of the round trip.
func (c signinContext) authorizeURL() string {
	q := url.Values{}
	for _, f := range oauthForwardFields {
		if v := c.get(f); v != "" {
			q.Set(f, v)
		}
	}
	return "/oauth/authorize?" + q.Encode()
}

// landing is where a completed sign-in goes, in the SPA's order
// (redirectAfterLogin): an OIDC interaction, an OAuth round trip, a validated
// return to a /ui page, else the UI's home.
func (c signinContext) landing() string {
	if uid := c.get("interaction"); interactionUID.MatchString(uid) {
		return "/oidc/interaction/" + uid + "/login"
	}
	if c.get("oauth") == "true" {
		return c.authorizeURL()
	}
	if t := safeUIRedirect(c.get("redirect")); t != "" {
		return t
	}
	return "/ui/"
}

// ssoURL decorates the bridge's login URL with the round trip context, as the
// SPA's externalIdpRedirectUrl does, and asks the bridge to return through the
// SSO landing page. loginURL must be the bridge's own relative login path.
func (c signinContext) ssoURL(loginURL string) string {
	u, err := url.Parse(loginURL)
	if err != nil || u.Host != "" || u.Path != "/auth/oidc/login" {
		return ""
	}
	q := u.Query()
	switch {
	case interactionUID.MatchString(c.get("interaction")):
		q.Set("interaction", c.get("interaction"))
	case c.get("oauth") == "true":
		for _, f := range ssoBridgeFields {
			if v := c.get(f); v != "" {
				q.Set("oauth_"+f, v)
			}
		}
	}
	back := "/ui/auth/callback"
	if t := safeUIRedirect(c.get("redirect")); t != "" {
		back += "?redirect=" + url.QueryEscape(t)
	}
	q.Set("return_url", back)
	u.RawQuery = q.Encode()
	return u.String()
}

// safeUIRedirect returns raw when it is a same-origin path under /ui, else "":
// the port of the SPA's safeUiRedirect (frontend/src/api/uiRedirect.ts). The
// path is parsed and cleaned first, so "..", encoded dots and encoded
// slashes cannot lead out of /ui, and a backslash, a control character, a
// scheme or a host (including "//host") is refused.
func safeUIRedirect(raw string) string {
	if raw == "" || raw[0] != '/' || strings.HasPrefix(raw, "//") {
		return ""
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] == 0x7f || raw[i] == '\\' {
			return ""
		}
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil {
		return ""
	}
	p := path.Clean(u.Path)
	if p != "/ui" && !strings.HasPrefix(p, "/ui/") {
		return ""
	}
	if strings.HasSuffix(u.Path, "/") && p != "/" {
		p += "/"
	}
	out := (&url.URL{Path: p}).EscapedPath()
	if u.RawQuery != "" {
		out += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		out += "#" + u.EscapedFragment()
	}
	return out
}

// ── The login theme ──────────────────────────────────────────────────────

// signinTheme is the branding of the public sign-in pages: the platform's login
// theme with the client's layered over it, as the upstream endpoint serves it.
// Every value is validated on the way in; the logo SVG is shown through an
// <img> data URL, which does not run scripts.
type signinTheme struct {
	BrandName, Subtitle, Footer string
	LogoURL                     string
	Primary, Accent, Background string
	CustomCSS                   string
}

var defaultSigninTheme = signinTheme{
	BrandName:  "FlowCatalyst",
	Subtitle:   "Platform Administration",
	Footer:     "Secure access to your FlowCatalyst platform",
	Primary:    "#102a43",
	Accent:     "#0967d2",
	Background: "linear-gradient(135deg, #102a43 0%, #0a1929 100%)",
}

var (
	cssColor    = regexp.MustCompile(`^(#[0-9a-fA-F]{3,8}|(rgb|rgba|hsl|hsla)\([0-9a-zA-Z.,%\s/]{1,80}\)|[a-zA-Z]{3,30})$`)
	cssGradient = regexp.MustCompile(`^(linear|radial|conic)-gradient\([#0-9a-zA-Z%.,\s()/-]{1,400}\)$`)
)

// themeResponse is the login theme wire shape (publicapi.loginThemeResponse).
type themeResponse struct {
	BrandName          *string `json:"brandName"`
	BrandSubtitle      *string `json:"brandSubtitle"`
	LogoURL            *string `json:"logoUrl"`
	LogoSVG            *string `json:"logoSvg"`
	PrimaryColor       *string `json:"primaryColor"`
	AccentColor        *string `json:"accentColor"`
	BackgroundColor    *string `json:"backgroundColor"`
	BackgroundGradient *string `json:"backgroundGradient"`
	FooterText         *string `json:"footerText"`
	CustomCSS          *string `json:"customCss"`
}

// theme loads the login theme for the branding client ("" = the platform's).
// Branding never takes the page down: any failure is the default theme.
func (s *SignIn) theme(r *http.Request, client string) signinTheme {
	th := defaultSigninTheme
	target := "/api/public/login-theme"
	if client != "" && len(client) <= 100 {
		target += "?client=" + url.QueryEscape(client)
	}
	resp := s.call(r, http.MethodGet, target, nil)
	var raw themeResponse
	if resp.Status != http.StatusOK || !resp.decode(&raw) {
		return th
	}
	text := func(dst *string, src *string) {
		if src != nil && strings.TrimSpace(*src) != "" && len(*src) <= 300 {
			*dst = *src
		}
	}
	text(&th.BrandName, raw.BrandName)
	text(&th.Subtitle, raw.BrandSubtitle)
	text(&th.Footer, raw.FooterText)
	color := func(dst *string, src *string) {
		if src != nil && cssColor.MatchString(strings.TrimSpace(*src)) {
			*dst = strings.TrimSpace(*src)
		}
	}
	color(&th.Primary, raw.PrimaryColor)
	color(&th.Accent, raw.AccentColor)
	// The SPA paints background-gradient when set, else the colour.
	switch {
	case raw.BackgroundGradient != nil && cssGradient.MatchString(strings.TrimSpace(*raw.BackgroundGradient)):
		th.Background = strings.TrimSpace(*raw.BackgroundGradient)
	case raw.BackgroundColor != nil && cssColor.MatchString(strings.TrimSpace(*raw.BackgroundColor)):
		th.Background = strings.TrimSpace(*raw.BackgroundColor)
	}
	switch {
	case raw.LogoURL != nil && safeLogoURL(*raw.LogoURL):
		th.LogoURL = *raw.LogoURL
	case raw.LogoSVG != nil && strings.TrimSpace(*raw.LogoSVG) != "" && len(*raw.LogoSVG) <= 64<<10:
		th.LogoURL = "data:image/svg+xml;base64," + base64.StdEncoding.EncodeToString([]byte(*raw.LogoSVG))
	}
	// The tenant's own stylesheet, as the SPA applies it; a "<" cannot end the
	// style element.
	if raw.CustomCSS != nil && len(*raw.CustomCSS) <= 32<<10 {
		th.CustomCSS = strings.ReplaceAll(*raw.CustomCSS, "<", "")
	}
	return th
}

// safeLogoURL accepts an https URL, a same-site path, or an image data URL.
// withoutCSS is the theme without the tenant's stylesheet, for the pages that
// carry a token in a hidden field (the second factor's pending token, the
// enroll token, the reset token): a stylesheet can read an attribute value
// (input[value^=...]) and, with img-src https:, send it out as a background
// image request. The colours, logo and text stay.
func (t signinTheme) withoutCSS() signinTheme {
	t.CustomCSS = ""
	return t
}

func safeLogoURL(raw string) bool {
	if raw == "" || len(raw) > 2048 {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] == 0x7f || raw[i] == '\\' || raw[i] == '"' || raw[i] == '<' {
			return false
		}
	}
	if strings.HasPrefix(raw, "data:image/") {
		return strings.Contains(raw[:min(len(raw), 40)], ";base64,") && !strings.HasPrefix(raw, "data:image/svg+xml;utf8")
	}
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//")
	}
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Host != ""
}

func (t signinTheme) pageStyle() string {
	return "min-height:100vh;display:flex;align-items:center;justify-content:center;padding:16px;" +
		"background:" + t.Background + ";--login-accent:" + t.Accent + ";--login-primary:" + t.Primary
}

// signinLinkStyle draws a submit button as a link in the accent colour.
func signinLinkStyle(t signinTheme) string {
	return "background:none;border:0;padding:0;cursor:pointer;font:inherit;color:" + t.Accent
}

func (t signinTheme) accentStyle() templ.Attributes {
	return templ.Attributes{"style": "background:" + t.Accent + ";border-color:" + t.Accent + ";color:#fff;width:100%"}
}

// ── Rendering ────────────────────────────────────────────────────────────

// signinStep names the step a page shows.
type signinStep string

// Steps of the login page.
const (
	stepEmail     signinStep = "email"
	stepPassword  signinStep = "password"
	stepSetup     signinStep = "setup"
	stepSetupSent signinStep = "setupSent"
	stepTwoFactor signinStep = "twofactor"
	stepEnroll    signinStep = "enroll"
)

// signinView is the login page's view model.
type signinView struct {
	Theme  signinTheme
	Ctx    signinContext
	Step   signinStep
	Email  string
	Error  string
	Notice string
	// Second factor: the pending token (a short-lived signed token, not a
	// session), the methods on offer, the active one, and the state of the
	// email code.
	MFAToken        string
	EnrollToken     string // the enrollment token of status enrollment_required (a short-lived signed token, not a session)
	Methods         []string
	Method          string
	RememberAllowed bool
	EmailSent       bool
}

func (v signinView) title() string {
	switch v.Step {
	case stepPassword:
		return "Enter your password"
	case stepSetup:
		return "Create your password"
	case stepSetupSent:
		return "Check your email"
	case stepTwoFactor:
		return "Verify it's you"
	case stepEnroll:
		return "Set up two-factor authentication"
	case stepEmail:
	}
	return "Sign in to your account"
}

func (v signinView) codePlaceholder() string {
	if v.Method == "RECOVERY_CODE" {
		return "XXXXX-XXXXX"
	}
	return "123456"
}

func (v signinView) hasMethod(m string) bool {
	return slices.Contains(v.Methods, m)
}

// initial is the email's first letter, for the avatar.
func (v signinView) initial() string {
	if v.Email == "" {
		return ""
	}
	return strings.ToUpper(string([]rune(v.Email)[:1]))
}

func (v signinView) loginLink() string {
	if q := v.Ctx.query("reset", ""); q != "" {
		return "/ui/auth/login?" + q
	}
	return "/ui/auth/login"
}

// legacyLink is the current (SPA) sign-in with the same context, for what the
// new pages do not cover.
// uiNext is where this sign-in is going when that is a /ui page, else "".
func (v signinView) uiNext() string {
	if n := v.Ctx.landing(); n == "/ui" || strings.HasPrefix(n, "/ui/") {
		return n
	}
	return ""
}

// passkeyOffered reports whether the passkey sign-in is offered: only for a
// /ui destination (the passkey page returns to a /ui page). The page is
// reached by a POST that carries the typed email, so the address is in no URL.
func (v signinView) passkeyOffered() bool { return v.uiNext() != "" }

func (v signinView) legacyLink() string {
	if q := v.Ctx.query("reset", ""); q != "" {
		return "/auth/login?" + q
	}
	return "/auth/login"
}

// render writes a sign-in page document.
func (s *SignIn) render(w http.ResponseWriter, r *http.Request, status int, title string, th signinTheme, portal bool, content templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = signinDocument(title, th, portal, content).Render(r.Context(), w)
}

func (s *SignIn) renderLogin(w http.ResponseWriter, r *http.Request, status int, v signinView) {
	v.Theme = s.theme(r, v.Ctx.client())
	if v.Step == stepTwoFactor || v.Step == stepEnroll {
		v.Theme = v.Theme.withoutCSS()
	}
	s.render(w, r, status, v.title(), v.Theme, false, signinLoginBody(v))
}

// leave ends a native form post by going to target. The pages keep the UI's
// CSP, whose form-action 'self' a browser applies to every redirect a form
// submission follows: a 303 to a page that redirects on to another origin (the
// OIDC bridge to the identity provider, /oauth/authorize or an OIDC
// interaction to the client's redirect_uri, a portal app, an invite's
// redirect) would be blocked at that hop. So only a /ui page is answered with a
// 303; anything else gets a 200 page that navigates there itself (a meta
// refresh, which is a new navigation rather than part of the submission, with
// a link as the fallback). form-action stays 'self'.
func (s *SignIn) leave(w http.ResponseWriter, r *http.Request, target string) {
	if target != "" && safeUIRedirect(target) == target {
		http.Redirect(w, r, target, http.StatusSeeOther)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_ = signinContinue(refreshURL(target)).Render(r.Context(), w)
}

// refreshURL makes a validated target safe inside a refresh's content: the
// characters the refresh parser treats as delimiters (quotes, whitespace) are
// percent-encoded; templ escapes the attribute.
func refreshURL(target string) string {
	return strings.NewReplacer(`"`, "%22", "'", "%27", " ", "%20").Replace(target)
}

// ── Handlers ─────────────────────────────────────────────────────────────

func (s *SignIn) loginPage(w http.ResponseWriter, r *http.Request) {
	v := signinView{Ctx: signinContextFrom(r), Step: stepEmail}
	if v.Ctx.get("reset") == "success" {
		v.Notice = "Your password has been reset. You can now sign in with your new password."
	}
	s.renderLogin(w, r, http.StatusOK, v)
}

// checkEmail is the email step: SSO discovery through the platform's
// check-domain. An external domain goes to its identity provider; an internal
// user who never set a password is offered the emailed link; everyone else
// gets the password step (an unknown address included).
func (s *SignIn) checkEmail(w http.ResponseWriter, r *http.Request) {
	v := signinView{Ctx: signinContextFrom(r), Step: stepEmail, Email: strings.TrimSpace(r.PostFormValue("email"))}
	if !strings.Contains(v.Email, "@") || len(v.Email) > 320 {
		v.Error = "Please enter a valid email address"
		s.renderLogin(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	resp := s.call(r, http.MethodPost, "/auth/check-domain", map[string]string{"email": v.Email})
	var cd struct {
		AuthMethod            string `json:"authMethod"`
		LoginURL              string `json:"loginUrl"`
		PasswordSetupRequired bool   `json:"passwordSetupRequired"`
	}
	if resp.Status != http.StatusOK || !resp.decode(&cd) {
		v.Error = resp.message("Could not check your email — please try again.")
		s.renderLogin(w, r, http.StatusBadGateway, v)
		return
	}
	switch {
	case cd.AuthMethod == "external" && cd.LoginURL != "":
		if target := v.Ctx.ssoURL(cd.LoginURL); target != "" {
			s.leave(w, r, target)
			return
		}
		v.Error = "Could not start your organization's sign-in — please try again."
		s.renderLogin(w, r, http.StatusBadGateway, v)
	case cd.AuthMethod == "internal" && cd.PasswordSetupRequired:
		v.Step = stepSetup
		s.renderLogin(w, r, http.StatusOK, v)
	default:
		v.Step = stepPassword
		s.renderLogin(w, r, http.StatusOK, v)
	}
}

// password is the password step: /auth/login decides everything (SSO
// enforcement, backoff, lockout, the same refusal for an unknown user and a
// wrong password, the second factor gate, the session cookie).
func (s *SignIn) password(w http.ResponseWriter, r *http.Request) {
	v := signinView{Ctx: signinContextFrom(r), Step: stepPassword, Email: strings.TrimSpace(r.PostFormValue("email"))}
	password := r.PostFormValue("password")
	if v.Email == "" || len(v.Email) > 320 {
		v.Step = stepEmail
		s.renderLogin(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	if password == "" {
		v.Error = "Enter your password"
		s.renderLogin(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	resp := s.call(r, http.MethodPost, "/auth/login", map[string]any{"email": v.Email, "password": password})
	s.afterCredentials(w, r, resp, v)
}

// afterCredentials maps the answer of /auth/login or /auth/2fa/verify: a
// session (cookies passed on, then the landing), a second factor challenge, or
// a refusal shown on the step v.
func (s *SignIn) afterCredentials(w http.ResponseWriter, r *http.Request, resp upstreamResponse, v signinView) {
	switch resp.Status {
	case http.StatusOK:
		var out struct {
			Status                string   `json:"status"`
			MFAToken              string   `json:"mfaToken"`
			EnrollToken           string   `json:"enrollToken"`
			Methods               []string `json:"methods"`
			RememberDeviceAllowed bool     `json:"rememberDeviceAllowed"`
		}
		if !resp.decode(&out) {
			v.Error = "Sign-in failed. Try again."
			s.renderLogin(w, r, http.StatusInternalServerError, v)
			return
		}
		switch out.Status {
		case "mfa_required":
			v.Step, v.MFAToken, v.Methods, v.RememberAllowed = stepTwoFactor, out.MFAToken, out.Methods, out.RememberDeviceAllowed
			v.Method = "EMAIL_PIN"
			if v.hasMethod("TOTP") {
				v.Method = "TOTP"
			}
			s.renderLogin(w, r, http.StatusOK, v)
		case "enrollment_required":
			// Enrolling a factor (QR code, recovery codes) is /ui/auth/2fa/enroll;
			// the token goes there by POST, never in a URL. A sign-in that is not
			// headed for a /ui page (an OAuth round trip) keeps using the current
			// sign-in, which can return to it.
			v.Step, v.EnrollToken = stepEnroll, out.EnrollToken
			s.renderLogin(w, r, http.StatusOK, v)
		default:
			resp.cookies(w)
			s.leave(w, r, v.Ctx.landing())
		}
	case http.StatusTooManyRequests:
		if n := resp.retryAfter(); n > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(n))
			v.Error = "Too many failed attempts. Try again in " + strconv.Itoa(n) + " seconds."
		} else {
			v.Error = "Too many attempts. Try again later."
		}
		s.renderLogin(w, r, http.StatusTooManyRequests, v)
	case http.StatusUnauthorized:
		v.Error = resp.message("Invalid credentials")
		s.renderLogin(w, r, http.StatusUnauthorized, v)
	case http.StatusForbidden:
		if resp.code() == "SSO_REQUIRED" {
			// The domain moved to its identity provider: send them back to
			// the email step, which takes them there.
			v.Step, v.Error = stepEmail, "This email domain signs in through your organization's identity provider. Continue with your email to sign in there."
			s.renderLogin(w, r, http.StatusForbidden, v)
			return
		}
		v.Error = resp.message("Sign-in is not allowed for this request.")
		s.renderLogin(w, r, http.StatusForbidden, v)
	default:
		v.Error = "Sign-in failed. Try again."
		s.renderLogin(w, r, http.StatusBadGateway, v)
	}
}

// twoFactor is the second factor step: verify a code (/auth/2fa/verify, with
// the same backoff as the password), email a code, or switch method.
func (s *SignIn) twoFactor(w http.ResponseWriter, r *http.Request) {
	v := signinView{
		Ctx: signinContextFrom(r), Step: stepTwoFactor, Email: strings.TrimSpace(r.PostFormValue("email")),
		MFAToken:        r.PostFormValue("mfaToken"),
		Method:          r.PostFormValue("method"),
		RememberAllowed: r.PostFormValue("remember") == "1",
	}
	for m := range strings.SplitSeq(r.PostFormValue("methods"), ",") {
		if m == "TOTP" || m == "EMAIL_PIN" {
			v.Methods = append(v.Methods, m)
		}
	}
	if v.Method != "TOTP" && v.Method != "EMAIL_PIN" && v.Method != "RECOVERY_CODE" {
		v.Method = "TOTP"
	}
	if v.MFAToken == "" || len(v.MFAToken) > 4096 {
		v.Step = stepEmail
		v.Error = "Your sign-in expired. Start again."
		s.renderLogin(w, r, http.StatusUnauthorized, v)
		return
	}
	if sw := r.PostFormValue("switch"); sw == "TOTP" || sw == "EMAIL_PIN" || sw == "RECOVERY_CODE" {
		v.Method = sw
		s.renderLogin(w, r, http.StatusOK, v)
		return
	}
	if r.PostFormValue("action") == "send" {
		resp := s.call(r, http.MethodPost, "/auth/2fa/challenge/email", map[string]string{"mfaToken": v.MFAToken})
		switch resp.Status {
		case http.StatusOK:
			v.EmailSent = true
			s.renderLogin(w, r, http.StatusOK, v)
		case http.StatusTooManyRequests:
			// A code was sent within the resend interval: that one still works.
			if n := resp.retryAfter(); n > 0 {
				w.Header().Set("Retry-After", strconv.Itoa(n))
			}
			v.EmailSent, v.Error = true, resp.message("A code was sent recently. Wait a minute before asking for another.")
			s.renderLogin(w, r, http.StatusTooManyRequests, v)
		case http.StatusBadRequest:
			v.Error = "Email codes are not available for your account."
			s.renderLogin(w, r, http.StatusBadRequest, v)
		default:
			v.Error = resp.message("Could not send code")
			s.renderLogin(w, r, http.StatusBadGateway, v)
		}
		return
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	if code == "" {
		v.EmailSent = v.Method == "EMAIL_PIN" && r.PostFormValue("sent") == "1"
		v.Error = "Enter your code"
		s.renderLogin(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	resp := s.call(r, http.MethodPost, "/auth/2fa/verify", map[string]any{
		"mfaToken": v.MFAToken, "method": v.Method, "code": code,
		"rememberDevice": v.RememberAllowed && r.PostFormValue("rememberDevice") != "",
	})
	v.EmailSent = v.Method == "EMAIL_PIN"
	switch resp.Status {
	case http.StatusUnauthorized:
		v.Error = "Invalid or expired code"
		s.renderLogin(w, r, http.StatusUnauthorized, v)
		return
	case http.StatusBadRequest:
		// A method the account cannot use (not held, or not allowed by the
		// domain; a recovery code without the authenticator app).
		v.Error = "That method is not available for your account."
		s.renderLogin(w, r, http.StatusBadRequest, v)
		return
	}
	s.afterCredentials(w, r, resp, v)
}

// requestSetup emails the set-password link to an internal user who never set
// a password (the answer is the same whoever the address belongs to).
func (s *SignIn) requestSetup(w http.ResponseWriter, r *http.Request) {
	v := signinView{Ctx: signinContextFrom(r), Step: stepSetup, Email: strings.TrimSpace(r.PostFormValue("email"))}
	body := map[string]any{"email": v.Email}
	// Only an OAuth round trip is carried through the link: the rebuilt
	// authorize URL is what brings the user back to the application.
	if v.Ctx.get("oauth") == "true" {
		body["redirectUri"] = v.Ctx.authorizeURL()
	}
	resp := s.call(r, http.MethodPost, "/auth/password-setup/request", body)
	if resp.Status != http.StatusOK {
		v.Error = "Could not send the email — please try again."
		s.renderLogin(w, r, http.StatusBadGateway, v)
		return
	}
	v.Step = stepSetupSent
	s.renderLogin(w, r, http.StatusOK, v)
}

// callback is where a sign-in through an organization's identity provider
// lands: the OIDC bridge has set the session cookie and returns here. With a
// session it continues to the validated destination; without one (the provider
// refused, or the browser dropped the cookie) it says so and offers a retry.
func (s *SignIn) callback(w http.ResponseWriter, r *http.Request) {
	ctx := signinContextFrom(r)
	th := s.theme(r, ctx.client())
	if e := r.URL.Query().Get("error"); e != "" {
		s.render(w, r, http.StatusUnauthorized, "Authentication failed", th, false, signinFailureBody(th, callbackErrorMessage(e)))
		return
	}
	if signedIn(r) {
		http.Redirect(w, r, ctx.landing(), http.StatusSeeOther)
		return
	}
	s.render(w, r, http.StatusUnauthorized, "Authentication failed", th, false,
		signinFailureBody(th, "The sign-in did not complete. Try signing in again."))
}

// callbackErrorMessage is the sentence shown for an error code the bridge
// returned with: a fixed one per known OAuth code, else a generic one. The
// error_description (and an unknown code) is never shown: it is whatever the
// link says, so it would let anyone put words on this page.
func callbackErrorMessage(code string) string {
	switch code {
	case "access_denied":
		return "The sign-in was cancelled or refused by your organization's identity provider."
	case "login_required", "interaction_required", "consent_required", "account_selection_required":
		return "Your organization's identity provider needs you to sign in again."
	case "invalid_request", "invalid_scope", "unauthorized_client", "unsupported_response_type", "invalid_client", "invalid_grant":
		return "The sign-in request was not accepted. Try signing in again."
	case "server_error", "temporarily_unavailable":
		return "Your organization's identity provider is unavailable. Try again shortly."
	}
	return "The sign-in did not complete. Try signing in again."
}

func (s *SignIn) logoutPage(w http.ResponseWriter, r *http.Request) {
	th := s.theme(r, "")
	s.render(w, r, http.StatusOK, "Log out", th, false, signinLogoutBody(th))
}

// logout ends the session: the platform's own /auth/logout clears the cookie
// with the attributes it was set with.
func (s *SignIn) logout(w http.ResponseWriter, r *http.Request) {
	s.call(r, http.MethodPost, "/auth/logout", nil).cookies(w)
	http.Redirect(w, r, "/ui/auth/login", http.StatusSeeOther)
}

// signedIn reports whether the request carries a valid session.
func signedIn(r *http.Request) bool { return auth.FromContext(r.Context()) != nil }

func queryEscape(s string) string { return url.QueryEscape(s) }
