package pages

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/login"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/guard"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The two-factor sign-in pages (/ui/auth/2fa, /ui/auth/2fa/enroll) and the
// signed-in security page (/ui/auth/security, in auth_mfa_security.go). They
// are the HTML of the platform's own second-factor flow, login.TwoFactor: the
// one the /auth/2fa/* endpoints call. Every rule (the usable factors, the
// domain's allowed methods, recovery codes, remember-device, the backoff, the
// enroll token's limits) and the session itself are that flow's; the pages
// carry the between-step token in a cookie, show the steps and map the
// refusals to messages. The password step belongs to the sign-in pages, which
// hand over the token /auth/login answered with (start).

// PrincipalLookup loads the principal a session names.
type PrincipalLookup interface {
	FindByID(ctx context.Context, id ids.PrincipalID) (*principal.Principal, error)
}

// AuthMFA serves the two-factor pages.
type AuthMFA struct {
	// Flow is the platform's second-factor flow (the login endpoint's
	// TwoFactor()): the rules, the cookies and the session.
	Flow *login.TwoFactor
	// Principals loads the signed-in user of the security page.
	Principals PrincipalLookup
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
)

// methodRecovery is the challenge method of a recovery code.
const methodRecovery = login.MethodRecoveryCode

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
}

// ── from the password step ─────────────────────────────────────────────

// startChallenge takes the pending token /auth/login answered with
// (mfa_required) from the sign-in page, keeps it in the challenge cookie and
// shows the challenge. A token the flow does not accept starts nothing.
func (h *AuthMFA) startChallenge(w http.ResponseWriter, r *http.Request) {
	token := r.PostFormValue("token")
	c, err := h.Flow.PendingChallenge(r.Context(), token)
	if err != nil || len(c.Methods) == 0 {
		h.expired(w, r)
		return
	}
	h.setChallengeCookie(w, token, h.Flow.PendingTTL())
	http.Redirect(w, r, authChallengePath+"?next="+url.QueryEscape(authNext(r.PostFormValue("next"))), http.StatusSeeOther)
}

// startEnroll is the same for the enroll token (enrollment_required).
func (h *AuthMFA) startEnroll(w http.ResponseWriter, r *http.Request) {
	token := r.PostFormValue("token")
	if _, err := h.Flow.PendingEnrollment(r.Context(), token); err != nil {
		h.expired(w, r)
		return
	}
	h.setChallengeCookie(w, token, h.Flow.EnrollTTL())
	http.Redirect(w, r, authEnrollPath+"?next="+url.QueryEscape(authNext(r.PostFormValue("next"))), http.StatusSeeOther)
}

// signedIn ends a step the flow completed (it set the session cookie): the
// challenge cookie goes, the browser goes on to next.
func (h *AuthMFA) signedIn(w http.ResponseWriter, r *http.Request, next string) {
	h.clearChallengeCookie(w)
	http.Redirect(w, r, authNext(next), http.StatusSeeOther) //nolint:gosec // G710: authNext allows only a same-site path under /ui
}

// ── the challenge ──────────────────────────────────────────────────────

type mfaChallengeView struct {
	Next string
	// Methods are the factor types the user can use; Active is the one shown
	// (a Methods entry or methodRecovery); Recovery whether a recovery code is
	// accepted (a usable authenticator app).
	Methods         []string
	Active          string
	Recovery        bool
	RememberAllowed bool
	EmailSent       bool
	Error           string
}

func (v mfaChallengeView) has(method string) bool { return slices.Contains(v.Methods, method) }

func (h *AuthMFA) challengePage(w http.ResponseWriter, r *http.Request) {
	c := h.pendingChallenge(r)
	if c == nil {
		h.expired(w, r)
		return
	}
	v := challengeView(c, authNext(r.URL.Query().Get("next")), r.URL.Query().Get("method"))
	h.render(w, r, http.StatusOK, "Two-factor verification", nil, mfaChallenge(v))
}

// pendingChallenge is the challenge the cookie's pending token stands for, or
// nil (no cookie, a token the flow refuses, no usable factor left).
func (h *AuthMFA) pendingChallenge(r *http.Request) *login.Challenge {
	token := challengeToken(r)
	if token == "" {
		return nil
	}
	c, err := h.Flow.PendingChallenge(r.Context(), token)
	if err != nil {
		if !errors.Is(err, login.ErrInvalidToken) {
			slog.ErrorContext(r.Context(), "ui: 2FA challenge not loaded", "err", err)
		}
		return nil
	}
	if len(c.Methods) == 0 {
		return nil
	}
	return c
}

// challengeView is the page for c showing method when c accepts it.
func challengeView(c *login.Challenge, next, method string) mfaChallengeView {
	v := mfaChallengeView{Next: next, Methods: c.Methods, RememberAllowed: c.RememberAllowed, Recovery: c.Allows(methodRecovery)}
	switch {
	case c.Allows(method):
		v.Active = method
	case v.has(string(mfa.MethodTOTP)):
		v.Active = string(mfa.MethodTOTP)
	default:
		v.Active = string(mfa.MethodEmailPin)
	}
	return v
}

func (h *AuthMFA) verify(w http.ResponseWriter, r *http.Request) {
	c := h.pendingChallenge(r)
	if c == nil {
		h.expired(w, r)
		return
	}
	next := authNext(r.PostFormValue("next"))
	method := strings.ToUpper(strings.TrimSpace(r.PostFormValue("method")))
	code := strings.TrimSpace(r.PostFormValue("code"))
	retry := func(status int, msg string) {
		v := challengeView(c, next, method)
		v.Error = msg
		v.EmailSent = v.Active == string(mfa.MethodEmailPin)
		h.render(w, r, status, "Two-factor verification", nil, mfaChallenge(v))
	}
	if code == "" {
		retry(http.StatusUnprocessableEntity, "Enter your code.")
		return
	}
	_, err := h.Flow.Verify(w, r, challengeToken(r), method, code, r.PostFormValue("remember") != "")
	var throttled *login.ThrottledError
	switch {
	case err == nil:
		h.signedIn(w, r, next)
	case errors.Is(err, login.ErrInvalidToken):
		h.expired(w, r)
	case errors.As(err, &throttled):
		w.Header().Set("Retry-After", strconv.FormatUint(uint64(throttled.RetryAfterSecs), 10))
		retry(http.StatusTooManyRequests, "Too many failed attempts. Try again later.")
	case errors.Is(err, login.ErrInvalidMethod):
		h.Shell.Deny(w, r, http.StatusBadRequest, "Unknown two-factor method.")
	case errors.Is(err, login.ErrMethodNotUsable):
		retry(http.StatusBadRequest, "That method is not available for your account.")
	case errors.Is(err, login.ErrInvalidCode):
		retry(http.StatusUnauthorized, "Invalid or expired code")
	default:
		slog.ErrorContext(r.Context(), "ui: 2FA verify failed", "err", err)
		h.Shell.Deny(w, r, http.StatusInternalServerError, "Could not verify the code. Try again.")
	}
}

// sendEmail sends the sign-in code by email (the "Email me a code" button).
func (h *AuthMFA) sendEmail(w http.ResponseWriter, r *http.Request) {
	c := h.pendingChallenge(r)
	if c == nil {
		h.expired(w, r)
		return
	}
	v := challengeView(c, authNext(r.PostFormValue("next")), string(mfa.MethodEmailPin))
	err := h.Flow.SendChallengeEmail(r.Context(), challengeToken(r))
	var resend *mfa.ResendError
	status := http.StatusUnprocessableEntity
	switch {
	case err == nil:
		v.EmailSent, status = true, http.StatusOK
	case errors.Is(err, login.ErrInvalidToken):
		h.expired(w, r)
		return
	case errors.As(err, &resend):
		// The code already sent is still the one to enter.
		secs := int(math.Ceil(resend.Wait.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		v.EmailSent, status = true, http.StatusTooManyRequests
		v.Error = "A code was sent recently. Wait " + strconv.Itoa(secs) + " seconds before asking for another."
	case errors.Is(err, login.ErrMethodNotUsable):
		v.Error = "Email codes are not set up for this account."
	case errors.Is(err, login.ErrNoEmail):
		v.Error = "This account has no email address."
	default:
		slog.ErrorContext(r.Context(), "ui: send email pin failed", "err", err)
		v.Error = "Could not send code"
	}
	h.render(w, r, status, "Two-factor verification", nil, mfaChallenge(v))
}

// ── tokens and cookies ─────────────────────────────────────────────────

// challengeToken is the between-step token the challenge cookie carries ("":
// none).
func challengeToken(r *http.Request) string {
	c, err := r.Cookie(authChallengeCookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func (h *AuthMFA) challengeCookie(value string, ttl time.Duration) *http.Cookie {
	c := platformmw.NewCookie(platformmw.CookieSpec{
		Name: authChallengeCookie, Value: value, Path: "/ui/auth", Secure: h.Flow != nil && h.Flow.CookieSecure, TTL: ttl,
	})
	c.HttpOnly = true
	return c
}

func (h *AuthMFA) setChallengeCookie(w http.ResponseWriter, token string, ttl time.Duration) {
	http.SetCookie(w, h.challengeCookie(token, ttl))
}

func (h *AuthMFA) clearChallengeCookie(w http.ResponseWriter) {
	http.SetCookie(w, h.challengeCookie("", -1))
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

// ── small helpers ──────────────────────────────────────────────────────

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
