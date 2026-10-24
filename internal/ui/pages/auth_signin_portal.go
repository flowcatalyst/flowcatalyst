package pages

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// The portal login (frontend PortalLoginPage): the sign-in page of the portal
// plane, reached through GET /portal/authorize?flow=... Portal identities are a
// separate population from platform users, so the page shows no platform
// branding, never touches the platform session and sets no cookie; it ends in
// a redirect back to the portal app with an authorization code. Each step is
// one of the portal endpoints (/portal/auth/*), which keep the per-(client,
// email) rate limit, the SSO enforcement, the uniform refusal and the app gate.
//
// /portal/authorize still bounces to the SPA's /portal/login (its LoginPagePath
// is unchanged); this page is reached by its own URL with the same ?flow=.

func (s *SignIn) mountPortal(r chi.Router) {
	r.Get("/auth/portal-login", s.portalPage)
	r.Post("/auth/portal-login/email", s.portalEmail)
	r.Post("/auth/portal-login/password", s.portalPassword)
	r.Post("/auth/portal-login/reset", s.portalReset)
}

type portalStep string

const (
	portalStepEmail    portalStep = "email"
	portalStepPassword portalStep = "password"
	portalStepExpired  portalStep = "expired"
)

// portalView is the portal login page's view model.
type portalView struct {
	Theme     signinTheme
	Flow      string
	Step      portalStep
	Email     string
	Error     string
	ResetSent bool
}

func (v portalView) title() string {
	if v.Step == portalStepExpired {
		return "Sign-in link expired"
	}
	return "Sign in"
}

func (s *SignIn) renderPortal(w http.ResponseWriter, r *http.Request, status int, v portalView) {
	// The platform's own theme gives the background; the brand is not shown.
	v.Theme = s.theme(r, "")
	s.render(w, r, status, v.title(), v.Theme, true, portalBody(v))
}

func portalFlow(r *http.Request) string {
	f := r.FormValue("flow")
	if len(f) > 256 {
		return ""
	}
	return f
}

func (s *SignIn) portalPage(w http.ResponseWriter, r *http.Request) {
	v := portalView{Flow: portalFlow(r), Step: portalStepEmail}
	if v.Flow == "" {
		v.Step = portalStepExpired
	}
	s.renderPortal(w, r, http.StatusOK, v)
}

// portalFailed shows a refused portal request on step v; a flow that ran out
// is the expired page, whatever step it was on.
func (s *SignIn) portalFailed(w http.ResponseWriter, r *http.Request, resp upstreamResponse, v portalView, fallback string) {
	status := http.StatusUnprocessableEntity
	switch {
	case resp.code() == "FLOW_EXPIRED":
		v.Step, v.Error = portalStepExpired, ""
		status = http.StatusGone
	case resp.Status == http.StatusTooManyRequests:
		v.Error = resp.message("Too many attempts. Try again later.")
		if n := resp.retryAfter(); n > 0 {
			w.Header().Set("Retry-After", strconv.Itoa(n))
		}
		status = http.StatusTooManyRequests
	case resp.Status == http.StatusUnauthorized || resp.Status == http.StatusForbidden:
		v.Error = resp.message(fallback)
		status = resp.Status
	case resp.Status >= http.StatusInternalServerError:
		v.Error = fallback
		status = http.StatusBadGateway
	default:
		v.Error = resp.message(fallback)
	}
	s.renderPortal(w, r, status, v)
}

func (s *SignIn) portalEmail(w http.ResponseWriter, r *http.Request) {
	v := portalView{Flow: portalFlow(r), Step: portalStepEmail, Email: strings.TrimSpace(r.PostFormValue("email"))}
	if v.Flow == "" {
		v.Step = portalStepExpired
		s.renderPortal(w, r, http.StatusGone, v)
		return
	}
	if !strings.Contains(v.Email, "@") || len(v.Email) > 320 {
		v.Error = "Enter a valid email address."
		s.renderPortal(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	resp := s.call(r, http.MethodPost, "/portal/auth/check-domain", map[string]string{"flowId": v.Flow, "email": v.Email})
	var out struct {
		Method      string `json:"method"`
		RedirectURL string `json:"redirectUrl"`
	}
	if resp.Status != http.StatusOK || !resp.decode(&out) {
		s.portalFailed(w, r, resp, v, "Something went wrong")
		return
	}
	// An organisation-owned domain: hand off to its identity provider through
	// the portal bridge (the endpoint's own relative URL).
	if out.Method == "SSO" && strings.HasPrefix(out.RedirectURL, "/portal/auth/oidc/login?") {
		http.Redirect(w, r, out.RedirectURL, http.StatusSeeOther)
		return
	}
	v.Step = portalStepPassword
	s.renderPortal(w, r, http.StatusOK, v)
}

func (s *SignIn) portalPassword(w http.ResponseWriter, r *http.Request) {
	v := portalView{Flow: portalFlow(r), Step: portalStepPassword, Email: strings.TrimSpace(r.PostFormValue("email"))}
	password := r.PostFormValue("password")
	switch {
	case v.Flow == "":
		v.Step = portalStepExpired
		s.renderPortal(w, r, http.StatusGone, v)
		return
	case v.Email == "" || len(v.Email) > 320:
		v.Step = portalStepEmail
		s.renderPortal(w, r, http.StatusUnprocessableEntity, v)
		return
	case password == "":
		v.Error = "Enter your password."
		s.renderPortal(w, r, http.StatusUnprocessableEntity, v)
		return
	}
	resp := s.call(r, http.MethodPost, "/portal/auth/login", map[string]string{"flowId": v.Flow, "email": v.Email, "password": password})
	var out struct {
		RedirectURL string `json:"redirectUrl"`
	}
	if resp.Status != http.StatusOK || !resp.decode(&out) || !safeInviteRedirect(out.RedirectURL) {
		s.portalFailed(w, r, resp, v, "Sign-in failed")
		return
	}
	// The portal app's registered redirect URI with the authorization code.
	http.Redirect(w, r, out.RedirectURL, http.StatusSeeOther)
}

func (s *SignIn) portalReset(w http.ResponseWriter, r *http.Request) {
	v := portalView{Flow: portalFlow(r), Step: portalStepPassword, Email: strings.TrimSpace(r.PostFormValue("email"))}
	if v.Flow == "" {
		v.Step = portalStepExpired
		s.renderPortal(w, r, http.StatusGone, v)
		return
	}
	resp := s.call(r, http.MethodPost, "/portal/auth/password-reset", map[string]string{"flowId": v.Flow, "email": v.Email})
	if resp.Status != http.StatusOK {
		s.portalFailed(w, r, resp, v, "Something went wrong")
		return
	}
	v.ResetSent = true
	s.renderPortal(w, r, http.StatusOK, v)
}
