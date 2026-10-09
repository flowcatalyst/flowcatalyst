// Package shell renders UI pages inside the FlowCatalyst app shell (the kit's
// app-shell: navy sidebar, user menu, main region) and the UI's error pages.
// Page handlers build their content component and call Shell.Render.
package shell

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// Shell renders pages. The zero value works (no display names, version "dev").
type Shell struct {
	// DisplayName resolves the signed-in principal's name for the sidebar
	// (the session carries only the email). Nil or "" falls back to the email.
	DisplayName func(ctx context.Context, principalID string) string
	// Version is shown in the user menu.
	Version string
}

// Page describes the page being rendered.
type Page struct {
	// Title is the document title (the page's h1 comes from its content).
	Title string
	// Path is the /ui path, used to mark the current sidebar entry.
	Path string
}

// SignOutPath is the UI's sign-out endpoint (POST).
const SignOutPath = "/ui/logout"

// Render writes a full page: the app shell for the signed-in user with
// content in <main>, status 200.
func (s *Shell) Render(w http.ResponseWriter, r *http.Request, page Page, content templ.Component) {
	s.render(w, r, http.StatusOK, page, content)
}

func (s *Shell) render(w http.ResponseWriter, r *http.Request, status int, page Page, content templ.Component) {
	ac := auth.FromContext(r.Context())
	props := kit.AppShellProps{
		LogoHref:      "/ui/",
		Nav:           Navigation(ac, page.Path),
		User:          s.user(r.Context(), ac),
		Version:       orDev(s.Version),
		ProfileHref:   "/profile",
		SignOutAction: SignOutPath,
	}
	collapsed := false
	if c, err := r.Cookie(kit.SidebarCookie); err == nil && c.Value == "collapsed" {
		collapsed = true
	}
	writeHTML(w, r, status, document(page.Title, collapsed, props, content))
}

func (s *Shell) user(ctx context.Context, ac *auth.AuthContext) kit.ShellUser {
	if ac == nil {
		return kit.ShellUser{}
	}
	name := ""
	if s.DisplayName != nil {
		name = s.DisplayName(ctx, ac.PrincipalID)
	}
	if name == "" {
		name = ac.Email
	}
	return kit.ShellUser{Name: name, Email: ac.Email, Initials: initials(name)}
}

// initials: first letters of the first two words ("Ada Lovelace" → "AL"),
// or of the address before "@".
func initials(name string) string {
	name, _, _ = strings.Cut(name, "@")
	var out []rune
	for _, word := range strings.FieldsFunc(name, func(r rune) bool { return r == ' ' || r == '.' || r == '_' || r == '-' }) {
		out = append(out, []rune(strings.ToUpper(word))[0])
		if len(out) == 2 {
			break
		}
	}
	return string(out)
}

func orDev(v string) string {
	if v == "" {
		return "dev"
	}
	return v
}

func writeHTML(w http.ResponseWriter, r *http.Request, status int, c templ.Component) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if err := c.Render(r.Context(), w); err != nil {
		slog.ErrorContext(r.Context(), "ui: render page", "path", r.URL.Path, "err", err)
	}
}

// Deny is the guard.Denier for UI requests: an HTML page when the browser
// asked for HTML (inside the shell when signed in), a bare status otherwise.
// Datastar shows nothing for a non-2xx answer; the shell's datastar-fetch
// listener turns a 401 into a navigation to the login page.
func (s *Shell) Deny(w http.ResponseWriter, r *http.Request, status int, message string) {
	title := http.StatusText(status)
	if status == http.StatusForbidden {
		title = "Access denied"
	}
	if ds.IsDatastar(r) || !strings.Contains(r.Header.Get("Accept"), "text/html") {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(message + "\n"))
		return
	}
	if ac := auth.FromContext(r.Context()); ac == nil || ac.IsRoleless() {
		writeHTML(w, r, status, bareDocument(title, problem(title, message)))
		return
	}
	s.render(w, r, status, Page{Title: title, Path: r.URL.Path}, problem(title, message))
}

// Fail answers a handler error. A use-case error with a 4xx status (an
// authorization check, not found, validation) is shown with its message;
// anything else is logged and answered 500 without details.
func (s *Shell) Fail(w http.ResponseWriter, r *http.Request, err error) {
	if ue, ok := errors.AsType[*usecase.Error](err); ok {
		if status := ue.HTTPStatus(); status >= 400 && status < 500 {
			s.Deny(w, r, status, ue.Message)
			return
		}
	}
	slog.ErrorContext(r.Context(), "ui: handler failed", "path", r.URL.Path, "err", err)
	s.Deny(w, r, http.StatusInternalServerError, "Something went wrong. Try again, or use the current UI.")
}
