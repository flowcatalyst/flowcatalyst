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
	// Flashes maps the fixed flash keys a native post/redirect/get may carry
	// (SetFlash) to their messages. A key not in the table is ignored.
	Flashes map[string]kit.FlashMessageProps
	// CookieSecure is the Secure flag of the UI's own cookies (the flash
	// cookie), matching the session cookie's.
	CookieSecure bool
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

// RenderStatus writes a full page with another status (a form page
// re-rendered with its errors after a native post: 422).
func (s *Shell) RenderStatus(w http.ResponseWriter, r *http.Request, status int, page Page, content templ.Component) {
	s.render(w, r, status, page, content)
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
	flashes := s.takeFlash(w, r)
	writeHTML(w, r, status, document(page.Title, collapsed, props, flashes, content))
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
// A Datastar request gets the bare status: Datastar ignores the body of a
// non-200 answer, and the flash stack's fallback shows a generic message
// (the shell's listener turns a 401 into a navigation to the login page).
func (s *Shell) Deny(w http.ResponseWriter, r *http.Request, status int, message string) {
	title := failureTitle(status)
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
// anything else is logged and answered as a 500 without details.
//
// For a page request that is an error page with that status. For a Datastar
// request it is the error contract (docs/ui/README.md, "Datastar errors"):
// status 200 and an error flash message, because Datastar discards the body
// of any non-200 answer; the real status is logged. Use FailSSE once the
// handler has opened its stream.
func (s *Shell) Fail(w http.ResponseWriter, r *http.Request, err error) {
	status, message := Classify(r, err)
	if ds.IsDatastar(r) {
		flashFailure(ds.NewSSE(w, r), r, status, message)
		return
	}
	s.Deny(w, r, status, message)
}

// FailSSE answers err on a Datastar stream the handler has already opened
// (the status line is sent, so the failure can only be a patch).
func (s *Shell) FailSSE(sse *ds.SSE, r *http.Request, err error) {
	status, message := Classify(r, err)
	flashFailure(sse, r, status, message)
}

// Flash appends a flash message to the page's flash stack (flash-stack.md):
// the feedback after a Datastar action. Success and info messages remove
// themselves after 3s and 5s; warnings and errors stay until dismissed.
func Flash(sse *ds.SSE, msg kit.FlashMessageProps) error {
	return sse.Patch(kit.FlashMessage(msg), ds.Append("#"+FlashStackID))
}

const genericFailure = "The request could not be completed. Try again, or use the current UI."

// Classify maps a handler error to a status and a message safe to show: a
// use-case 4xx keeps both; anything else is logged and becomes a 500 with a
// generic message. Form handlers use it for their in-dialog error region.
func Classify(r *http.Request, err error) (int, string) {
	if ue, ok := errors.AsType[*usecase.Error](err); ok {
		if status := ue.HTTPStatus(); status >= 400 && status < 500 {
			return status, ue.Message
		}
	}
	slog.ErrorContext(r.Context(), "ui: handler failed", "path", r.URL.Path, "err", err)
	return http.StatusInternalServerError, genericFailure
}

func flashFailure(sse *ds.SSE, r *http.Request, status int, message string) {
	if status < http.StatusInternalServerError {
		slog.InfoContext(r.Context(), "ui: action refused", "path", r.URL.Path, "status", status, "message", message)
	}
	if err := Flash(sse, kit.FlashMessageProps{Kind: kit.FlashError, Summary: failureTitle(status) + ".", Detail: message}); err != nil {
		slog.DebugContext(r.Context(), "ui: failure patch not delivered", "err", err)
	}
}

// failureTitle is the heading of an error page or message.
func failureTitle(status int) string {
	switch status {
	case http.StatusForbidden:
		return "Access denied"
	case http.StatusInternalServerError:
		return "Something went wrong"
	}
	if t := http.StatusText(status); t != "" {
		return t
	}
	return "Request failed"
}
