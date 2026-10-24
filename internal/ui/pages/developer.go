package pages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/eventtype"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/openapispecs"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// DeveloperApps is the slice of the application repository the portal reads
// (GET /bff/developer/applications and .../{appId}).
type DeveloperApps interface {
	FindActive(ctx context.Context) ([]application.Application, error)
	FindByID(ctx context.Context, id string) (*application.Application, error)
}

// DeveloperSpecs is the slice of the OpenAPI spec repository the portal reads.
type DeveloperSpecs interface {
	FindCurrentByApplication(ctx context.Context, applicationID string) (*openapispecs.OpenApiSpec, error)
	FindAllByApplication(ctx context.Context, applicationID string) ([]openapispecs.OpenApiSpec, error)
}

// DeveloperEventTypes lists an application's event types
// (GET /bff/developer/applications/{appId}/event-types).
type DeveloperEventTypes interface {
	FindByApplication(ctx context.Context, applicationCode string) ([]eventtype.EventType, error)
}

// Developer is the developer portal (/ui/developer), mirroring the SPA's
// DeveloperApplicationsListPage, DeveloperApplicationDetailPage (API docs,
// event types and processes tabs) and DeveloperApiVersionsPage. It is
// read-only: every handler makes the BFF's check (CanReadDeveloperPortal;
// the processes tab also the process list's CanReadProcesses) and reads the
// repositories the BFF handlers read.
type Developer struct {
	Apps       DeveloperApps
	Specs      DeveloperSpecs
	EventTypes DeveloperEventTypes
	Processes  ProcessStore
	Shell      *shell.Shell
}

const (
	devPath     = "/ui/developer"
	devAppsPath = devPath + "/applications"

	devTabSig  = "devTab"
	devTabDocs = "api-docs"
	devTabET   = "event-types"
	devTabProc = "processes"

	// The panel ids: a tab's content is patched in by id.
	devDocsID = "dev-docs"
	devETID   = "dev-et"
	devProcID = "dev-proc"

	devETFormID   = "dev-et-filters"
	devProcFormID = "dev-proc-filters"
)

var devTabs = []kit.TabItem{
	{Value: devTabDocs, Label: "API Docs"},
	{Value: devTabET, Label: "Event Types"},
	{Value: devTabProc, Label: "Processes"},
}

// Mount registers the portal's routes. There is nothing to post.
func (h *Developer) Mount(r chi.Router) {
	r.Get("/developer", h.list)
	r.Get("/developer/applications/{id}", h.detail)
	r.Get("/developer/applications/{id}/tabs/{tab}", h.tab)
	r.Get("/developer/applications/{id}/operation", h.operation)
	r.Get("/developer/applications/{id}/openapi.json", h.download)
	r.Get("/developer/applications/{id}/versions", h.versions)
}

func devAppURL(id string) string { return devAppsPath + "/" + url.PathEscape(id) }

// devDetailURL is the application page on a tab, with that tab's view state.
func devDetailURL(id, tab string, state url.Values) string {
	v := url.Values{"tab": {tab}}
	maps.Copy(v, state)
	return devAppURL(id) + "?" + v.Encode()
}

func devTabURL(id, tab string) string { return devAppURL(id) + "/tabs/" + tab }

// ── applications list ──────────────────────────────────────────────────

type devAppRow struct {
	ID, Code, Name, Description, Version string
	Platform                             bool
	Synced                               time.Time
	Href                                 string
}

func (h *Developer) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := auth.CanReadDeveloperPortal(auth.FromContext(ctx)); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	apps, err := h.Apps.FindActive(ctx)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "list active applications failed", err))
		return
	}
	rows := make([]devAppRow, 0, len(apps))
	for i := range apps {
		current, err := h.Specs.FindCurrentByApplication(ctx, string(apps[i].ID))
		if err != nil {
			h.Shell.Fail(w, r, usecase.Internal("REPO", "find current spec failed", err))
			return
		}
		row := devAppRow{
			ID: string(apps[i].ID), Code: apps[i].Code, Name: apps[i].Name, Description: deref(apps[i].Description),
			Platform: apps[i].Code == "platform", Href: devAppURL(string(apps[i].ID)),
		}
		if current != nil {
			row.Version, row.Synced = current.Version, current.SyncedAt
		}
		rows = append(rows, row)
	}
	h.Shell.Render(w, r, shell.Page{Title: "Developer Portal", Path: devPath}, devListPage(rows))
}

// ── application page ───────────────────────────────────────────────────

// devApp is the application the portal pages are about.
type devApp struct {
	ID, Code, Name, Description, Version string
}

func (a devApp) href() string { return devAppURL(a.ID) }

// loadApp reads an application as GET /bff/developer/applications/{appId}
// does (the check, then the row and its current spec's version).
func (h *Developer) loadApp(ctx context.Context, ac *auth.AuthContext, id string) (devApp, error) {
	if err := auth.CanReadDeveloperPortal(ac); err != nil {
		return devApp{}, err
	}
	if !kit.ValidID(id) {
		return devApp{}, httperror.NotFound("Application", id)
	}
	app, err := h.Apps.FindByID(ctx, id)
	if err != nil {
		return devApp{}, usecase.Internal("REPO", "find application failed", err)
	}
	if app == nil {
		return devApp{}, httperror.NotFound("Application", id)
	}
	current, err := h.Specs.FindCurrentByApplication(ctx, string(app.ID))
	if err != nil {
		return devApp{}, usecase.Internal("REPO", "find current spec failed", err)
	}
	d := devApp{ID: string(app.ID), Code: app.Code, Name: app.Name, Description: deref(app.Description)}
	if current != nil {
		d.Version = current.Version
	}
	return d, nil
}

type devDetailView struct {
	App   devApp
	Tab   string
	Panel templ.Component
}

func devTabOf(q url.Values) string {
	if t := q.Get("tab"); slices.ContainsFunc(devTabs, func(i kit.TabItem) bool { return i.Value == t }) {
		return t
	}
	return devTabDocs
}

// panel builds one tab's content and the view state it shows (the query
// parameters that rebuild it).
func (h *Developer) panel(ctx context.Context, ac *auth.AuthContext, app devApp, tab string, q url.Values) (templ.Component, url.Values, error) {
	switch tab {
	case devTabET:
		return h.eventTypesPanel(ctx, app, q)
	case devTabProc:
		return h.processesPanel(ctx, ac, app, q)
	}
	c, err := h.docsPanel(ctx, app)
	return c, nil, err
}

func (h *Developer) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	app, err := h.loadApp(ctx, ac, chi.URLParam(r, "id"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := r.URL.Query()
	tab := devTabOf(q)
	panel, _, err := h.panel(ctx, ac, app, tab, q)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: app.Name, Path: devPath}, devDetailPage(devDetailView{App: app, Tab: tab, Panel: panel}))
}

// tab answers a tab's content: the lazy load when the tab is first shown and
// each filter or selection change. A Datastar request patches the panel and
// the address; a native one (the filter form without scripts) goes to the
// page on that tab.
func (h *Developer) tab(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	app, err := h.loadApp(ctx, ac, chi.URLParam(r, "id"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	tab := chi.URLParam(r, "tab")
	if !slices.ContainsFunc(devTabs, func(i kit.TabItem) bool { return i.Value == tab }) {
		h.Shell.Fail(w, r, httperror.NotFound("Tab", tab))
		return
	}
	panel, state, err := h.panel(ctx, ac, app, tab, r.URL.Query())
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		http.Redirect(w, r, devDetailURL(app.ID, tab, state), http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	if err := errors.Join(sse.Patch(panel), sse.ReplaceURL(devDetailURL(app.ID, tab, state))); err != nil {
		slog.DebugContext(ctx, "ui: developer tab patch not delivered", "err", err)
	}
}

// ── event types tab ────────────────────────────────────────────────────

// The tab's view state, in the page's query: the filters, and the selected
// event type. Status defaults to CURRENT; an empty value means every status.
const (
	devETSub    = "etsub"
	devETAgg    = "etagg"
	devETStatus = "etstatus"
	devETSel    = "et"
)

var devStatusOptions = []kit.Option{
	{Value: "CURRENT", Label: "Current"},
	{Value: "ARCHIVED", Label: "Archived"},
}

// devStatusOf reads a status filter: CURRENT when the parameter is absent,
// "" (all) when it is empty or unknown.
func devStatusOf(q url.Values, key string) string {
	if !q.Has(key) {
		return "CURRENT"
	}
	if s := q.Get(key); s == "CURRENT" || s == "ARCHIVED" {
		return s
	}
	return ""
}

// devStatusValues writes a status filter back: the default is not written.
func devStatusValues(v url.Values, key, status string) {
	if status != "CURRENT" {
		v.Set(key, status)
	}
}

type devETRow struct {
	ID, Code, Name, Description, Status string
	Selected                            bool
	Href                                string
}

type devETView struct {
	App                 devApp
	Rows                []devETRow
	SubOptions, AggOpts []kit.Option
	Sub, Agg, Status    string
	Total               int
	// Viewer is the selected event type's schema viewer (nil: none).
	Viewer     *etViewer
	SelectedID string
	// Why is the viewer's empty text.
	Why string
}

func (v devETView) filtered() bool { return v.Sub != "" || v.Agg != "" || v.Status != "CURRENT" }

func (v devETView) state() url.Values {
	s := url.Values{}
	if v.Sub != "" {
		s.Set(devETSub, v.Sub)
	}
	if v.Agg != "" {
		s.Set(devETAgg, v.Agg)
	}
	devStatusValues(s, devETStatus, v.Status)
	if v.SelectedID != "" {
		s.Set(devETSel, v.SelectedID)
	}
	return s
}

func (v devETView) rowURL(id string) string {
	s := v.state()
	s.Set(devETSel, id)
	return devTabURL(v.App.ID, devTabET) + "?" + s.Encode()
}

func (v devETView) clearURL() string { return devTabURL(v.App.ID, devTabET) }

func (h *Developer) eventTypesPanel(ctx context.Context, app devApp, q url.Values) (templ.Component, url.Values, error) {
	ets, err := h.EventTypes.FindByApplication(ctx, app.Code)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "list event types failed", err)
	}
	slices.SortFunc(ets, func(a, b eventtype.EventType) int { return strings.Compare(a.Code, b.Code) })
	v := buildDevETView(app, q, ets)
	return devETPanel(v), v.state(), nil
}

func buildDevETView(app devApp, q url.Values, ets []eventtype.EventType) devETView {
	v := devETView{App: app, Total: len(ets)}
	var subs, aggs []string
	for i := range ets {
		subs, aggs = append(subs, ets[i].Subdomain), append(aggs, ets[i].Aggregate)
	}
	v.SubOptions, v.AggOpts = distinctOptions(subs), distinctOptions(aggs)
	if s := q.Get(devETSub); hasOption(v.SubOptions, s) {
		v.Sub = s
	}
	if a := q.Get(devETAgg); hasOption(v.AggOpts, a) {
		v.Agg = a
	}
	v.Status = devStatusOf(q, devETStatus)

	var shown []*eventtype.EventType
	for i := range ets {
		et := &ets[i]
		if (v.Sub != "" && et.Subdomain != v.Sub) || (v.Agg != "" && et.Aggregate != v.Agg) || (v.Status != "" && string(et.Status) != v.Status) {
			continue
		}
		shown = append(shown, et)
	}
	// The selection is the asked-for event type when it is shown, else the
	// first one shown (the SPA's).
	var sel *eventtype.EventType
	if len(shown) > 0 {
		sel = shown[0]
		if i := slices.IndexFunc(shown, func(et *eventtype.EventType) bool { return et.ID == q.Get(devETSel) }); i >= 0 {
			sel = shown[i]
		}
		v.SelectedID = sel.ID
	}
	for _, et := range shown {
		v.Rows = append(v.Rows, devETRow{
			ID: et.ID, Code: et.Code, Name: et.Name, Description: deref(et.Description), Status: string(et.Status),
			Selected: et.ID == v.SelectedID,
		})
	}
	for i := range v.Rows {
		v.Rows[i].Href = v.rowURL(v.Rows[i].ID)
	}
	switch {
	case sel == nil:
		v.Why = "Select an event type above to see its schema and sample code."
	case len(sel.SpecVersions) == 0:
		v.Why = "No schema published for " + sel.Code + " yet."
	default:
		sv := devSpecVersion(sel)
		viewer := toETViewer(sel, sv)
		v.Viewer = &viewer
	}
	return v
}

// devSpecVersion is the version the viewer opens: CURRENT, then FINALISING,
// then the first.
func devSpecVersion(et *eventtype.EventType) eventtype.SpecVersion {
	for _, want := range []eventtype.SpecVersionStatus{eventtype.SpecCurrent, eventtype.SpecFinalising} {
		if i := slices.IndexFunc(et.SpecVersions, func(sv eventtype.SpecVersion) bool { return sv.Status == want }); i >= 0 {
			return et.SpecVersions[i]
		}
	}
	return et.SpecVersions[0]
}

// ── processes tab ──────────────────────────────────────────────────────

const (
	devProcSub    = "psub"
	devProcStatus = "pstatus"
	devProcSel    = "proc"
)

type devProcRow struct {
	ID, Code, Name, Description string
	Tags                        []string
	Archived, Selected          bool
	Href, EditHref              string
}

type devProcView struct {
	App              devApp
	Rows             []devProcRow
	SubOptions       []kit.Option
	Sub, Status      string
	CanEdit          bool
	Selected         *process.Process
	SelectedID       string
	SelectedOpenHref string
	SelectedEditHref string
	// SelectedCanEdit: the caller may edit, and the process is not archived.
	SelectedCanEdit  bool
	SelectedArchived bool
}

func (v devProcView) filtered() bool { return v.Sub != "" || v.Status != "CURRENT" }

func (v devProcView) state() url.Values {
	s := url.Values{}
	if v.Sub != "" {
		s.Set(devProcSub, v.Sub)
	}
	devStatusValues(s, devProcStatus, v.Status)
	if v.SelectedID != "" {
		s.Set(devProcSel, v.SelectedID)
	}
	return s
}

func (v devProcView) rowURL(id string) string {
	s := v.state()
	s.Set(devProcSel, id)
	return devTabURL(v.App.ID, devTabProc) + "?" + s.Encode()
}

func (v devProcView) clearURL() string { return devTabURL(v.App.ID, devTabProc) }

// processesPanel lists the application's processes the way the SPA does
// (the process list filtered by application code, the rest in the browser).
// It needs the process list's own permission, which the portal's does not
// imply: without it the tab says so.
func (h *Developer) processesPanel(ctx context.Context, ac *auth.AuthContext, app devApp, q url.Values) (templ.Component, url.Values, error) {
	if auth.CanReadProcesses(ac) != nil {
		return devProcDenied(), nil, nil
	}
	procs, err := h.Processes.FindWithFilters(ctx, &app.Code, nil, nil)
	if err != nil {
		return nil, nil, usecase.Internal("REPO", "find_with_filters failed", err)
	}
	v := buildDevProcView(ac, app, q, procs)
	return devProcPanel(v), v.state(), nil
}

func buildDevProcView(ac *auth.AuthContext, app devApp, q url.Values, procs []process.Process) devProcView {
	v := devProcView{App: app, CanEdit: auth.CanUpdateProcesses(ac) == nil}
	subs := make([]string, 0, len(procs))
	for i := range procs {
		subs = append(subs, procs[i].Subdomain)
	}
	v.SubOptions = distinctOptions(subs)
	if s := q.Get(devProcSub); hasOption(v.SubOptions, s) {
		v.Sub = s
	}
	v.Status = devStatusOf(q, devProcStatus)
	for i := range procs {
		p := &procs[i]
		if (v.Sub != "" && p.Subdomain != v.Sub) || (v.Status != "" && string(p.Status) != v.Status) {
			continue
		}
		if p.ID == q.Get(devProcSel) {
			v.Selected, v.SelectedID = p, p.ID
		}
		v.Rows = append(v.Rows, devProcRow{
			ID: p.ID, Code: p.Code, Name: p.Name, Description: deref(p.Description), Tags: p.Tags,
			Archived: p.Status == process.StatusArchived, Selected: p.ID == q.Get(devProcSel),
			EditHref: procURL(p.ID) + "/edit",
		})
	}
	for i := range v.Rows {
		v.Rows[i].Href = v.rowURL(v.Rows[i].ID)
	}
	if v.Selected != nil {
		v.SelectedOpenHref = procURL(v.Selected.ID)
		v.SelectedEditHref = v.SelectedOpenHref + "/edit"
		v.SelectedArchived = v.Selected.Status == process.StatusArchived
		v.SelectedCanEdit = v.CanEdit && !v.SelectedArchived
	}
	return v
}

// ── API docs tab ───────────────────────────────────────────────────────

type devDocsView struct {
	App          devApp
	Doc          *oaDoc
	Version      string
	Synced       time.Time
	DownloadHref string
	// OpenAll opens every tag group (small documents).
	OpenAll bool
}

// devOpenAllUnder is the operation count up to which every tag group starts
// open; a larger document starts with the groups closed.
const devOpenAllUnder = 80

func (h *Developer) docsPanel(ctx context.Context, app devApp) (templ.Component, error) {
	spec, err := h.Specs.FindCurrentByApplication(ctx, app.ID)
	if err != nil {
		return nil, usecase.Internal("REPO", "find current spec failed", err)
	}
	if spec == nil {
		return devDocsMissing(), nil
	}
	doc, err := parseOpenAPI(spec.Spec)
	if err != nil {
		slog.ErrorContext(ctx, "ui: developer portal: OpenAPI document unreadable", "application", app.ID, "err", err)
		return devDocsUnreadable(), nil
	}
	return devDocsPanel(devDocsView{
		App: app, Doc: doc, Version: spec.Version, Synced: spec.SyncedAt,
		DownloadHref: devAppURL(app.ID) + "/openapi.json", OpenAll: doc.Total <= devOpenAllUnder,
	}), nil
}

// opHref is a row's detail address: the inline expansion fetches it, and
// without scripts it is a page of its own.
func (v devDocsView) opHref(op oaOp) string {
	return devAppURL(v.App.ID) + "/operation?" + url.Values{"m": {op.Method}, "p": {op.Path}, "n": {strconv.Itoa(op.N)}}.Encode()
}

// operation answers one operation's detail: patched into its row when asked
// through Datastar, otherwise a page.
func (h *Developer) operation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	app, err := h.loadApp(ctx, ac, chi.URLParam(r, "id"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	spec, err := h.Specs.FindCurrentByApplication(ctx, app.ID)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find current spec failed", err))
		return
	}
	if spec == nil {
		h.Shell.Fail(w, r, httperror.NotFound("OpenApiSpec", app.ID))
		return
	}
	doc, err := parseOpenAPI(spec.Spec)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("OPENAPI", "OpenAPI document unreadable", err))
		return
	}
	q := r.URL.Query()
	op, ok := doc.operation(q.Get("m"), q.Get("p"))
	if !ok {
		h.Shell.Fail(w, r, httperror.NotFound("Operation", q.Get("m")+" "+q.Get("p")))
		return
	}
	if ds.IsDatastar(r) {
		n, err := strconv.Atoi(q.Get("n"))
		if err != nil || n < 1 || n > 1_000_000 {
			h.Shell.Fail(w, r, usecase.Validation("INVALID_ROW", "n must be the operation's row number"))
			return
		}
		if err := ds.NewSSE(w, r).Patch(devOpBody(n, op)); err != nil {
			slog.DebugContext(ctx, "ui: operation patch not delivered", "err", err)
		}
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: op.Method + " " + op.Path, Path: devPath}, devOpPage(app, op))
}

// download sends the application's current OpenAPI document as a file, as
// the SPA's Download OpenAPI does (formatted, named after the title).
func (h *Developer) download(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := h.loadApp(ctx, auth.FromContext(ctx), chi.URLParam(r, "id"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	spec, err := h.Specs.FindCurrentByApplication(ctx, app.ID)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find current spec failed", err))
		return
	}
	if spec == nil {
		h.Shell.Fail(w, r, httperror.NotFound("OpenApiSpec", app.ID))
		return
	}
	var title struct {
		Info struct {
			Title string `json:"title"`
		} `json:"info"`
	}
	_ = json.Unmarshal(spec.Spec, &title)
	var body bytes.Buffer
	if err := json.Indent(&body, spec.Spec, "", "  "); err != nil {
		h.Shell.Fail(w, r, usecase.Internal("OPENAPI", "OpenAPI document unreadable", err))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": downloadName(title.Info.Title, app.ID, spec.Version)}))
	if _, err := w.Write(body.Bytes()); err != nil {
		slog.DebugContext(ctx, "ui: OpenAPI download not delivered", "err", err)
	}
}

// ── API versions ───────────────────────────────────────────────────────

type devVersionRow struct {
	Version, Status, Changes string
	Current, Breaking        bool
	Synced                   time.Time
}

func (h *Developer) versions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	app, err := h.loadApp(ctx, auth.FromContext(ctx), chi.URLParam(r, "id"))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	specs, err := h.Specs.FindAllByApplication(ctx, app.ID)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "list specs failed", err))
		return
	}
	rows := make([]devVersionRow, 0, len(specs))
	for i := range specs {
		s := &specs[i]
		rows = append(rows, devVersionRow{
			Version: s.Version, Status: string(s.Status), Changes: cmpOrString(deref(s.ChangeNotesText), "Initial version"),
			Current: s.Status == openapispecs.StatusCurrent, Breaking: s.ChangeNotes != nil && s.ChangeNotes.HasBreaking, Synced: s.SyncedAt,
		})
	}
	h.Shell.Render(w, r, shell.Page{Title: "API Versions", Path: devPath}, devVersionsPage(app, rows))
}

// subtitle is the application header's subtitle: the code, then the
// description.
func (a devApp) subtitle() string {
	if a.Description == "" {
		return a.Code
	}
	return fmt.Sprintf("%s · %s", a.Code, a.Description)
}

func devUpper(s string) string { return strings.ToUpper(s) }
