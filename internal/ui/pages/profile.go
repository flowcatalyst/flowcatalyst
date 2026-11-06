package pages

import (
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/passwordpolicy"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// Profile is the signed-in user's own page (/ui/profile), at parity with the
// SPA's ProfilePage: who they are, the password change, passkeys, two-factor,
// the developer API credential and the sign-in activity. Everything acts on
// the session's principal and on nothing a form names.
//
// Nothing here re-implements an auth rule. The password change, its
// second-factor step and the sign-in history are the platform's own
// /auth/change-password, /auth/change-password/send-email-code,
// /auth/login-history and /auth/me, called in-process through Upstream (as
// the sign-in pages do), so the current-password check, the password policy,
// the 2FA requirement, the SSO refusal and the session hygiene after a change
// are theirs. The two-factor and passkey cards are the security page's
// (auth_mfa_security.go): the same state, the same components, the same
// /ui/auth/security/* actions. The developer credential runs the principal
// use cases (DeveloperUserCommands), as the Developer Users page does.
//
// Unlike the other pages it is mounted outside the role-less gate
// (guard.ProfileOnly): a user without a platform role may reach only this.
type Profile struct {
	// Upstream serves the platform's session-gated /auth/* endpoints (the
	// platform's root router).
	Upstream http.Handler
	// Principals loads the user's record (the developer credential's state).
	Principals PrincipalLookup
	// Credentials sets, rotates and revokes the user's developer credential.
	Credentials DeveloperUserCommands
	// Security supplies the two-factor and passkey cards; nil leaves them out.
	Security *AuthMFA

	Shell *shell.Shell
}

const (
	profilePath = "/ui/profile"

	profilePasswordID = "profile-password"
)

// Mount registers the page's routes on r, inside the session guard but not
// behind the role-less gate. POSTs are behind parseForm.
func (h *Profile) Mount(r chi.Router) {
	r.Get("/profile", h.page)
	post := r.With(parseForm)
	post.Post("/profile/password", h.changePassword)
	post.Post("/profile/password/email-code", h.sendEmailCode)
	post.Post("/profile/developer-credential", h.setCredential)
	post.Post("/profile/developer-credential/revoke", h.revokeCredential)
}

// ── the view ───────────────────────────────────────────────────────────

// profileMe is what /auth/me answers (the SPA's authStore.user).
type profileMe struct {
	PrincipalID string   `json:"principalId"`
	Name        string   `json:"name"`
	Email       string   `json:"email"`
	Roles       []string `json:"roles"`
	SsoManaged  bool     `json:"ssoManaged"`
}

func (m profileMe) displayName() string {
	if m.Name != "" {
		return m.Name
	}
	return m.Email
}

// profilePassword is the change-password dialog's state.
type profilePassword struct {
	// Open renders the dialog open (after a refusal or the second-factor step).
	Open  bool
	Error string
	// Notice is the dialog's neutral line (a code was sent).
	Notice string
	// MFA is the second-factor step: the user has 2FA, so a code is needed.
	MFA                bool
	Methods            []string
	Current, New, Conf string
}

func (p profilePassword) hasEmail() bool { return slices.Contains(p.Methods, "EMAIL_PIN") }
func (p profilePassword) hasTOTP() bool  { return slices.Contains(p.Methods, "TOTP") }

type profileDeveloper struct {
	Has                bool
	Rotated            time.Time
	TokenURL, ClientID string
}

// curl is the SPA's example: the token request with the user's own id.
func (d profileDeveloper) curl() string {
	return "curl -X POST " + d.TokenURL + " \\\n  -d grant_type=client_credentials \\\n  -d client_id=" + d.ClientID + " \\\n  -d client_secret=<your secret>"
}

type profileAttempt struct {
	When       time.Time
	OK         bool
	Device, IP string
	UserAgent  string
}

type profileActivity struct {
	Shown    bool
	Attempts []profileAttempt
	Error    string
}

type profileView struct {
	Me       profileMe
	Initials string

	// Message and Error are the page's banners.
	Message, Error string

	Password profilePassword

	// Developer is nil when the user may not manage a developer credential.
	Developer *profileDeveloper
	// Secret is the new developer secret, in the response that created it only.
	Secret *devSecret

	Activity profileActivity

	// Security carries the two-factor and passkey cards (ShowPasskeys false
	// leaves the passkeys out). HasSecurity false leaves both out.
	HasSecurity bool
	Security    securityView
}

// me reads the signed-in user as the SPA's auth store holds them.
func (h *Profile) me(r *http.Request) (profileMe, error) {
	resp := callUpstream(h.Upstream, r, http.MethodGet, "/auth/me", nil)
	var me profileMe
	if resp.Status != http.StatusOK || !resp.decode(&me) {
		return me, usecase.Internal("PROFILE", resp.message("Could not load your profile."), nil)
	}
	return me, nil
}

// view builds the page's state for the session. The caller adds what the
// request is about (a banner, the dialog state, a new secret).
func (h *Profile) view(r *http.Request, ac *auth.AuthContext) (profileView, *principal.Principal, error) {
	ctx := r.Context()
	me, err := h.me(r)
	if err != nil {
		return profileView{}, nil, err
	}
	v := profileView{Me: me, Initials: devInitials(me.displayName())}
	var p *principal.Principal
	if ac.PrincipalID != "" && h.Principals != nil {
		p, err = h.Principals.FindByID(ctx, ids.PrincipalID(ac.PrincipalID))
		if err != nil {
			return v, nil, err
		}
	}
	if auth.CanManageOwnDeveloperCredential(ac) == nil && p != nil {
		d := &profileDeveloper{Has: p.HasDevClientSecret(), TokenURL: baseURL(r) + "/oauth/token", ClientID: ac.PrincipalID}
		if p.UserIdentity != nil && p.UserIdentity.DevClientSecretUpdatedAt != nil {
			d.Rotated = *p.UserIdentity.DevClientSecretUpdatedAt
		}
		v.Developer = d
	}
	if h.Security != nil && p != nil {
		v.HasSecurity = true
		v.Security.DonePath = profilePath
		if err := h.Security.securityState(ctx, p, &v.Security); err != nil {
			return v, p, err
		}
	}
	return v, p, nil
}

// baseURL is the origin the browser used (the SPA's window.location.origin).
func baseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (h *Profile) render(w http.ResponseWriter, r *http.Request, status int, v profileView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Profile", Path: profilePath}, profilePage(v))
}

func (h *Profile) page(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	v, _, err := h.view(r, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if r.URL.Query().Get("activity") == "1" {
		h.loadActivity(r, &v)
	}
	h.render(w, r, http.StatusOK, v)
}

// ── sign-in activity ───────────────────────────────────────────────────

// loadActivity reads /auth/login-history (the last 20 attempts).
func (h *Profile) loadActivity(r *http.Request, v *profileView) {
	v.Activity.Shown = true
	resp := callUpstream(h.Upstream, r, http.MethodGet, "/auth/login-history", nil)
	var body struct {
		Attempts []struct {
			Outcome     string    `json:"outcome"`
			IPAddress   string    `json:"ipAddress"`
			UserAgent   string    `json:"userAgent"`
			AttemptedAt time.Time `json:"attemptedAt"`
		} `json:"attempts"`
	}
	if resp.Status != http.StatusOK || !resp.decode(&body) {
		slog.WarnContext(r.Context(), "ui: sign-in activity not loaded", "status", resp.Status)
		v.Activity.Error = "Could not load sign-in activity."
		return
	}
	for _, a := range body.Attempts {
		v.Activity.Attempts = append(v.Activity.Attempts, profileAttempt{
			When: a.AttemptedAt, OK: strings.EqualFold(a.Outcome, "SUCCESS"),
			Device: deviceLabel(a.UserAgent), UserAgent: a.UserAgent, IP: a.IPAddress,
		})
	}
}

// deviceLabel condenses a user agent into "Browser on OS" (the SPA's).
func deviceLabel(ua string) string {
	if ua == "" {
		return "Unknown device"
	}
	browser := "Browser"
	switch {
	case strings.Contains(ua, "Edg/"):
		browser = "Edge"
	case strings.Contains(ua, "Chrome/"):
		browser = "Chrome"
	case strings.Contains(ua, "Firefox/"):
		browser = "Firefox"
	case strings.Contains(ua, "Safari/"):
		browser = "Safari"
	}
	os := ""
	switch {
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Mac OS X"), strings.Contains(ua, "Macintosh"):
		os = "macOS"
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "iPhone"), strings.Contains(ua, "iPad"), strings.Contains(ua, "iOS"):
		os = "iOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}
	if os == "" {
		return browser
	}
	return browser + " on " + os
}

// ── change password ────────────────────────────────────────────────────

// changePassword is the SPA's submitChangePassword: the client-side checks
// (policy, confirmation, the code once asked for), then /auth/change-password.
// MFA_REQUIRED opens the second-factor step (and, for an email-only factor,
// sends the code at once); success is a flash and a redirect.
func (h *Profile) changePassword(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	v, _, err := h.view(r, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	pw := h.readPassword(r)
	v.Password = pw
	v.Password.Open = true
	fail := func(msg string) {
		v.Password.Error = msg
		h.render(w, r, http.StatusUnprocessableEntity, v)
	}
	if v.Me.SsoManaged {
		v.Password = profilePassword{}
		v.Error = "Your password is managed by your organisation's identity provider."
		h.render(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	if viol := passwordpolicy.Validate(pw.New, v.Me.Email, v.Me.Name); viol != nil {
		fail(viol.Message)
		return
	}
	if pw.New != pw.Conf {
		fail("New password and confirmation do not match.")
		return
	}
	code := strings.TrimSpace(r.PostFormValue("code"))
	if pw.MFA && code == "" {
		fail("Enter your two-factor code.")
		return
	}
	body := map[string]string{"currentPassword": pw.Current, "newPassword": pw.New}
	if pw.MFA {
		body["code"] = code
	}
	resp := callUpstream(h.Upstream, r, http.MethodPost, "/auth/change-password", body)
	if resp.Status == http.StatusOK {
		h.Shell.SetFlash(w, flashProfilePassword)
		http.Redirect(w, r, profilePath, http.StatusSeeOther)
		return
	}
	if resp.code() == "MFA_REQUIRED" {
		var mfa struct {
			Methods []string `json:"methods"`
		}
		resp.decode(&mfa)
		v.Password.MFA = true
		v.Password.Methods = knownMethods(mfa.Methods)
		// An email-only factor gets its code now, as the SPA does.
		if v.Password.hasEmail() && !v.Password.hasTOTP() {
			if sent := h.emailCode(r); sent != "" {
				v.Password.Notice = sent
			}
		}
		fail(resp.message("Two-factor verification required."))
		return
	}
	fail(resp.message("Could not change your password."))
}

// sendEmailCode asks for an email code from inside the dialog's second-factor
// step; the typed passwords come back with the page.
func (h *Profile) sendEmailCode(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	v, _, err := h.view(r, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Password = h.readPassword(r)
	v.Password.Open, v.Password.MFA = true, true
	status := http.StatusOK
	if msg := h.emailCode(r); msg != "" {
		v.Password.Notice = msg
	} else {
		v.Password.Error = "Could not send a code."
		status = http.StatusUnprocessableEntity
	}
	h.render(w, r, status, v)
}

// emailCode sends the change-password email code; "" when it was not sent,
// else the platform's message.
func (h *Profile) emailCode(r *http.Request) string {
	resp := callUpstream(h.Upstream, r, http.MethodPost, "/auth/change-password/send-email-code", nil)
	if resp.Status != http.StatusOK {
		return ""
	}
	return resp.message("A code has been sent to your email.")
}

func (h *Profile) readPassword(r *http.Request) profilePassword {
	return profilePassword{
		MFA:     r.PostFormValue("mfa") == "1",
		Methods: knownMethods(strings.Split(r.PostFormValue("methods"), ",")),
		Current: r.PostFormValue("currentPassword"),
		New:     r.PostFormValue("newPassword"),
		Conf:    r.PostFormValue("confirmPassword"),
	}
}

// knownMethods keeps the factor types the dialog knows how to ask for.
func knownMethods(in []string) []string {
	var out []string
	for _, m := range in {
		if (m == "TOTP" || m == "EMAIL_PIN") && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

// ── developer credential ───────────────────────────────────────────────

// setCredential creates or rotates the user's own developer secret and shows
// it once, in this response (no-store) and nowhere else.
func (h *Profile) setCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	w = noStore{w}
	if err := devCredentialAccess(ac, ac.PrincipalID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	secret, err := h.Credentials.SetCredential(ctx, ac.PrincipalID)
	if err != nil {
		h.credentialRefused(w, r, ac, err)
		return
	}
	v, _, err := h.view(r, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	v.Message = "Developer client secret updated."
	v.Secret = &devSecret{ClientID: ac.PrincipalID, Secret: secret}
	h.render(w, r, http.StatusOK, v)
}

// revokeCredential clears the user's own developer secret.
func (h *Profile) revokeCredential(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := devCredentialAccess(ac, ac.PrincipalID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Credentials.RevokeCredential(ctx, ac.PrincipalID); err != nil {
		h.credentialRefused(w, r, ac, err)
		return
	}
	h.Shell.SetFlash(w, flashDevRevoked)
	http.Redirect(w, r, profilePath, http.StatusSeeOther)
}

// credentialRefused shows the use case's refusal (a user who does not hold the
// developer role) as the page's error banner, as the SPA's toast does.
func (h *Profile) credentialRefused(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, err error) {
	status, msg := shell.Classify(r, err)
	v, _, verr := h.view(r, ac)
	if verr != nil {
		h.Shell.Fail(w, r, verr)
		return
	}
	v.Error = msg
	h.render(w, r, status, v)
}
