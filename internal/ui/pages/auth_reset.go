package pages

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"
)

// The forgot and reset password pages (frontend ForgotPasswordPage and
// ResetPasswordPage; /ui/auth/set-password is the invite framing of the reset
// page). Like the rest of the sign-in pages they call the platform's
// /auth/password-reset/* endpoints, which keep the single-use token, the
// password policy, the factor gate, the 2FA and approval rules and the same
// answer whether or not the account exists.

func (s *SignIn) mountReset(r chi.Router) {
	r.Get("/auth/forgot-password", s.forgotPage)
	r.Post("/auth/forgot-password", s.forgot)
	r.Get("/auth/reset-password", s.resetPage)
	r.Get("/auth/set-password", s.resetPage)
	r.Post("/auth/reset-password", s.resetConfirm)
	r.Post("/auth/set-password", s.resetConfirm)
}

// forgotView is the forgot password page's view model.
type forgotView struct {
	Theme     signinTheme
	Ctx       signinContext
	Email     string
	Submitted bool
	Error     string
}

func (v forgotView) loginLink() string {
	if q := v.Ctx.query("reset", ""); q != "" {
		return "/ui/auth/login?" + q
	}
	return "/ui/auth/login"
}

func (s *SignIn) renderForgot(w http.ResponseWriter, r *http.Request, status int, v forgotView) {
	v.Theme = s.theme(r, v.Ctx.client())
	s.render(w, r, status, "Reset your password", v.Theme, false, forgotBody(v))
}

func (s *SignIn) forgotPage(w http.ResponseWriter, r *http.Request) {
	v := forgotView{Ctx: signinContextFrom(r), Email: strings.TrimSpace(r.URL.Query().Get("email"))}
	if len(v.Email) > 320 {
		v.Email = ""
	}
	s.renderForgot(w, r, http.StatusOK, v)
}

// forgot asks for a reset link. The answer to a well-formed request is the
// same whether the account exists (the endpoint's silent success); a failure of
// the request itself is shown.
func (s *SignIn) forgot(w http.ResponseWriter, r *http.Request) {
	v := forgotView{Ctx: signinContextFrom(r), Email: strings.TrimSpace(r.PostFormValue("email"))}
	if v.Email == "" || len(v.Email) > 320 {
		v.Error = "Enter your email address."
		s.renderForgot(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	body := map[string]any{"email": v.Email}
	// Mid OAuth round trip the server brings the user back to the authorize
	// URL once the reset is done.
	if v.Ctx.get("oauth") == "true" {
		body["redirectUri"] = v.Ctx.authorizeURL()
	}
	resp := s.call(r, http.MethodPost, "/auth/password-reset/request", body)
	switch resp.Status {
	case http.StatusOK:
		v.Submitted = true
		s.renderForgot(w, r, http.StatusOK, v)
	case http.StatusTooManyRequests:
		v.Error = resp.message("Too many requests. Try again later.")
		s.renderForgot(w, r, http.StatusTooManyRequests, v)
	default:
		v.Error = "Something went wrong. Please try again."
		s.renderForgot(w, r, http.StatusBadGateway, v)
	}
}

// resetState is what the reset page shows.
type resetState string

// States of the reset page.
const (
	resetForm       resetState = "form"
	resetInvalid    resetState = "invalid"
	resetEnroll     resetState = "enroll"
	resetPortalDone resetState = "portalDone"
)

// resetView is the reset (or set-password) page's view model.
type resetView struct {
	Theme          signinTheme
	Ctx            signinContext
	State          resetState
	Invite         bool
	Portal         bool
	Token          string
	RequiresFactor bool
	Expired        bool
	Error          string
}

func (v resetView) title() string {
	switch {
	case v.State == resetInvalid:
		return "Link invalid or expired"
	case v.State == resetEnroll:
		return "Set up two-factor authentication"
	case v.State == resetPortalDone:
		return "Password set"
	case v.Invite:
		return "Set your password"
	}
	return "Reset your password"
}

func (v resetView) action() string {
	if v.Invite {
		return "/ui/auth/set-password"
	}
	return "/ui/auth/reset-password"
}

func (v resetView) loginLink() string { return "/ui/auth/login?" + v.Ctx.query("reset", "success") }

func (v resetView) forgotLink() string {
	if q := v.Ctx.query(); q != "" {
		return "/ui/auth/forgot-password?" + q
	}
	return "/ui/auth/forgot-password"
}

func (s *SignIn) renderReset(w http.ResponseWriter, r *http.Request, status int, v resetView) {
	v.Theme = s.theme(r, v.Ctx.client())
	s.render(w, r, status, v.title(), v.Theme, v.Portal, resetBody(v))
}

// resetPage validates the token (the endpoint answers valid, or why not) and
// shows the form, or why the link cannot be used.
func (s *SignIn) resetPage(w http.ResponseWriter, r *http.Request) {
	v := resetView{Ctx: signinContextFrom(r), Invite: strings.HasSuffix(r.URL.Path, "/set-password"), Token: r.URL.Query().Get("token")}
	if v.Token == "" || len(v.Token) > 512 {
		v.State = resetInvalid
		s.renderReset(w, r, http.StatusOK, v)
		return
	}
	resp := s.call(r, http.MethodGet, "/auth/password-reset/validate?token="+url.QueryEscape(v.Token), nil)
	var out struct {
		Valid          bool    `json:"valid"`
		Reason         *string `json:"reason"`
		RequiresFactor bool    `json:"requiresFactor"`
		Portal         bool    `json:"portal"`
	}
	if resp.Status != http.StatusOK || !resp.decode(&out) || !out.Valid {
		v.State, v.Token = resetInvalid, ""
		v.Expired = out.Reason != nil && *out.Reason == "expired"
		s.renderReset(w, r, http.StatusOK, v)
		return
	}
	v.State, v.RequiresFactor, v.Portal = resetForm, out.RequiresFactor, out.Portal
	s.renderReset(w, r, http.StatusOK, v)
}

// resetConfirm sets the password. The endpoint enforces the policy and the
// factor; the page checks only what it can see (the two entries agree) and acts
// on the answer: a redirect it validated when the invite was minted, the
// session an invite establishes, the 2FA enrolment a domain compels, or back to
// sign-in.
func (s *SignIn) resetConfirm(w http.ResponseWriter, r *http.Request) {
	v := resetView{
		Ctx: signinContextFrom(r), Invite: strings.HasSuffix(r.URL.Path, "/set-password"), State: resetForm,
		Token:          r.PostFormValue("token"),
		RequiresFactor: r.PostFormValue("requiresFactor") == "1",
		Portal:         r.PostFormValue("portal") == "1",
	}
	password, factor := r.PostFormValue("password"), strings.TrimSpace(r.PostFormValue("factorCode"))
	switch {
	case v.Token == "" || len(v.Token) > 512:
		v.State, v.Token = resetInvalid, ""
	case password == "":
		v.Error = "Enter a password."
	case password != r.PostFormValue("confirmPassword"):
		v.Error = "Passwords do not match."
	case v.RequiresFactor && factor == "":
		v.Error = "Enter the code from your authenticator app."
	}
	if v.State == resetInvalid || v.Error != "" {
		s.renderReset(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	body := map[string]any{"token": v.Token, "password": password}
	if v.RequiresFactor {
		body["factorCode"] = factor
	}
	resp := s.call(r, http.MethodPost, "/auth/password-reset/confirm", body)
	if resp.Status != http.StatusOK {
		v.Error = resp.message("Failed to reset password. Please try again.")
		status := http.StatusUnprocessableEntity
		if resp.Status == http.StatusTooManyRequests || resp.Status >= http.StatusInternalServerError {
			status = resp.Status
		}
		s.renderReset(w, r, status, v)
		return
	}
	var out struct {
		Status             string  `json:"status"`
		EnrollToken        string  `json:"enrollToken"`
		RedirectURI        *string `json:"redirectUri"`
		Portal             bool    `json:"portal"`
		SessionEstablished bool    `json:"sessionEstablished"`
	}
	if !resp.decode(&out) {
		v.Error = "Failed to reset password. Please try again."
		s.renderReset(w, r, http.StatusBadGateway, v)
		return
	}
	resp.cookies(w)
	switch {
	case out.Status == "enrollment_required" && out.EnrollToken != "":
		// The domain requires a second factor; setting one up (QR code,
		// recovery codes) is done by the current sign-in page.
		v.State = resetEnroll
		s.renderReset(w, r, http.StatusOK, v)
	case out.RedirectURI != nil && safeInviteRedirect(*out.RedirectURI):
		http.Redirect(w, r, *out.RedirectURI, http.StatusSeeOther)
	case out.SessionEstablished:
		http.Redirect(w, r, "/ui/", http.StatusSeeOther)
	case out.Portal:
		// A portal identity cannot sign in to the platform: no bounce to its login.
		v.State = resetPortalDone
		s.renderReset(w, r, http.StatusOK, v)
	default:
		http.Redirect(w, r, v.loginLink(), http.StatusSeeOther)
	}
}

// safeInviteRedirect accepts the redirect the endpoint stored when the invite
// was minted (validated there): a same-site path or an http(s) URL; never a
// scheme that would run code.
func safeInviteRedirect(raw string) bool {
	if raw == "" || len(raw) > 2048 {
		return false
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] < 0x20 || raw[i] == 0x7f || raw[i] == '\\' {
			return false
		}
	}
	if strings.HasPrefix(raw, "/") {
		return !strings.HasPrefix(raw, "//")
	}
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Host != ""
}

// safeHref wraps a validated link for an anchor.
func safeHref(s string) templ.SafeURL { return templ.SafeURL(s) }
