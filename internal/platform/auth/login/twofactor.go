package login

import (
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	platformmw "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/middleware"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// The /auth/2fa/* JSON endpoints of a password sign-in. The rules are
// TwoFactor's (twofactor_flow.go); these handlers decode, call and answer.

// Token TTL fallbacks (overridable via Config).
const (
	defaultPendingTokenTTL = 10 * time.Minute
	defaultEnrollTokenTTL  = 30 * time.Minute
)

// Trusted-device cookie. In production (Secure) we use the __Host- prefix for
// the strongest binding the platform offers; over plain-HTTP localhost the
// prefix is invalid, so dev falls back to a bare name.
const (
	trustedDeviceCookieProd = "__Host-fc_td"
	trustedDeviceCookieDev  = "fc_td"
)

// twoFactorResponse is the body for the two pending outcomes of /auth/login.
type twoFactorResponse struct {
	Status string `json:"status"` // "mfa_required" | "enrollment_required"
	// MFAToken gates the challenge endpoints (status mfa_required).
	MFAToken string `json:"mfaToken,omitempty"`
	// EnrollToken gates the enrollment endpoints (status enrollment_required).
	EnrollToken string `json:"enrollToken,omitempty"`
	// Methods are the factor types the user can challenge with (mfa_required).
	Methods []string `json:"methods,omitempty"`
	// AllowedMethods are the factor types the user may enroll
	// (enrollment_required).
	AllowedMethods []string `json:"allowedMethods,omitempty"`
	// RememberDeviceAllowed advertises the per-domain remember-device option.
	RememberDeviceAllowed bool `json:"rememberDeviceAllowed,omitempty"`
}

// RegisterTwoFactorRoutes mounts the /auth/2fa/* endpoints. No-op when MFA
// isn't wired, so callers can register unconditionally.
func (e *Endpoint) RegisterTwoFactorRoutes(r chi.Router) {
	if e.tf == nil || e.cfg.MFATokens == nil {
		return
	}
	r.Post("/auth/2fa/verify", e.handle2FAVerify)
	r.Post("/auth/2fa/challenge/email", e.handle2FAChallengeEmail)
	r.Post("/auth/2fa/enroll/totp/begin", e.handle2FAEnrollTOTPBegin)
	r.Post("/auth/2fa/enroll/totp/confirm", e.handle2FAEnrollTOTPConfirm)
	r.Post("/auth/2fa/enroll/email/begin", e.handle2FAEnrollEmailBegin)
	r.Post("/auth/2fa/enroll/email/confirm", e.handle2FAEnrollEmailConfirm)
}

// TwoFactor is the endpoint's second-factor flow (nil when MFA isn't wired),
// for the server-rendered sign-in pages to share.
func (e *Endpoint) TwoFactor() *TwoFactor { return e.tf }

// ── decision (called from handleLogin after the password verifies) ─────────

// maybeChallenge2FA writes the mfa_required / enrollment_required answer and
// returns handled=true when a second factor is owed; handled=false lets the
// caller complete the login. A non-nil error means "couldn't decide" and the
// caller fails closed.
func (e *Endpoint) maybeChallenge2FA(w http.ResponseWriter, r *http.Request, p *principal.Principal) (bool, error) {
	d, err := e.tf.Decide(r, p)
	if err != nil {
		return false, err
	}
	switch d.Kind {
	case ChallengeOwed:
		writeJSON(w, http.StatusOK, twoFactorResponse{
			Status: "mfa_required", MFAToken: d.Token, Methods: d.Methods, RememberDeviceAllowed: d.RememberAllowed,
		})
		return true, nil
	case EnrollmentOwed:
		writeJSON(w, http.StatusOK, twoFactorResponse{
			Status: "enrollment_required", EnrollToken: d.Token, AllowedMethods: d.AllowedMethods,
		})
		return true, nil
	case NoSecondFactor:
	}
	return false, nil
}

// ── challenge / verify ─────────────────────────────────────────────────────

type verifyRequest struct {
	MFAToken       string `json:"mfaToken"`
	Method         string `json:"method"`
	Code           string `json:"code"`
	RememberDevice bool   `json:"rememberDevice"`
}

func (e *Endpoint) handle2FAVerify(w http.ResponseWriter, r *http.Request) {
	var req verifyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := e.tf.Verify(w, r, req.MFAToken, req.Method, req.Code, req.RememberDevice)
	if err != nil {
		if errors.Is(err, ErrInvalidCode) {
			writeUnauthorized(w, "Invalid or expired code")
			return
		}
		e.write2FAErr(w, err)
		return
	}
	e.writeLoginOK(w, r, p, nil)
}

type challengeEmailRequest struct {
	MFAToken string `json:"mfaToken"`
}

func (e *Endpoint) handle2FAChallengeEmail(w http.ResponseWriter, r *http.Request) {
	var req challengeEmailRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if err := e.tf.SendChallengeEmail(r.Context(), req.MFAToken); err != nil {
		e.write2FAErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "A verification code has been sent to your email."})
}

// ── enrollment (gated by an enroll token) ──────────────────────────────────

type enrollBeginRequest struct {
	EnrollToken string `json:"enrollToken"`
}

type enrollConfirmRequest struct {
	EnrollToken string `json:"enrollToken"`
	Code        string `json:"code"`
}

func (e *Endpoint) handle2FAEnrollTOTPBegin(w http.ResponseWriter, r *http.Request) {
	var req enrollBeginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	enr, err := e.tf.BeginEnrollment(r.Context(), req.EnrollToken, mfa.MethodTOTP)
	if err != nil {
		e.writeEnrollErr(w, mfa.MethodTOTP, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": enr.Secret, "uri": enr.URI, "qr": enr.QR})
}

func (e *Endpoint) handle2FAEnrollTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	e.enrollConfirm(w, r, mfa.MethodTOTP)
}

func (e *Endpoint) handle2FAEnrollEmailBegin(w http.ResponseWriter, r *http.Request) {
	var req enrollBeginRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if _, err := e.tf.BeginEnrollment(r.Context(), req.EnrollToken, mfa.MethodEmailPin); err != nil {
		e.writeEnrollErr(w, mfa.MethodEmailPin, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "A verification code has been sent to your email."})
}

func (e *Endpoint) handle2FAEnrollEmailConfirm(w http.ResponseWriter, r *http.Request) {
	e.enrollConfirm(w, r, mfa.MethodEmailPin)
}

// enrollConfirm confirms the enrolment and completes the login, returning the
// first recovery codes once.
func (e *Endpoint) enrollConfirm(w http.ResponseWriter, r *http.Request, method mfa.MethodType) {
	var req enrollConfirmRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	p, codes, err := e.tf.CompleteEnrollment(w, r, req.EnrollToken, method, req.Code)
	if err != nil {
		e.writeEnrollErr(w, method, err)
		return
	}
	e.writeLoginOK(w, r, p, codes)
}

// ── answers ───────────────────────────────────────────────────────────────

// write2FAErr answers a refusal of the flow.
func (e *Endpoint) write2FAErr(w http.ResponseWriter, err error) {
	var throttled *ThrottledError
	var resend *mfa.ResendError
	switch {
	case errors.Is(err, ErrInvalidToken):
		writeUnauthorized(w, "Invalid or expired session")
	case errors.As(err, &throttled):
		writeTooManyRequests(w, throttled.RetryAfterSecs)
	case errors.As(err, &resend):
		secs := int(math.Ceil(resend.Wait.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(secs))
		writeJSON(w, http.StatusTooManyRequests, errBody("CODE_RECENTLY_SENT",
			"A code was sent recently. Wait "+strconv.Itoa(secs)+" seconds before asking for another."))
	case errors.Is(err, ErrInvalidMethod):
		writeJSON(w, http.StatusBadRequest, errBody("INVALID_METHOD", "unknown 2FA method"))
	case errors.Is(err, ErrMethodNotUsable):
		writeJSON(w, http.StatusBadRequest, errBody("METHOD_NOT_ALLOWED", "that method is not available for this account"))
	case errors.Is(err, ErrNoEmail):
		writeJSON(w, http.StatusBadRequest, errBody("NO_EMAIL", "account has no email"))
	case errors.Is(err, ErrSessionNotIssued):
		// The factor passed; a mint failure is a server-side fault (signing key
		// missing or unreadable). Internal keeps the cause off the wire.
		httperror.Write(w, usecase.Internal("MINT_FAILED", "failed to mint session token", err))
	case errors.Is(err, ErrEmailNotSent):
		slog.Error("send email pin failed", "err", err)
		writeJSON(w, http.StatusBadGateway, errBody("EMAIL_SEND_FAILED", "could not send code"))
	default:
		slog.Error("2FA verify failed", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody("VERIFY_FAILED", "could not verify code"))
	}
}

// writeEnrollErr answers a refused enrolment step of method.
func (e *Endpoint) writeEnrollErr(w http.ResponseWriter, method mfa.MethodType, err error) {
	switch {
	case errors.Is(err, ErrInvalidToken):
		writeUnauthorized(w, "Invalid or expired session")
	case errors.Is(err, ErrEnrollmentNotOwed):
		writeJSON(w, http.StatusConflict, errBody("ENROLLMENT_NOT_REQUIRED", "a second factor is already set up for this account; sign in again"))
	case errors.Is(err, ErrInvalidCode):
		writeJSON(w, http.StatusBadRequest, errBody("INVALID_CODE", "that code didn't match — try again"))
	case errors.Is(err, ErrMethodNotAllowed):
		msg := "email codes are not permitted for this domain"
		if method == mfa.MethodTOTP {
			msg = "authenticator app is not permitted for this domain"
		}
		writeJSON(w, http.StatusForbidden, errBody("METHOD_NOT_ALLOWED", msg))
	case errors.Is(err, ErrInvalidMethod):
		writeJSON(w, http.StatusBadRequest, errBody("INVALID_METHOD", "unknown 2FA method"))
	case errors.Is(err, ErrNoEmail):
		writeJSON(w, http.StatusBadRequest, errBody("NO_EMAIL", "account has no email"))
	case errors.Is(err, ErrSessionNotIssued):
		httperror.Write(w, usecase.Internal("MINT_FAILED", "failed to mint session token", err))
	case errors.Is(err, mfa.ErrAlreadyEnrolled):
		writeJSON(w, http.StatusConflict, errBody("ALREADY_ENROLLED", "that method is already set up"))
	case errors.Is(err, mfa.ErrEncryptionUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, errBody("TOTP_UNAVAILABLE", "authenticator-app 2FA is not available"))
	case errors.Is(err, mfa.ErrNoPendingEnrollment):
		writeJSON(w, http.StatusBadRequest, errBody("NO_PENDING_ENROLLMENT", "start enrollment first"))
	default:
		slog.Error("2FA enrollment error", "err", err)
		writeJSON(w, http.StatusInternalServerError, errBody("ENROLL_FAILED", "could not complete enrollment"))
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

// clearTrustedDeviceCookie expires the browser's trusted-device cookie. The
// server-side revocation is what actually invalidates the device; this just
// stops the client presenting a dead token on every login until it expires.
// Attributes match rememberDevice's so the browser treats it as the same cookie.
func (e *Endpoint) clearTrustedDeviceCookie(w http.ResponseWriter) {
	http.SetCookie(w, platformmw.NewCookie(platformmw.CookieSpec{
		Name: e.trustedDeviceCookieName(), Secure: e.cfg.CookieSecure,
		SameSite: http.SameSiteStrictMode, TTL: -1,
	}))
}

func (e *Endpoint) trustedDeviceCookieName() string {
	return trustedDeviceCookieName(e.cfg.CookieSecure)
}

func trustedDeviceCookieName(secure bool) string {
	if secure {
		return trustedDeviceCookieProd
	}
	return trustedDeviceCookieDev
}

// ── small pure helpers ─────────────────────────────────────────────────────

func emailOf(p *principal.Principal) string {
	if p.UserIdentity != nil {
		return p.UserIdentity.Email
	}
	return ""
}

func methodStrings(ms []mfa.MethodType) []string {
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, string(m))
	}
	return out
}

func intersect(a, allowed []string) []string {
	out := make([]string, 0, len(a))
	for _, x := range a {
		if slices.Contains(allowed, x) {
			out = append(out, x)
		}
	}
	return out
}

func userAgentLabel(r *http.Request) *string {
	ua := strings.TrimSpace(r.UserAgent())
	if ua == "" {
		return nil
	}
	if len(ua) > 250 {
		ua = ua[:250]
	}
	return &ua
}

// decodeJSON decodes the request body into v, writing a 400 (returning false)
// on malformed input.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": "INVALID_JSON", "message": "malformed request body"})
		return false
	}
	return true
}
