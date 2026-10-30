package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/a-h/templ"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchpool"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// The record drawer and the state/delete actions (the SPA's
// DispatchPoolDetailDrawer).

const (
	poolDrawerID   = "pool-drawer"
	poolDrawerSig  = "selPool"
	poolDialogSlot = "pool-dialog-slot"

	poolActivateDialog = "pool-activate"
	poolSuspendDialog  = "pool-suspend"
	poolDeleteDialog   = "pool-delete"
)

// poolDetail is the drawer's view of one pool.
type poolDetail struct {
	ID, Code, Name, Description string
	RateLimit, Concurrency      string
	Scope                       string
	Status                      string
	RawStatus                   dispatchpool.Status
	StatusVariant               kit.BadgeVariant
	Created, Updated            string
	EditHref, ActionBase        string
	Perms                       poolPerms
}

func toPoolDetail(p *dispatchpool.DispatchPool, perms poolPerms) poolDetail {
	d := poolDetail{
		ID:            p.ID,
		Code:          p.Code,
		Name:          p.Name,
		Description:   deref(p.Description),
		RateLimit:     "Unlimited (concurrency-only)",
		Concurrency:   strconv.Itoa(int(p.Concurrency)),
		Scope:         "Anchor-level (no client)",
		Status:        string(p.Status),
		RawStatus:     p.Status,
		StatusVariant: poolStatusVariant(p.Status),
		Created:       p.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		Updated:       p.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		EditHref:      poolURL(p.ID) + "/edit",
		ActionBase:    poolURL(p.ID),
		Perms:         perms,
	}
	if p.RateLimit != nil {
		d.RateLimit = strconv.Itoa(int(*p.RateLimit)) + " / minute"
	}
	if p.ClientIdentifier != nil && *p.ClientIdentifier != "" {
		d.Scope = *p.ClientIdentifier
	}
	return d
}

// Archived pools are read-only in the SPA's drawer: no Edit, no Actions.
func (d poolDetail) archived() bool { return d.RawStatus == dispatchpool.StatusArchived }
func (d poolDetail) canEdit() bool  { return d.Perms.Update && !d.archived() }
func (d poolDetail) showActions() bool {
	return !d.archived() && (d.Perms.Update || d.Perms.Delete)
}

func (d poolDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: poolDrawerID, Signal: poolDrawerSig, Label: "Dispatch pool", Title: d.Name, Subtitle: d.Code, Extras: poolStatusBadge(d.Status, d.StatusVariant)}
}

// loadPool reads a pool by id with the API's by-id checks: the read
// permission was checked by the caller; a client-scoped pool needs access to
// its client (GET /api/dispatch-pools/{id}).
func (h *DispatchPools) loadPool(ctx context.Context, ac *auth.AuthContext, id string) (*dispatchpool.DispatchPool, error) {
	p, err := h.Pools.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("DispatchPool", id)
	}
	if p.ClientID != nil && !ac.CanAccessClient(string(*p.ClientID)) {
		return nil, httperror.Forbidden("No access to this dispatch pool")
	}
	return p, nil
}

// detail opens the drawer for a pool: patched into the page for Datastar (a
// row's name link or View details), or the list page with the drawer open for
// a navigation to /ui/dispatch-pools/{id}.
func (h *DispatchPools) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanReadDispatchPools(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := poolID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.loadPool(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := toPoolDetail(p, poolPermsFor(ac))
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := openDrawer(sse, d); err != nil {
			slog.DebugContext(ctx, "ui: pool drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parsePoolQuery(nil))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

// openDrawer patches the drawer's content for d, selects it and moves focus
// to its heading (drawer.md, server-loaded record).
func openDrawer(sse *ds.SSE, d poolDetail) error {
	return errors.Join(
		sse.Patch(poolDrawerContent(d), ds.Inner("#"+poolDrawerID)),
		sse.PatchSignals(map[string]any{poolDrawerSig: d.ID}),
		sse.Focus(poolDrawerID+"-title"),
	)
}

// poolAction is one of the drawer's state/delete actions.
type poolAction struct {
	check   func(*auth.AuthContext) error
	run     func(PoolCommands, context.Context, string) error
	flash   string
	deleted bool
}

var (
	suspendAction  = poolAction{check: auth.CanUpdateDispatchPools, run: PoolCommands.Suspend, flash: flashPoolSuspended}
	activateAction = poolAction{check: auth.CanUpdateDispatchPools, run: PoolCommands.Activate, flash: flashPoolActivated}
	deleteAction   = poolAction{check: auth.CanDeleteDispatchPools, run: PoolCommands.Delete, flash: flashPoolDeleted, deleted: true}
)

func (h *DispatchPools) suspend(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, suspendAction)
}

func (h *DispatchPools) activate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, activateAction)
}

func (h *DispatchPools) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deleteAction)
}

// runAction makes the API handler's permission check, runs the use case and
// answers: for Datastar the list as the user sees it, the drawer when it
// shows this pool (closed after a delete) and a flash; for a native post a
// redirect to the list with the flash key. A failure is an error flash
// (Datastar) or an error page.
func (h *DispatchPools) runAction(w http.ResponseWriter, r *http.Request, a poolAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := poolID(r)
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
		http.Redirect(w, r, poolsPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, a.deleted)
	if err := flashKey(sse, a.flash); err != nil {
		slog.DebugContext(ctx, "ui: pool action flash not delivered", "err", err)
	}
}

// afterChange re-renders what a change to pool id affects: the table and
// summary of the list the user is on, and the drawer when it shows this pool
// (the form's hidden "drawer" field carries $selPool). A deleted pool's
// drawer is closed.
func (h *DispatchPools) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererQuery(r)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := errors.Join(sse.Patch(poolsTable(view)), sse.Patch(poolsSummary(view))); err != nil {
		slog.DebugContext(ctx, "ui: pools table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{poolDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: pool drawer close not delivered", "err", err)
		}
		return
	}
	p, err := h.loadPool(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(poolDrawerContent(toPoolDetail(p, poolPermsFor(ac))), ds.Inner("#"+poolDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: pool drawer patch not delivered", "err", err)
	}
}

// flashKey appends the message of a flash key to the page's flash stack.
func flashKey(sse *ds.SSE, key string) error {
	msg, ok := FlashMessages[key]
	if !ok {
		return errors.New("ui: unknown flash key " + key)
	}
	return shell.Flash(sse, msg)
}

// poolConfirm is one shared confirm dialog. Its rendered action is the list
// (which accepts no POST); the trigger sets the record's action.
func poolConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: poolsPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(poolDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
