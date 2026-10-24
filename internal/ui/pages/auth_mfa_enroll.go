package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
)

// Enrolling a second factor, in the two places the current UI does it: after
// the password when the domain compels a factor the user has none of (a
// between-step enroll token; the session comes at the end), and from the
// security page when signed in. The views and the steps are shared; the
// handlers of the second place are in auth_mfa_security.go.

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
	// Action posts the Continue button (token mode); Href is the link of
	// the signed-in page.
	Action, Next, Href string
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

// enrollable is the factor types p may enroll: the domain's allowed set when
// it compels 2FA, both otherwise.
func (h *AuthMFA) enrollable(ctx context.Context, p *principal.Principal) []string {
	ev := h.Policy.Evaluate(ctx, mfaEmailOf(p))
	if ev.Requires2FA() {
		return ev.AllowedMethods()
	}
	return allMFAMethods
}

// methodAllowed reports whether p may enroll method t: always, unless the
// domain compels 2FA with a restricted set.
func (h *AuthMFA) methodAllowed(ctx context.Context, p *principal.Principal, t mfa.MethodType) bool {
	ev := h.Policy.Evaluate(ctx, mfaEmailOf(p))
	return !ev.Requires2FA() || slices.Contains(ev.AllowedMethods(), string(t))
}

// enrollFailure is the endpoint's writeEnrollErr: a status and a message safe
// to show.
func enrollFailure(ctx context.Context, err error) (int, string) {
	switch {
	case errors.Is(err, mfa.ErrAlreadyEnrolled):
		return http.StatusConflict, "That method is already set up."
	case errors.Is(err, mfa.ErrEncryptionUnavailable):
		return http.StatusServiceUnavailable, "Authenticator-app 2FA is not available."
	case errors.Is(err, mfa.ErrNoPendingEnrollment):
		return http.StatusBadRequest, "Start the setup first."
	}
	slog.ErrorContext(ctx, "ui: 2FA enrollment failed", "err", err)
	return http.StatusInternalServerError, "Could not complete the setup."
}

// beginTOTP starts an authenticator enrollment for p. A non-zero status is the
// refusal to show.
func (h *AuthMFA) beginTOTP(ctx context.Context, p *principal.Principal, action, next string) (mfaTOTPSetup, int, string) {
	if !h.methodAllowed(ctx, p, mfa.MethodTOTP) {
		return mfaTOTPSetup{}, http.StatusForbidden, "The authenticator app is not permitted for this domain."
	}
	enr, err := h.MFA.BeginTOTPEnrollment(ctx, string(p.ID), mfaEmailOf(p))
	if err != nil {
		status, msg := enrollFailure(ctx, err)
		return mfaTOTPSetup{}, status, msg
	}
	setup, ok := totpSetupFor(action, next, enr.URI)
	if !ok {
		slog.ErrorContext(ctx, "ui: the enrollment URI is not an authenticator URI", "principal", p.ID)
		return mfaTOTPSetup{}, http.StatusInternalServerError, "Could not start the setup."
	}
	return setup, 0, ""
}

// beginEmail sends the enrollment code to p's address.
func (h *AuthMFA) beginEmail(ctx context.Context, p *principal.Principal) (int, string) {
	if !h.methodAllowed(ctx, p, mfa.MethodEmailPin) {
		return http.StatusForbidden, "Email codes are not permitted for this domain."
	}
	email := mfaEmailOf(p)
	if email == "" {
		return http.StatusBadRequest, "This account has no email address."
	}
	if err := h.MFA.BeginEmailEnrollment(ctx, string(p.ID), email); err != nil {
		return enrollFailure(ctx, err)
	}
	return 0, ""
}

// confirmEnrollment checks the code of a begun enrollment. ok is false for a
// wrong code (a status 0 refusal); a refusal with a status is an error.
func (h *AuthMFA) confirmEnrollment(ctx context.Context, p *principal.Principal, method mfa.MethodType, code string) (ok bool, status int, msg string) {
	var err error
	if method == mfa.MethodTOTP {
		ok, err = h.MFA.ConfirmTOTPEnrollment(ctx, string(p.ID), code)
	} else {
		ok, err = h.MFA.ConfirmEmailEnrollment(ctx, string(p.ID), code)
	}
	if err != nil {
		status, msg = enrollFailure(ctx, err)
		return false, status, msg
	}
	if !ok {
		return false, http.StatusUnprocessableEntity, "That code didn't match — try again."
	}
	op := "2FA_TOTP_ENROLLED"
	if method == mfa.MethodEmailPin {
		op = "2FA_EMAIL_ENROLLED"
	}
	h.Notifier.TwoFactorEnrolled(ctx, mfaEmailOf(p), string(method))
	h.auditMFA(ctx, string(p.ID), op)
	return true, 0, ""
}

// ensureRecoveryCodes generates a first recovery-code set only when the user
// has none, returning the codes (shown once) or nil. Recovery codes only back
// authenticator-app 2FA; email is its own recovery channel.
func (h *AuthMFA) ensureRecoveryCodes(ctx context.Context, p *principal.Principal) []string {
	confirmed, err := h.MFA.ConfirmedMethods(ctx, string(p.ID))
	if err != nil || !slices.Contains(confirmed, mfa.MethodTOTP) {
		return nil
	}
	n, err := h.MFA.RemainingRecoveryCodes(ctx, string(p.ID))
	if err != nil || n > 0 {
		return nil
	}
	codes, err := h.MFA.GenerateRecoveryCodes(ctx, string(p.ID))
	if err != nil {
		slog.ErrorContext(ctx, "ui: recovery code generation failed", "principal", p.ID, "err", err)
		return nil
	}
	h.Notifier.RecoveryCodesRegenerated(ctx, mfaEmailOf(p))
	return codes
}

// ── after the password: the enroll token ───────────────────────────────

func (h *AuthMFA) enrollPage(w http.ResponseWriter, r *http.Request) {
	p := h.tokenPrincipal(r, mfaEnrollPurpose)
	if p == nil {
		h.expired(w, r)
		return
	}
	v := mfaEnrollView{Next: authNext(r.URL.Query().Get("next")), Methods: h.enrollable(r.Context(), p)}
	h.render(w, r, http.StatusOK, "Set up two-factor authentication", nil, mfaEnroll(v))
}

func (h *AuthMFA) enrollRefused(w http.ResponseWriter, r *http.Request, p *principal.Principal, next string, status int, msg string) {
	v := mfaEnrollView{Next: next, Methods: h.enrollable(r.Context(), p), Error: msg}
	h.render(w, r, status, "Set up two-factor authentication", nil, mfaEnroll(v))
}

func (h *AuthMFA) enrollTOTPBegin(w http.ResponseWriter, r *http.Request) {
	p := h.tokenPrincipal(r, mfaEnrollPurpose)
	if p == nil {
		h.expired(w, r)
		return
	}
	next := authNext(r.PostFormValue("next"))
	setup, status, msg := h.beginTOTP(r.Context(), p, authEnrollPath+"/totp/confirm", next)
	if status != 0 {
		h.enrollRefused(w, r, p, next, status, msg)
		return
	}
	h.render(w, r, http.StatusOK, "Set up two-factor authentication", nil, mfaTOTP(setup))
}

func (h *AuthMFA) enrollTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	p := h.tokenPrincipal(r, mfaEnrollPurpose)
	if p == nil {
		h.expired(w, r)
		return
	}
	next := authNext(r.PostFormValue("next"))
	ok, status, msg := h.confirmEnrollment(r.Context(), p, mfa.MethodTOTP, strings.TrimSpace(r.PostFormValue("code")))
	if !ok {
		setup, drawn := totpSetupFor(authEnrollPath+"/totp/confirm", next, r.PostFormValue("uri"))
		if !drawn {
			h.enrollRefused(w, r, p, next, status, msg)
			return
		}
		setup.Error = msg
		h.render(w, r, status, "Set up two-factor authentication", nil, mfaTOTP(setup))
		return
	}
	h.enrolled(w, r, p, next)
}

func (h *AuthMFA) enrollEmailBegin(w http.ResponseWriter, r *http.Request) {
	p := h.tokenPrincipal(r, mfaEnrollPurpose)
	if p == nil {
		h.expired(w, r)
		return
	}
	next := authNext(r.PostFormValue("next"))
	if status, msg := h.beginEmail(r.Context(), p); status != 0 {
		h.enrollRefused(w, r, p, next, status, msg)
		return
	}
	h.render(w, r, http.StatusOK, "Set up two-factor authentication", nil, mfaEmail(mfaEmailSetup{Action: authEnrollPath + "/email/confirm", Next: next}))
}

func (h *AuthMFA) enrollEmailConfirm(w http.ResponseWriter, r *http.Request) {
	p := h.tokenPrincipal(r, mfaEnrollPurpose)
	if p == nil {
		h.expired(w, r)
		return
	}
	next := authNext(r.PostFormValue("next"))
	ok, status, msg := h.confirmEnrollment(r.Context(), p, mfa.MethodEmailPin, strings.TrimSpace(r.PostFormValue("code")))
	if !ok {
		h.render(w, r, status, "Set up two-factor authentication", nil, mfaEmail(mfaEmailSetup{Action: authEnrollPath + "/email/confirm", Next: next, Error: msg}))
		return
	}
	h.enrolled(w, r, p, next)
}

// enrolled follows a confirmed enrollment: the first recovery codes are shown
// once, then the user continues; with none to show the sign-in completes now.
func (h *AuthMFA) enrolled(w http.ResponseWriter, r *http.Request, p *principal.Principal, next string) {
	if codes := h.ensureRecoveryCodes(r.Context(), p); len(codes) > 0 {
		h.render(w, r, http.StatusOK, "Recovery codes", nil, mfaCodes(mfaCodesView{Codes: codes, Action: authEnrollPath + "/done", Next: next}))
		return
	}
	h.clearChallengeCookie(w)
	h.completeSignIn(w, r, p, next)
}

// enrollDone ends an enrollment: the session is issued only for a user who
// now holds a factor the domain accepts (or whose domain no longer compels
// one), so the enroll token alone never gets past this point.
func (h *AuthMFA) enrollDone(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := h.tokenPrincipal(r, mfaEnrollPurpose)
	if p == nil {
		h.expired(w, r)
		return
	}
	next := authNext(r.PostFormValue("next"))
	ev := h.Policy.Evaluate(ctx, mfaEmailOf(p))
	usable, err := h.usable(ctx, p, ev)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if len(usable) == 0 && ev.Requires2FA() {
		http.Redirect(w, r, authEnrollPath+"?next="+url.QueryEscape(next), http.StatusSeeOther)
		return
	}
	h.clearChallengeCookie(w)
	h.completeSignIn(w, r, p, next)
}
