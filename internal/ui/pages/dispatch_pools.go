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

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
)

// PoolStore is the slice of the dispatch-pool repository the page reads
// (the calls GET /api/dispatch-pools and GET /api/dispatch-pools/{id} make).
type PoolStore interface {
	FindWithFilters(ctx context.Context, status, clientID *string) ([]dispatchpool.DispatchPool, error)
	FindByID(ctx context.Context, id string) (*dispatchpool.DispatchPool, error)
}

// DispatchPools is the dispatch pools page (/ui/dispatch-pools): the list,
// the record drawer, the create and edit dialogs and the state and delete
// actions, mirroring the SPA's DispatchPoolListPage, DispatchPoolCreateDrawer
// and DispatchPoolDetailDrawer. Every action makes the API handler's
// permission check and runs the API's use case (PoolCommands).
type DispatchPools struct {
	Pools   PoolStore
	Ops     PoolCommands
	Clients ClientDirectory
	Shell   *shell.Shell
}

const poolsPath = "/ui/dispatch-pools"

// Mount registers the page's routes. GET routes never change state (the
// CSRF guard relies on it); every POST answers Datastar with patches and a
// native post with a redirect.
func (h *DispatchPools) Mount(r chi.Router) {
	r.Get("/dispatch-pools", h.list)
	r.Get("/dispatch-pools/new", h.createForm)
	r.Get("/dispatch-pools/{id}", h.detail)
	r.Get("/dispatch-pools/{id}/edit", h.editForm)
	r.Get("/dispatch-pools/{id}/export", h.export)
	r.Get("/dispatch-pools/{id}/export.json", h.exportFile)
	post := r.With(parseForm)
	post.Post("/dispatch-pools/new", h.create)
	post.Post("/dispatch-pools/new/import", h.importDefinition)
	post.Post("/dispatch-pools/{id}/edit", h.update)
	post.Post("/dispatch-pools/{id}/suspend", h.suspend)
	post.Post("/dispatch-pools/{id}/activate", h.activate)
	post.Post("/dispatch-pools/{id}/delete", h.delete)
}

// list renders the page, or, for a Datastar request (the live search,
// status filter, Clear All and sort links), patches the table, the summary
// and the toolbar's state part and replaces the address bar URL so the view
// stays shareable.
func (h *DispatchPools) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// Same check as GET /api/dispatch-pools.
	if err := auth.CanReadDispatchPools(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parsePoolQuery(r.URL.Query())
	view, err := h.listView(ctx, ac, q)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			sse.Patch(poolsTable(view)),
			sse.Patch(poolsSummary(view)),
			// The toolbar's state part (hidden sort, Clear All, Filters badge),
			// never the search box or the open filters popover.
			sse.Patch(kit.TableToolbarState(view.toolbar().State())),
			sse.ReplaceURL(q.URL()),
		); err != nil {
			slog.DebugContext(ctx, "ui: dispatch pools patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

// listView reads the pools for q with the API list's status filter and
// client scoping.
func (h *DispatchPools) listView(ctx context.Context, ac *auth.AuthContext, q poolQuery) (poolsView, error) {
	var status *string
	if q.Status != "" {
		status = &q.Status
	}
	pools, err := h.Pools.FindWithFilters(ctx, status, nil)
	if err != nil {
		return poolsView{}, fmt.Errorf("find dispatch pools: %w", err)
	}
	// Same per-client visibility as the API list.
	visible := auth.FilterClientScoped(ac, pools, func(p *dispatchpool.DispatchPool) *string { return ids.StringPtr(p.ClientID) })
	return buildPoolsView(ac, q, visible), nil
}

func (h *DispatchPools) renderPage(w http.ResponseWriter, r *http.Request, status int, view poolsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Dispatch Pools", Path: poolsPath}, poolsPage(view))
}

// poolList is the pools list's URL state (list.go).
var poolList = &listSpec{
	path:     poolsPath,
	sortKeys: []string{"code", "name", "status", "created"},
	statusOK: func(s string) bool { _, ok := dispatchpool.ParseStatus(s); return ok },
}

// poolQuery is the pools list's view state.
type poolQuery = listQuery

// parsePoolQuery reads the pools list's view from a query string.
func parsePoolQuery(v url.Values) poolQuery { return parseListQuery(v, poolList) }

// refererQuery is the pools list view the request came from (list.go).
func refererQuery(r *http.Request) poolQuery { return refererList(r, poolList) }

const maxQueryLen = 200

const poolsFormID = "pools-list"

// ── view model ─────────────────────────────────────────────────────────

type poolRow struct {
	ID, Code, Name, Scope  string
	RateLimit, Concurrency string
	Status                 string
	RawStatus              dispatchpool.Status
	StatusVariant          kit.BadgeVariant
	Created, CreatedISO    string
	DetailHref, EditHref   string
}

// poolPerms are the actions the caller may run, by the API handlers'
// checks: create needs CanCreateDispatchPools; edit, suspend and activate
// need CanUpdateDispatchPools; delete needs CanDeleteDispatchPools.
type poolPerms struct {
	Create, Update, Delete bool
}

func poolPermsFor(ac *auth.AuthContext) poolPerms {
	return poolPerms{
		Create: auth.CanCreateDispatchPools(ac) == nil,
		Update: auth.CanUpdateDispatchPools(ac) == nil,
		Delete: auth.CanDeleteDispatchPools(ac) == nil,
	}
}

type poolsView struct {
	Query poolQuery
	Rows  []poolRow
	Total int // visible pools before the search (status is filtered in SQL)
	Perms poolPerms
	// Dialog is a form dialog rendered with the page (a navigation to
	// /new or /{id}/edit, or a native post re-rendered with errors).
	Dialog templ.Component
	// Drawer is the record the drawer opens with (a navigation to /{id}).
	Drawer *poolDetail
}

// FilterCount is the number of active filters in the Filters popover.
func (v poolsView) FilterCount() int {
	if v.Query.Status != "" {
		return 1
	}
	return 0
}

// Filtered reports whether search or a filter narrows the list.
func (v poolsView) Filtered() bool { return v.Query.Q != "" || v.Query.Status != "" }

func buildPoolsView(ac *auth.AuthContext, q poolQuery, pools []dispatchpool.DispatchPool) poolsView {
	v := poolsView{Query: q, Total: len(pools), Perms: poolPermsFor(ac)}
	needle := strings.ToLower(q.Q)
	matched := make([]dispatchpool.DispatchPool, 0, len(pools))
	for _, p := range pools {
		// The SPA's quick filter searched code, name and client identifier.
		hay := strings.ToLower(p.Code + " " + p.Name + " " + deref(p.ClientIdentifier))
		if needle == "" || strings.Contains(hay, needle) {
			matched = append(matched, p)
		}
	}
	slices.SortStableFunc(matched, func(a, b dispatchpool.DispatchPool) int {
		var c int
		switch q.Sort {
		case "name":
			c = cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
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
	for _, p := range matched {
		v.Rows = append(v.Rows, toPoolRow(p))
	}
	return v
}

func poolURL(id string) string { return poolsPath + "/" + url.PathEscape(id) }

func toPoolRow(p dispatchpool.DispatchPool) poolRow {
	row := poolRow{
		ID:            p.ID,
		Code:          p.Code,
		Name:          p.Name,
		Scope:         "Anchor-level",
		RateLimit:     "—",
		Concurrency:   strconv.Itoa(int(p.Concurrency)),
		Status:        string(p.Status),
		RawStatus:     p.Status,
		StatusVariant: poolStatusVariant(p.Status),
		Created:       p.CreatedAt.UTC().Format("2006-01-02"),
		CreatedISO:    p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		DetailHref:    poolURL(p.ID),
		EditHref:      poolURL(p.ID) + "/edit",
	}
	if p.ClientIdentifier != nil && *p.ClientIdentifier != "" {
		row.Scope = *p.ClientIdentifier
	}
	if p.RateLimit != nil {
		row.RateLimit = strconv.Itoa(int(*p.RateLimit)) + "/min"
	}
	return row
}

// poolStatusVariant is the SPA's getStatusSeverity.
func poolStatusVariant(s dispatchpool.Status) kit.BadgeVariant {
	switch s {
	case dispatchpool.StatusActive:
		return kit.BadgeSuccess
	case dispatchpool.StatusSuspended:
		return kit.BadgeWarn
	case dispatchpool.StatusArchived:
	}
	return kit.BadgeSecondary
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// ── row actions ────────────────────────────────────────────────────────

// archivedReadOnly is why an archived pool's Edit is disabled (the SPA's
// drawer refuses to edit an archived pool).
const archivedReadOnly = "Archived pools cannot be edited."

// liveLink fetches a link's URL through Datastar instead of navigating
// (the server answers with a dialog or the drawer).
const liveLink = "@get(el.href)"

// rowActions mirrors the SPA: Edit inline (it opens the edit form); View
// details and the detail drawer's actions in the menu (Activate or Suspend,
// Delete; none for an archived pool). Actions the user has no permission
// for are left out.
func (v poolsView) rowActions(row poolRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "pool-" + row.ID + "-actions", RowLabel: row.Name}
	archived := row.RawStatus == dispatchpool.StatusArchived
	if v.Perms.Update {
		edit := kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}}
		if archived {
			edit.DisabledReason = archivedReadOnly
		}
		p.Actions = append(p.Actions, edit)
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	if archived {
		return p
	}
	if v.Perms.Update {
		if row.RawStatus == dispatchpool.StatusActive {
			p.Menu = append(p.Menu, kit.RowAction{Icon: "pause", Label: "Suspend…", Attrs: confirmTrigger(poolSuspendDialog, row.DetailHref+"/suspend")})
		} else {
			p.Menu = append(p.Menu, kit.RowAction{Icon: "play", Label: "Activate…", Attrs: confirmTrigger(poolActivateDialog, row.DetailHref+"/activate")})
		}
	}
	if v.Perms.Delete {
		p.Menu = append(p.Menu, kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, Attrs: confirmTrigger(poolDeleteDialog, row.DetailHref+"/delete")})
	}
	return p
}

// confirmTrigger opens the shared confirm dialog for one record
// (confirm-dialog.md, "One dialog for many records").
func confirmTrigger(dialogID, action string) templ.Attributes {
	return templ.Attributes{"data-on:click": kit.OpenConfirm(dialogID), "data-action": action, "aria-haspopup": "dialog"}
}

// poolID reads and checks the {id} path parameter: an id that could not be
// an element id is not a pool (and must never reach an expression).
func poolID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("DispatchPool", id)
	}
	return id, nil
}
