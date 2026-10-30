package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// PortalApps is the portal apps page (/ui/portal-apps), mirroring the SPA's
// PortalAppsPage: the portals a client runs for its customers, each with the
// OAuth client provisioned for it. Create shows the new client's credentials
// once; assigning users with no portal app and delete sit behind
// confirmations. Reads make GET /api/portal-apps's check (CanReadPortalUsers
// for the client); every change CanManagePortalUsers, as the API's handlers.
type PortalApps struct {
	Store   PortalStore
	Ops     PortalCommands
	Clients ClientStore
	Shell   *shell.Shell
}

const (
	portalAppsPath = "/ui/portal-apps"

	portalAppDialogSlot  = "portal-app-dialog-slot"
	portalAppAssignID    = "portal-app-assign"
	portalAppDeleteID    = "portal-app-delete"
	portalAppsRegionID   = "portal-apps-region"
	portalAppsClientForm = "portal-apps-client"
)

// Mount registers the page's routes (POSTs behind parseForm).
func (h *PortalApps) Mount(r chi.Router) {
	r.Get("/portal-apps", h.list)
	r.Get("/portal-apps/new", h.createForm)
	r.Get("/portal-apps/{id}/edit", h.editForm)
	post := r.With(parseForm)
	post.Post("/portal-apps/new", h.create)
	post.Post("/portal-apps/{id}/edit", h.update)
	post.Post("/portal-apps/{id}/delete", h.delete)
	post.Post("/portal-apps/{id}/assign-unassigned", h.assignUnassigned)
}

// ── view ───────────────────────────────────────────────────────────────

// portalOAuthRef is an OAuth client fronting an app.
type portalOAuthRef struct {
	ID, ClientID, Name string
}

type portalAppRow struct {
	ID, Code, Name, Description string
	Active                      bool
	Users                       int
	OAuth                       []portalOAuthRef
	EditHref, ActionBase        string
}

type portalAppsView struct {
	Client        string
	ClientName    string
	ClientOptions []kit.Option
	Rows          []portalAppRow
	// Unassigned counts the client's portal users granted no portal app.
	Unassigned int64
	CanManage  bool
	Dialog     templ.Component
}

// view reads the page for the chosen client (empty: none chosen yet) with
// the API's read check.
func (h *PortalApps) view(ctx context.Context, ac *auth.AuthContext, requested string) (portalAppsView, error) {
	opts, err := portalClientOptions(ctx, h.Clients, ac)
	if err != nil {
		return portalAppsView{}, err
	}
	clientID, opts := portalSelectedClient(ac, opts, requested)
	v := portalAppsView{Client: clientID, ClientOptions: opts, ClientName: portalClientName(opts, clientID)}
	if clientID == "" {
		return v, nil
	}
	if err := auth.CanReadPortalUsers(ac, clientID); err != nil {
		return portalAppsView{}, err
	}
	v.CanManage = auth.CanManagePortalUsers(ac, clientID) == nil
	apps, err := h.Store.Apps(ctx, clientID)
	if err != nil {
		return portalAppsView{}, usecase.Internal("REPO", "list_portal_apps failed", err)
	}
	ids := make([]string, len(apps))
	for n, a := range apps {
		ids[n] = a.ID
	}
	counts, err := h.Store.GrantCounts(ctx, ids)
	if err != nil {
		return portalAppsView{}, usecase.Internal("REPO", "portal_app_grant_counts failed", err)
	}
	linked, err := h.Store.LinkedOAuthClients(ctx, ids)
	if err != nil {
		return portalAppsView{}, usecase.Internal("REPO", "portal_app_oauth_clients failed", err)
	}
	if v.Unassigned, err = h.Store.CountUnassigned(ctx, clientID); err != nil {
		return portalAppsView{}, usecase.Internal("REPO", "count_unassigned failed", err)
	}
	for _, a := range apps {
		row := portalAppRow{
			ID: a.ID, Code: a.Code, Name: a.Name, Description: deref(a.Description), Active: a.Active, Users: counts[a.ID],
			EditHref: portalAppsPath + "/" + url.PathEscape(a.ID) + "/edit?" + portalClientParam + "=" + url.QueryEscape(clientID),
		}
		row.ActionBase = portalAppsPath + "/" + url.PathEscape(a.ID)
		for _, oc := range linked[a.ID] {
			row.OAuth = append(row.OAuth, portalOAuthRef{ID: oc.ID, ClientID: oc.ClientID, Name: oc.ClientName})
		}
		v.Rows = append(v.Rows, row)
	}
	return v, nil
}

func (h *PortalApps) renderPage(w http.ResponseWriter, r *http.Request, status int, v portalAppsView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Portal Apps", Path: portalAppsPath}, portalAppsPage(v))
}

func (v portalAppsView) patchRegion(sse *ds.SSE) error {
	return sse.Patch(portalAppsRegion(v))
}

func (v portalAppsView) url() string {
	if v.Client == "" {
		return portalAppsPath
	}
	return portalAppsPath + "?" + portalClientParam + "=" + url.QueryEscape(v.Client)
}

func (v portalAppsView) newHref() string {
	return portalAppsPath + "/new?" + portalClientParam + "=" + url.QueryEscape(v.Client)
}

func (v portalAppsView) rowActions(row portalAppRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "portal-app-" + row.ID + "-actions", RowLabel: row.Name}
	if !v.CanManage {
		return p
	}
	assign := kit.RowAction{Icon: "users", Label: "Assign unassigned users…", Attrs: confirmTrigger(portalAppAssignID, row.ActionBase+"/assign-unassigned")}
	switch {
	case v.Unassigned == 0:
		assign = kit.RowAction{Icon: "users", Label: "Assign unassigned users…", DisabledReason: "No portal users are without a portal app."}
	case !row.Active:
		assign = kit.RowAction{Icon: "users", Label: "Assign unassigned users…", DisabledReason: "An inactive portal app takes no new grants."}
	}
	p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	p.Menu = append(p.Menu, assign,
		kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, Attrs: confirmTrigger(portalAppDeleteID, row.ActionBase+"/delete")})
	return p
}

func (v portalAppsView) confirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: portalAppsPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{{Name: portalClientParam, Value: v.Client}},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}

// ── list ───────────────────────────────────────────────────────────────

func (h *PortalApps) list(w http.ResponseWriter, r *http.Request) {
	ac := auth.FromContext(r.Context())
	v, err := h.view(r.Context(), ac, r.URL.Query().Get(portalClientParam))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.renderPage(w, r, http.StatusOK, v)
}

// manageView is the view for a change: the client the form names, with the
// manage check the API's handlers make (CanManagePortalUsers). Every mutation
// starts here, so a post for a client the caller may not manage is refused
// before anything else is read.
func (h *PortalApps) manageView(ctx context.Context, ac *auth.AuthContext, clientID string) (portalAppsView, error) {
	if clientID == "" {
		return portalAppsView{}, usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
	}
	if err := auth.CanManagePortalUsers(ac, clientID); err != nil {
		return portalAppsView{}, err
	}
	return h.view(ctx, ac, clientID)
}

func errPortalAppNotFound(id string) error { return httperror.NotFound("PortalApp", id) }

func portalAppID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("PortalApp", id)
	}
	return id, nil
}

// findApp is one of the view's apps by id (an app of another client is not
// found, as the use cases' client scoping).
func (v portalAppsView) findApp(id string) (portalAppRow, bool) {
	i := slices.IndexFunc(v.Rows, func(row portalAppRow) bool { return row.ID == id })
	if i < 0 {
		return portalAppRow{}, false
	}
	return v.Rows[i], true
}

// ── actions ────────────────────────────────────────────────────────────

func (h *PortalApps) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashPortalAppDeleted, func(ctx context.Context, clientID string, app portalAppRow) (string, error) {
		if err := h.Ops.DeleteApp(ctx, portalidentity.DeleteAppCommand{ClientID: clientID, ID: app.ID}); err != nil {
			return "", err
		}
		return "Portal app \"" + app.Name + "\" deleted", nil
	})
}

func (h *PortalApps) assignUnassigned(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashPortalAppAssigned, func(ctx context.Context, clientID string, app portalAppRow) (string, error) {
		res, err := h.Ops.AssignUnassigned(ctx, portalidentity.AssignUnassignedCommand{ClientID: clientID, PortalAppID: app.ID})
		if err != nil {
			return "", err
		}
		return strconv.Itoa(len(res.IdentityIDs)) + " user(s) can now sign in to " + app.Name, nil
	})
}

// runAction runs one confirmed action on an app and refreshes the table. The
// detail it returns replaces the flash key's text on a Datastar answer (the
// SPA's toast names the app or the count).
func (h *PortalApps) runAction(w http.ResponseWriter, r *http.Request, flash string, run func(ctx context.Context, clientID string, app portalAppRow) (string, error)) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	v, err := h.manageView(ctx, ac, r.PostFormValue(portalClientParam))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := portalAppID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	app, ok := v.findApp(id)
	if !ok {
		h.Shell.Fail(w, r, errPortalAppNotFound(id))
		return
	}
	detail, err := run(ctx, v.Client, app)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, v.url(), http.StatusSeeOther) //nolint:gosec // G710: the target is a page constant; the client id is query-escaped and checked by the manage check
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, v.Client)
	if err := portalFlash(sse, flash, detail); err != nil {
		slog.DebugContext(ctx, "ui: portal app flash not delivered", "err", err)
	}
}

// afterChange re-renders the table (and the banner above it) for the client.
func (h *PortalApps) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, clientID string) {
	v, err := h.view(r.Context(), ac, clientID)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := v.patchRegion(sse); err != nil {
		slog.DebugContext(r.Context(), "ui: portal apps patch not delivered", "err", err)
	}
}

// portalFlash appends the flash key's message with a detail the server built
// (the SPA's toasts name the record).
func portalFlash(sse *ds.SSE, key, detail string) error {
	msg, ok := FlashMessages[key]
	if !ok {
		return errors.New("ui: unknown flash key " + key)
	}
	msg.Detail = detail
	return shell.Flash(sse, msg)
}
