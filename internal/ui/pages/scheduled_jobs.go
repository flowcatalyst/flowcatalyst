package pages

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/scheduledjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/schemacodegen"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// ScheduledJobStore is the slice of the scheduled-job repository the page
// reads (the SPA's GET /bff/scheduled-jobs and /{id}).
type ScheduledJobStore interface {
	FindWithFilters(ctx context.Context, f scheduledjob.ListFilters) ([]scheduledjob.ScheduledJob, error)
	CountWithFilters(ctx context.Context, f scheduledjob.ListFilters) (int64, error)
	FindByID(ctx context.Context, id string) (*scheduledjob.ScheduledJob, error)
}

// ScheduledJobInstanceStore is the slice of the instance repository the page
// reads (firings, one firing, its logs, the "running" flag).
type ScheduledJobInstanceStore interface {
	List(ctx context.Context, f scheduledjob.InstanceListFilters) ([]scheduledjob.ScheduledJobInstance, error)
	Count(ctx context.Context, f scheduledjob.InstanceListFilters) (int64, error)
	FindByID(ctx context.Context, id string) (*scheduledjob.ScheduledJobInstance, error)
	HasActiveInstance(ctx context.Context, jobID string, tracksCompletion bool) (bool, error)
	ListLogs(ctx context.Context, instanceID string, limit int64) ([]scheduledjob.ScheduledJobInstanceLog, error)
}

// ScheduledJobs is the scheduled jobs page (/ui/scheduled-jobs), mirroring
// the SPA's ScheduledJobListPage, ScheduledJobCreateDrawer,
// ScheduledJobDetailDrawer, ScheduledJobInstanceListPage and
// ScheduledJobInstanceDetailPage. Reads make the BFF's checks
// (CanReadScheduledJobs, client visibility, platform jobs anchor-only);
// actions check the permission for the operation and run the API's use
// cases (ScheduledJobCommands).
type ScheduledJobs struct {
	Jobs      ScheduledJobStore
	Instances ScheduledJobInstanceStore
	Ops       ScheduledJobCommands
	Clients   ClientStore
	Apps      ApplicationStore
	Shell     *shell.Shell
}

const (
	sjPath   = "/ui/scheduled-jobs"
	sjFormID = "sj-list"

	sjDrawerID   = "sj-drawer"
	sjDrawerSig  = "selSj"
	sjDialogSlot = "sj-dialog-slot"

	sjArchiveDialog = "sj-archive"
	sjDeleteDialog  = "sj-delete"

	sjLiveGet = "@get('/ui/scheduled-jobs', {contentType: 'form'})"

	// Facets: the SPA's URL keys.
	sjFacetClients  = "clients"
	sjFacetApps     = "applications"
	sjFacetStatuses = "statuses"

	// sjPlatform is the client filter's value for platform-scoped jobs.
	sjPlatform = "platform"
)

var sjList = &listSpec{
	path:        sjPath,
	sortKeys:    []string{"code"},
	statusOK:    func(string) bool { return false }, // statuses is a facet
	facets:      []string{sjFacetClients, sjFacetApps, sjFacetStatuses},
	rows:        []int{10, 20, 50, 100},
	defaultRows: 20,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *ScheduledJobs) Mount(r chi.Router) {
	r.Get("/scheduled-jobs", h.list)
	r.Get("/scheduled-jobs/new", h.createForm)
	r.Get("/scheduled-jobs/instances/{instanceId}", h.instancePage)
	r.Get("/scheduled-jobs/{id}", h.detail)
	r.Get("/scheduled-jobs/{id}/edit", h.editForm)
	r.Get("/scheduled-jobs/{id}/instances", h.instancesPage)
	post := r.With(parseForm)
	post.Post("/scheduled-jobs/new", h.create)
	post.Post("/scheduled-jobs/{id}/edit", h.update)
	post.Post("/scheduled-jobs/{id}/pause", h.pause)
	post.Post("/scheduled-jobs/{id}/resume", h.resume)
	post.Post("/scheduled-jobs/{id}/archive", h.archive)
	post.Post("/scheduled-jobs/{id}/delete", h.delete)
	post.Post("/scheduled-jobs/{id}/fire", h.fire)
}

// ── list ───────────────────────────────────────────────────────────────

func (h *ScheduledJobs) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), sjList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		errs := []error{view.patchTable(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())}
		for _, f := range []string{sjFacetClients, sjFacetApps, sjFacetStatuses} {
			errs = append(errs, sse.Patch(kit.MultiSelectSummary(view.facetProps(f))))
		}
		if err := errors.Join(errs...); err != nil {
			slog.DebugContext(ctx, "ui: scheduled jobs patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *ScheduledJobs) renderPage(w http.ResponseWriter, r *http.Request, status int, view sjView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Scheduled Jobs", Path: sjPath}, sjPage(view))
}

// sjPerms: the API's coarse gate for create, update, pause, resume and
// archive is any of create/update/delete (CanWriteScheduledJobs); the page
// checks the permission for the operation. Fire and delete have their own.
type sjPerms struct {
	Create, Update, Fire, Delete bool
}

func sjPermsFor(ac *auth.AuthContext) sjPerms {
	return sjPerms{
		Create: auth.CanCreateScheduledJobs(ac) == nil,
		Update: auth.CanUpdateScheduledJobs(ac) == nil,
		Fire:   auth.CanFireScheduledJobs(ac) == nil,
		Delete: auth.CanDeleteScheduledJobs(ac) == nil,
	}
}

type sjRow struct {
	ID, Code, Name               string
	Client, Application          string
	Crons, Timezone              string
	Status                       string
	StatusVariant                kit.BadgeVariant
	LastFired                    string
	Running                      bool
	DetailHref, EditHref         string
	PlatformScope, NoApplication bool
}

type sjView struct {
	Query       listQuery
	Rows        []sjRow
	Total       int
	Page, Pages int
	Facets      map[string][]kit.Option
	Perms       sjPerms
	Dialog      templ.Component
	Drawer      *sjDetail
}

// names are the client and application names by id (the BFF's lookups).
type sjNames struct{ clients, apps map[string]string }

func (h *ScheduledJobs) names(ctx context.Context) (sjNames, error) {
	n := sjNames{clients: map[string]string{}, apps: map[string]string{}}
	if h.Clients != nil {
		cs, err := h.Clients.FindAll(ctx)
		if err != nil {
			return n, fmt.Errorf("find clients: %w", err)
		}
		for _, c := range cs {
			n.clients[string(c.ID)] = c.Name
		}
	}
	if h.Apps != nil {
		as, err := h.Apps.FindWithFilters(ctx, nil, nil)
		if err != nil {
			return n, fmt.Errorf("find applications: %w", err)
		}
		for _, a := range as {
			n.apps[string(a.ID)] = a.Name
		}
	}
	return n, nil
}

// filterOptions are the BFF's: "Platform-scoped" (anchor only) then the
// active clients the caller can access, the active applications, the three
// statuses; sorted by label.
func (h *ScheduledJobs) filterOptions(ctx context.Context, ac *auth.AuthContext) (map[string][]kit.Option, error) {
	out := map[string][]kit.Option{
		sjFacetStatuses: {{Value: "ACTIVE", Label: "Active"}, {Value: "PAUSED", Label: "Paused"}, {Value: "ARCHIVED", Label: "Archived"}},
	}
	var cls []kit.Option
	if h.Clients != nil {
		cs, err := h.Clients.FindAll(ctx)
		if err != nil {
			return nil, fmt.Errorf("find clients: %w", err)
		}
		for _, c := range cs {
			if c.Status != client.StatusActive || (!ac.IsAnchor() && !ac.CanAccessClient(string(c.ID))) {
				continue
			}
			cls = append(cls, kit.Option{Value: string(c.ID), Label: c.Name})
		}
	}
	slices.SortFunc(cls, func(a, b kit.Option) int { return strings.Compare(a.Label, b.Label) })
	if ac.IsAnchor() {
		cls = append([]kit.Option{{Value: sjPlatform, Label: "Platform-scoped"}}, cls...)
	}
	out[sjFacetClients] = cls
	var apps []kit.Option
	if h.Apps != nil {
		as, err := h.Apps.FindWithFilters(ctx, nil, nil)
		if err != nil {
			return nil, fmt.Errorf("find applications: %w", err)
		}
		for _, a := range as {
			if a.Active {
				apps = append(apps, kit.Option{Value: string(a.ID), Label: a.Name})
			}
		}
	}
	slices.SortFunc(apps, func(a, b kit.Option) int { return strings.Compare(a.Label, b.Label) })
	out[sjFacetApps] = apps
	return out, nil
}

// canViewJob is the BFF's: platform-scoped jobs are anchor-only, a client's
// job needs access to the client.
func canViewJob(ac *auth.AuthContext, j *scheduledjob.ScheduledJob) bool {
	if j.ClientID == nil {
		return ac.IsAnchor()
	}
	return ac.CanAccessClient(string(*j.ClientID))
}

func (h *ScheduledJobs) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (sjView, error) {
	v := sjView{Perms: sjPermsFor(ac)}
	opts, err := h.filterOptions(ctx, ac)
	if err != nil {
		return sjView{}, err
	}
	v.Facets = opts
	for _, f := range []string{sjFacetClients, sjFacetApps, sjFacetStatuses} {
		q = q.withFacet(f, prune(q.facet(f), opts[f]))
	}
	// The BFF's list: filters in SQL, non-anchor callers confined to their
	// clients (so the count and the page agree with what they may see).
	filters := scheduledjob.ListFilters{ClientIDs: q.facet(sjFacetClients), ApplicationIDs: q.facet(sjFacetApps), Statuses: q.facet(sjFacetStatuses)}
	if q.Q != "" {
		search := q.Q
		filters.Search = &search
	}
	if !ac.IsAnchor() {
		allowed := ac.Clients
		if len(filters.ClientIDs) > 0 {
			allowed = slices.DeleteFunc(slices.Clone(filters.ClientIDs), func(id string) bool { return !ac.CanAccessClient(id) })
		}
		if len(allowed) == 0 {
			v.Query, v.Page, v.Pages = q, 1, 1
			return v, nil
		}
		filters.ClientIDs = allowed
	}
	total, err := h.Jobs.CountWithFilters(ctx, filters)
	if err != nil {
		return sjView{}, usecase.Internal("REPO", "count scheduled jobs failed", err)
	}
	v.Total = int(total)
	var lo int
	q.Page, v.Pages, lo, _ = q.pageBounds(v.Total)
	limit, offset := int64(q.Rows), int64(lo)
	filters.Limit, filters.Offset = &limit, &offset
	rows, err := h.Jobs.FindWithFilters(ctx, filters)
	if err != nil {
		return sjView{}, usecase.Internal("REPO", "list scheduled jobs failed", err)
	}
	names, err := h.names(ctx)
	if err != nil {
		return sjView{}, err
	}
	for i := range rows {
		j := &rows[i]
		if !canViewJob(ac, j) {
			continue
		}
		running, err := h.Instances.HasActiveInstance(ctx, j.ID, j.TracksCompletion)
		if err != nil {
			return sjView{}, usecase.Internal("REPO", "has_active_instance failed", err)
		}
		v.Rows = append(v.Rows, toSjRow(j, names, running))
	}
	v.Page, v.Query = q.Page, q
	return v, nil
}

func sjURL(id string) string { return sjPath + "/" + url.PathEscape(id) }

// sjStatusVariant is the SPA's statusSeverity.
func sjStatusVariant(s scheduledjob.Status) kit.BadgeVariant {
	switch s {
	case scheduledjob.StatusActive:
		return kit.BadgeSuccess
	case scheduledjob.StatusPaused:
		return kit.BadgeWarn
	case scheduledjob.StatusArchived:
		return kit.BadgeSecondary
	}
	return kit.BadgeInfo
}

// formatCrons is the SPA's: the first expression, then (+n).
func formatCrons(crons []string) string {
	switch len(crons) {
	case 0:
		return "—"
	case 1:
		return crons[0]
	}
	return crons[0] + " (+" + strconv.Itoa(len(crons)-1) + ")"
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return "—"
	}
	return t.UTC().Format("2006-01-02 15:04:05 UTC")
}

func toSjRow(j *scheduledjob.ScheduledJob, n sjNames, running bool) sjRow {
	row := sjRow{
		ID: j.ID, Code: j.Code, Name: j.Name, Crons: formatCrons(j.Crons), Timezone: j.Timezone,
		Status: string(j.Status), StatusVariant: sjStatusVariant(j.Status), LastFired: fmtTime(j.LastFiredAt),
		Running: running, DetailHref: sjURL(j.ID), EditHref: sjURL(j.ID) + "/edit",
	}
	if j.ClientID == nil {
		row.Client, row.PlatformScope = "Platform", true
	} else {
		row.Client = cmp.Or(n.clients[string(*j.ClientID)], string(*j.ClientID))
	}
	if j.ApplicationID == nil || n.apps[string(*j.ApplicationID)] == "" {
		row.Application, row.NoApplication = "—", true
	} else {
		row.Application = n.apps[string(*j.ApplicationID)]
	}
	return row
}

func (v sjView) rowActions(row sjRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "sj-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update && row.Status != "ARCHIVED" {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Actions = append(p.Actions, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	return p
}

func (v sjView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(sjTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func sjID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("ScheduledJob", id)
	}
	return id, nil
}

// ── drawer ─────────────────────────────────────────────────────────────

type sjInstanceRow struct {
	ID, FiredAt, ScheduledFor, Trigger string
	Status                             string
	StatusVariant                      kit.BadgeVariant
	Attempts, Delivered, Completed     string
	Error, Href                        string
}

type sjDetail struct {
	ID, Code, Name, Description string
	Scope, Application          string
	Timezone, LastFired         string
	Created, Updated            string
	MaxAttempts, Timeout        string
	Concurrent, Tracks          string
	TargetURL                   string
	Crons                       []string
	Payload                     string
	Status                      string
	RawStatus                   scheduledjob.Status
	StatusVariant               kit.BadgeVariant
	Running                     bool
	Recent                      []sjInstanceRow
	EditHref, ActionBase        string
	InstancesHref               string
	Perms                       sjPerms
}

func (d sjDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: sjDrawerID, Signal: sjDrawerSig, Label: "Scheduled job", Size: kit.DrawerWide, Title: d.Name, Subtitle: d.Code, Extras: sjTags(d.Status, d.StatusVariant, d.Running)}
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

// prettyJSON indents a stored JSON document (the SPA's JSON.stringify(x,
// null, 2): key order kept, < > & not escaped); a value that is not JSON is
// shown as stored.
func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if p, ok := schemacodegen.PrettyJSON(string(raw)); ok {
		return p
	}
	return string(raw)
}

// instanceVariant is the SPA's instanceStatusSeverity.
func instanceVariant(s scheduledjob.InstanceStatus) kit.BadgeVariant {
	switch s {
	case scheduledjob.InstanceStatusDelivered, scheduledjob.InstanceStatusCompleted:
		return kit.BadgeSuccess
	case scheduledjob.InstanceStatusQueued, scheduledjob.InstanceStatusInFlight:
		return kit.BadgeInfo
	case scheduledjob.InstanceStatusFailed, scheduledjob.InstanceStatusDeliveryFailed:
		return kit.BadgeDestructive
	}
	return kit.BadgeSecondary
}

func instanceURL(id string) string { return sjPath + "/instances/" + url.PathEscape(id) }

func toInstanceRow(in scheduledjob.ScheduledJobInstance) sjInstanceRow {
	return sjInstanceRow{
		ID: in.ID, FiredAt: fmtTime(&in.FiredAt), ScheduledFor: fmtTime(in.ScheduledFor), Trigger: string(in.TriggerKind),
		Status: string(in.Status), StatusVariant: instanceVariant(in.Status), Attempts: strconv.Itoa(int(in.DeliveryAttempts)),
		Delivered: fmtTime(in.DeliveredAt), Completed: fmtTime(in.CompletedAt), Error: deref(in.DeliveryError), Href: instanceURL(in.ID),
	}
}

// loadJob reads a job with the BFF's by-id rule (not visible = not found).
func (h *ScheduledJobs) loadJob(ctx context.Context, ac *auth.AuthContext, id string) (*scheduledjob.ScheduledJob, error) {
	j, err := h.Jobs.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find scheduled job failed", err)
	}
	if j == nil || !canViewJob(ac, j) {
		return nil, httperror.NotFound("ScheduledJob", id)
	}
	return j, nil
}

func (h *ScheduledJobs) detailOf(ctx context.Context, ac *auth.AuthContext, j *scheduledjob.ScheduledJob) (sjDetail, error) {
	names, err := h.names(ctx)
	if err != nil {
		return sjDetail{}, err
	}
	running, err := h.Instances.HasActiveInstance(ctx, j.ID, j.TracksCompletion)
	if err != nil {
		return sjDetail{}, usecase.Internal("REPO", "has_active_instance failed", err)
	}
	row := toSjRow(j, names, running)
	d := sjDetail{
		ID: j.ID, Code: j.Code, Name: j.Name, Description: deref(j.Description),
		Scope: row.Client, Application: row.Application, Timezone: j.Timezone, LastFired: row.LastFired,
		Created: fmtTime(&j.CreatedAt), Updated: fmtTime(&j.UpdatedAt),
		MaxAttempts: strconv.Itoa(int(j.DeliveryMaxAttempts)), Concurrent: yesNo(j.Concurrent), Tracks: yesNo(j.TracksCompletion),
		TargetURL: deref(j.TargetURL), Crons: j.Crons, Payload: prettyJSON(j.Payload),
		Status: string(j.Status), RawStatus: j.Status, StatusVariant: sjStatusVariant(j.Status), Running: running,
		EditHref: sjURL(j.ID) + "/edit", ActionBase: sjURL(j.ID), InstancesHref: sjURL(j.ID) + "/instances", Perms: sjPermsFor(ac),
	}
	if j.TimeoutSeconds != nil {
		d.Timeout = strconv.Itoa(int(*j.TimeoutSeconds))
	}
	// The SPA's ten most recent firings.
	limit, offset := int64(10), int64(0)
	recent, err := h.Instances.List(ctx, scheduledjob.InstanceListFilters{ScheduledJobID: &j.ID, Limit: &limit, Offset: &offset})
	if err != nil {
		return sjDetail{}, usecase.Internal("REPO", "list instances failed", err)
	}
	for _, in := range recent {
		d.Recent = append(d.Recent, toInstanceRow(in))
	}
	return d, nil
}

func (h *ScheduledJobs) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := sjID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, j)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := openSjDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: scheduled job drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, sjList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openSjDrawer(sse *ds.SSE, d sjDetail) error {
	return errors.Join(
		sse.Patch(sjDrawerContent(d), ds.Inner("#"+sjDrawerID)),
		sse.PatchSignals(map[string]any{sjDrawerSig: d.ID}),
		sse.Focus(sjDrawerID+"-title"),
	)
}

// ── actions ────────────────────────────────────────────────────────────

type sjAction struct {
	check   func(*auth.AuthContext) error
	run     func(ScheduledJobCommands, context.Context, string) error
	flash   string
	removed bool
}

var (
	pauseSjAction   = sjAction{check: auth.CanUpdateScheduledJobs, run: ScheduledJobCommands.Pause, flash: flashSjPaused}
	resumeSjAction  = sjAction{check: auth.CanUpdateScheduledJobs, run: ScheduledJobCommands.Resume, flash: flashSjResumed}
	archiveSjAction = sjAction{check: auth.CanUpdateScheduledJobs, run: ScheduledJobCommands.Archive, flash: flashSjArchived, removed: true}
	deleteSjAction  = sjAction{check: auth.CanDeleteScheduledJobs, run: ScheduledJobCommands.Delete, flash: flashSjDeleted, removed: true}
)

func (h *ScheduledJobs) pause(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, pauseSjAction)
}

func (h *ScheduledJobs) resume(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, resumeSjAction)
}

func (h *ScheduledJobs) archive(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, archiveSjAction)
}

func (h *ScheduledJobs) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deleteSjAction)
}

// runAction: archive and delete close the drawer, as the SPA does.
func (h *ScheduledJobs) runAction(w http.ResponseWriter, r *http.Request, a sjAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := sjID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := a.run(h.Ops, ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, a.removed, FlashMessages[a.flash], a.flash)
}

// fire is the SPA's Fire Now: a flash naming the new instance.
func (h *ScheduledJobs) fire(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanFireScheduledJobs(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := sjID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	instanceID, err := h.Ops.Fire(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	msg := kit.FlashMessageProps{Kind: kit.FlashSuccess, Summary: "Fired", Detail: "Instance " + instanceID + " created"}
	h.done(w, r, ac, id, false, msg, flashSjFired)
}

func (h *ScheduledJobs) done(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id string, removed bool, msg kit.FlashMessageProps, key string) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, key)
		http.Redirect(w, r, sjPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, removed)
	if err := shell.Flash(sse, msg); err != nil {
		slog.DebugContext(ctx, "ui: scheduled job flash not delivered", "err", err)
	}
}

func (h *ScheduledJobs) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, removed bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, sjList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: scheduled jobs table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if removed {
		if err := sse.PatchSignals(map[string]any{sjDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: scheduled job drawer close not delivered", "err", err)
		}
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	d, err := h.detailOf(ctx, ac, j)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(sjDrawerContent(d), ds.Inner("#"+sjDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: scheduled job drawer patch not delivered", "err", err)
	}
}

func sjConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: sjPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(sjDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}

// ── firings ────────────────────────────────────────────────────────────

var instanceStatusOptions = []kit.Option{
	{Value: "QUEUED", Label: "Queued"},
	{Value: "IN_FLIGHT", Label: "In flight"},
	{Value: "DELIVERED", Label: "Delivered"},
	{Value: "COMPLETED", Label: "Completed"},
	{Value: "FAILED", Label: "Failed"},
	{Value: "DELIVERY_FAILED", Label: "Delivery failed"},
}

var triggerOptions = []kit.Option{{Value: "CRON", Label: "Cron"}, {Value: "MANUAL", Label: "Manual"}, {Value: "BACKFILL", Label: "Backfill"}}

type sjInstancesView struct {
	JobID, JobCode, JobName string
	Query                   listQuery
	Status, Trigger         string
	Rows                    []sjInstanceRow
	Total, Page, Pages      int
}

// instancesList is the firings list's URL state: the SPA's status and
// trigger selects (held as facets), 50 rows by default.
func instancesList(jobID string) *listSpec {
	return &listSpec{
		path: sjURL(jobID) + "/instances", sortKeys: []string{"fired"}, statusOK: func(string) bool { return false },
		facets: []string{"status", "trigger"}, rows: []int{20, 50, 100, 200}, defaultRows: 50,
	}
}

func firstFacet(q listQuery, name string, ok func(string) bool) string {
	if v := q.facet(name); len(v) > 0 && ok(v[0]) {
		return v[0]
	}
	return ""
}

func (h *ScheduledJobs) instancesPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := sjID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	j, err := h.loadJob(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parseListQuery(r.URL.Query(), instancesList(id))
	v := sjInstancesView{JobID: j.ID, JobCode: j.Code, JobName: j.Name}
	v.Status = firstFacet(q, "status", func(s string) bool { _, ok := scheduledjob.ParseInstanceStatus(s); return ok })
	v.Trigger = firstFacet(q, "trigger", func(s string) bool { _, ok := scheduledjob.ParseTriggerKind(s); return ok })
	q = q.withFacet("status", nonEmpty(v.Status)).withFacet("trigger", nonEmpty(v.Trigger))
	f := scheduledjob.InstanceListFilters{ScheduledJobID: &j.ID}
	if v.Status != "" {
		st := scheduledjob.InstanceStatus(v.Status)
		f.Status = &st
	}
	if v.Trigger != "" {
		tk := scheduledjob.TriggerKind(v.Trigger)
		f.TriggerKind = &tk
	}
	total, err := h.Instances.Count(ctx, f)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "count instances failed", err))
		return
	}
	v.Total = int(total)
	var lo int
	q.Page, v.Pages, lo, _ = q.pageBounds(v.Total)
	limit, offset := int64(q.Rows), int64(lo)
	f.Limit, f.Offset = &limit, &offset
	rows, err := h.Instances.List(ctx, f)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "list instances failed", err))
		return
	}
	for _, in := range rows {
		v.Rows = append(v.Rows, toInstanceRow(in))
	}
	v.Page, v.Query = q.Page, q
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(sse.Patch(sjInstancesTable(v)), sse.Patch(kit.Paginator(v.pager())), sse.Patch(sjInstancesClear(v)), sse.ReplaceURL(q.URL())); err != nil {
			slog.DebugContext(ctx, "ui: firings patch not delivered", "err", err)
		}
		return
	}
	h.Shell.Render(w, r, shell.Page{Title: "Firings · " + j.Name, Path: sjPath}, sjInstancesPage(v))
}

func nonEmpty(v string) []string {
	if v == "" {
		return nil
	}
	return []string{v}
}

type sjLogRow struct {
	Time, Level, Message, Metadata string
	LevelVariant                   kit.BadgeVariant
}

type sjInstanceView struct {
	In                                          sjInstanceRow
	JobID, JobCode, Correlation, CompletionText string
	CompletionStatus                            string
	CompletionOK                                bool
	Logs                                        []sjLogRow
}

// logVariant is the SPA's logLevelSeverity.
func logVariant(level string) kit.BadgeVariant {
	switch level {
	case "ERROR":
		return kit.BadgeDestructive
	case "WARN":
		return kit.BadgeWarn
	case "DEBUG":
		return kit.BadgeSecondary
	}
	return kit.BadgeInfo
}

func (h *ScheduledJobs) instancePage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadScheduledJobs(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id := chi.URLParam(r, "instanceId")
	if !kit.ValidID(id) {
		h.Shell.Fail(w, r, httperror.NotFound("ScheduledJobInstance", id))
		return
	}
	in, err := h.Instances.FindByID(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "find instance failed", err))
		return
	}
	// The BFF's canViewInstance: platform instances anchor-only.
	if in == nil || (in.ClientID == nil && !ac.IsAnchor()) || (in.ClientID != nil && !ac.CanAccessClient(*in.ClientID)) {
		h.Shell.Fail(w, r, httperror.NotFound("ScheduledJobInstance", id))
		return
	}
	logs, err := h.Instances.ListLogs(ctx, id, 0)
	if err != nil {
		h.Shell.Fail(w, r, usecase.Internal("REPO", "list instance logs failed", err))
		return
	}
	v := sjInstanceView{
		In: toInstanceRow(*in), JobID: in.ScheduledJobID, JobCode: in.JobCode, Correlation: cmp.Or(deref(in.CorrelationID), "—"),
		CompletionText: prettyJSON(in.CompletionResult), CompletionStatus: deref(in.CompletionStatus), CompletionOK: deref(in.CompletionStatus) == "SUCCESS",
	}
	for _, l := range logs {
		meta := ""
		if len(l.Metadata) > 0 && string(l.Metadata) != "null" {
			meta = string(l.Metadata)
		}
		v.Logs = append(v.Logs, sjLogRow{Time: fmtTime(&l.CreatedAt), Level: l.Level, LevelVariant: logVariant(l.Level), Message: l.Message, Metadata: meta})
	}
	h.Shell.Render(w, r, shell.Page{Title: "Firing " + in.ID, Path: sjPath}, sjInstancePage(v))
}
