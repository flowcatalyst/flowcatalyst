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
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/subscription"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// SubscriptionStore is the slice of the subscription repository the page
// reads (GET /api/subscriptions and GET /api/subscriptions/{id}).
type SubscriptionStore interface {
	FindWithFilters(ctx context.Context, status, clientID *string) ([]subscription.Subscription, error)
	FindByID(ctx context.Context, id string) (*subscription.Subscription, error)
}

// Subscriptions is the subscriptions page (/ui/subscriptions), mirroring the
// SPA's SubscriptionListPage, SubscriptionCreateDrawer and
// SubscriptionDetailDrawer. The create form's choices are read with the
// checks of the calls the SPA makes for them (active dispatch pools, active
// connections, current event types, the client search).
type Subscriptions struct {
	Subs        SubscriptionStore
	Ops         SubscriptionCommands
	Pools       PoolStore
	Connections ConnectionStore
	EventTypes  EventTypeStore
	Apps        EventTypeApps
	Clients     ClientDirectory
	Shell       *shell.Shell
}

const (
	subsPath   = "/ui/subscriptions"
	subsFormID = "subs-list"

	subDrawerID   = "sub-drawer"
	subDrawerSig  = "selSub"
	subDialogSlot = "sub-dialog-slot"

	subPauseDialog  = "sub-pause"
	subResumeDialog = "sub-resume"
	subDeleteDialog = "sub-delete"

	subsLiveGet  = "@get('/ui/subscriptions', {contentType: 'form'})"
	subFacetApp  = "app"
	subAppFilter = "subs-filter-app"
)

var subList = &listSpec{
	path:        subsPath,
	sortKeys:    []string{"code", "application", "name", "pool", "status", "created"},
	statusOK:    func(s string) bool { _, ok := subscription.ParseStatus(s); return ok },
	facets:      []string{subFacetApp},
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *Subscriptions) Mount(r chi.Router) {
	r.Get("/subscriptions", h.list)
	r.Get("/subscriptions/new", h.createForm)
	r.Get("/subscriptions/event-type-options", h.eventTypeOptions)
	r.Get("/subscriptions/{id}", h.detail)
	r.Get("/subscriptions/{id}/edit", h.editForm)
	post := r.With(parseForm)
	post.Post("/subscriptions/new", h.create)
	post.Post("/subscriptions/{id}/edit", h.update)
	post.Post("/subscriptions/{id}/pause", h.pause)
	post.Post("/subscriptions/{id}/resume", h.resume)
	post.Post("/subscriptions/{id}/delete", h.delete)
}

// ── list ───────────────────────────────────────────────────────────────

func (h *Subscriptions) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/subscriptions's check.
	if err := auth.CanReadSubscriptions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), subList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			view.patchTable(sse),
			sse.Patch(kit.TableToolbarState(view.toolbar().State())),
			sse.Patch(kit.MultiSelectSummary(view.appFacet())),
			sse.ReplaceURL(view.Query.URL()),
		); err != nil {
			slog.DebugContext(ctx, "ui: subscriptions patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (v subsView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(subsTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func (h *Subscriptions) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (subsView, error) {
	// The SPA loads every subscription (no filter) and filters in the page.
	subs, err := h.Subs.FindWithFilters(ctx, nil, nil)
	if err != nil {
		return subsView{}, fmt.Errorf("find subscriptions: %w", err)
	}
	visible := auth.FilterClientScoped(ac, subs, func(s *subscription.Subscription) *string { return ids.StringPtr(s.ClientID) })
	if err := h.withPoolCodes(ctx, visible); err != nil {
		return subsView{}, err
	}
	return buildSubsView(ac, q, visible), nil
}

// withPoolCodes fills the pool code of subscriptions that reference a pool
// by id only. The stored code is set by the SDK sync alone; a subscription
// created in the UI or through the API has the id and no code (recorded in
// docs/ui/README.md), and both lists would show an empty Pool column.
func (h *Subscriptions) withPoolCodes(ctx context.Context, subs []subscription.Subscription) error {
	if h.Pools == nil || !slices.ContainsFunc(subs, needsPoolCode) {
		return nil
	}
	pools, err := h.Pools.FindWithFilters(ctx, nil, nil)
	if err != nil {
		return fmt.Errorf("find dispatch pools: %w", err)
	}
	codes := make(map[string]string, len(pools))
	for _, p := range pools {
		codes[p.ID] = p.Code
	}
	for i := range subs {
		if needsPoolCode(subs[i]) {
			if c, ok := codes[*subs[i].DispatchPoolID]; ok {
				subs[i].DispatchPoolCode = &c
			}
		}
	}
	return nil
}

func needsPoolCode(s subscription.Subscription) bool {
	return s.DispatchPoolCode == nil && s.DispatchPoolID != nil
}

func (h *Subscriptions) renderPage(w http.ResponseWriter, r *http.Request, status int, view subsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Subscriptions", Path: subsPath}, subsPage(view))
}

// subPerms are the actions the user may run. The API's coarse gate is any of
// create/update/delete (CanWriteSubscriptions); the page checks the
// permission for the operation: create CanCreateSubscriptions; edit, pause
// and resume CanUpdateSubscriptions; delete CanDeleteSubscriptions.
type subPerms struct {
	Create, Update, Delete bool
}

func subPermsFor(ac *auth.AuthContext) subPerms {
	return subPerms{
		Create: auth.CanCreateSubscriptions(ac) == nil,
		Update: auth.CanUpdateSubscriptions(ac) == nil,
		Delete: auth.CanDeleteSubscriptions(ac) == nil,
	}
}

type subRow struct {
	ID, Code, Application, Name, Scope string
	EventTypes, Pool, Mode             string
	Status                             string
	RawStatus                          subscription.Status
	StatusVariant                      kit.BadgeVariant
	Created, CreatedISO                string
	DetailHref, EditHref               string
}

type subsView struct {
	Query       listQuery
	Rows        []subRow
	Total       int
	Page, Pages int
	Apps        []kit.Option
	Perms       subPerms
	Dialog      templ.Component
	Drawer      *subDetail
}

func subApp(s subscription.Subscription) string      { return deref(s.ApplicationCode) }
func subPoolCode(s subscription.Subscription) string { return deref(s.DispatchPoolCode) }

// buildSubsView applies the SPA's table filters (application, status, the
// quick search over code, name, connection id, application and client) and
// sort, then pages (100 rows by default).
func buildSubsView(ac *auth.AuthContext, q listQuery, subs []subscription.Subscription) subsView {
	v := subsView{Perms: subPermsFor(ac)}
	var codes []string
	for _, s := range subs {
		if c := subApp(s); c != "" && !slices.Contains(codes, c) {
			codes = append(codes, c)
		}
	}
	slices.Sort(codes)
	for _, c := range codes {
		v.Apps = append(v.Apps, kit.Option{Value: c, Label: c})
	}
	q = q.withFacet(subFacetApp, prune(q.facet(subFacetApp), v.Apps))
	apps := q.facet(subFacetApp)

	needle := strings.ToLower(q.Q)
	shown := make([]subscription.Subscription, 0, len(subs))
	for _, s := range subs {
		if q.Status != "" && string(s.Status) != q.Status {
			continue
		}
		if len(apps) > 0 && !slices.Contains(apps, subApp(s)) {
			continue
		}
		hay := strings.ToLower(strings.Join([]string{s.Code, s.Name, deref(s.ConnectionID), subApp(s), deref(s.ClientIdentifier)}, " "))
		if needle != "" && !strings.Contains(hay, needle) {
			continue
		}
		shown = append(shown, s)
	}
	slices.SortStableFunc(shown, func(a, b subscription.Subscription) int {
		var c int
		switch q.Sort {
		case "application":
			c = cmp.Compare(subApp(a), subApp(b))
		case "name":
			c = cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		case "pool":
			c = cmp.Compare(subPoolCode(a), subPoolCode(b))
		case "status":
			c = cmp.Compare(a.Status, b.Status)
		case "created":
			c = a.CreatedAt.Compare(b.CreatedAt)
		default:
			c = cmp.Compare(a.Code, b.Code)
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
	for _, s := range shown[lo:hi] {
		v.Rows = append(v.Rows, toSubRow(s))
	}
	return v
}

func subURL(id string) string { return subsPath + "/" + url.PathEscape(id) }

// subScope is the SPA's getScopeLabel.
func subScope(s subscription.Subscription, anchor string) string {
	if s.ClientIdentifier != nil && *s.ClientIdentifier != "" {
		return *s.ClientIdentifier
	}
	return anchor
}

// modeLabel is the SPA's getModeLabel.
func modeLabel(m common.DispatchMode) string {
	switch m {
	case common.DispatchImmediate:
		return "Immediate"
	case common.DispatchNextOnError:
		return "Next on Error"
	case common.DispatchBlockOnError:
		return "Block on Error"
	}
	return string(m)
}

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return strconv.Itoa(n) + " " + one + "s"
}

func toSubRow(s subscription.Subscription) subRow {
	return subRow{
		ID: s.ID, Code: s.Code, Application: subApp(s), Name: s.Name, Scope: subScope(s, "Anchor-level"),
		EventTypes: plural(len(s.EventTypes), "event type"), Pool: subPoolCode(s), Mode: modeLabel(s.Mode),
		Status: string(s.Status), RawStatus: s.Status, StatusVariant: subStatusVariant(s.Status),
		Created: s.CreatedAt.UTC().Format("2006-01-02"), CreatedISO: s.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		DetailHref: subURL(s.ID), EditHref: subURL(s.ID) + "/edit",
	}
}

// subStatusVariant is the SPA's getStatusSeverity.
func subStatusVariant(s subscription.Status) kit.BadgeVariant {
	switch s {
	case subscription.StatusActive:
		return kit.BadgeSuccess
	case subscription.StatusPaused:
		return kit.BadgeWarn
	}
	return kit.BadgeSecondary
}

// rowActions: the SPA's row pencil (Edit) and row click (View details).
func (v subsView) rowActions(row subRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "sub-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Actions = append(p.Actions, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	return p
}

func subID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Subscription", id)
	}
	return id, nil
}

// ── drawer ─────────────────────────────────────────────────────────────

type subBinding struct{ Code, Version string }

type subDetail struct {
	ID, Code, Name, Description string
	Scope, Source, Endpoint     string
	Connection, Queue, Pool     string
	Mode                        string
	MaxAge, Delay, Timeout      string
	Sequence                    string
	Status                      string
	RawStatus                   subscription.Status
	StatusVariant               kit.BadgeVariant
	Created, Updated            string
	Bindings                    []subBinding
	EditHref, ActionBase        string
	Perms                       subPerms
}

func seconds(n int32) string { return strconv.Itoa(int(n)) + " seconds" }

func toSubDetail(s *subscription.Subscription, perms subPerms) subDetail {
	d := subDetail{
		ID: s.ID, Code: s.Code, Name: s.Name, Description: deref(s.Description),
		Scope: subScope(*s, "Anchor-level (no client)"), Source: string(s.Source), Endpoint: s.Endpoint,
		Connection: deref(s.ConnectionID), Queue: deref(s.Queue), Pool: subPoolCode(*s), Mode: modeLabel(s.Mode),
		MaxAge: seconds(s.MaxAgeSeconds), Delay: seconds(s.DelaySeconds), Timeout: seconds(s.TimeoutSeconds),
		Sequence: strconv.Itoa(int(s.Sequence)),
		Status:   string(s.Status), RawStatus: s.Status, StatusVariant: subStatusVariant(s.Status),
		Created: s.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), Updated: s.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		EditHref: subURL(s.ID) + "/edit", ActionBase: subURL(s.ID), Perms: perms,
	}
	for _, b := range s.EventTypes {
		d.Bindings = append(d.Bindings, subBinding{Code: b.EventTypeCode, Version: deref(b.SpecVersion)})
	}
	return d
}

func (d subDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: subDrawerID, Signal: subDrawerSig, Label: "Subscription", Size: kit.DrawerWide, Title: d.Name, Subtitle: d.Code, Extras: poolStatusBadge(d.Status, d.StatusVariant)}
}

func (d subDetail) showActions() bool { return d.Perms.Update || d.Perms.Delete }

// loadSubscription reads one subscription with GET /api/subscriptions/{id}'s
// checks (the read permission is the caller's).
func (h *Subscriptions) loadSubscription(ctx context.Context, ac *auth.AuthContext, id string) (*subscription.Subscription, error) {
	s, err := h.Subs.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if s == nil {
		return nil, httperror.NotFound("Subscription", id)
	}
	if s.ClientID != nil && !ac.CanAccessClient(string(*s.ClientID)) {
		return nil, httperror.Forbidden("No access to this subscription")
	}
	one := []subscription.Subscription{*s}
	if err := h.withPoolCodes(ctx, one); err != nil {
		return nil, err
	}
	return &one[0], nil
}

func (h *Subscriptions) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadSubscriptions(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := subID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	s, err := h.loadSubscription(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := toSubDetail(s, subPermsFor(ac))
	if ds.IsDatastar(r) {
		if err := openSubDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: subscription drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, subList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openSubDrawer(sse *ds.SSE, d subDetail) error {
	return errors.Join(
		sse.Patch(subDrawerContent(d), ds.Inner("#"+subDrawerID)),
		sse.PatchSignals(map[string]any{subDrawerSig: d.ID}),
		sse.Focus(subDrawerID+"-title"),
	)
}

// ── actions ────────────────────────────────────────────────────────────

type subAction struct {
	check   func(*auth.AuthContext) error
	run     func(SubscriptionCommands, context.Context, string) error
	flash   string
	deleted bool
}

var (
	pauseSubAction  = subAction{check: auth.CanUpdateSubscriptions, run: SubscriptionCommands.Pause, flash: flashSubPaused}
	resumeSubAction = subAction{check: auth.CanUpdateSubscriptions, run: SubscriptionCommands.Resume, flash: flashSubResumed}
	deleteSubAction = subAction{check: auth.CanDeleteSubscriptions, run: SubscriptionCommands.Delete, flash: flashSubDeleted, deleted: true}
)

func (h *Subscriptions) pause(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, pauseSubAction)
}

func (h *Subscriptions) resume(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, resumeSubAction)
}

func (h *Subscriptions) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deleteSubAction)
}

func (h *Subscriptions) runAction(w http.ResponseWriter, r *http.Request, a subAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := subID(r)
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
		http.Redirect(w, r, subsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, a.deleted)
	if err := flashKey(sse, a.flash); err != nil {
		slog.DebugContext(ctx, "ui: subscription flash not delivered", "err", err)
	}
}

func (h *Subscriptions) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, subList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: subscriptions table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{subDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: subscription drawer close not delivered", "err", err)
		}
		return
	}
	s, err := h.loadSubscription(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(subDrawerContent(toSubDetail(s, subPermsFor(ac))), ds.Inner("#"+subDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: subscription drawer patch not delivered", "err", err)
	}
}

func subConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: subsPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(subDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
