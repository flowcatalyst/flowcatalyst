package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/login"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/mfa"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// The security page (/ui/auth/security): the signed-in user's own two-factor
// methods, recovery codes, remembered devices and passkeys, as the current
// UI's profile cards (TwoFactorSection, TwoFactorSetup, PasskeysSection).
// Everything is self-service: every action acts on the session's principal
// and on nothing a form names but the item to act on, which must be theirs.

const authSecurityPath = "/ui/auth/security"

// MountSession registers the security page; mount it inside the session guard.
func (h *AuthMFA) MountSession(r chi.Router) {
	r.Get("/auth/security", h.securityPage)
	post := r.With(parseForm)
	post.Post("/auth/security/2fa/totp", h.selfTOTPBegin)
	post.Post("/auth/security/2fa/totp/confirm", h.selfTOTPConfirm)
	post.Post("/auth/security/2fa/email", h.selfEmailBegin)
	post.Post("/auth/security/2fa/email/confirm", h.selfEmailConfirm)
	post.Post("/auth/security/2fa/{method}/remove", h.selfRemove)
	post.Post("/auth/security/2fa/recovery-codes", h.selfRegenerate)
	post.Post("/auth/security/2fa/devices/{id}/revoke", h.selfRevokeDevice)
	post.Post("/auth/security/passkeys/{id}/remove", h.selfRemovePasskey)
}

type securityMethod struct{ Type, Label string }

type securityDevice struct {
	ID, Label         string
	LastUsed, Expires time.Time
}

type securityPasskey struct {
	ID, Name        string
	Added, LastUsed time.Time
}

type securityView struct {
	// Required is the domain compelling 2FA ("Your organisation requires 2FA").
	Required     bool
	Methods      []securityMethod
	Addable      []securityMethod
	HasTOTP      bool
	RecoveryLeft int
	Devices      []securityDevice
	// Passkeys are listed when the page has a passkey store.
	ShowPasskeys bool
	Passkeys     []securityPasskey
	// Message and Error are the page's banners (the SPA's success and error
	// banners); Panel replaces the add buttons with a setup step or the new
	// recovery codes.
	Message, Error string
	Panel          templ.Component
}

func methodLabel(method string) string {
	switch method {
	case string(mfa.MethodTOTP):
		return "Authenticator app"
	case string(mfa.MethodEmailPin):
		return "Email code"
	}
	return method
}

// session is the signed-in user's active principal, or nil after answering
// (the guards mean this only fails for a principal removed mid-session).
func (h *AuthMFA) session(w http.ResponseWriter, r *http.Request) *principal.Principal {
	ac := auth.FromContext(r.Context())
	if ac == nil || ac.PrincipalID == "" {
		h.Shell.Deny(w, r, http.StatusUnauthorized, "Not signed in.")
		return nil
	}
	p, err := h.Principals.FindByID(r.Context(), ids.PrincipalID(ac.PrincipalID))
	if err != nil || p == nil || !p.Active {
		h.Shell.Deny(w, r, http.StatusUnauthorized, "Not signed in.")
		return nil
	}
	return p
}

func (h *AuthMFA) securityPage(w http.ResponseWriter, r *http.Request) {
	p := h.session(w, r)
	if p == nil {
		return
	}
	h.renderSecurity(w, r, p, http.StatusOK, securityView{})
}

// renderSecurity fills v (its banners and panel as the caller set them) with
// the current state and writes the page.
func (h *AuthMFA) renderSecurity(w http.ResponseWriter, r *http.Request, p *principal.Principal, status int, v securityView) {
	ctx := r.Context()
	st, err := h.Flow.Status(ctx, p)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Required = st.Required
	for _, m := range st.Methods {
		v.Methods = append(v.Methods, securityMethod{Type: m, Label: methodLabel(m)})
	}
	// Recovery codes apply while the authenticator app is usable.
	v.HasTOTP = st.RecoveryCodes
	for _, m := range st.AllowedMethods {
		if !slices.Contains(st.Methods, m) {
			v.Addable = append(v.Addable, securityMethod{Type: m, Label: methodLabel(m)})
		}
	}
	v.RecoveryLeft = st.RecoveryCodesLeft
	for _, d := range st.TrustedDevices {
		row := securityDevice{ID: d.ID, Expires: d.ExpiresAt}
		if d.Label != nil {
			row.Label = *d.Label
		}
		if d.LastUsedAt != nil {
			row.LastUsed = *d.LastUsedAt
		}
		v.Devices = append(v.Devices, row)
	}
	if h.Passkeys != nil {
		v.ShowPasskeys = true
		keys, err := h.Passkeys.FindByPrincipal(ctx, string(p.ID))
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		for _, k := range keys {
			row := securityPasskey{ID: k.ID, Name: "Unnamed passkey", Added: k.CreatedAt}
			if k.Name != nil && *k.Name != "" {
				row.Name = *k.Name
			}
			if k.LastUsedAt != nil {
				row.LastUsed = *k.LastUsedAt
			}
			v.Passkeys = append(v.Passkeys, row)
		}
	}
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Security", Path: authSecurityPath}, securityPageContent(v))
}

// refuse re-renders the page with an error banner.
func (h *AuthMFA) refuse(w http.ResponseWriter, r *http.Request, p *principal.Principal, status int, msg string) {
	h.renderSecurity(w, r, p, status, securityView{Error: msg})
}

// ── adding a method ────────────────────────────────────────────────────

func (h *AuthMFA) selfTOTPBegin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := h.session(w, r)
	if p == nil {
		return
	}
	enr, err := h.Flow.BeginMethod(ctx, p, mfa.MethodTOTP)
	if err != nil {
		status, msg := enrollFailure(ctx, mfa.MethodTOTP, err)
		h.refuse(w, r, p, status, msg)
		return
	}
	setup, err := totpSetup(ctx, enr, authSecurityPath+"/2fa/totp/confirm", "")
	if err != nil {
		h.refuse(w, r, p, http.StatusInternalServerError, "Could not start the setup.")
		return
	}
	h.renderSecurity(w, r, p, http.StatusOK, securityView{Panel: mfaTOTPPanel(setup)})
}

func (h *AuthMFA) selfTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := h.session(w, r)
	if p == nil {
		return
	}
	codes, err := h.Flow.ConfirmMethod(ctx, p, mfa.MethodTOTP, r.PostFormValue("code"))
	if err != nil {
		status, msg := enrollFailure(ctx, mfa.MethodTOTP, err)
		setup, drawn := totpSetupFor(authSecurityPath+"/2fa/totp/confirm", "", r.PostFormValue("uri"))
		if !drawn {
			h.refuse(w, r, p, status, msg)
			return
		}
		setup.Error = msg
		h.renderSecurity(w, r, p, status, securityView{Panel: mfaTOTPPanel(setup)})
		return
	}
	h.enrolledSelf(w, r, p, codes)
}

func (h *AuthMFA) selfEmailBegin(w http.ResponseWriter, r *http.Request) {
	p := h.session(w, r)
	if p == nil {
		return
	}
	if _, err := h.Flow.BeginMethod(r.Context(), p, mfa.MethodEmailPin); err != nil {
		status, msg := enrollFailure(r.Context(), mfa.MethodEmailPin, err)
		h.refuse(w, r, p, status, msg)
		return
	}
	h.renderSecurity(w, r, p, http.StatusOK, securityView{Panel: mfaEmailPanel(mfaEmailSetup{Action: authSecurityPath + "/2fa/email/confirm"})})
}

func (h *AuthMFA) selfEmailConfirm(w http.ResponseWriter, r *http.Request) {
	p := h.session(w, r)
	if p == nil {
		return
	}
	codes, err := h.Flow.ConfirmMethod(r.Context(), p, mfa.MethodEmailPin, r.PostFormValue("code"))
	if err != nil {
		status, msg := enrollFailure(r.Context(), mfa.MethodEmailPin, err)
		h.renderSecurity(w, r, p, status, securityView{Panel: mfaEmailPanel(mfaEmailSetup{Action: authSecurityPath + "/2fa/email/confirm", Error: msg})})
		return
	}
	h.enrolledSelf(w, r, p, codes)
}

// enrolledSelf shows a confirmed enrollment: the first recovery codes, once,
// when there are any to show.
func (h *AuthMFA) enrolledSelf(w http.ResponseWriter, r *http.Request, p *principal.Principal, codes []string) {
	v := securityView{Message: "Two-factor method added."}
	if len(codes) > 0 {
		v.Panel = mfaCodesPanel(mfaCodesView{Codes: codes, Href: authSecurityPath})
	}
	h.renderSecurity(w, r, p, http.StatusOK, v)
}

// ── removing and regenerating ──────────────────────────────────────────

func (h *AuthMFA) selfRemove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := h.session(w, r)
	if p == nil {
		return
	}
	method := chi.URLParam(r, "method")
	switch err := h.Flow.RemoveMethod(ctx, p, method); {
	case err == nil:
		h.renderSecurity(w, r, p, http.StatusOK, securityView{Message: methodLabel(method) + " removed."})
	case errors.Is(err, login.ErrInvalidMethod):
		h.refuse(w, r, p, http.StatusBadRequest, "Unknown two-factor method.")
	case errors.Is(err, login.ErrLastFactor):
		h.refuse(w, r, p, http.StatusConflict, "Your organisation requires 2FA — add another method before removing this one.")
	default:
		slog.ErrorContext(ctx, "ui: remove 2FA method failed", "principal", p.ID, "err", err)
		h.refuse(w, r, p, http.StatusInternalServerError, "Could not remove method")
	}
}

func (h *AuthMFA) selfRegenerate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := h.session(w, r)
	if p == nil {
		return
	}
	codes, err := h.Flow.RegenerateRecoveryCodes(ctx, p)
	if errors.Is(err, login.ErrNoTOTP) {
		h.refuse(w, r, p, http.StatusBadRequest, "Recovery codes apply to authenticator-app 2FA.")
		return
	}
	if err != nil {
		slog.ErrorContext(ctx, "ui: recovery code regeneration failed", "principal", p.ID, "err", err)
		h.refuse(w, r, p, http.StatusInternalServerError, "Could not regenerate codes")
		return
	}
	h.renderSecurity(w, r, p, http.StatusOK, securityView{
		Message: "New recovery codes generated. Save them now.",
		Panel:   mfaCodesPanel(mfaCodesView{Codes: codes, Href: authSecurityPath}),
	})
}

func (h *AuthMFA) selfRevokeDevice(w http.ResponseWriter, r *http.Request) {
	p := h.session(w, r)
	if p == nil {
		return
	}
	// Scoped to the owner in the service: another user's device id removes nothing.
	if err := h.Flow.RevokeTrustedDevice(r.Context(), p, chi.URLParam(r, "id")); err != nil {
		slog.ErrorContext(r.Context(), "ui: revoke trusted device failed", "principal", p.ID, "err", err)
		h.refuse(w, r, p, http.StatusInternalServerError, "Could not revoke device")
		return
	}
	h.renderSecurity(w, r, p, http.StatusOK, securityView{})
}

// selfRemovePasskey removes one of the user's own passkeys. Another user's
// credential id is a not-found, as the API answers, and is not revoked.
func (h *AuthMFA) selfRemovePasskey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	p := h.session(w, r)
	if p == nil {
		return
	}
	if h.Passkeys == nil {
		h.Shell.Deny(w, r, http.StatusNotFound, "Passkeys are not available.")
		return
	}
	id := chi.URLParam(r, "id")
	if err := h.removePasskey(ctx, p, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.renderSecurity(w, r, p, http.StatusOK, securityView{Message: "Passkey removed."})
}

func (h *AuthMFA) removePasskey(ctx context.Context, p *principal.Principal, id string) error {
	owned, err := h.Passkeys.FindByPrincipal(ctx, string(p.ID))
	if err != nil {
		return err
	}
	if !slices.ContainsFunc(owned, func(c passkeyCredential) bool { return c.ID == id }) {
		return httperror.NotFound("Credential", id)
	}
	return h.PasskeyOps.Revoke(ctx, id)
}
