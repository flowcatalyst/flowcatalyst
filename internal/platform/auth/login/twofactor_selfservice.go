package login

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// The session-gated 2FA management endpoints (Profile screen). The rules are
// TwoFactor's (twofactor_manage.go); these handlers decode, call and answer.

// RegisterTwoFactorSelfServiceRoutes mounts the session-gated 2FA management
// endpoints (Profile screen). Mount INSIDE the auth middleware. No-op when MFA
// isn't wired.
func (e *Endpoint) RegisterTwoFactorSelfServiceRoutes(r chi.Router) {
	if e.tf == nil {
		return
	}
	r.Get("/auth/2fa/status", e.handle2FAStatus)
	r.Post("/auth/2fa/methods/totp/begin", e.handle2FASelfTOTPBegin)
	r.Post("/auth/2fa/methods/totp/confirm", e.handle2FASelfTOTPConfirm)
	r.Post("/auth/2fa/methods/email/begin", e.handle2FASelfEmailBegin)
	r.Post("/auth/2fa/methods/email/confirm", e.handle2FASelfEmailConfirm)
	r.Delete("/auth/2fa/methods/{method}", e.handle2FARemoveMethod)
	r.Post("/auth/2fa/recovery-codes/regenerate", e.handle2FARegenRecovery)
	r.Get("/auth/2fa/trusted-devices", e.handle2FAListTrustedDevices)
	r.Delete("/auth/2fa/trusted-devices/{id}", e.handle2FARevokeTrustedDevice)
}

// principalFromSession loads the authenticated principal, writing 401 (and
// returning nil) when there's no valid session.
func (e *Endpoint) principalFromSession(w http.ResponseWriter, r *http.Request) *principal.Principal {
	ac := auth.FromContext(r.Context())
	if ac == nil || ac.PrincipalID == "" {
		writeUnauthorized(w, "Not authenticated")
		return nil
	}
	p, err := e.cfg.Principals.FindByID(r.Context(), ids.PrincipalID(ac.PrincipalID))
	if err != nil || p == nil || !p.Active {
		writeUnauthorized(w, "Not authenticated")
		return nil
	}
	return p
}

// ── status ─────────────────────────────────────────────────────────────────

type twoFactorStatusResponse struct {
	Methods               []string `json:"methods"`               // confirmed factor types
	Required              bool     `json:"required"`              // domain compels 2FA
	AllowedMethods        []string `json:"allowedMethods"`        // enrollable methods
	RecoveryCodesLeft     int      `json:"recoveryCodesLeft"`     //
	RememberDeviceEnabled bool     `json:"rememberDeviceEnabled"` //
	TrustedDeviceCount    int      `json:"trustedDeviceCount"`    //
}

func (e *Endpoint) handle2FAStatus(w http.ResponseWriter, r *http.Request) {
	p := e.principalFromSession(w, r)
	if p == nil {
		return
	}
	s, err := e.tf.Status(r.Context(), p)
	if err != nil {
		writeServerError(w, "STATUS_FAILED", "could not load 2FA status")
		return
	}
	writeJSON(w, http.StatusOK, twoFactorStatusResponse{
		Methods:               s.Methods,
		Required:              s.Required,
		AllowedMethods:        s.AllowedMethods,
		RecoveryCodesLeft:     s.RecoveryCodesLeft,
		RememberDeviceEnabled: s.RememberDeviceEnabled,
		TrustedDeviceCount:    len(s.TrustedDevices),
	})
}

// ── voluntary enrollment ─────────────────────────────────────────────────

func (e *Endpoint) handle2FASelfTOTPBegin(w http.ResponseWriter, r *http.Request) {
	p := e.principalFromSession(w, r)
	if p == nil {
		return
	}
	enr, err := e.tf.BeginMethod(r.Context(), p, mfa.MethodTOTP)
	if err != nil {
		e.writeEnrollErr(w, mfa.MethodTOTP, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secret": enr.Secret, "uri": enr.URI, "qr": enr.QR})
}

func (e *Endpoint) handle2FASelfTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	e.selfConfirm(w, r, mfa.MethodTOTP)
}

func (e *Endpoint) handle2FASelfEmailBegin(w http.ResponseWriter, r *http.Request) {
	p := e.principalFromSession(w, r)
	if p == nil {
		return
	}
	if _, err := e.tf.BeginMethod(r.Context(), p, mfa.MethodEmailPin); err != nil {
		e.writeEnrollErr(w, mfa.MethodEmailPin, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "A verification code has been sent to your email."})
}

func (e *Endpoint) handle2FASelfEmailConfirm(w http.ResponseWriter, r *http.Request) {
	e.selfConfirm(w, r, mfa.MethodEmailPin)
}

func (e *Endpoint) selfConfirm(w http.ResponseWriter, r *http.Request, method mfa.MethodType) {
	p := e.principalFromSession(w, r)
	if p == nil {
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	codes, err := e.tf.ConfirmMethod(r.Context(), p, method, req.Code)
	if err != nil {
		e.writeEnrollErr(w, method, err)
		return
	}
	writeJSON(w, http.StatusOK, recoveryCodesBody(codes))
}

// ── remove / regenerate ────────────────────────────────────────────────────

func (e *Endpoint) handle2FARemoveMethod(w http.ResponseWriter, r *http.Request) {
	p := e.principalFromSession(w, r)
	if p == nil {
		return
	}
	switch err := e.tf.RemoveMethod(r.Context(), p, chi.URLParam(r, "method")); {
	case err == nil:
		writeJSON(w, http.StatusOK, map[string]any{"message": "Two-factor method removed."})
	case errors.Is(err, ErrInvalidMethod):
		writeJSON(w, http.StatusBadRequest, errBody("INVALID_METHOD", "unknown 2FA method"))
	case errors.Is(err, ErrLastFactor):
		writeJSON(w, http.StatusConflict, errBody("LAST_FACTOR",
			"your organisation requires 2FA — add another method before removing this one"))
	default:
		writeServerError(w, "REMOVE_FAILED", "could not remove method")
	}
}

func (e *Endpoint) handle2FARegenRecovery(w http.ResponseWriter, r *http.Request) {
	p := e.principalFromSession(w, r)
	if p == nil {
		return
	}
	codes, err := e.tf.RegenerateRecoveryCodes(r.Context(), p)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, recoveryCodesBody(codes))
	case errors.Is(err, ErrNoTOTP):
		writeJSON(w, http.StatusBadRequest, errBody("NO_TOTP", "recovery codes apply to authenticator-app 2FA"))
	default:
		writeServerError(w, "REGEN_FAILED", "could not generate recovery codes")
	}
}

// ── trusted devices ────────────────────────────────────────────────────────

func (e *Endpoint) handle2FAListTrustedDevices(w http.ResponseWriter, r *http.Request) {
	p := e.principalFromSession(w, r)
	if p == nil {
		return
	}
	devices, err := e.tf.TrustedDevices(r.Context(), p)
	if err != nil {
		writeServerError(w, "LIST_FAILED", "could not list devices")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": devices})
}

func (e *Endpoint) handle2FARevokeTrustedDevice(w http.ResponseWriter, r *http.Request) {
	p := e.principalFromSession(w, r)
	if p == nil {
		return
	}
	if err := e.tf.RevokeTrustedDevice(r.Context(), p, chi.URLParam(r, "id")); err != nil {
		writeServerError(w, "REVOKE_FAILED", "could not revoke device")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"message": "Device removed."})
}

// ── small helpers ──────────────────────────────────────────────────────────

func recoveryCodesBody(codes []string) map[string]any {
	if codes == nil {
		codes = []string{}
	}
	return map[string]any{"recoveryCodes": codes}
}

func errBody(code, msg string) map[string]any {
	return map[string]any{"code": code, "message": msg}
}

func writeServerError(w http.ResponseWriter, code, msg string) {
	writeJSON(w, http.StatusInternalServerError, errBody(code, msg))
}
