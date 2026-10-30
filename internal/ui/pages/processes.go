package pages

import (
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

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/process"
	procops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/process/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
	"github.com/flowcatalyst/go-sdk/usecaseop"
	"github.com/flowcatalyst/go-sdk/usecasepgx"
)

// ProcessStore is the slice of the process repository the page reads
// (GET /bff/processes, GET /bff/processes/{id}).
type ProcessStore interface {
	FindWithFilters(ctx context.Context, application, subdomain, status *string) ([]process.Process, error)
	FindByID(ctx context.Context, id string) (*process.Process, error)
}

// ProcessCommands are the process operations the processes page runs;
// ProcessOps runs the API's use cases.
type ProcessCommands interface {
	Create(ctx context.Context, cmd procops.CreateCommand) (string, error)
	Update(ctx context.Context, cmd procops.UpdateCommand) error
	Archive(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}

// ProcessOps runs the process use cases through the unit of work, exactly as
// internal/platform/process/api does (the SPA calls its /bff alias).
type ProcessOps struct {
	Repo *process.Repository
	UoW  *usecasepgx.UnitOfWork
}

// Create runs CreateProcess and returns the new process's id.
func (o ProcessOps) Create(ctx context.Context, cmd procops.CreateCommand) (string, error) {
	ev, err := usecaseop.Run(ctx, o.UoW, procops.CreateProcess(o.Repo), cmd, auth.NewExecutionContext(ctx))
	if err != nil {
		return "", err
	}
	return ev.ProcessID, nil
}

// Update runs UpdateProcess.
func (o ProcessOps) Update(ctx context.Context, cmd procops.UpdateCommand) error {
	_, err := usecaseop.Run(ctx, o.UoW, procops.UpdateProcess(o.Repo), cmd, auth.NewExecutionContext(ctx))
	return err
}

// Archive runs ArchiveProcess.
func (o ProcessOps) Archive(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, procops.ArchiveProcess(o.Repo), procops.ArchiveCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Delete runs DeleteProcess.
func (o ProcessOps) Delete(ctx context.Context, id string) error {
	_, err := usecaseop.Run(ctx, o.UoW, procops.DeleteProcess(o.Repo), procops.DeleteCommand{ID: id}, auth.NewExecutionContext(ctx))
	return err
}

// Processes is the processes page (/ui/processes), mirroring the SPA's
// ProcessListPage, ProcessDetailDrawer, ProcessCreatePage and
// ProcessEditPage. The drawer draws a Mermaid diagram in the browser
// (kit.Diagram) with its source below; there are no SVG/PNG downloads.
type Processes struct {
	Store ProcessStore
	Ops   ProcessCommands
	Shell *shell.Shell
}

const (
	procsPath   = "/ui/processes"
	procsFormID = "procs-list"

	procDrawerID   = "proc-drawer"
	procDrawerSig  = "selProc"
	procDialogSlot = "proc-dialog-slot"

	procArchiveDialog = "proc-archive"
	procDeleteDialog  = "proc-delete"

	// procSubdomainSlot wraps the subdomain filter, whose options follow
	// the chosen application (patched by a live answer).
	procSubdomainSlot = "procs-subdomain-filter"

	procsLiveGet = "@get('/ui/processes', {contentType: 'form'})"

	procArchivedNoEdit = process.CannotEditReason
)

// The SPA's URL keys: q, app, subdomain, status. Its table is not sortable
// (the API orders by code), so code is the only sort key.
const (
	procAppFacet = "app"
	procSubFacet = "subdomain"
)

var procList = &listSpec{
	path:     procsPath,
	sortKeys: []string{"code"},
	statusOK: func(s string) bool {
		_, ok := process.ParseStatus(s)
		return ok
	},
	facets:      []string{procAppFacet, procSubFacet},
	rows:        []int{25, 50, 100, 250},
	defaultRows: 50,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *Processes) Mount(r chi.Router) {
	r.Get("/processes", h.list)
	r.Get("/processes/new", h.createForm)
	r.Get("/processes/{id}", h.detail)
	r.Get("/processes/{id}/edit", h.editForm)
	post := r.With(parseForm)
	post.Post("/processes/new", h.create)
	post.Post("/processes/{id}/edit", h.update)
	post.Post("/processes/{id}/archive", h.archive)
	post.Post("/processes/{id}/delete", h.delete)
}

// ── list ───────────────────────────────────────────────────────────────

func (h *Processes) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /bff/processes's check.
	if err := auth.CanReadProcesses(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), procList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			view.patchTable(sse),
			sse.Patch(procSubdomainFilter(view)),
			sse.Patch(kit.TableToolbarState(view.toolbar().State())),
			sse.ReplaceURL(view.Query.URL()),
		); err != nil {
			slog.DebugContext(ctx, "ui: processes patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *Processes) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (procsView, error) {
	// The SPA loads every process (both statuses) once and filters in the
	// browser; the volume is low (hand-curated documentation).
	all, err := h.Store.FindWithFilters(ctx, nil, nil, nil)
	if err != nil {
		return procsView{}, fmt.Errorf("find processes: %w", err)
	}
	return buildProcsView(ac, q, all), nil
}

func (h *Processes) renderPage(w http.ResponseWriter, r *http.Request, status int, view procsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Processes", Path: procsPath}, procsPage(view))
}

// procPerms are the API's per-operation checks: create needs
// CanCreateProcesses, edit and archive CanUpdateProcesses, delete
// CanDeleteProcesses.
type procPerms struct {
	Create, Update, Delete bool
}

func procPermsFor(ac *auth.AuthContext) procPerms {
	return procPerms{
		Create: auth.CanCreateProcesses(ac) == nil,
		Update: auth.CanUpdateProcesses(ac) == nil,
		Delete: auth.CanDeleteProcesses(ac) == nil,
	}
}

type procRow struct {
	ID, Code, Name, Description string
	Tags                        []string
	Archived                    bool
	DetailHref, EditHref        string
}

type procsView struct {
	Query       listQuery
	Rows        []procRow
	Total       int
	Page, Pages int
	// AppOptions are the applications of every process; SubOptions the
	// subdomains of the chosen application's processes (all when none).
	AppOptions, SubOptions []kit.Option
	App, Sub               string
	Perms                  procPerms
	Dialog                 templ.Component
	Drawer                 *procDetail
}

func distinctOptions(vals []string) []kit.Option {
	slices.Sort(vals)
	vals = slices.Compact(vals)
	out := make([]kit.Option, 0, len(vals))
	for _, v := range vals {
		if v != "" {
			out = append(out, kit.Option{Value: v, Label: v})
		}
	}
	return out
}

func hasOption(opts []kit.Option, v string) bool {
	return slices.ContainsFunc(opts, func(o kit.Option) bool { return o.Value == v })
}

// buildProcsView applies the SPA's table filters (application, subdomain,
// status and the quick search over code and name) and paging. A chosen
// application or subdomain that is not an option is dropped from the
// canonical URL.
func buildProcsView(ac *auth.AuthContext, q listQuery, all []process.Process) procsView {
	v := procsView{Perms: procPermsFor(ac)}
	apps := make([]string, 0, len(all))
	for _, p := range all {
		apps = append(apps, p.Application)
	}
	v.AppOptions = distinctOptions(apps)
	v.App = firstFacet(q, procAppFacet, func(a string) bool { return hasOption(v.AppOptions, a) })
	subs := make([]string, 0, len(all))
	for _, p := range all {
		if v.App == "" || p.Application == v.App {
			subs = append(subs, p.Subdomain)
		}
	}
	v.SubOptions = distinctOptions(subs)
	v.Sub = firstFacet(q, procSubFacet, func(s string) bool { return hasOption(v.SubOptions, s) })
	q = q.withFacet(procAppFacet, nonEmpty(v.App)).withFacet(procSubFacet, nonEmpty(v.Sub))

	needle := strings.ToLower(q.Q)
	shown := make([]process.Process, 0, len(all))
	for _, p := range all {
		switch {
		case v.App != "" && p.Application != v.App,
			v.Sub != "" && p.Subdomain != v.Sub,
			q.Status != "" && string(p.Status) != q.Status,
			needle != "" && !strings.Contains(strings.ToLower(p.Code+" "+p.Name), needle):
			continue
		}
		shown = append(shown, p)
	}
	// The repository orders by code; the only sort is code, either way.
	slices.SortStableFunc(shown, func(a, b process.Process) int {
		c := strings.Compare(a.Code, b.Code)
		if q.Dir == kit.SortDesc {
			c = -c
		}
		return c
	})
	v.Total = len(shown)
	var lo, hi int
	q.Page, v.Pages, lo, hi = q.pageBounds(len(shown))
	v.Page, v.Query = q.Page, q
	for _, p := range shown[lo:hi] {
		v.Rows = append(v.Rows, procRow{
			ID: p.ID, Code: p.Code, Name: p.Name, Description: deref(p.Description), Tags: p.Tags,
			Archived: p.Status == process.StatusArchived, DetailHref: procURL(p.ID), EditHref: procURL(p.ID) + "/edit",
		})
	}
	return v
}

func procURL(id string) string { return procsPath + "/" + url.PathEscape(id) }

// rowActions mirrors the SPA's row: Edit (here disabled for an archived
// process, as the drawer hides it) and View details.
func (v procsView) rowActions(row procRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "proc-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update {
		edit := kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}}
		if row.Archived {
			edit.DisabledReason = procArchivedNoEdit
		}
		p.Actions = append(p.Actions, edit)
	}
	p.Actions = append(p.Actions, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	return p
}

func (v procsView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(procsTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func procID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Process", id)
	}
	return id, nil
}

// ── drawer ─────────────────────────────────────────────────────────────

type procDetail struct {
	ID, Code, Name, Description string
	Tags                        []string
	Archived                    bool
	Body, DiagramType           string
	EditHref, ActionBase        string
	Perms                       procPerms
}

func (d procDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: procDrawerID, Signal: procDrawerSig, Label: "Process", Size: kit.DrawerWide, Title: d.Name, Subtitle: d.Code, Extras: procStatusTag(d.Archived)}
}

func (d procDetail) canEdit() bool    { return d.Perms.Update && !d.Archived }
func (d procDetail) canArchive() bool { return d.Perms.Update && !d.Archived }
func (d procDetail) canDelete() bool  { return d.Perms.Delete && d.Archived }

// mermaid reports whether the body is in the one syntax the SPA renders.
func (d procDetail) mermaid() bool { return d.DiagramType == "mermaid" }

// loadProcess reads a process with GET /bff/processes/{id}'s check.
func (h *Processes) loadProcess(ctx context.Context, ac *auth.AuthContext, id string) (*process.Process, error) {
	if err := auth.CanReadProcesses(ac); err != nil {
		return nil, err
	}
	p, err := h.Store.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Process", id)
	}
	return p, nil
}

func procDetailOf(ac *auth.AuthContext, p *process.Process) procDetail {
	return procDetail{
		ID: p.ID, Code: p.Code, Name: p.Name, Description: deref(p.Description), Tags: p.Tags,
		Archived: p.Status == process.StatusArchived, Body: p.Body, DiagramType: p.DiagramType,
		EditHref: procURL(p.ID) + "/edit", ActionBase: procURL(p.ID),
		Perms: procPermsFor(ac),
	}
}

func (h *Processes) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := procID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.loadProcess(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := procDetailOf(ac, p)
	if ds.IsDatastar(r) {
		if err := openProcDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: process drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, procList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openProcDrawer(sse *ds.SSE, d procDetail) error {
	return errors.Join(
		sse.Patch(procDrawerContent(d), ds.Inner("#"+procDrawerID)),
		sse.PatchSignals(map[string]any{procDrawerSig: d.ID}),
		sse.Focus(procDrawerID+"-title"),
	)
}

// ── state actions ──────────────────────────────────────────────────────

type procAction struct {
	check   func(*auth.AuthContext) error
	run     func(ProcessCommands, context.Context, string) error
	flash   string
	deleted bool
}

var (
	archiveProcAction = procAction{check: auth.CanUpdateProcesses, run: ProcessCommands.Archive, flash: flashProcArchived}
	deleteProcAction  = procAction{check: auth.CanDeleteProcesses, run: ProcessCommands.Delete, flash: flashProcDeleted, deleted: true}
)

func (h *Processes) archive(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, archiveProcAction)
}

func (h *Processes) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deleteProcAction)
}

func (h *Processes) runAction(w http.ResponseWriter, r *http.Request, a procAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := procID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := a.run(h.Ops, ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, a.flash)
		http.Redirect(w, r, procsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, a.deleted)
	if err := flashKey(sse, a.flash); err != nil {
		slog.DebugContext(ctx, "ui: process flash not delivered", "err", err)
	}
}

// afterChange re-renders the table the user is on (and the subdomain
// options, which a create or delete can change), then refreshes the drawer
// when it shows id, or closes it after a delete.
func (h *Processes) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, procList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := errors.Join(view.patchTable(sse), sse.Patch(procAppFilter(view)), sse.Patch(procSubdomainFilter(view))); err != nil {
		slog.DebugContext(ctx, "ui: processes table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{procDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: process drawer close not delivered", "err", err)
		}
		return
	}
	p, err := h.loadProcess(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(procDrawerContent(procDetailOf(ac, p)), ds.Inner("#"+procDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: process drawer patch not delivered", "err", err)
	}
}

func procConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: procsPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(procDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}

// procRenderNote says why the drawer shows a diagram's source and not a drawing.
func procRenderNote(d procDetail) string {
	return "This process's diagram type is " + d.DiagramType + ", which is not drawn; its source is shown below."
}
