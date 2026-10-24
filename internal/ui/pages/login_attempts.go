package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// LoginAttemptStore is the slice of the login attempt repository the page
// reads (the SPA's GET /api/login-attempts).
type LoginAttemptStore interface {
	FindPage(ctx context.Context, p loginattempt.ListParams) ([]loginattempt.LoginAttempt, error)
}

// LoginPrincipals resolves the principal a detail row names, to link it to
// its user or service account page.
type LoginPrincipals interface {
	FindByID(ctx context.Context, id ids.PrincipalID) (*principal.Principal, error)
}

// LoginAttempts is the login attempts page (/ui/login-attempts), mirroring
// the SPA's LoginAttemptListPage: a read-only, newest-first list filtered by
// type, outcome, identifier and date, with a detail drawer. Like the API it
// needs an anchor caller with platform:admin:login-attempt:view.
type LoginAttempts struct {
	Store      LoginAttemptStore
	Principals LoginPrincipals
	Shell      *shell.Shell
}

const (
	loginPath   = "/ui/login-attempts"
	loginFormID = "login-list"

	loginDrawerID  = "login-drawer"
	loginDrawerSig = "selLogin"

	loginLiveGet = "@get('/ui/login-attempts', {contentType: 'form'})"

	// loginPageSize is the SPA's fixed page size.
	loginPageSize = 100
)

// loginKeyPattern is a row's key in the URL and in the drawer signal:
// "<attempted_at in unix microseconds>.<id>" (see formatCursor). It never
// enters an expression unless it matches.
var loginKeyPattern = regexp.MustCompile(`^[0-9]{1,19}\.[A-Za-z0-9_-]{1,64}$`)

var (
	loginTypeOptions = []kit.Option{
		{Value: string(loginattempt.AttemptUserLogin), Label: string(loginattempt.AttemptUserLogin)},
		{Value: string(loginattempt.AttemptServiceAccountToken), Label: string(loginattempt.AttemptServiceAccountToken)},
	}
	loginOutcomeOptions = []kit.Option{
		{Value: string(loginattempt.OutcomeSuccess), Label: string(loginattempt.OutcomeSuccess)},
		{Value: string(loginattempt.OutcomeFailure), Label: string(loginattempt.OutcomeFailure)},
	}
)

// Mount registers the page's routes (read-only: no POSTs).
func (h *LoginAttempts) Mount(r chi.Router) {
	r.Get("/login-attempts", h.list)
	r.Get("/login-attempts/{key}", h.detail)
}

// ── URL state ──────────────────────────────────────────────────────────

// loginQuery is one view of the list: the filters and the keyset position.
// From and To are whole UTC days (YYYY-MM-DD), as on the jobs lists.
type loginQuery struct {
	Type, Outcome, Identifier string
	From, To                  string
	After                     *loginCursor
}

type loginCursor struct {
	At time.Time
	ID string
}

func parseLoginQuery(v url.Values) loginQuery {
	q := loginQuery{
		Type: queryValue(v.Get("attemptType")), Outcome: queryValue(v.Get("outcome")), Identifier: queryValue(v.Get("q")),
		From: validDay(v.Get("from")), To: validDay(v.Get("to")),
	}
	if at, id, ok := parseCursor(v.Get("after")); ok {
		q.After = &loginCursor{At: at, ID: id}
	}
	return q
}

func (q loginQuery) filtered() bool {
	return q.Type != "" || q.Outcome != "" || q.Identifier != "" || q.From != "" || q.To != ""
}

// activeFilters counts the Filters popover's controls in use (the search box
// has its own).
func (q loginQuery) activeFilters() int {
	n := 0
	for _, s := range []string{q.Type, q.Outcome, q.From, q.To} {
		if s != "" {
			n++
		}
	}
	return n
}

// URL is the canonical URL of this view.
func (q loginQuery) URL() string {
	v := url.Values{}
	for k, s := range map[string]string{"attemptType": q.Type, "outcome": q.Outcome, "q": q.Identifier, "from": q.From, "to": q.To} {
		if s != "" {
			v.Set(k, s)
		}
	}
	if q.After != nil {
		v.Set("after", formatCursor(q.After.At, q.After.ID))
	}
	return encodeURL(loginPath, v)
}

func (q loginQuery) first() loginQuery {
	q.After = nil
	return q
}

func (q loginQuery) params() loginattempt.ListParams {
	p := loginattempt.ListParams{Limit: loginPageSize + 1}
	if q.Type != "" {
		p.AttemptType = &q.Type
	}
	if q.Outcome != "" {
		p.Outcome = &q.Outcome
	}
	if q.Identifier != "" {
		p.Identifier = &q.Identifier
	}
	// The whole-day range: the "to" day is included (FindPage's bound is
	// inclusive, so the last microsecond of that day).
	since, until := dayBounds(q.From, q.To)
	p.DateFrom = since
	if until != nil {
		last := until.Add(-time.Microsecond)
		p.DateTo = &last
	}
	if q.After != nil {
		p.AfterTime, p.AfterID = &q.After.At, &q.After.ID
	}
	return p
}

// ── list ───────────────────────────────────────────────────────────────

type loginRow struct {
	ID, Key, Href string
	Type          string
	Outcome       string
	Identifier    string
	Reason        string
	IP            string
	Time          time.Time
}

type loginView struct {
	Query     loginQuery
	Rows      []loginRow
	OlderHref string
	Drawer    *loginDetail
}

func (h *LoginAttempts) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.CanReadLoginAttempts(auth.FromContext(ctx)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, parseLoginQuery(r.URL.Query()))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := view.patchList(ds.NewSSE(w, r)); err != nil {
			slog.DebugContext(ctx, "ui: login attempts patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, view)
}

func (h *LoginAttempts) renderPage(w http.ResponseWriter, r *http.Request, view loginView) {
	h.Shell.Render(w, r, shell.Page{Title: "Login Attempts", Path: loginPath}, loginPage(view))
}

// patchList replaces the table, the pager, the toolbar's live parts and the
// address.
func (v loginView) patchList(sse *ds.SSE) error {
	return errors.Join(
		sse.Patch(loginTable(v), ds.WithMode(ds.ModeReplace)), sse.Patch(kit.CursorPager(v.pager())),
		sse.Patch(kit.TableToolbarState(v.toolbar().State())), sse.ReplaceURL(v.Query.URL()),
	)
}

func (h *LoginAttempts) listView(ctx context.Context, q loginQuery) (loginView, error) {
	v := loginView{Query: q}
	attempts, err := h.Store.FindPage(ctx, q.params())
	if err != nil {
		return loginView{}, usecase.Internal("REPO", "find_page failed", err)
	}
	more := len(attempts) > loginPageSize
	if more {
		attempts = attempts[:loginPageSize]
	}
	for i := range attempts {
		v.Rows = append(v.Rows, toLoginRow(&attempts[i]))
	}
	if more {
		last := attempts[len(attempts)-1]
		next := q
		next.After = &loginCursor{At: last.AttemptedAt, ID: last.ID}
		v.OlderHref = next.URL()
	}
	return v, nil
}

func loginKey(a *loginattempt.LoginAttempt) string { return formatCursor(a.AttemptedAt, a.ID) }

func loginURL(key string) string { return loginPath + "/" + url.PathEscape(key) }

func toLoginRow(a *loginattempt.LoginAttempt) loginRow {
	return loginRow{
		ID: a.ID, Key: loginKey(a), Href: loginURL(loginKey(a)), Type: string(a.AttemptType), Outcome: string(a.Outcome),
		Identifier: deref(a.Identifier), Reason: failureReason(a.FailureReason), IP: deref(a.IPAddress), Time: a.AttemptedAt,
	}
}

// attemptTypeLabel is the SPA's formatAttemptType.
func attemptTypeLabel(t string) string {
	if t == string(loginattempt.AttemptUserLogin) {
		return "User Login"
	}
	return "Service Account"
}

func attemptTypeSeverity(t string) kit.TagSeverity {
	if t == string(loginattempt.AttemptUserLogin) {
		return kit.TagInfo
	}
	return kit.TagSecondary
}

func outcomeSeverity(o string) kit.TagSeverity {
	if o == string(loginattempt.OutcomeSuccess) {
		return kit.TagSuccess
	}
	return kit.TagDanger
}

// failureReason is the SPA's formatFailureReason: "BAD_PASSWORD" → "Bad
// password".
func failureReason(r *string) string {
	s := strings.ToLower(strings.ReplaceAll(deref(r), "_", " "))
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ── drawer ─────────────────────────────────────────────────────────────

type loginDetail struct {
	Key, Type, Outcome, Reason    string
	Identifier, PrincipalID       string
	IdentifierHref, PrincipalHref string
	IP, UserAgent                 string
	Time                          time.Time
}

func (d loginDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: loginDrawerID, Signal: loginDrawerSig, Label: "Login attempt", Size: kit.DrawerWide, Title: "Login Attempt Details", Subtitle: attemptTypeLabel(d.Type)}
}

// principalHref is where the SPA's link for a principal goes: the user's
// page for a USER, the service account's page for a SERVICE. A caller who
// cannot read principals, or a lookup that fails, gets plain text (the SPA
// leaves the row unresolved on any error).
func (h *LoginAttempts) principalHref(ctx context.Context, ac *auth.AuthContext, id string) string {
	if id == "" || h.Principals == nil || auth.CanReadPrincipals(ac) != nil {
		return ""
	}
	p, err := h.Principals.FindByID(ctx, ids.PrincipalID(id))
	if err != nil || p == nil {
		return ""
	}
	switch {
	case p.Type == principal.TypeUser:
		return "/ui/users/" + url.PathEscape(id)
	case p.Type == principal.TypeService && p.ServiceAccountID != nil:
		return "/ui/service-accounts/" + url.PathEscape(*p.ServiceAccountID)
	}
	return ""
}

func (h *LoginAttempts) detailOf(ctx context.Context, ac *auth.AuthContext, a *loginattempt.LoginAttempt) loginDetail {
	d := loginDetail{
		Key: loginKey(a), Type: string(a.AttemptType), Outcome: string(a.Outcome), Reason: failureReason(a.FailureReason),
		Identifier: deref(a.Identifier), PrincipalID: deref(a.PrincipalID), IP: deref(a.IPAddress), UserAgent: deref(a.UserAgent),
		Time: a.AttemptedAt,
	}
	d.PrincipalHref = h.principalHref(ctx, ac, d.PrincipalID)
	// The identifier of a user login (a user's page) or a developer token (the
	// user's own id) goes where the principal goes; a service account token's
	// is an OAuth client id, which has no page here.
	switch a.AttemptType {
	case loginattempt.AttemptUserLogin:
		if strings.HasPrefix(d.PrincipalHref, "/ui/users/") {
			d.IdentifierHref = d.PrincipalHref
		}
	case loginattempt.AttemptDeveloperToken:
		d.IdentifierHref = d.PrincipalHref
	case loginattempt.AttemptServiceAccountToken:
	}
	return d
}

func (h *LoginAttempts) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadLoginAttempts(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	key := chi.URLParam(r, "key")
	at, id, ok := parseCursor(key)
	if !ok || !loginKeyPattern.MatchString(key) {
		h.Shell.Fail(w, r, httperror.NotFound("LoginAttempt", key))
		return
	}
	// There is no read by id: the key carries the attempt's own time, so the
	// page of that one instant holds it.
	found, err := h.Store.FindPage(ctx, loginattempt.ListParams{DateFrom: &at, DateTo: &at, Limit: loginPageSize})
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find_page failed", err))
		return
	}
	i := slices.IndexFunc(found, func(a loginattempt.LoginAttempt) bool { return a.ID == id })
	if i < 0 {
		h.Shell.Fail(w, r, httperror.NotFound("LoginAttempt", key))
		return
	}
	d := h.detailOf(ctx, ac, &found[i])
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			sse.Patch(loginDrawerContent(d), ds.Inner("#"+loginDrawerID)),
			sse.PatchSignals(map[string]any{loginDrawerSig: d.Key}),
			sse.Focus(loginDrawerID+"-title"),
		); err != nil {
			slog.DebugContext(ctx, "ui: login attempt drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, parseLoginQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, view)
}
