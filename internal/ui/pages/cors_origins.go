package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors"
	corsops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/cors/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// CorsStore is the slice of the CORS origin repository the page reads (the
// SPA's GET /api/platform/cors).
type CorsStore interface {
	FindAll(ctx context.Context) ([]cors.AllowedOrigin, error)
	FindByID(ctx context.Context, id string) (*cors.AllowedOrigin, error)
}

// CorsCommands are the operations the CORS origins page runs; CorsOps runs
// the API's use cases.
type CorsCommands interface {
	Add(ctx context.Context, cmd corsops.AddCommand) (string, error)
	Delete(ctx context.Context, id string) error
}

// CorsOps runs the CORS origin use cases through the unit of work, exactly
// as internal/platform/cors/api does.
type CorsOps struct {
	Repo *cors.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Add runs AddOrigin and returns the new origin's id.
func (o CorsOps) Add(ctx context.Context, cmd corsops.AddCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, corsops.AddOrigin(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.OriginID, nil
}

// Delete runs DeleteOrigin.
func (o CorsOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, corsops.DeleteOrigin(o.Repo), corsops.DeleteCommand{OriginID: id}, auth.NewExecutionContext(ctx))
	return err
}

// CorsOrigins is the CORS origins page (/ui/cors-origins), mirroring the
// SPA's CorsOriginsPage: the allowed origins with search, sorting and
// paging, an add dialog and a remove confirmation. The API's checks apply:
// an anchor caller with cors-origin:view to read, :create to add, :delete to
// remove; the controls are hidden without them and a forged post is refused.
type CorsOrigins struct {
	Store CorsStore
	Ops   CorsCommands
	Shell *shell.Shell
}

const (
	corsPath   = "/ui/cors-origins"
	corsFormID = "cors-list"

	corsDialogSlot = "cors-dialog-slot"
	corsAddDialog  = "cors-add"
	corsDelDialog  = "cors-delete"
	corsAddSignal  = "corsAdd"

	corsLiveGet = "@get('/ui/cors-origins', {contentType: 'form'})"

	// corsCacheNote is the SPA's page note and removal warning: the platform
	// caches the allowed origins for five minutes.
	corsCacheNote = "Origins are cached for 5 minutes."
)

var corsList = &listSpec{
	path:     corsPath,
	sortKeys: []string{"origin", "description", "added"},
	statusOK: func(string) bool { return false },
	rows:     []int{10, 25, 50},
	// defaultRows is the SPA's page size.
	defaultRows: 10,
}

var corsFieldCodes = map[string]string{
	"ORIGIN_REQUIRED":       "origin",
	"INVALID_ORIGIN_FORMAT": "origin",
	"ORIGIN_ALREADY_EXISTS": "origin",
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *CorsOrigins) Mount(r chi.Router) {
	r.Get("/cors-origins", h.list)
	r.Get("/cors-origins/new", h.addForm)
	r.Get("/cors-origins/{id}/delete", h.deleteForm)
	post := r.With(parseForm)
	post.Post("/cors-origins/new", h.add)
	post.Post("/cors-origins/{id}/delete", h.delete)
}

// ── list ───────────────────────────────────────────────────────────────

type corsPerms struct{ Create, Delete bool }

func corsPermsFor(ac *auth.AuthContext) corsPerms {
	return corsPerms{Create: auth.CanCreateCorsOrigins(ac) == nil, Delete: auth.CanDeleteCorsOrigins(ac) == nil}
}

type corsRow struct {
	ID, Origin, Description string
	Added                   time.Time
	DeleteHref              string
}

type corsView struct {
	Query       listQuery
	Rows        []corsRow
	Total       int
	Page, Pages int
	Perms       corsPerms
	Dialog      templ.Component
}

func (h *CorsOrigins) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadCorsOrigins(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), corsList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patchTable(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: cors origins patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *CorsOrigins) renderPage(w http.ResponseWriter, r *http.Request, status int, view corsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "CORS Origins", Path: corsPath}, corsPage(view))
}

func (h *CorsOrigins) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (corsView, error) {
	all, err := h.Store.FindAll(ctx)
	if err != nil {
		return corsView{}, usecase.Internal("REPO", "find_all failed", err)
	}
	return buildCorsView(ac, q, all), nil
}

// buildCorsView applies the SPA's search (origin or description contains the
// text, any case), sort and paging.
func buildCorsView(ac *auth.AuthContext, q listQuery, all []cors.AllowedOrigin) corsView {
	needle := strings.ToLower(q.Q)
	shown := make([]cors.AllowedOrigin, 0, len(all))
	for _, o := range all {
		if needle == "" || strings.Contains(strings.ToLower(o.Origin), needle) || strings.Contains(strings.ToLower(deref(o.Description)), needle) {
			shown = append(shown, o)
		}
	}
	slices.SortStableFunc(shown, func(a, b cors.AllowedOrigin) int {
		var c int
		switch q.Sort {
		case "description":
			c = strings.Compare(strings.ToLower(deref(a.Description)), strings.ToLower(deref(b.Description)))
		case "added":
			c = a.CreatedAt.Compare(b.CreatedAt)
		default:
			c = strings.Compare(a.Origin, b.Origin)
		}
		if q.Dir == kit.SortDesc {
			c = -c
		}
		return c
	})
	v := corsView{Perms: corsPermsFor(ac), Total: len(shown)}
	var lo, hi int
	q.Page, v.Pages, lo, hi = q.pageBounds(len(shown))
	v.Page, v.Query = q.Page, q
	for _, o := range shown[lo:hi] {
		v.Rows = append(v.Rows, corsRow{ID: o.ID, Origin: o.Origin, Description: deref(o.Description), Added: o.CreatedAt, DeleteHref: corsDeleteURL(o.ID)})
	}
	return v
}

func corsDeleteURL(id string) string { return corsPath + "/" + url.PathEscape(id) + "/delete" }

func (v corsView) patchTable(sse *ds.SSE) error {
	// The table is replaced, not morphed: a morph resets each row's <time> to
	// its UTC fallback without localising it again (see patchDjTable).
	return errors.Join(sse.Patch(corsTable(v), ds.WithMode(ds.ModeReplace)), sse.Patch(kit.Paginator(v.pager())))
}

func corsID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("CorsOrigin", id)
	}
	return id, nil
}

// ── add ────────────────────────────────────────────────────────────────

type corsForm struct {
	Origin, Description string
	Errs                formErrors
	Open                bool
}

func (f corsForm) id(key string) string         { return fieldID(corsAddDialog, key) }
func (f corsForm) fieldError(key string) string { return f.Errs.Fields[key] }

func (f corsForm) fields() []templ.Component {
	return []templ.Component{corsField(f, "origin"), corsField(f, "description")}
}

func (f corsForm) firstInvalid() string {
	for _, k := range []string{"origin", "description"} {
		if f.fieldError(k) != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f corsForm) dialog() kit.FormDialogProps {
	return kit.FormDialogProps{
		ID: corsAddDialog, Signal: corsAddSignal, Action: corsPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Add CORS Origin", Description: "Add an origin URL that should be allowed to make cross-origin requests to the API.", SubmitLabel: "Add Origin",
	}
}

func (f corsForm) fieldProps(key, label, help string, required bool) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: required, Description: help, Error: f.fieldError(key), Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f corsForm) describedBy(key string) string {
	if f.fieldError(key) != "" {
		return f.id(key) + "-err"
	}
	return ""
}

func submittedCorsForm(r *http.Request) corsForm {
	return corsForm{Origin: strings.TrimSpace(r.PostFormValue("origin")), Description: strings.TrimSpace(r.PostFormValue("description"))}
}

func (h *CorsOrigins) addForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateCorsOrigins(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showDialog(w, r, ac, corsAddDialogC(corsForm{Open: true}))
}

func (h *CorsOrigins) add(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := submittedCorsForm(r)
	if err := auth.CanCreateCorsOrigins(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.addFailed(w, r, ac, f)
		return
	}
	// The SPA's checks before it posts; the origin is stored in lower case.
	origin := strings.ToLower(f.Origin)
	switch {
	case origin == "":
		f.Errs.field("origin", "Origin URL is required")
	case !strings.HasPrefix(origin, "http://") && !strings.HasPrefix(origin, "https://"):
		f.Errs.field("origin", "Origin must start with http:// or https://")
	}
	if f.Errs.any() {
		h.addFailed(w, r, ac, f)
		return
	}
	if _, err := h.Ops.Add(ctx, corsops.AddCommand{Origin: origin, Description: optionalText(f.Description)}); err != nil {
		f.Errs.apply(r, err, corsFieldCodes)
		h.addFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashCorsAdded)
		http.Redirect(w, r, corsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(corsAddDialog); err != nil {
		slog.DebugContext(ctx, "ui: cors add close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac)
	h.flash(ctx, sse, "CORS origin \""+origin+"\" added successfully")
}

func (h *CorsOrigins) addFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f corsForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, corsAddDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	f.Open = true
	h.showDialogStatus(w, r, ac, corsAddDialogC(f), cmp.Or(f.Errs.Status, http.StatusUnprocessableEntity))
}

// ── remove ─────────────────────────────────────────────────────────────

// deleteForm opens the removal confirmation, naming the origin (the SPA's
// dialog shows it).
func (h *CorsOrigins) deleteForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteCorsOrigins(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	o, err := h.origin(ctx, r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showDialog(w, r, ac, corsDeleteDialogC(o.ID, o.Origin))
}

func (h *CorsOrigins) origin(ctx context.Context, r *http.Request) (*cors.AllowedOrigin, error) {
	id, err := corsID(r)
	if err != nil {
		return nil, err
	}
	o, err := h.Store.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if o == nil {
		return nil, httperror.NotFound("CorsOrigin", id)
	}
	return o, nil
}

func (h *CorsOrigins) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteCorsOrigins(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	o, err := h.origin(ctx, r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Ops.Delete(ctx, o.ID); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashCorsRemoved)
		http.Redirect(w, r, corsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac)
	h.flash(ctx, sse, "CORS origin \""+o.Origin+"\" removed")
}

// ── shared ─────────────────────────────────────────────────────────────

// afterChange re-renders the table the user is on.
func (h *CorsOrigins) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext) {
	ctx := r.Context()
	view, err := h.listView(ctx, ac, refererList(r, corsList))
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: cors origins table patch not delivered", "err", err)
	}
}

// flash is the SPA's toast: "Success" and the sentence naming the origin.
func (h *CorsOrigins) flash(ctx context.Context, sse *ds.SSE, detail string) {
	if err := shell.Flash(sse, success(detail)); err != nil {
		slog.DebugContext(ctx, "ui: cors origins flash not delivered", "err", err)
	}
}

func (h *CorsOrigins) showDialog(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, dialog templ.Component) {
	h.showDialogStatus(w, r, ac, dialog, http.StatusOK)
}

func (h *CorsOrigins) showDialogStatus(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, dialog templ.Component, status int) {
	if ds.IsDatastar(r) {
		if err := patchDialogSlot(ds.NewSSE(w, r), corsDialogSlot, dialog); err != nil {
			slog.DebugContext(r.Context(), "ui: cors origins dialog patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, corsList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = dialog
	h.renderPage(w, r, status, view)
}
