package pages

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// PortalUsers is the portal users page (/ui/portal-users), mirroring the
// SPA's PortalUsersPage: a client's portal identities with their state and
// app grants, the prefix search the API serves, suspend and reactivate,
// grant and revoke of portal apps and offboarding. There is deliberately no
// invite: portals invite their own users (POST /api/portal-users). Reads make
// GET /api/portal-users's check (CanReadPortalUsers); every change
// CanManagePortalUsers for the client, as the API's handlers.
type PortalUsers struct {
	Store   PortalStore
	Ops     PortalCommands
	Clients ClientStore
	Shell   *shell.Shell
}

const (
	portalUsersPath  = "/ui/portal-users"
	portalUsersForm  = "portal-users-list"
	portalUsersLive  = "@get('/ui/portal-users', {contentType: 'form'})"
	portalUsersTable = "portal-users-region"

	portalUserDialogSlot = "portal-user-dialog-slot"
	portalUserSuspendID  = "portal-user-suspend"
	portalUserReviveID   = "portal-user-reactivate"
	portalUserDeleteID   = "portal-user-delete"
	portalUserRevokeID   = "portal-user-revoke"

	// portalUnassigned is the app filter's value for users granted no portal
	// app at all (the SPA's sentinel; not a valid app code).
	portalUnassigned = "__unassigned__"
	portalAppFacet   = "app"
)

var portalUserList = &listSpec{
	path:        portalUsersPath,
	sortKeys:    []string{"created"}, // the API orders newest first; no sortable column
	statusOK:    func(string) bool { return false },
	facets:      []string{portalClientParam, portalAppFacet},
	rows:        []int{25, 50, 100},
	defaultRows: 25,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *PortalUsers) Mount(r chi.Router) {
	r.Get("/portal-users", h.list)
	r.Get("/portal-users/{id}/grant", h.grantForm)
	post := r.With(parseForm)
	post.Post("/portal-users/{id}/grant", h.grant)
	post.Post("/portal-users/{id}/suspend", h.suspend)
	post.Post("/portal-users/{id}/reactivate", h.reactivate)
	post.Post("/portal-users/{id}/delete", h.delete)
	post.Post("/portal-users/{id}/apps/{code}/revoke", h.revoke)
}

// ── view ───────────────────────────────────────────────────────────────

// portalGrant is one portal app an identity holds.
type portalGrant struct {
	Code, Name, Title string
	RevokeAction      string
}

type portalUserRow struct {
	ID, Email, Name       string
	State, StateTitle     string
	StateSeverity         kit.TagSeverity
	Suspended             bool
	Grants                []portalGrant
	Source                string
	LastLogin, Created    time.Time
	GrantHref, ActionBase string
	CanGrant              bool
}

type portalUsersView struct {
	Query         listQuery
	Client        string
	ClientName    string
	ClientOptions []kit.Option
	AppOptions    []kit.Option
	App           string
	Rows          []portalUserRow
	Total         int
	Page, Pages   int
	CanManage     bool
	Dialog        templ.Component
}

// portalStateLabels are the SPA's state tags.
var portalStateLabels = map[portalidentity.AccessState]string{
	portalidentity.StateInvited:       "Invited",
	portalidentity.StateInviteExpired: "Invite expired",
	portalidentity.StateActive:        "Active",
	portalidentity.StateSuspended:     "Suspended",
}

var portalStateSeverities = map[portalidentity.AccessState]kit.TagSeverity{
	portalidentity.StateInvited:       kit.TagInfo,
	portalidentity.StateInviteExpired: kit.TagWarn,
	portalidentity.StateActive:        kit.TagSuccess,
	portalidentity.StateSuspended:     kit.TagDanger,
}

// portalStateTitle is the hover detail of the state tag: when the invite went
// out or lapses.
func portalStateTitle(i *portalidentity.Identity, state portalidentity.AccessState) string {
	switch {
	case state == portalidentity.StateInvited && i.InviteExpiresAt != nil:
		return "Invite expires " + i.InviteExpiresAt.UTC().Format("2006-01-02 15:04 UTC")
	case state == portalidentity.StateInviteExpired && i.InviteExpiresAt != nil:
		return "Invite expired " + i.InviteExpiresAt.UTC().Format("2006-01-02 15:04 UTC") + " — the portal can re-send it"
	case state == portalidentity.StateInvited && i.InvitedAt != nil:
		return "Invited " + i.InvitedAt.UTC().Format("2006-01-02 15:04 UTC")
	}
	return ""
}

// grantableApps are the active apps the identity does not hold yet.
func grantableApps(apps []portalidentity.App, i *portalidentity.Identity) []portalidentity.App {
	return slices.DeleteFunc(slices.Clone(apps), func(a portalidentity.App) bool {
		return !a.Active || slices.ContainsFunc(i.Apps, func(g portalidentity.AppGrant) bool { return g.AppID == a.ID })
	})
}

// first is the first value of a facet (the page's single-valued filters).
func first(vals []string) string {
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// view reads the page for the request's query with the API's read check.
func (h *PortalUsers) view(ctx context.Context, ac *auth.AuthContext, q listQuery) (portalUsersView, error) {
	opts, err := portalClientOptions(ctx, h.Clients, ac)
	if err != nil {
		return portalUsersView{}, err
	}
	clientID, opts := portalSelectedClient(ac, opts, first(q.facet(portalClientParam)))
	wanted := first(q.facet(portalAppFacet))
	q = q.withFacet(portalClientParam, nil).withFacet(portalAppFacet, nil)
	v := portalUsersView{Query: q, Client: clientID, ClientOptions: opts, ClientName: portalClientName(opts, clientID), Page: 1, Pages: 1}
	if clientID == "" {
		return v, nil
	}
	if err := auth.CanReadPortalUsers(ac, clientID); err != nil {
		return portalUsersView{}, err
	}
	v.CanManage = auth.CanManagePortalUsers(ac, clientID) == nil
	apps, err := h.Store.Apps(ctx, clientID)
	if err != nil {
		return portalUsersView{}, usecase.Internal("REPO", "list_portal_apps failed", err)
	}
	filter := portalidentity.SearchFilter{ClientID: clientID, Query: q.Q}
	// The app filter is a code of the client's or the unassigned sentinel; a
	// hand-edited value is dropped from the canonical URL. The SPA offers it
	// only when the client has apps.
	if len(apps) > 0 {
		v.AppOptions = []kit.Option{{Value: portalUnassigned, Label: "No portal app (unassigned)"}}
		for _, a := range apps {
			v.AppOptions = append(v.AppOptions, kit.Option{Value: a.Code, Label: a.Name + " (" + a.Code + ")"})
		}
		if i := slices.IndexFunc(apps, func(a portalidentity.App) bool { return a.Code == wanted }); i >= 0 {
			filter.AppID, v.App = apps[i].ID, wanted
		} else if wanted == portalUnassigned {
			filter.Unassigned, v.App = true, wanted
		}
	}
	q = q.withFacet(portalClientParam, []string{clientID})
	if v.App != "" {
		q = q.withFacet(portalAppFacet, []string{v.App})
	}
	rows, total, err := h.search(ctx, &q, filter)
	if err != nil {
		return portalUsersView{}, err
	}
	v.Query, v.Total, v.Page = q, int(total), q.Page
	_, v.Pages, _, _ = q.pageBounds(v.Total)
	byID := make(map[string]portalidentity.App, len(apps))
	for _, a := range apps {
		byID[a.ID] = a
	}
	now := time.Now().UTC()
	for n := range rows {
		v.Rows = append(v.Rows, portalRow(&rows[n], apps, byID, v.CanManage, now))
	}
	return v, nil
}

// search reads the page of identities q asks for, moving q to the last page
// when it is past the end.
func (h *PortalUsers) search(ctx context.Context, q *listQuery, filter portalidentity.SearchFilter) ([]portalidentity.Identity, int64, error) {
	filter.Offset, filter.Limit = (q.Page-1)*q.Rows, q.Rows
	rows, total, err := h.Store.Search(ctx, filter)
	if err != nil {
		return nil, 0, usecase.Internal("REPO", "search_portal_identities failed", err)
	}
	if page, _, _, _ := q.pageBounds(int(total)); page != q.Page {
		q.Page = page
		filter.Offset = (page - 1) * q.Rows
		if rows, total, err = h.Store.Search(ctx, filter); err != nil {
			return nil, 0, usecase.Internal("REPO", "search_portal_identities failed", err)
		}
	}
	return rows, total, nil
}

func portalRow(i *portalidentity.Identity, apps []portalidentity.App, byID map[string]portalidentity.App, canManage bool, now time.Time) portalUserRow {
	state := i.State(now)
	base := portalUsersPath + "/" + url.PathEscape(i.ID)
	row := portalUserRow{
		ID: i.ID, Email: i.Email, Name: i.Name, State: portalStateLabels[state], StateSeverity: portalStateSeverities[state],
		StateTitle: portalStateTitle(i, state), Suspended: i.Status == portalidentity.StatusDisabled,
		Source: "Invite", Created: i.CreatedAt, ActionBase: base, CanGrant: len(grantableApps(apps, i)) > 0,
	}
	if i.Source == portalidentity.SourceJIT {
		row.Source = "SSO sign-in"
	}
	if i.LastLoginAt != nil {
		row.LastLogin = *i.LastLoginAt
	}
	for _, g := range i.Apps {
		// A grant of an app that is gone shows its id (the API's fallback).
		a, ok := byID[g.AppID]
		if !ok {
			a = portalidentity.App{ID: g.AppID, Code: g.AppID, Name: g.AppID}
		}
		row.Grants = append(row.Grants, portalGrant{
			Code: a.Code, Name: a.Name, Title: a.Code + " · granted " + g.GrantedAt.UTC().Format("2006-01-02 15:04 UTC") + " (" + string(g.Source) + ")",
			RevokeAction: base + "/apps/" + url.PathEscape(a.Code) + "/revoke",
		})
	}
	return row
}

func (h *PortalUsers) renderPage(w http.ResponseWriter, r *http.Request, status int, v portalUsersView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Portal Users", Path: portalUsersPath}, portalUsersPage(v))
}

func (v portalUsersView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(portalUsersRegion(v)), sse.Patch(kit.Paginator(v.pager())))
}

// url is the view's canonical URL.
func (v portalUsersView) url() string { return v.Query.URL() }

func (v portalUsersView) filterCount() int {
	if v.App != "" {
		return 1
	}
	return 0
}

// clearURL is the view without search and filter, on the same client.
func (v portalUsersView) clearURL() string {
	return listQuery{spec: portalUserList, Sort: v.Query.Sort, Dir: v.Query.Dir, Page: 1, Rows: v.Query.Rows}.
		withFacet(portalClientParam, []string{v.Client}).URL()
}

func (v portalUsersView) confirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: portalUsersPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{{Name: portalClientParam, Value: v.Client}},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}

func (v portalUsersView) rowActions(row portalUserRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "portal-user-" + row.ID + "-actions", RowLabel: row.Email}
	if !v.CanManage {
		return p
	}
	grant := kit.RowAction{Icon: "circle-plus", Label: "Grant portal app", Href: row.ActionBase + "/grant?" + portalClientParam + "=" + url.QueryEscape(v.Client), Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}}
	if !row.CanGrant {
		grant = kit.RowAction{Icon: "circle-plus", Label: "Grant portal app", DisabledReason: "There is no other active portal app to grant."}
	}
	p.Actions = append(p.Actions, grant)
	if row.Suspended {
		p.Menu = append(p.Menu, kit.RowAction{Icon: "circle-check", Label: "Reactivate…", Attrs: confirmTrigger(portalUserReviveID, row.ActionBase+"/reactivate")})
	} else {
		p.Menu = append(p.Menu, kit.RowAction{Icon: "ban", Label: "Suspend…", Attrs: confirmTrigger(portalUserSuspendID, row.ActionBase+"/suspend")})
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, Attrs: confirmTrigger(portalUserDeleteID, row.ActionBase+"/delete")})
	return p
}

// ── list ───────────────────────────────────────────────────────────────

func (h *PortalUsers) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	v, err := h.view(ctx, auth.FromContext(ctx), parseListQuery(r.URL.Query(), portalUserList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(v.patchTable(sse), sse.Patch(kit.TableToolbarState(v.toolbar().State())), sse.ReplaceURL(v.url())); err != nil {
			slog.DebugContext(ctx, "ui: portal users patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, v)
}

// manageView is the view for a change: the client the form names, with the
// manage check the API's handlers make (CanManagePortalUsers). Every
// mutation starts here, so a post for a client the caller may not manage is
// refused before anything else is read. The list it shows afterwards is the
// one the user is on (the Referer), on that client.
func (h *PortalUsers) manageView(r *http.Request, ac *auth.AuthContext) (portalUsersView, error) {
	clientID := strings.TrimSpace(r.PostFormValue(portalClientParam))
	if clientID == "" {
		clientID = strings.TrimSpace(r.URL.Query().Get(portalClientParam))
	}
	if clientID == "" {
		return portalUsersView{}, usecase.Validation("CLIENT_ID_REQUIRED", "clientId is required")
	}
	if err := auth.CanManagePortalUsers(ac, clientID); err != nil {
		return portalUsersView{}, err
	}
	q := refererList(r, portalUserList).withFacet(portalClientParam, []string{clientID})
	return h.view(r.Context(), ac, q)
}

func portalUserID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("PortalUser", id)
	}
	return id, nil
}

// identity is the portal identity of the request's {id} on the client, as
// the use cases scope it (another client's identity is not found).
func (h *PortalUsers) identity(r *http.Request, clientID string) (*portalidentity.Identity, error) {
	id, err := portalUserID(r)
	if err != nil {
		return nil, err
	}
	i, err := h.Store.Identity(r.Context(), id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find portal identity failed", err)
	}
	if i == nil || i.ClientID != clientID {
		return nil, httperror.NotFound("PortalIdentity", id)
	}
	return i, nil
}

// ── actions ────────────────────────────────────────────────────────────

func (h *PortalUsers) suspend(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashPortalUserSuspended, func(ctx context.Context, clientID string, i *portalidentity.Identity) (string, error) {
		err := h.Ops.SetStatus(ctx, portalidentity.SetStatusCommand{ID: i.ID, ClientID: clientID, Status: string(portalidentity.StatusDisabled)})
		return i.Email + " suspended", err
	})
}

func (h *PortalUsers) reactivate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashPortalUserReactivated, func(ctx context.Context, clientID string, i *portalidentity.Identity) (string, error) {
		err := h.Ops.SetStatus(ctx, portalidentity.SetStatusCommand{ID: i.ID, ClientID: clientID, Status: string(portalidentity.StatusActive)})
		return i.Email + " reactivated", err
	})
}

func (h *PortalUsers) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashPortalUserDeleted, func(ctx context.Context, clientID string, i *portalidentity.Identity) (string, error) {
		err := h.Ops.DeleteUser(ctx, portalidentity.DeleteCommand{ID: i.ID, ClientID: clientID})
		return i.Email + " deleted", err
	})
}

// revoke removes one grant: the API's handler resolves the app by code (404
// when the client has none), then revokes.
func (h *PortalUsers) revoke(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, flashPortalAccessRevoked, func(ctx context.Context, clientID string, i *portalidentity.Identity) (string, error) {
		app, err := h.Store.AppByCode(ctx, clientID, chi.URLParam(r, "code"))
		if err != nil {
			return "", usecase.Internal("REPO", "find_portal_app failed", err)
		}
		if app == nil {
			return "", httperror.NotFound("PortalApp", portalidentity.NormalizeAppCode(chi.URLParam(r, "code")))
		}
		if err := h.Ops.RevokeApp(ctx, portalidentity.AppGrantCommand{ClientID: clientID, IdentityID: i.ID, PortalAppID: app.ID}); err != nil {
			return "", err
		}
		return i.Email + " no longer has access to " + app.Name, nil
	})
}

// runAction runs one confirmed action on an identity and refreshes the list.
// The detail it returns replaces the flash key's text on a Datastar answer
// (the SPA's toast names the user).
func (h *PortalUsers) runAction(w http.ResponseWriter, r *http.Request, flash string, run func(ctx context.Context, clientID string, i *portalidentity.Identity) (string, error)) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	v, err := h.manageView(r, ac)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	i, err := h.identity(r, v.Client)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	detail, err := run(ctx, v.Client, i)
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
	h.afterChange(sse, r, ac, v.Query)
	if err := portalFlash(sse, flash, detail); err != nil {
		slog.DebugContext(ctx, "ui: portal user flash not delivered", "err", err)
	}
}

// afterChange re-renders the list the user is on.
func (h *PortalUsers) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, q listQuery) {
	v, err := h.view(r.Context(), ac, q)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := v.patchTable(sse); err != nil {
		slog.DebugContext(r.Context(), "ui: portal users patch not delivered", "err", err)
	}
}
