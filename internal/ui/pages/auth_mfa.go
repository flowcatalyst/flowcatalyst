package pages

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/audit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/loginbackoff"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/mfatoken"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/twofa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/notify"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/guard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The two-factor sign-in pages (/ui/auth/2fa, /ui/auth/2fa/enroll) and the
// signed-in security page (/ui/auth/security, in auth_mfa_security.go). They
// do what the current UI's /auth/2fa/* endpoints and TwoFactorChallenge,
// TwoFactorSetup and TwoFactorSection components do, with the same services
// (mfa.Service, the mfatoken issuer, twofa.Policy, the login-attempt backoff)
// and the same rules. The password step belongs to the sign-in pages, which
// hand over the token /auth/login answered with (start); the session is issued
// here, once the factor is satisfied, as /auth/2fa/verify does.

// MFAService is the part of mfa.Service the pages use.
type MFAService interface {
	ConfirmedMethods(ctx context.Context, principalID string) ([]mfa.MethodType, error)
	VerifyTOTP(ctx context.Context, principalID, code string) (bool, error)
	VerifyLoginEmailPin(ctx context.Context, principalID, code string) (bool, error)
	SendLoginEmailPin(ctx context.Context, principalID, emailAddr string) error
	VerifyRecoveryCode(ctx context.Context, principalID, code string) (bool, error)
	BeginTOTPEnrollment(ctx context.Context, principalID, accountName string) (*mfa.TOTPEnrollment, error)
	ConfirmTOTPEnrollment(ctx context.Context, principalID, code string) (bool, error)
	BeginEmailEnrollment(ctx context.Context, principalID, emailAddr string) error
	ConfirmEmailEnrollment(ctx context.Context, principalID, code string) (bool, error)
	RemoveMethod(ctx context.Context, principalID string, t mfa.MethodType) error
	GenerateRecoveryCodes(ctx context.Context, principalID string) ([]string, error)
	RemainingRecoveryCodes(ctx context.Context, principalID string) (int, error)
	IssueTrustedDevice(ctx context.Context, principalID string, label *string, ttl time.Duration) (string, error)
	VerifyTrustedDevice(ctx context.Context, principalID, raw string) (bool, error)
	ListTrustedDevices(ctx context.Context, principalID string) ([]mfa.TrustedDevice, error)
	RevokeTrustedDevice(ctx context.Context, principalID, id string) error
}

// MFATokens is the between-step token issuer (mfatoken.Issuer): the tokens
// /auth/login hands out with a pending or enrollment answer.
type MFATokens interface {
	Parse(token string, want mfatoken.Purpose) (*mfatoken.Claims, error)
}

// SessionIssuer mints the session JWT (provider.Provider), as login does.
type SessionIssuer interface {
	MintSessionToken(ctx context.Context, principalID string, ttl time.Duration) (string, error)
}

// TwoFactorPolicy resolves a domain's 2FA stance (twofa.Policy).
type TwoFactorPolicy interface {
	Evaluate(ctx context.Context, email string) twofa.Eval
}

// PrincipalLookup loads the principal a token or a session names.
type PrincipalLookup interface {
	FindByID(ctx context.Context, id ids.PrincipalID) (*principal.Principal, error)
}

// AuthMFA serves the two-factor pages.
type AuthMFA struct {
	MFA        MFAService
	Tokens     MFATokens
	Policy     TwoFactorPolicy
	Principals PrincipalLookup
	// Attempts feeds the second factor's brute-force backoff and records the
	// failures, keyed per (email, IP) like the password step (nil: neither).
	Attempts *loginattempt.Repository
	Backoff  loginbackoff.Policy
	// Notifier and Audit are the best-effort security emails and audit rows
	// of the 2FA endpoints (both optional).
	Notifier *notify.Notifier
	Audit    *audit.Repository
	// CookieSecure is the Secure flag of the cookies the pages set.
	CookieSecure bool
	// PendingTTL and EnrollTTL bound the challenge cookie (zero: 10 and 30
	// minutes, the login endpoint's token lifetimes).
	PendingTTL, EnrollTTL time.Duration
	// Sessions and SessionTTL issue the session once the second factor is
	// satisfied: the same JWT and cookie as POST /auth/login (zero TTL: 24h).
	Sessions   SessionIssuer
	SessionTTL time.Duration
	// SignInPath is where an expired or missing challenge sends the browser
	// (zero: the current sign-in page).
	SignInPath string

	// Passkeys and PasskeyOps back the passkey list of the security page.
	Passkeys   PasskeyStore
	PasskeyOps PasskeyCommands

	Shell *shell.Shell
}

const (
	authChallengePath = "/ui/auth/2fa"
	authEnrollPath    = "/ui/auth/2fa/enroll"

	// authChallengeCookie carries the between-step token (pending or enroll)
	// from the password step to these pages: HttpOnly, scoped to /ui/auth,
	// gone when the step ends.
	authChallengeCookie = "fc_ui_mfa"

	defaultPendingTTL = 10 * time.Minute
	defaultEnrollTTL  = 30 * time.Minute

	// Trusted-device cookie names, as the login endpoint's (the __Host-
	// prefix needs Secure).
	trustedDeviceCookieProd = "__Host-fc_td"
	trustedDeviceCookieDev  = "fc_td"
)

// mfaEnrollPurpose is the token purpose of the enrollment pages.
const mfaEnrollPurpose = mfatoken.PurposeEnroll

// methodRecovery is the challenge method of a recovery code.
const methodRecovery = "RECOVERY_CODE"

// allMFAMethods is the menu when a domain does not restrict the methods.
var allMFAMethods = []string{string(mfa.MethodTOTP), string(mfa.MethodEmailPin)}

// MountPublic registers the pages that need no session: the challenge and
// the enrollment a password-verified user is sent to, and the two POST
// endpoints that hand them the token /auth/login answered with (start). Mount
// it in a group without the session guard.
func (h *AuthMFA) MountPublic(r chi.Router) {
	r.Get("/auth/2fa", h.challengePage)
	r.Get("/auth/2fa/enroll", h.enrollPage)
	post := r.With(parseForm)
	post.Post("/auth/2fa/start", h.startChallenge)
	post.Post("/auth/2fa/enroll/start", h.startEnroll)
	post.Post("/auth/2fa/verify", h.verify)
	post.Post("/auth/2fa/email", h.sendEmail)
	post.Post("/auth/2fa/enroll/totp", h.enrollTOTPBegin)
	post.Post("/auth/2fa/enroll/totp/confirm", h.enrollTOTPConfirm)
	post.Post("/auth/2fa/enroll/email", h.enrollEmailBegin)
	post.Post("/auth/2fa/enroll/email/confirm", h.enrollEmailConfirm)
	post.Post("/auth/2fa/enroll/done", h.enrollDone)
}

// usable is the factor types p can challenge with: their confirmed ones,
// narrowed to the domain's allowed set when the domain enforces 2FA.
func (h *AuthMFA) usable(ctx context.Context, p *principal.Principal, ev twofa.Eval) ([]string, error) {
	confirmed, err := h.MFA.ConfirmedMethods(ctx, string(p.ID))
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(confirmed))
	for _, m := range confirmed {
		out = append(out, string(m))
	}
	if ev.Requires2FA() {
		out = slices.DeleteFunc(out, func(m string) bool { return !slices.Contains(ev.AllowedMethods(), m) })
	}
	return out, nil
}

// ── from the password step ─────────────────────────────────────────────

// startChallenge takes the pending token /auth/login answered with
// (mfa_required) from the sign-in page, keeps it in the challenge cookie and
// shows the challenge. A token that does not parse as a pending token for an
// active user starts nothing.
func (h *AuthMFA) startChallenge(w http.ResponseWriter, r *http.Request) {
	h.start(w, r, mfatoken.PurposePending, h.pendingTTL(), authChallengePath)
}

// startEnroll is the same for the enroll token (enrollment_required).
func (h *AuthMFA) startEnroll(w http.ResponseWriter, r *http.Request) {
	h.start(w, r, mfaEnrollPurpose, h.enrollTTL(), authEnrollPath)
}

func (h *AuthMFA) start(w http.ResponseWriter, r *http.Request, purpose mfatoken.Purpose, ttl time.Duration, page string) {
	token := r.PostFormValue("token")
	if token == "" || len(token) > 4096 {
		h.expired(w, r)
		return
	}
	claims, err := h.Tokens.Parse(token, purpose)
	if err != nil {
		h.expired(w, r)
		return
	}
	if p, err := h.Principals.FindByID(r.Context(), ids.PrincipalID(claims.Subject)); err != nil || p == nil || !p.Active {
		h.expired(w, r)
		return
	}
	h.setChallengeCookie(w, token, ttl)
	http.Redirect(w, r, page+"?next="+url.QueryEscape(authNext(r.PostFormValue("next"))), http.StatusSeeOther)
}

// completeSignIn issues the session once the second factor is satisfied, as
// the login endpoint's completeLogin does: the session JWT in the session
// cookie, the successful attempt recorded, then on to next. The challenge
// cookie ends here.
func (h *AuthMFA) completeSignIn(w http.ResponseWriter, r *http.Request, p *principal.Principal, next string) {
	ttl := h.SessionTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	token, err := h.Sessions.MintSessionToken(r.Context(), string(p.ID), ttl)
	if err != nil {
		slog.ErrorContext(r.Context(), "ui: session token not minted", "principal", p.ID, "err", err)
		h.Shell.Deny(w, r, http.StatusInternalServerError, "Could not sign you in. Try again.")
		return
	}
	h.clearChallengeCookie(w)
	http.SetCookie(w, platformmw.SessionCookie(token, h.CookieSecure, ttl, true))
	h.recordAttempt(r.Context(), p, ratelimit.ClientIP(r), loginattempt.OutcomeSuccess, "")
	http.Redirect(w, r, authNext(next), http.StatusSeeOther) //nolint:gosec // G710: authNext allows only a same-site path under /ui
}

// ── the challenge ──────────────────────────────────────────────────────

type mfaChallengeView struct {
	Next string
	// Methods are the factor types the user can use; Active is the one shown
	// (a Methods entry or methodRecovery).
	Methods         []string
	Active          string
	RememberAllowed bool
	EmailSent       bool
	Error           string
}

func (v mfaChallengeView) has(method string) bool { return slices.Contains(v.Methods, method) }

func (h *AuthMFA) challengePage(w http.ResponseWriter, r *http.Request) {
	p := h.tokenPrincipal(r, mfatoken.PurposePending)
	if p == nil {
		h.expired(w, r)
		return
	}
	v, ok, err := h.challengeView(r, p, authNext(r.URL.Query().Get("next")), r.URL.Query().Get("method"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ok {
		h.expired(w, r)
		return
	}
	h.render(w, r, http.StatusOK, "Two-factor verification", nil, mfaChallenge(v))
}

// challengeView builds the page for p; ok is false when p has nothing to
// challenge with (their factors were removed since the password step).
func (h *AuthMFA) challengeView(r *http.Request, p *principal.Principal, next, method string) (mfaChallengeView, bool, error) {
	ctx := r.Context()
	ev := h.Policy.Evaluate(ctx, mfaEmailOf(p))
	usable, err := h.usable(ctx, p, ev)
	if err != nil {
		return mfaChallengeView{}, false, err
	}
	if len(usable) == 0 {
		return mfaChallengeView{}, false, nil
	}
	v := mfaChallengeView{Next: next, Methods: usable, RememberAllowed: ev.RememberEnabled()}
	switch {
	case method == methodRecovery, v.has(method):
		v.Active = method
	case v.has(string(mfa.MethodTOTP)):
		v.Active = string(mfa.MethodTOTP)
	default:
		v.Active = string(mfa.MethodEmailPin)
	}
	return v, true, nil
}

func (h *AuthMFA) verify(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := h.tokenPrincipal(r, mfatoken.PurposePending)
	if p == nil {
		h.expired(w, r)
		return
	}
	next := authNext(r.PostFormValue("next"))
	method := strings.ToUpper(strings.TrimSpace(r.PostFormValue("method")))
	code := strings.TrimSpace(r.PostFormValue("code"))
	retry := func(status int, msg string) {
		v, ok, err := h.challengeView(r, p, next, method)
		if err != nil || !ok {
			h.expired(w, r)
			return
		}
		v.Error = msg
		v.EmailSent = method == string(mfa.MethodEmailPin)
		h.render(w, r, status, "Two-factor verification", nil, mfaChallenge(v))
	}

	// Brute-force backoff on the second factor, keyed per (email, IP): the
	// same store and policy as the password step.
	email, ip := mfaEmailOf(p), ratelimit.ClientIP(r)
	if h.Attempts != nil {
		if d, derr := loginbackoff.Check(ctx, h.Attempts, h.Backoff, email, ip); derr == nil && !d.Allowed {
			w.Header().Set("Retry-After", strconv.FormatUint(uint64(d.RetryAfterSecs), 10))
			retry(http.StatusTooManyRequests, "Too many failed attempts. Try again later.")
			return
		}
	}
	if code == "" {
		retry(http.StatusUnprocessableEntity, "Enter your code.")
		return
	}

	var ok bool
	var err error
	switch method {
	case string(mfa.MethodTOTP):
		ok, err = h.MFA.VerifyTOTP(ctx, string(p.ID), code)
	case string(mfa.MethodEmailPin):
		ok, err = h.MFA.VerifyLoginEmailPin(ctx, string(p.ID), code)
	case methodRecovery:
		ok, err = h.MFA.VerifyRecoveryCode(ctx, string(p.ID), code)
	default:
		h.Shell.Deny(w, r, http.StatusBadRequest, "Unknown two-factor method.")
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "ui: 2FA verify failed", "principal", p.ID, "err", err)
		h.Shell.Deny(w, r, http.StatusInternalServerError, "Could not verify the code. Try again.")
		return
	}
	if !ok {
		h.recordAttempt(ctx, p, ip, loginattempt.OutcomeFailure, "Invalid 2FA code")
		retry(http.StatusUnauthorized, "Invalid or expired code")
		return
	}

	// A recovery code just got burned: tell the user out-of-band.
	if method == methodRecovery {
		h.Notifier.RecoveryCodeUsed(ctx, email)
	}
	if r.PostFormValue("remember") != "" {
		h.rememberDevice(w, r, p)
	}
	h.clearChallengeCookie(w)
	h.completeSignIn(w, r, p, next)
}

// sendEmail sends the sign-in code by email (the "Email me a code" button).
func (h *AuthMFA) sendEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := h.tokenPrincipal(r, mfatoken.PurposePending)
	if p == nil {
		h.expired(w, r)
		return
	}
	v, ok, err := h.challengeView(r, p, authNext(r.PostFormValue("next")), string(mfa.MethodEmailPin))
	if err != nil || !ok {
		h.expired(w, r)
		return
	}
	email := mfaEmailOf(p)
	switch {
	case email == "":
		v.Error = "This account has no email address."
	case !v.has(string(mfa.MethodEmailPin)):
		v.Error = "Email codes are not set up for this account."
	default:
		if err := h.MFA.SendLoginEmailPin(ctx, string(p.ID), email); err != nil {
			slog.ErrorContext(ctx, "ui: send email pin failed", "principal", p.ID, "err", err)
			v.Error = "Could not send code"
		} else {
			v.EmailSent = true
		}
	}
	status := http.StatusOK
	if v.Error != "" {
		status = http.StatusUnprocessableEntity
	}
	h.render(w, r, status, "Two-factor verification", nil, mfaChallenge(v))
}

// ── trusted devices ────────────────────────────────────────────────────

// rememberDevice issues a trusted-device token and sets the cookie, when the
// domain allows it (a no-op otherwise, like the login endpoint).
func (h *AuthMFA) rememberDevice(w http.ResponseWriter, r *http.Request, p *principal.Principal) {
	ev := h.Policy.Evaluate(r.Context(), mfaEmailOf(p))
	if !ev.RememberEnabled() {
		return
	}
	ttl := time.Duration(ev.RememberDays()) * 24 * time.Hour
	var label *string
	if ua := strings.TrimSpace(r.UserAgent()); ua != "" {
		ua = ua[:min(len(ua), 250)]
		label = &ua
	}
	raw, err := h.MFA.IssueTrustedDevice(r.Context(), string(p.ID), label, ttl)
	if err != nil {
		slog.ErrorContext(r.Context(), "ui: issue trusted device failed", "principal", p.ID, "err", err)
		return
	}
	http.SetCookie(w, platformmw.NewCookie(platformmw.CookieSpec{
		Name: h.trustedDeviceCookieName(), Value: raw, Secure: h.CookieSecure,
		SameSite: http.SameSiteStrictMode, TTL: ttl, SetExpires: true,
	}))
	name := ""
	if label != nil {
		name = *label
	}
	h.Notifier.NewTrustedDevice(r.Context(), mfaEmailOf(p), name)
}

func (h *AuthMFA) trustedDeviceCookieName() string {
	if h.CookieSecure {
		return trustedDeviceCookieProd
	}
	return trustedDeviceCookieDev
}

// ── tokens and cookies ─────────────────────────────────────────────────

// tokenPrincipal is the active principal the challenge cookie's token names
// for the wanted purpose, or nil: no cookie, a bad, expired or other-purpose
// token (an enroll token is not a pending token), an unknown or inactive user.
func (h *AuthMFA) tokenPrincipal(r *http.Request, purpose mfatoken.Purpose) *principal.Principal {
	c, err := r.Cookie(authChallengeCookie)
	if err != nil || c.Value == "" {
		return nil
	}
	claims, err := h.Tokens.Parse(c.Value, purpose)
	if err != nil {
		return nil
	}
	p, err := h.Principals.FindByID(r.Context(), ids.PrincipalID(claims.Subject))
	if err != nil || p == nil || !p.Active {
		return nil
	}
	return p
}

func (h *AuthMFA) challengeCookie(value string, ttl time.Duration) *http.Cookie {
	return platformmw.NewCookie(platformmw.CookieSpec{
		Name: authChallengeCookie, Value: value, Path: "/ui/auth", Secure: h.CookieSecure, TTL: ttl,
	})
}

func (h *AuthMFA) setChallengeCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	c := h.challengeCookie(token, ttl)
	c.HttpOnly = true
	http.SetCookie(w, c)
}

func (h *AuthMFA) clearChallengeCookie(w http.ResponseWriter) {
	c := h.challengeCookie("", -1)
	c.HttpOnly = true
	http.SetCookie(w, c)
}

func (h *AuthMFA) pendingTTL() time.Duration {
	if h.PendingTTL > 0 {
		return h.PendingTTL
	}
	return defaultPendingTTL
}

func (h *AuthMFA) enrollTTL() time.Duration {
	if h.EnrollTTL > 0 {
		return h.EnrollTTL
	}
	return defaultEnrollTTL
}

// expired sends a browser whose challenge is gone back to sign in.
func (h *AuthMFA) expired(w http.ResponseWriter, r *http.Request) {
	h.clearChallengeCookie(w)
	to := h.SignInPath
	if to == "" {
		to = guard.LoginPath
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// recordAttempt logs a USER_LOGIN attempt for p; failures count with the
// password failures in the backoff.
func (h *AuthMFA) recordAttempt(ctx context.Context, p *principal.Principal, ip string, outcome loginattempt.Outcome, reason string) {
	if h.Attempts == nil {
		return
	}
	email := strings.ToLower(strings.TrimSpace(mfaEmailOf(p)))
	a := loginattempt.New(loginattempt.AttemptUserLogin, outcome)
	a.Identifier = &email
	a.PrincipalID = ids.StringPtr(&p.ID)
	if ip != "" {
		a.IPAddress = &ip
	}
	if reason != "" {
		a.FailureReason = &reason
	}
	_ = h.Attempts.Record(ctx, a)
}

// auditMFA records a 2FA state change (best effort), as the endpoint does.
func (h *AuthMFA) auditMFA(ctx context.Context, principalID, operation string) {
	if h.Audit == nil {
		return
	}
	pid := principalID
	_ = h.Audit.Insert(ctx, &audit.Log{
		ID:          tsid.Generate(tsid.AuditLog),
		EntityType:  "PRINCIPAL",
		EntityID:    principalID,
		Operation:   operation,
		PrincipalID: &pid,
		PerformedAt: time.Now().UTC(),
	})
}

// ── small helpers ──────────────────────────────────────────────────────

func mfaEmailOf(p *principal.Principal) string {
	if p.UserIdentity == nil {
		return ""
	}
	return p.UserIdentity.Email
}

// authNext is the page to return to after sign-in: a path on this site under
// /ui (the SPA's safeUiRedirect rule), else /ui/. The query is kept.
// Anything with a scheme or host, a backslash or control character, dot
// segments or an encoded dot, slash or backslash is refused.
func authNext(raw string) string {
	const fallback = "/ui/"
	if raw == "" || strings.ContainsAny(raw, "\\\r\n\t ") {
		return fallback
	}
	if l := strings.ToLower(raw); strings.Contains(l, "%2e") || strings.Contains(l, "%2f") || strings.Contains(l, "%5c") {
		return fallback
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.User != nil || u.Opaque != "" {
		return fallback
	}
	if u.Path != "/ui" && !strings.HasPrefix(u.Path, "/ui/") {
		return fallback
	}
	for seg := range strings.SplitSeq(u.Path, "/") {
		if seg == "." || seg == ".." {
			return fallback
		}
	}
	return u.RequestURI()
}

// render writes a sign-in page: the bare document, no app shell (the user is
// not signed in yet).
func (h *AuthMFA) render(w http.ResponseWriter, r *http.Request, status int, title string, scripts []string, content templ.Component) {
	renderAuthPage(w, r, status, title, scripts, content)
}
