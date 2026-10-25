package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/login"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
)

// Enrolling a second factor, in the two places the current UI does it: after
// the password when the domain compels a factor the user has none of (a
// between-step enroll token; the flow issues the session when the new factor
// is confirmed), and from the security page when signed in. The views and the
// messages are shared; the handlers of the second place are in
// auth_mfa_security.go.

// mfaEnrollView is the method menu.
type mfaEnrollView struct {
	Next    string
	Methods []string
	Error   string
}

// mfaTOTPSetup is the authenticator-app step: the QR code, the key for manual
// entry and the code field. URI is echoed back with a wrong code so the user
// can retry without scanning again; it is only drawn, the server confirms
// against the secret it stored.
type mfaTOTPSetup struct {
	Action, Next string
	URI, Secret  string
	QR           templ.Component
	Error        string
}

// mfaEmailSetup is the email step: the code field.
type mfaEmailSetup struct {
	Action, Next string
	Error        string
}

// mfaCodesView is the recovery-code step. The codes exist nowhere else: they
// are in this response (never cached) and in no URL, signal or cookie.
type mfaCodesView struct {
	Codes []string
	// Href is where the Continue button goes.
	Href string
}

// totpSetupFor draws the setup for an otpauth URI. ok is false for anything
// that is not an authenticator URI with a secret.
func totpSetupFor(action, next, uri string) (mfaTOTPSetup, bool) {
	if !strings.HasPrefix(uri, "otpauth://totp/") {
		return mfaTOTPSetup{}, false
	}
	u, err := url.Parse(uri)
	if err != nil {
		return mfaTOTPSetup{}, false
	}
	secret := u.Query().Get("secret")
	if secret == "" {
		return mfaTOTPSetup{}, false
	}
	qr, err := qrSVG(uri)
	if err != nil {
		slog.Warn("ui: QR code not drawn; the key is still shown", "err", err)
	}
	return mfaTOTPSetup{Action: action, Next: next, URI: uri, Secret: secret, QR: qr}, true
}

// errSetupNotDrawn: the flow's enrolment URI is not an authenticator URI.
var errSetupNotDrawn = errors.New("ui: the enrollment URI is not an authenticator URI")

// totpSetup draws the authenticator step for a begun enrolment.
func totpSetup(ctx context.Context, enr *mfa.TOTPEnrollment, action, next string) (mfaTOTPSetup, error) {
	if enr == nil {
		return mfaTOTPSetup{}, errSetupNotDrawn
	}
	setup, ok := totpSetupFor(action, next, enr.URI)
	if !ok {
		slog.ErrorContext(ctx, "ui: the enrollment URI is not an authenticator URI")
		return mfaTOTPSetup{}, errSetupNotDrawn
	}
	return setup, nil
}

// enrollFailure is a refused enrolment step: a status and a message safe to
// show. method names the factor of a domain refusal.
func enrollFailure(ctx context.Context, method mfa.MethodType, err error) (int, string) {
	switch {
	case errors.Is(err, login.ErrInvalidCode):
		return http.StatusUnprocessableEntity, "That code didn't match — try again."
	case errors.Is(err, login.ErrMethodNotAllowed):
		if method == mfa.MethodTOTP {
			return http.StatusForbidden, "The authenticator app is not permitted for this domain."
		}
		return http.StatusForbidden, "Email codes are not permitted for this domain."
	case errors.Is(err, login.ErrNoEmail):
		return http.StatusBadRequest, "This account has no email address."
	case errors.Is(err, mfa.ErrAlreadyEnrolled):
		return http.StatusConflict, "That method is already set up."
	case errors.Is(err, mfa.ErrEncryptionUnavailable):
		return http.StatusServiceUnavailable, "Authenticator-app 2FA is not available."
	case errors.Is(err, mfa.ErrNoPendingEnrollment):
		return http.StatusBadRequest, "Start the setup first."
	case errors.Is(err, login.ErrSessionNotIssued):
		slog.ErrorContext(ctx, "ui: session not issued after enrolment", "err", err)
		return http.StatusInternalServerError, "Could not sign you in. Try again."
	}
	slog.ErrorContext(ctx, "ui: 2FA enrollment failed", "err", err)
	return http.StatusInternalServerError, "Could not complete the setup."
}

// ── after the password: the enroll token ───────────────────────────────

// enrollGone reports whether err ends the enrolment: a token the flow refuses,
// or an account that has a usable factor by now (it must sign in again and
// pass the challenge).
func enrollGone(err error) bool {
	return errors.Is(err, login.ErrInvalidToken) || errors.Is(err, login.ErrEnrollmentNotOwed)
}

func (h *AuthMFA) enrollPage(w http.ResponseWriter, r *http.Request) {
	token := challengeToken(r)
	if token == "" {
		h.expired(w, r)
		return
	}
	en, err := h.Flow.PendingEnrollment(r.Context(), token)
	if err != nil {
		if !enrollGone(err) {
			h.Shell.Fail(w, r, err)
			return
		}
		h.expired(w, r)
		return
	}
	v := mfaEnrollView{Next: authNext(r.URL.Query().Get("next")), Methods: en.Methods}
	h.render(w, r, http.StatusOK, "Set up two-factor authentication", nil, mfaEnroll(v))
}

// enrollRefused shows the method menu with the refusal (or ends the enrolment).
func (h *AuthMFA) enrollRefused(w http.ResponseWriter, r *http.Request, next string, status int, msg string) {
	en, err := h.Flow.PendingEnrollment(r.Context(), challengeToken(r))
	if err != nil {
		h.expired(w, r)
		return
	}
	h.render(w, r, status, "Set up two-factor authentication", nil, mfaEnroll(mfaEnrollView{Next: next, Methods: en.Methods, Error: msg}))
}

func (h *AuthMFA) enrollTOTPBegin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	next := authNext(r.PostFormValue("next"))
	enr, err := h.Flow.BeginEnrollment(ctx, challengeToken(r), mfa.MethodTOTP)
	if err == nil {
		var setup mfaTOTPSetup
		if setup, err = totpSetup(ctx, enr, authEnrollPath+"/totp/confirm", next); err == nil {
			h.render(w, r, http.StatusOK, "Set up two-factor authentication", nil, mfaTOTP(setup))
			return
		}
	}
	if enrollGone(err) {
		h.expired(w, r)
		return
	}
	status, msg := http.StatusInternalServerError, "Could not start the setup."
	if !errors.Is(err, errSetupNotDrawn) {
		status, msg = enrollFailure(ctx, mfa.MethodTOTP, err)
	}
	h.enrollRefused(w, r, next, status, msg)
}

func (h *AuthMFA) enrollTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	next := authNext(r.PostFormValue("next"))
	_, codes, err := h.Flow.CompleteEnrollment(w, r, challengeToken(r), mfa.MethodTOTP, r.PostFormValue("code"))
	if err == nil {
		h.enrolled(w, r, codes, next)
		return
	}
	if enrollGone(err) {
		h.expired(w, r)
		return
	}
	status, msg := enrollFailure(ctx, mfa.MethodTOTP, err)
	setup, drawn := totpSetupFor(authEnrollPath+"/totp/confirm", next, r.PostFormValue("uri"))
	if !drawn {
		h.enrollRefused(w, r, next, status, msg)
		return
	}
	setup.Error = msg
	h.render(w, r, status, "Set up two-factor authentication", nil, mfaTOTP(setup))
}

func (h *AuthMFA) enrollEmailBegin(w http.ResponseWriter, r *http.Request) {
	next := authNext(r.PostFormValue("next"))
	if _, err := h.Flow.BeginEnrollment(r.Context(), challengeToken(r), mfa.MethodEmailPin); err != nil {
		if enrollGone(err) {
			h.expired(w, r)
			return
		}
		status, msg := enrollFailure(r.Context(), mfa.MethodEmailPin, err)
		h.enrollRefused(w, r, next, status, msg)
		return
	}
	h.render(w, r, http.StatusOK, "Set up two-factor authentication", nil, mfaEmail(mfaEmailSetup{Action: authEnrollPath + "/email/confirm", Next: next}))
}

func (h *AuthMFA) enrollEmailConfirm(w http.ResponseWriter, r *http.Request) {
	next := authNext(r.PostFormValue("next"))
	_, codes, err := h.Flow.CompleteEnrollment(w, r, challengeToken(r), mfa.MethodEmailPin, r.PostFormValue("code"))
	if err == nil {
		h.enrolled(w, r, codes, next)
		return
	}
	if enrollGone(err) {
		h.expired(w, r)
		return
	}
	status, msg := enrollFailure(r.Context(), mfa.MethodEmailPin, err)
	h.render(w, r, status, "Set up two-factor authentication", nil, mfaEmail(mfaEmailSetup{Action: authEnrollPath + "/email/confirm", Next: next, Error: msg}))
}

// enrolled follows a confirmed enrolment, for which the flow has already
// issued the session: the first recovery codes are shown once, then the user
// continues to next; with none to show they go there now.
func (h *AuthMFA) enrolled(w http.ResponseWriter, r *http.Request, codes []string, next string) {
	if len(codes) == 0 {
		h.signedIn(w, r, next)
		return
	}
	h.clearChallengeCookie(w)
	h.render(w, r, http.StatusOK, "Recovery codes", nil, mfaCodes(mfaCodesView{Codes: codes, Href: authNext(next)}))
}
