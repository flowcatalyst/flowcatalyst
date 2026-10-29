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
	"unicode/utf8"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/platformconfig/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// ConfigService is the slice of platformconfig.Service the page uses: the
// access rules the HTTP API runs (list, update and write gates, the client
// confinement, client resolution, delete and the audited reveal), so the two
// cannot drift.
type ConfigService interface {
	List(ctx context.Context, ac *auth.AuthContext, app string) ([]platformconfig.Config, error)
	AuthorizeUpdate(ctx context.Context, ac *auth.AuthContext, app string) error
	AuthorizeWrite(ctx context.Context, ac *auth.AuthContext, app string) error
	ReachFor(ctx context.Context, ac *auth.AuthContext, app string) (platformconfig.Reach, error)
	AuthorizeClientWrite(ctx context.Context, ac *auth.AuthContext, app, clientID string) error
	AuthorizeRevealApp(ctx context.Context, ac *auth.AuthContext, app string) error
	ResolveClientID(ctx context.Context, ref string) (string, error)
	Delete(ctx context.Context, ac *auth.AuthContext, app, section, property, clientRef string) error
	Reveal(ctx context.Context, ac *auth.AuthContext, app, section, property, clientRef string) (*platformconfig.Revealed, error)
}

// ConfigCommands is the set-property operation the page runs. ConfigOps runs
// the same use case as PUT /api/config/{app}/{section}/{property}.
type ConfigCommands interface {
	Set(ctx context.Context, cmd operations.SetPropertyCommand) error
}

// ConfigOps runs SetProperty through the unit of work, as the API handler
// does; the access checks are the caller's (ConfigService).
type ConfigOps struct {
	Repo *platformconfig.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Set runs SetProperty.
func (o ConfigOps) Set(ctx context.Context, cmd operations.SetPropertyCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, operations.SetProperty(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// ConfigApps lists the applications the select offers.
type ConfigApps interface {
	FindActive(ctx context.Context) ([]application.Application, error)
}

// PlatformConfig is the platform Configuration page (/ui/platform/config),
// mirroring the SPA's PlatformConfigPage: an application's properties, the
// add/edit dialog, delete, and the audited reveal of a secret. A secret's
// value is never part of the page: only the reveal response carries one.
type PlatformConfig struct {
	Config ConfigService
	Ops    ConfigCommands
	Apps   ConfigApps
	// ClientIdentifier names a client-scoped value's client (optional).
	ClientIdentifier func(ctx context.Context, clientID string) (string, bool)
	Shell            *shell.Shell
}

const (
	cfgPath   = "/ui/platform/config"
	cfgFormID = "config-list"

	// cfgPlatformApp is the application the page opens on (the SPA's).
	cfgPlatformApp = "platform"

	cfgDialogSlot = "cfg-dialog-slot"
	cfgRevealSlot = "cfg-reveal-slot"

	cfgDeleteDialog = "cfg-delete"
	cfgRevealDialog = "cfg-reveal"
)

var cfgList = &listSpec{
	path:        cfgPath,
	sortKeys:    []string{"section", "property", "updated"},
	statusOK:    func(string) bool { return false },
	facets:      []string{"app"},
	rows:        []int{25, 50, 100},
	defaultRows: 25,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *PlatformConfig) Mount(r chi.Router) {
	r.Get("/platform/config", h.list)
	r.Get("/platform/config/new", h.createForm)
	r.Get("/platform/config/edit", h.editForm)
	post := r.With(parseForm)
	post.Post("/platform/config/save", h.save)
	post.Post("/platform/config/delete", h.delete)
	post.Post("/platform/config/reveal", h.reveal)
}

// ── list ───────────────────────────────────────────────────────────────

func (h *PlatformConfig) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/platform-config/{app}'s coarse gate; List repeats the full rule.
	if err := auth.RequireConfigView(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parseListQuery(r.URL.Query(), cfgList)
	view, err := h.listView(ctx, ac, q, "")
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patch(sse), sse.Patch(cfgActions(view)), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: configuration patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *PlatformConfig) renderPage(w http.ResponseWriter, r *http.Request, status int, view cfgView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Configuration", Path: cfgPath}, cfgPage(view))
}

// cfgPerms are what the viewer is offered; the server decides again on every
// post. Edit: the API's write rule for the application. Reveal: the reveal
// permission reaching the application (and, per row, the row's client).
type cfgPerms struct {
	Edit, Reveal bool
}

type cfgRow struct {
	ID, Section, Property string
	// Client is the client-scoped value's client (identifier when known);
	// empty for the application-wide value.
	Client              string
	Secret              bool
	Value, ValueFull    string
	Description         string
	Updated, UpdatedISO string
	EditHref, Query     string
	CanReveal           bool
	// CanEdit: the viewer may write the application and this row's client
	// (or, for the application-wide value, that value).
	CanEdit bool
	// Reserved: the entry belongs to an ingestion (section ingest) and is
	// edited there; IngestionsHref lists that application's ingestions of the
	// client.
	Reserved       bool
	IngestionsHref string
	// Hidden: the read service withheld the value (an ingestion's settings
	// seen by someone who cannot update ingestions).
	Hidden                bool
	updatedAt             int64
	sortSection, sortProp string
	sortClient            string
}

type cfgView struct {
	Query listQuery
	// App is the application shown; Apps the select's options.
	App   string
	Apps  []kit.Option
	Rows  []cfgRow
	Total int
	Page  int
	Pages int
	Perms cfgPerms
	// Dialog is the add/edit form (a navigation to /new or /edit); Reveal the
	// shown-once secret value (a native reveal post).
	Dialog templ.Component
	Reveal templ.Component
}

// listView reads the application's properties with the API's checks and
// builds the page. app overrides the query's application (a post's own).
func (h *PlatformConfig) listView(ctx context.Context, ac *auth.AuthContext, q listQuery, app string) (cfgView, error) {
	opts, err := h.appOptions(ctx, ac)
	if err != nil {
		return cfgView{}, err
	}
	if app == "" {
		app = pickConfigApp(q, opts)
	}
	q = q.withFacet("app", []string{app})
	if !slices.ContainsFunc(opts, func(o kit.Option) bool { return o.Value == app }) {
		opts = append(opts, kit.Option{Value: app, Label: app})
	}
	rows, err := h.Config.List(ctx, ac, app)
	if err != nil {
		return cfgView{}, err
	}
	v := cfgView{Query: q, App: app, Apps: opts}
	v.Perms.Edit = h.Config.AuthorizeWrite(ctx, ac, app) == nil
	v.Perms.Reveal = h.Config.AuthorizeRevealApp(ctx, ac, app) == nil
	reach, err := h.Config.ReachFor(ctx, ac, app)
	if err != nil {
		return cfgView{}, err
	}
	needle := strings.ToLower(q.Q)
	matched := make([]cfgRow, 0, len(rows))
	for i := range rows {
		row := h.toRow(ctx, ac, &rows[i], v.Perms)
		// An entry of an ingestion's settings is changed on the ingestion, never here.
		row.CanEdit = v.Perms.Edit && !row.Reserved && reach.CanWrite(deref(ids.StringPtr(rows[i].ClientID)))
		// The SPA's quick filter: section, property, description.
		if needle == "" || strings.Contains(strings.ToLower(row.Section+" "+row.Property+" "+row.Description), needle) {
			matched = append(matched, row)
		}
	}
	slices.SortStableFunc(matched, func(a, b cfgRow) int {
		var c int
		switch q.Sort {
		case "property":
			c = cmp.Or(cmp.Compare(a.sortProp, b.sortProp), cmp.Compare(a.sortSection, b.sortSection))
		case "updated":
			c = cmp.Compare(a.updatedAt, b.updatedAt)
		default:
			c = cmp.Or(cmp.Compare(a.sortSection, b.sortSection), cmp.Compare(a.sortProp, b.sortProp))
		}
		if c == 0 {
			c = cmp.Compare(a.sortClient, b.sortClient)
		}
		if q.Dir == kit.SortDesc {
			c = -c
		}
		return c
	})
	var lo, hi int
	v.Total = len(matched)
	v.Page, v.Pages, lo, hi = q.pageBounds(v.Total)
	v.Rows = matched[lo:hi]
	return v, nil
}

// pickConfigApp is the application the query names when the select offers
// it; else the platform's, else the first option.
func pickConfigApp(q listQuery, opts []kit.Option) string {
	if want := q.facet("app"); len(want) > 0 {
		return want[0]
	}
	if slices.ContainsFunc(opts, func(o kit.Option) bool { return o.Value == cfgPlatformApp }) || len(opts) == 0 {
		return cfgPlatformApp
	}
	return opts[0].Value
}

// appOptions is the SPA's application select: the platform and the active
// applications (GET /api/applications?activeOnly), the ones the viewer may
// read; an application-scoped account does not get "platform" (it may not
// reach it).
func (h *PlatformConfig) appOptions(ctx context.Context, ac *auth.AuthContext) ([]kit.Option, error) {
	var codes []string
	if !ac.IsAnchor() || !ac.IsApplicationScoped() {
		codes = append(codes, cfgPlatformApp)
	}
	if auth.CanReadApplications(ac) == nil {
		apps, err := h.Apps.FindActive(ctx)
		if err != nil {
			return nil, err
		}
		for i := range apps {
			if auth.CanReadAllApplications(ac) || ac.CanAccessApplication(string(apps[i].ID)) {
				codes = append(codes, apps[i].Code)
			}
		}
	}
	// The picker lists them by name, ignoring case, so the order does not
	// depend on how the codes are cased.
	slices.SortFunc(codes, func(a, b string) int {
		return cmp.Or(strings.Compare(strings.ToLower(a), strings.ToLower(b)), strings.Compare(a, b))
	})
	codes = slices.Compact(codes)
	opts := make([]kit.Option, len(codes))
	for i, c := range codes {
		opts[i] = kit.Option{Value: c, Label: c}
	}
	return opts, nil
}

// maxShownValue caps a plain value in the table; the rest is in the edit
// dialog.
const maxShownValue = 120

func (h *PlatformConfig) toRow(ctx context.Context, ac *auth.AuthContext, c *platformconfig.Config, perms cfgPerms) cfgRow {
	row := cfgRow{
		ID: c.ID, Section: c.Section, Property: c.Property, Secret: c.ValueType == platformconfig.ValueSecret,
		Description: deref(c.Description),
		Updated:     c.UpdatedAt.UTC().Format("2006-01-02 15:04"), UpdatedISO: c.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		updatedAt: c.UpdatedAt.UnixNano(), sortSection: strings.ToLower(c.Section), sortProp: strings.ToLower(c.Property),
	}
	clientID := ids.StringPtr(c.ClientID)
	var cid string
	if clientID != nil {
		cid = *clientID
		row.Client, row.sortClient = cid, cid
		if h.ClientIdentifier != nil {
			if ident, ok := h.ClientIdentifier(ctx, cid); ok {
				row.Client = ident
			}
		}
	}
	// A secret's value is not read here at all: only the mask is shown. A
	// hidden value is blank already; the row says why.
	row.Hidden = c.Hidden
	if !row.Secret && !row.Hidden {
		row.ValueFull = c.Value
		row.Value = c.Value
		if utf8.RuneCountInString(row.Value) > maxShownValue {
			row.Value = string([]rune(row.Value)[:maxShownValue]) + "…"
		}
	}
	if strings.EqualFold(c.Section, platformconfig.ReservedSection) {
		row.Reserved = true
		q := url.Values{"application": {c.ApplicationCode}}
		if cid != "" {
			q.Set("client", cid)
		}
		row.IngestionsHref = ingsPath + "?" + q.Encode()
	}
	row.Query = cfgQuery(c.ApplicationCode, c.Section, c.Property, cid)
	row.EditHref = cfgPath + "/edit?" + row.Query
	row.CanReveal = row.Secret && perms.Reveal && platformconfig.RevealReach(ac, cid) == nil
	return row
}

// cfgNewHref is the add dialog for an application.
// cfgAppHref is the address of the list for an application (the application
// picker's options link to it).
func cfgAppHref(app string) string { return cfgPath + "?app=" + url.QueryEscape(app) }

func cfgNewHref(app string) string { return cfgPath + "/new?app=" + url.QueryEscape(app) }

// cfgQuery addresses one property: the application, section, property and the
// client's id (empty for the application-wide value).
func cfgQuery(app, section, property, clientID string) string {
	v := url.Values{"app": {app}, "section": {section}, "property": {property}}
	if clientID != "" {
		v.Set("client", clientID)
	}
	return v.Encode()
}

// patch sends the regions a list change touches: the table, the pager, the
// toolbar's state (it carries the application and the sort).
func (v cfgView) patch(sse *ds.SSE) error {
	return errors.Join(
		sse.Patch(cfgTable(v)),
		sse.Patch(kit.Paginator(v.pager())),
		sse.Patch(kit.TableToolbarState(v.toolbar().State())),
	)
}

// rowActions: Reveal (secrets, when allowed) and Edit inline, Delete in the
// menu behind a confirmation. Left out when the viewer has no permission.
func (v cfgView) rowActions(row cfgRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "cfg-" + row.ID + "-actions", RowLabel: row.Section + "/" + row.Property}
	if row.CanReveal {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "eye", Label: "Reveal…", Attrs: confirmTrigger(cfgRevealDialog, cfgPath+"/reveal?"+row.Query)})
	}
	if row.CanEdit {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
		p.Menu = append(p.Menu, kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, Attrs: confirmTrigger(cfgDeleteDialog, cfgPath+"/delete?"+row.Query)})
	}
	return p
}

// cfgConfirm is one shared confirm dialog; the trigger sets the property's
// action.
func cfgConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: cfgPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}

// ── delete ─────────────────────────────────────────────────────────────

func (h *PlatformConfig) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	app := r.FormValue("app")
	// The settings of an ingestion are deleted on the ingestion.
	if err := platformconfig.RequireWritableSection(r.FormValue("section")); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	// DELETE /api/config/...'s rule, run by the service the API runs.
	if err := h.Config.Delete(ctx, ac, app, r.FormValue("section"), r.FormValue("property"), r.FormValue("client")); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.changed(w, r, ac, app, flashCfgDeleted, false)
}

// changed answers a successful save or delete: the form closed (a save), the
// table the user is on and a flash; a native post redirects with the flash
// key.
func (h *PlatformConfig) changed(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, app, flash string, closeForm bool) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, cfgPath+"?app="+url.QueryEscape(app), http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if closeForm {
		if err := sse.CloseDialog(cfgFormDialog); err != nil {
			slog.DebugContext(ctx, "ui: configuration form close not delivered", "err", err)
		}
	}
	if view, err := h.listView(ctx, ac, refererList(r, cfgList), app); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patch(sse); err != nil {
		slog.DebugContext(ctx, "ui: configuration table patch not delivered", "err", err)
	}
	if err := flashKey(sse, flash); err != nil {
		slog.DebugContext(ctx, "ui: configuration flash not delivered", "err", err)
	}
}

// ── reveal ─────────────────────────────────────────────────────────────

// noStore makes the response uncacheable. The stream helper sets
// "Cache-Control: no-cache" itself when it starts a stream, so the header is
// set on the way out, whenever the response begins.
type noStore struct{ http.ResponseWriter }

func (w noStore) stamp() { w.Header().Set("Cache-Control", "no-store") }

func (w noStore) WriteHeader(code int) {
	w.stamp()
	w.ResponseWriter.WriteHeader(code)
}

func (w noStore) Write(b []byte) (int, error) {
	w.stamp()
	return w.ResponseWriter.Write(b)
}

func (w noStore) FlushError() error {
	w.stamp()
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w noStore) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// reveal shows a secret's value once: the service's audited reveal (the
// API's), then a dialog in this response only. The value is in no URL, flash
// or signal, the dialog removes itself when closed, and the response is
// no-store. A native post gets the page itself (post/redirect/get would lose
// the value).
func (h *PlatformConfig) reveal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	w = noStore{w}
	app := r.FormValue("app")
	rv, err := h.Config.Reveal(ctx, ac, app, r.FormValue("section"), r.FormValue("property"), r.FormValue("client"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	dialog := cfgRevealDialogC(cfgRevealed{Section: rv.Config.Section, Property: rv.Config.Property, Value: rv.Value})
	if !ds.IsDatastar(r) {
		view, err := h.listView(ctx, ac, parseListQuery(nil, cfgList), app)
		if err != nil {
			h.Shell.Fail(w, r, err)
			return
		}
		view.Reveal = dialog
		h.renderPage(w, r, http.StatusOK, view)
		return
	}
	if err := patchDialogSlot(ds.NewSSE(w, r), cfgRevealSlot, dialog); err != nil {
		slog.DebugContext(ctx, "ui: reveal dialog not delivered", "err", err)
	}
}

// cfgRevealed is what the reveal dialog shows.
type cfgRevealed struct {
	Section, Property, Value string
}
