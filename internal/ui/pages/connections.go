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

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// ConnectionStore is the slice of the connection repository the page reads
// (the calls GET /api/connections and GET /api/connections/{id} make).
type ConnectionStore interface {
	FindWithFilters(ctx context.Context, status, clientID *string) ([]connection.Connection, error)
	FindByID(ctx context.Context, id string) (*connection.Connection, error)
}

// Connections is the connections page (/ui/connections), mirroring the SPA's
// ConnectionListPage, ConnectionCreateDrawer and ConnectionDetailDrawer: the
// list, the drawer, the create and edit dialogs, pause/activate and delete.
// Same structure as DispatchPools (docs/ui/README.md, "Forms and actions").
type Connections struct {
	Connections     ConnectionStore
	Ops             ConnectionCommands
	Clients         ClientDirectory
	ServiceAccounts ServiceAccountDirectory
	Shell           *shell.Shell
}

const (
	connsPath   = "/ui/connections"
	connsFormID = "connections-list"

	connDrawerID   = "conn-drawer"
	connDrawerSig  = "selConn"
	connDialogSlot = "conn-dialog-slot"

	connActivateDialog = "conn-activate"
	connPauseDialog    = "conn-pause"
	connDeleteDialog   = "conn-delete"
)

var connList = &listSpec{
	path:     connsPath,
	sortKeys: []string{"code", "name", "status", "created"},
	statusOK: func(s string) bool { _, ok := connection.ParseStatus(s); return ok },
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *Connections) Mount(r chi.Router) {
	r.Get("/connections", h.list)
	r.Get("/connections/new", h.createForm)
	r.Get("/connections/{id}", h.detail)
	r.Get("/connections/{id}/edit", h.editForm)
	r.Get("/connections/{id}/export", h.export)
	r.Get("/connections/{id}/export.json", h.exportFile)
	post := r.With(parseForm)
	post.Post("/connections/new", h.create)
	post.Post("/connections/new/import", h.importDefinition)
	post.Post("/connections/{id}/edit", h.update)
	post.Post("/connections/{id}/pause", h.pause)
	post.Post("/connections/{id}/activate", h.activate)
	post.Post("/connections/{id}/delete", h.delete)
}

// ── list ───────────────────────────────────────────────────────────────

func (h *Connections) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// Same check as GET /api/connections.
	if err := auth.CanReadConnections(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parseListQuery(r.URL.Query(), connList)
	view, err := h.listView(ctx, ac, q)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(
			sse.Patch(connsTable(view)),
			sse.Patch(connsSummary(view)),
			sse.Patch(kit.TableToolbarState(view.toolbar().State())),
			sse.ReplaceURL(q.URL()),
		); err != nil {
			slog.DebugContext(ctx, "ui: connections patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *Connections) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (connsView, error) {
	var status *string
	if q.Status != "" {
		status = &q.Status
	}
	conns, err := h.Connections.FindWithFilters(ctx, status, nil)
	if err != nil {
		return connsView{}, fmt.Errorf("find connections: %w", err)
	}
	// Same per-client visibility as the API list.
	visible := auth.FilterClientScoped(ac, conns, func(c *connection.Connection) *string { return ids.StringPtr(c.ClientID) })
	return buildConnsView(ac, q, visible), nil
}

func (h *Connections) renderPage(w http.ResponseWriter, r *http.Request, status int, view connsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Connections", Path: connsPath}, connsPage(view))
}

// connPerms are the API handlers' checks: create needs
// CanCreateConnections; update, pause and activate CanUpdateConnections;
// delete CanDeleteConnections.
type connPerms struct {
	Create, Update, Delete bool
}

func connPermsFor(ac *auth.AuthContext) connPerms {
	return connPerms{
		Create: auth.CanCreateConnections(ac) == nil,
		Update: auth.CanUpdateConnections(ac) == nil,
		Delete: auth.CanDeleteConnections(ac) == nil,
	}
}

type connRow struct {
	ID, Code, Name, Scope string
	Status                string
	RawStatus             connection.Status
	StatusVariant         kit.BadgeVariant
	Created, CreatedISO   string
	DetailHref, EditHref  string
}

type connsView struct {
	Query  listQuery
	Rows   []connRow
	Total  int
	Perms  connPerms
	Dialog templ.Component
	Drawer *connDetail
}

func (v connsView) FilterCount() int {
	if v.Query.Status != "" {
		return 1
	}
	return 0
}

func buildConnsView(ac *auth.AuthContext, q listQuery, conns []connection.Connection) connsView {
	v := connsView{Query: q, Total: len(conns), Perms: connPermsFor(ac)}
	needle := strings.ToLower(q.Q)
	matched := make([]connection.Connection, 0, len(conns))
	for _, c := range conns {
		// The SPA's quick filter: code, name, client identifier.
		hay := strings.ToLower(c.Code + " " + c.Name + " " + deref(c.ClientIdentifier))
		if needle == "" || strings.Contains(hay, needle) {
			matched = append(matched, c)
		}
	}
	slices.SortStableFunc(matched, func(a, b connection.Connection) int {
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
	for _, c := range matched {
		v.Rows = append(v.Rows, toConnRow(c))
	}
	return v
}

func connURL(id string) string { return connsPath + "/" + url.PathEscape(id) }

func toConnRow(c connection.Connection) connRow {
	row := connRow{
		ID: c.ID, Code: c.Code, Name: c.Name, Scope: "Anchor-level",
		Status: string(c.Status), RawStatus: c.Status, StatusVariant: connStatusVariant(c.Status),
		Created: c.CreatedAt.UTC().Format("2006-01-02"), CreatedISO: c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		DetailHref: connURL(c.ID), EditHref: connURL(c.ID) + "/edit",
	}
	if c.ClientIdentifier != nil && *c.ClientIdentifier != "" {
		row.Scope = *c.ClientIdentifier
	}
	return row
}

// connStatusVariant is the SPA's getStatusSeverity.
func connStatusVariant(s connection.Status) kit.BadgeVariant {
	switch s {
	case connection.StatusActive:
		return kit.BadgeSuccess
	case connection.StatusPaused:
		return kit.BadgeWarn
	}
	return kit.BadgeSecondary
}

// rowActions: Edit inline (the SPA's pencil); View details and the
// drawer's actions (Pause or Activate, Delete) in the menu. Left out when the
// user has no permission.
func (v connsView) rowActions(row connRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "conn-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	if v.Perms.Update {
		switch row.RawStatus {
		case connection.StatusActive:
			p.Menu = append(p.Menu, kit.RowAction{Icon: "pause", Label: "Pause…", Attrs: confirmTrigger(connPauseDialog, row.DetailHref+"/pause")})
		case connection.StatusPaused:
			p.Menu = append(p.Menu, kit.RowAction{Icon: "play", Label: "Activate…", Attrs: confirmTrigger(connActivateDialog, row.DetailHref+"/activate")})
		}
	}
	if v.Perms.Delete {
		p.Menu = append(p.Menu, kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, Attrs: confirmTrigger(connDeleteDialog, row.DetailHref+"/delete")})
	}
	return p
}

// connID reads and checks the {id} path parameter.
func connID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Connection", id)
	}
	return id, nil
}

// ── drawer ─────────────────────────────────────────────────────────────

type connDetail struct {
	ID, Code, Name, Description, ExternalID string
	ServiceAccount                          saRef
	Scope                                   string
	Status                                  string
	RawStatus                               connection.Status
	StatusVariant                           kit.BadgeVariant
	Created, Updated                        string
	EditHref, ActionBase                    string
	Perms                                   connPerms
}

func toConnDetail(c *connection.Connection, perms connPerms) connDetail {
	d := connDetail{
		ID: c.ID, Code: c.Code, Name: c.Name, Description: deref(c.Description), ExternalID: deref(c.ExternalID),
		ServiceAccount: saRef{ID: c.ServiceAccountID},
		Scope:          "Anchor-level (no client)",
		Status:         string(c.Status), RawStatus: c.Status, StatusVariant: connStatusVariant(c.Status),
		Created:  c.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		Updated:  c.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		EditHref: connURL(c.ID) + "/edit", ActionBase: connURL(c.ID),
		Perms: perms,
	}
	if c.ClientIdentifier != nil && *c.ClientIdentifier != "" {
		d.Scope = *c.ClientIdentifier
	}
	return d
}

func (d connDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: connDrawerID, Signal: connDrawerSig, Label: "Connection", Title: d.Name, Subtitle: d.Code, Extras: poolStatusBadge(d.Status, d.StatusVariant)}
}

func (d connDetail) showActions() bool { return d.Perms.Update || d.Perms.Delete }

// loadConnection reads a connection with the API's by-id checks (the read
// permission is the caller's).
func (h *Connections) loadConnection(ctx context.Context, ac *auth.AuthContext, id string) (*connection.Connection, error) {
	c, err := h.Connections.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("Connection", id)
	}
	if c.ClientID != nil && !ac.CanAccessClient(string(*c.ClientID)) {
		return nil, httperror.Forbidden("No access to this connection")
	}
	return c, nil
}

// connDetail is toConnDetail with the service account named (resolveSA).
func (h *Connections) connDetail(ctx context.Context, ac *auth.AuthContext, c *connection.Connection) connDetail {
	d := toConnDetail(c, connPermsFor(ac))
	d.ServiceAccount = resolveSA(ctx, h.ServiceAccounts, ac, c.ServiceAccountID)
	return d
}

// saRef is a service account as a drawer shows it: name and code linked to
// its drawer, or only the id when the viewer may not read service accounts
// (or it is gone), as the SPA's drawers fall back.
type saRef struct {
	ID, Name, Code, Href string
	Inactive             bool
}

func resolveSA(ctx context.Context, dir ServiceAccountDirectory, ac *auth.AuthContext, id string) saRef {
	ref := saRef{ID: id}
	if id == "" || dir == nil || auth.CanReadServiceAccounts(ac) != nil {
		return ref
	}
	sa, err := dir.FindByID(ctx, id)
	if err != nil {
		slog.DebugContext(ctx, "ui: service account not resolved", "id", id, "err", err)
		return ref
	}
	if sa == nil {
		return ref
	}
	ref.Name, ref.Code, ref.Href, ref.Inactive = sa.Name, sa.Code, saURL(sa.ID), !sa.Active
	return ref
}

func (h *Connections) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadConnections(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := connID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	c, err := h.loadConnection(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := h.connDetail(ctx, ac, c)
	if ds.IsDatastar(r) {
		if err := openConnDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: connection drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, connList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openConnDrawer(sse *ds.SSE, d connDetail) error {
	return errors.Join(
		sse.Patch(connDrawerContent(d), ds.Inner("#"+connDrawerID)),
		sse.PatchSignals(map[string]any{connDrawerSig: d.ID}),
		sse.Focus(connDrawerID+"-title"),
	)
}

// ── actions ────────────────────────────────────────────────────────────

type connAction struct {
	check   func(*auth.AuthContext) error
	run     func(ConnectionCommands, context.Context, string) error
	flash   string
	deleted bool
}

var (
	pauseConnAction    = connAction{check: auth.CanUpdateConnections, run: ConnectionCommands.Pause, flash: flashConnPaused}
	activateConnAction = connAction{check: auth.CanUpdateConnections, run: ConnectionCommands.Activate, flash: flashConnActivated}
	deleteConnAction   = connAction{check: auth.CanDeleteConnections, run: ConnectionCommands.Delete, flash: flashConnDeleted, deleted: true}
)

func (h *Connections) pause(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, pauseConnAction)
}

func (h *Connections) activate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, activateConnAction)
}

func (h *Connections) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deleteConnAction)
}

// runAction: the API handler's check, the use case, then the list, the
// drawer (when it shows this connection) and a flash; a native post
// redirects with the flash key. A refusal (e.g. delete with subscriptions,
// 409 HAS_SUBSCRIPTIONS) is an error flash or an error page.
func (h *Connections) runAction(w http.ResponseWriter, r *http.Request, a connAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := connID(r)
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
		http.Redirect(w, r, connsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, a.deleted)
	if err := flashKey(sse, a.flash); err != nil {
		slog.DebugContext(ctx, "ui: connection flash not delivered", "err", err)
	}
}

func (h *Connections) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, connList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := errors.Join(sse.Patch(connsTable(view)), sse.Patch(connsSummary(view))); err != nil {
		slog.DebugContext(ctx, "ui: connections table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{connDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: connection drawer close not delivered", "err", err)
		}
		return
	}
	c, err := h.loadConnection(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(connDrawerContent(h.connDetail(ctx, ac, c)), ds.Inner("#"+connDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: connection drawer patch not delivered", "err", err)
	}
}

// connConfirm is one shared confirm dialog (the SPA's confirmations).
func connConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: connsPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(connDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
