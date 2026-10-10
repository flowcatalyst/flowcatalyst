package pages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// ClientStore is the slice of the client repository the clients page reads
// (GET /api/clients, GET /api/clients/{id}).
type ClientStore interface {
	FindAll(ctx context.Context) ([]client.Client, error)
	FindByID(ctx context.Context, id string) (*client.Client, error)
}

// ClientConfigReader reads a client's application configs (the enabled
// state GET /api/clients/{id}/applications reports).
type ClientConfigReader interface {
	FindByClient(ctx context.Context, clientID string) ([]application.ClientConfig, error)
}

// ClientsPage is the clients page (/ui/clients), mirroring the SPA's
// ClientListPage, ClientCreateDrawer and ClientDetailDrawer: the list, the
// drawer with the applications picker, create and edit, activate, suspend
// and deactivate. Login Branding stays on the SPA (its theme editor is
// shared with the platform settings, not ported yet).
type ClientsPage struct {
	Clients ClientStore
	Ops     ClientCommands
	Apps    ApplicationStore
	Configs ClientConfigReader
	Shell   *shell.Shell
}

const (
	clientsPath   = "/ui/clients"
	clientsFormID = "clients-list"

	cliDrawerID   = "cli-drawer"
	cliDrawerSig  = "selCli"
	cliDialogSlot = "cli-dialog-slot"

	cliActivateDialog   = "cli-activate"
	cliSuspendDialog    = "cli-suspend"
	cliDeactivateDialog = "cli-deactivate"

	clientsLiveGet = "@get('/ui/clients', {contentType: 'form'})"
)

var clientList = &listSpec{
	path:        clientsPath,
	sortKeys:    []string{"identifier", "name", "status", "created"},
	statusOK:    func(string) bool { return false }, // the SPA's list has no filters
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm). The client
// select's search (/ui/clients/options) is ClientOptions'.
func (h *ClientsPage) Mount(r chi.Router) {
	r.Get("/clients", h.list)
	r.Get("/clients/new", h.createForm)
	r.Get("/clients/{id}", h.detail)
	r.Get("/clients/{id}/edit", h.editForm)
	post := r.With(parseForm)
	post.Post("/clients/new", h.create)
	post.Post("/clients/{id}/edit", h.update)
	post.Post("/clients/{id}/activate", h.activate)
	post.Post("/clients/{id}/suspend", h.suspend)
	post.Post("/clients/{id}/deactivate", h.deactivate)
	post.Post("/clients/{id}/applications", h.saveApplications)
}

// ── list ───────────────────────────────────────────────────────────────

func (h *ClientsPage) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/clients's check (anchor scope and the view permission).
	if err := auth.CanReadClients(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), clientList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patchTable(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: clients patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *ClientsPage) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (clientsView, error) {
	all, err := h.Clients.FindAll(ctx)
	if err != nil {
		return clientsView{}, fmt.Errorf("find clients: %w", err)
	}
	return buildClientsView(ac, q, all), nil
}

func (h *ClientsPage) renderPage(w http.ResponseWriter, r *http.Request, status int, view clientsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Clients", Path: clientsPath}, clientsPage(view))
}

// cliPerms are the API's per-operation checks (all anchor-only).
type cliPerms struct {
	Create, Update, Activate, Suspend, Deactivate, Applications bool
}

func cliPermsFor(ac *auth.AuthContext) cliPerms {
	return cliPerms{
		Create:     auth.CanCreateClients(ac) == nil,
		Update:     auth.CanUpdateClients(ac) == nil,
		Activate:   auth.CanActivateClients(ac) == nil,
		Suspend:    auth.CanSuspendClients(ac) == nil,
		Deactivate: auth.CanDeactivateClients(ac) == nil,
		// PUT /api/clients/{id}/applications: anchor scope.
		Applications: ac.IsAnchor(),
	}
}

type cliRow struct {
	ID, Identifier, Name string
	Status               string
	StatusVariant        kit.BadgeVariant
	Created, CreatedISO  string
	DetailHref, EditHref string
}

type clientsView struct {
	Query       listQuery
	Rows        []cliRow
	Total       int
	Page, Pages int
	Perms       cliPerms
	Dialog      templ.Component
	Drawer      *cliDetail
}

// buildClientsView searches identifier and name (the SPA's quick filter),
// filters by status, sorts and pages. (The SPA requests server pages the
// API ignores and filters the page it has; here the search covers every
// client.)
func buildClientsView(ac *auth.AuthContext, q listQuery, all []client.Client) clientsView {
	v := clientsView{Perms: cliPermsFor(ac)}
	needle := strings.ToLower(q.Q)
	shown := make([]client.Client, 0, len(all))
	for _, c := range all {
		if q.Status != "" && string(c.Status) != q.Status {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(c.Identifier+" "+c.Name), needle) {
			continue
		}
		shown = append(shown, c)
	}
	slices.SortStableFunc(shown, func(a, b client.Client) int {
		var c int
		switch q.Sort {
		case "name":
			c = cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		case "status":
			c = cmp.Compare(a.Status, b.Status)
		case "created":
			c = a.CreatedAt.Compare(b.CreatedAt)
		default:
			c = cmp.Compare(a.Identifier, b.Identifier)
		}
		if q.Dir == kit.SortDesc {
			c = -c
		}
		return c
	})
	v.Total = len(shown)
	var lo, hi int
	q.Page, v.Pages, lo, hi = q.pageBounds(len(shown))
	v.Page, v.Query = q.Page, q
	for _, c := range shown[lo:hi] {
		v.Rows = append(v.Rows, cliRow{
			ID: string(c.ID), Identifier: c.Identifier, Name: c.Name,
			Status: string(c.Status), StatusVariant: cliStatusVariant(c.Status),
			Created: c.CreatedAt.UTC().Format("2006-01-02"), CreatedISO: c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			DetailHref: cliURL(string(c.ID)), EditHref: cliURL(string(c.ID)) + "/edit",
		})
	}
	return v
}

func cliURL(id string) string { return clientsPath + "/" + url.PathEscape(id) }

// cliStatusVariant is the SPA's getStatusSeverity.
func cliStatusVariant(s client.Status) kit.BadgeVariant {
	switch s {
	case client.StatusActive:
		return kit.BadgeSuccess
	case client.StatusSuspended:
		return kit.BadgeWarn
	case client.StatusInactive:
	}
	return kit.BadgeSecondary
}

func (v clientsView) rowActions(row cliRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "cli-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Actions = append(p.Actions, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	return p
}

func (v clientsView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(clientsTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func cliID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Client", id)
	}
	return id, nil
}

// ── drawer ─────────────────────────────────────────────────────────────

type cliDetail struct {
	ID, Identifier, Name string
	Status               string
	RawStatus            client.Status
	StatusVariant        kit.BadgeVariant
	StatusReason         string
	Created, Updated     string
	Apps                 []kit.PickItem
	AppsAvailable        int
	EditHref, ActionBase string
	Perms                cliPerms
}

func (d cliDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: cliDrawerID, Signal: cliDrawerSig, Label: "Client", Size: kit.DrawerWide, Title: d.Name, Subtitle: d.Identifier, Extras: poolStatusBadge(d.Status, d.StatusVariant)}
}

func (h *ClientsPage) loadClient(ctx context.Context, id string) (*client.Client, error) {
	c, err := h.Clients.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Client", id)
	}
	return c, nil
}

// detailOf builds the drawer, with GET /api/clients/{id}/applications'
// list: every application, enabled per the client's configs.
func (h *ClientsPage) detailOf(ctx context.Context, ac *auth.AuthContext, c *client.Client) (cliDetail, error) {
	d := cliDetail{
		ID: string(c.ID), Identifier: c.Identifier, Name: c.Name,
		Status: string(c.Status), RawStatus: c.Status, StatusVariant: cliStatusVariant(c.Status), StatusReason: deref(c.StatusReason),
		Created: c.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), Updated: c.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		EditHref: cliURL(string(c.ID)) + "/edit", ActionBase: cliURL(string(c.ID)), Perms: cliPermsFor(ac),
	}
	if h.Apps == nil {
		return d, nil
	}
	apps, err := h.Apps.FindWithFilters(ctx, nil, nil)
	if err != nil {
		return cliDetail{}, usecase.Internal("REPO", "find applications failed", err)
	}
	enabled := map[string]bool{}
	if h.Configs != nil {
		cfgs, err := h.Configs.FindByClient(ctx, string(c.ID))
		if err != nil {
			return cliDetail{}, usecase.Internal("REPO", "find client configs failed", err)
		}
		for _, cfg := range cfgs {
			enabled[string(cfg.ApplicationID)] = cfg.Enabled
		}
	}
	for _, a := range apps {
		it := kit.PickItem{Value: string(a.ID), Name: a.Name, Code: a.Code, Checked: enabled[string(a.ID)]}
		if !a.Active {
			it.Note = "Inactive"
		}
		if !it.Checked {
			d.AppsAvailable++
		}
		d.Apps = append(d.Apps, it)
	}
	return d, nil
}

func (h *ClientsPage) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadClients(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := cliID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, err := h.loadClient(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, c)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := openCliDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: client drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, clientList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openCliDrawer(sse *ds.SSE, d cliDetail) error {
	return errors.Join(
		sse.Patch(cliDrawerContent(d), ds.Inner("#"+cliDrawerID)),
		sse.PatchSignals(map[string]any{cliDrawerSig: d.ID}),
		sse.Focus(cliDrawerID+"-title"),
	)
}

// ── actions ────────────────────────────────────────────────────────────

type cliAction struct {
	check   func(*auth.AuthContext) error
	run     func(ClientCommands, context.Context, string) error
	flash   string
	removed bool
}

var (
	activateCliAction = cliAction{check: auth.CanActivateClients, run: ClientCommands.Activate, flash: flashCliActivated}
	// The SPA suspends and deactivates with fixed reasons.
	suspendCliAction = cliAction{
		check: auth.CanSuspendClients, flash: flashCliSuspended,
		run: func(o ClientCommands, ctx context.Context, id string) error {
			return o.Suspend(ctx, id, "Manual suspension")
		},
	}
	deactivateCliAction = cliAction{check: auth.CanDeactivateClients, run: ClientCommands.Deactivate, flash: flashCliDeactivated, removed: true}
)

func (h *ClientsPage) activate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, activateCliAction)
}

func (h *ClientsPage) suspend(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, suspendCliAction)
}

func (h *ClientsPage) deactivate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deactivateCliAction)
}

func (h *ClientsPage) runAction(w http.ResponseWriter, r *http.Request, a cliAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := cliID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := a.run(h.Ops, ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, a.flash, a.removed)
}

// done answers a successful change: the list and the drawer (closed when
// the client is gone) and a flash; a native post redirects.
func (h *ClientsPage) done(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id, flash string, removed bool) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, clientsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, removed)
	if err := flashKey(sse, flash); err != nil {
		slog.DebugContext(ctx, "ui: client flash not delivered", "err", err)
	}
}

func (h *ClientsPage) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, removed bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, clientList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: clients table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if removed {
		if err := sse.PatchSignals(map[string]any{cliDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: client drawer close not delivered", "err", err)
		}
		return
	}
	c, err := h.loadClient(ctx, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, c)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(cliDrawerContent(d), ds.Inner("#"+cliDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: client drawer patch not delivered", "err", err)
	}
}

// saveApplications replaces the client's enabled applications (the SPA's
// pick list Save: PUT /api/clients/{id}/applications, anchor-only).
func (h *ClientsPage) saveApplications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if !ac.IsAnchor() {
		h.Shell.Fail(w, r, httperror.Forbidden("Anchor scope required"))
		return
	}
	id, err := cliID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Ops.UpdateApplications(ctx, id, facetValues(r.PostForm["applicationIds"])); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, flashCliApps, false)
}

func cliConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: clientsPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(cliDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}

// ── create and edit ────────────────────────────────────────────────────

const (
	cliFormDialog = "cli-form"
	cliFormSignal = "cliForm"
)

var cliFieldCodes = map[string]string{
	"NAME_REQUIRED":       "name",
	"IDENTIFIER_REQUIRED": "identifier",
	"INVALID_IDENTIFIER":  "identifier",
	"RESERVED_IDENTIFIER": "identifier",
	"IDENTIFIER_EXISTS":   "identifier",
}

type cliForm struct {
	Edit             bool
	CliID, Subtitle  string
	Name, Identifier string
	Errs             formErrors
	Open             bool
}

func (f cliForm) id(key string) string { return fieldID(cliFormDialog, key) }

func (f cliForm) keys() []string {
	if f.Edit {
		return []string{"name"}
	}
	return []string{"name", "identifier"}
}

func (f cliForm) firstInvalid() string {
	for _, k := range f.keys() {
		if f.Errs.Fields[k] != "" {
			return f.id(k)
		}
	}
	return ""
}

func (f cliForm) fields() []templ.Component {
	out := make([]templ.Component, 0, 2)
	for _, k := range f.keys() {
		out = append(out, cliField(f, k))
	}
	return out
}

func (f cliForm) dialog() kit.FormDialogProps {
	p := kit.FormDialogProps{
		ID: cliFormDialog, Signal: cliFormSignal, Action: clientsPath + "/new", Open: f.Open, Error: f.Errs.Form,
		Title: "Create Client", Description: "Add a new customer client to the platform", SubmitLabel: "Create Client",
	}
	if f.Edit {
		p.Action = cliURL(f.CliID) + "/edit"
		p.Title, p.Description, p.SubmitLabel, p.RequireChange = "Edit Client", f.Subtitle, "Save", true
		p.Hidden = []kit.HiddenField{drawerField(cliDrawerSig)}
	}
	return p
}

func (f cliForm) fieldProps(key, label, help string) kit.FieldProps {
	return kit.FieldProps{Label: label, For: f.id(key), Required: true, Description: help, Error: f.Errs.Fields[key], Attrs: templ.Attributes{"id": f.id(key) + "-field"}}
}

func (f cliForm) inputProps(key, value, placeholder string, attrs templ.Attributes) kit.InputProps {
	p := kit.InputProps{ID: f.id(key), Name: key, Value: value, Placeholder: placeholder, Required: true, Invalid: f.Errs.Fields[key] != "", Attrs: attrs}
	if p.Invalid {
		p.DescribedBy = f.id(key) + "-err"
	}
	return p
}

func (h *ClientsPage) createForm(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	if err := auth.CanCreateClients(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, cliForm{Open: true})
}

func (h *ClientsPage) create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := cliForm{Name: r.PostFormValue("name"), Identifier: strings.TrimSpace(r.PostFormValue("identifier"))}
	if err := auth.CanCreateClients(ac); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	newID, err := h.Ops.Create(ctx, clientops.CreateCommand{Name: f.Name, Identifier: f.Identifier})
	if err != nil {
		f.Errs.apply(r, err, cliFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashCliCreated)
		http.Redirect(w, r, clientsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(cliFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: client form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, newID, false)
	if c, err := h.loadClient(ctx, newID); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if d, err := h.detailOf(ctx, ac, c); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := openCliDrawer(sse, d); err != nil {
		slog.DebugContext(ctx, "ui: client drawer patch not delivered", "err", err)
	}
	if err := flashKey(sse, flashCliCreated); err != nil {
		slog.DebugContext(ctx, "ui: client flash not delivered", "err", err)
	}
}

func (h *ClientsPage) editForm(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := firstError(auth.CanReadClients(ac), auth.CanUpdateClients(ac)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := cliID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, err := h.loadClient(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.showForm(w, r, ac, cliForm{Edit: true, CliID: string(c.ID), Subtitle: c.Name + " (" + c.Identifier + ")", Name: c.Name, Open: true})
}

func (h *ClientsPage) update(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	f := cliForm{Edit: true, Name: r.PostFormValue("name")}
	if err := firstError(auth.CanReadClients(ac), auth.CanUpdateClients(ac)); err != nil {
		f.Errs.apply(r, err, nil)
		h.formFailed(w, r, ac, f)
		return
	}
	id, err := cliID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, err := h.loadClient(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.CliID, f.Subtitle = string(c.ID), c.Name+" ("+c.Identifier+")"
	if err := h.Ops.Update(ctx, clientops.UpdateCommand{ID: id, Name: &f.Name}); err != nil {
		f.Errs.apply(r, err, cliFieldCodes)
		h.formFailed(w, r, ac, f)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashCliUpdated)
		http.Redirect(w, r, clientsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := sse.CloseDialog(cliFormDialog); err != nil {
		slog.DebugContext(ctx, "ui: client form close not delivered", "err", err)
	}
	h.afterChange(sse, r, ac, id, false)
	if err := flashKey(sse, flashCliUpdated); err != nil {
		slog.DebugContext(ctx, "ui: client flash not delivered", "err", err)
	}
}

func (h *ClientsPage) showForm(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f cliForm) {
	if ds.IsDatastar(r) {
		if err := ds.NewSSE(w, r).Patch(cliFormDialogC(f), ds.Inner("#"+cliDialogSlot)); err != nil {
			slog.DebugContext(r.Context(), "ui: client form patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, clientList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Dialog = cliFormDialogC(f)
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *ClientsPage) formFailed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, f cliForm) {
	if ds.IsDatastar(r) {
		patchFormFailure(ds.NewSSE(w, r), r, cliFormDialog, f.fields(), f.Errs.Form, f.firstInvalid())
		return
	}
	view, err := h.listView(r.Context(), ac, parseListQuery(nil, clientList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	f.Open = true
	view.Dialog = cliFormDialogC(f)
	h.renderPage(w, r, cmpOr(f.Errs.Status, http.StatusUnprocessableEntity), view)
}
