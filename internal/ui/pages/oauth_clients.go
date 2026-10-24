package pages

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/portalidentity"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// Flash keys of this area (shell.SetFlash after a native post). A flash key
// carries no request data, so the SPA's toasts that name the client are
// worded without it.
const (
	flashOCCreated     = "oauth-client-created"
	flashOCUpdated     = "oauth-client-updated"
	flashOCDeleted     = "oauth-client-deleted"
	flashOCActivated   = "oauth-client-activated"
	flashOCDeactivated = "oauth-client-deactivated"
	flashOCRotated     = "oauth-client-secret-rotated"
	flashOCRevoked     = "oauth-client-previous-secret-revoked"
)

func init() {
	maps.Copy(FlashMessages, map[string]kit.FlashMessageProps{
		flashOCCreated:     success("OAuth client created successfully"),
		flashOCUpdated:     success("OAuth client updated successfully"),
		flashOCDeleted:     success("OAuth client deleted"),
		flashOCActivated:   {Kind: kit.FlashSuccess, Summary: "Activated", Detail: "OAuth client has been activated"},
		flashOCDeactivated: {Kind: kit.FlashSuccess, Summary: "Deactivated", Detail: "OAuth client has been deactivated"},
		flashOCRotated:     success("Client secret rotated successfully"),
		flashOCRevoked:     success("Previous client secret revoked"),
	})
}

// OAuthClientStore is the slice of the OAuth client repository the page reads
// (GET /api/oauth-clients, GET /api/oauth-clients/{id}). The secret
// references on the rows are never read by the page.
type OAuthClientStore interface {
	FindAll(ctx context.Context) ([]platformauth.OAuthClient, error)
	FindByID(ctx context.Context, id string) (*platformauth.OAuthClient, error)
}

// OAuthClientPortalApps lists a client's portal apps (the Portal app select;
// GET /api/portal-apps?clientId=).
type OAuthClientPortalApps interface {
	Apps(ctx context.Context, clientID string) ([]portalidentity.App, error)
}

// OAuthClients is the OAuth clients page (/ui/oauth-clients), mirroring the
// SPA's OAuthClientListPage, OAuthClientCreateDrawer and
// OAuthClientDetailDrawer: the list, create with the shown-once secret, the
// drawer with edit, secret rotation (the 24-hour overlap and its early end),
// activation and delete.
type OAuthClients struct {
	Clients OAuthClientStore
	Ops     OAuthClientCommands
	// Apps names the applications a client is scoped to and offers them in the
	// form; ClientNames offers the portal owner clients; Portal the portal apps.
	Apps        ApplicationStore
	ClientNames ClientStore
	Portal      OAuthClientPortalApps
	Shell       *shell.Shell
}

const (
	ocPath   = "/ui/oauth-clients"
	ocFormID = "oc-list"

	ocDrawerID   = "oc-drawer"
	ocDrawerSig  = "selOc"
	ocDialogSlot = "oc-dialog-slot"

	ocActivateDialog   = "oc-activate"
	ocDeactivateDialog = "oc-deactivate"
	ocRotateDialog     = "oc-rotate"
	ocRotateNowDialog  = "oc-rotate-now"
	ocRevokeDialog     = "oc-revoke"
	ocDeleteDialog     = "oc-delete"

	ocLiveGet = "@get('/ui/oauth-clients', {contentType: 'form'})"
)

var ocList = &listSpec{
	path:        ocPath,
	sortKeys:    []string{"name", "clientId", "type", "status", "created"},
	statusOK:    func(string) bool { return false },
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *OAuthClients) Mount(r chi.Router) {
	r.Get("/oauth-clients", h.list)
	r.Get("/oauth-clients/new", h.createForm)
	r.Get("/oauth-clients/portal-app-options", h.portalAppOptions)
	r.Get("/oauth-clients/{id}", h.detail)
	r.Get("/oauth-clients/{id}/edit", h.editForm)
	post := r.With(parseForm)
	post.Post("/oauth-clients/new", h.create)
	post.Post("/oauth-clients/{id}/edit", h.update)
	post.Post("/oauth-clients/{id}/activate", h.activate)
	post.Post("/oauth-clients/{id}/deactivate", h.deactivate)
	post.Post("/oauth-clients/{id}/delete", h.delete)
	post.Post("/oauth-clients/{id}/rotate-secret", h.rotateSecret)
	post.Post("/oauth-clients/{id}/rotate-secret-now", h.rotateSecretNow)
	post.Post("/oauth-clients/{id}/revoke-previous-secret", h.revokePreviousSecret)
}

// ── permissions ────────────────────────────────────────────────────────

// ocPerms are the API's checks: every route is anchor-only and needs the
// permission for its operation (create, update for edit and activation,
// delete; the regenerate-secret permission for rotation and revoking the
// previous secret).
type ocPerms struct {
	Create, Update, Delete, Rotate bool
}

func ocPermsFor(ac *auth.AuthContext) ocPerms {
	return ocPerms{
		Create: auth.CanCreateOAuthClients(ac) == nil,
		Update: auth.CanUpdateOAuthClients(ac) == nil,
		Delete: auth.CanDeleteOAuthClients(ac) == nil,
		Rotate: auth.CanRotateOAuthClientSecrets(ac) == nil,
	}
}

func ocID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("OAuthClient", id)
	}
	return id, nil
}

// ── list ───────────────────────────────────────────────────────────────

func (h *OAuthClients) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/oauth-clients's check.
	if err := auth.CanReadOAuthClients(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), ocList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patchTable(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: oauth clients patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

// ocApps are the application names the page shows and the applications it
// offers. The names are the API's (it resolves every id, whatever the caller
// may list); the options need the application list permission, as the SPA's
// applicationsApi call does, and an application-scoped caller is offered only
// its own.
type ocApps struct {
	names   map[string]string
	options []kit.Option
	canPick bool
}

func (a ocApps) label(id string) string {
	if n := a.names[id]; n != "" {
		return n
	}
	return id
}

func (h *OAuthClients) apps(ctx context.Context, ac *auth.AuthContext) ocApps {
	var out ocApps
	if h.Apps == nil {
		return out
	}
	all, err := h.Apps.FindWithFilters(ctx, nil, nil)
	if err != nil {
		slog.WarnContext(ctx, "ui: applications unavailable for oauth clients", "err", err)
		return out
	}
	out.names = make(map[string]string, len(all))
	for _, a := range all {
		out.names[string(a.ID)] = a.Name
	}
	if auth.CanReadApplications(ac) != nil {
		return out
	}
	out.canPick = true
	readAll := auth.CanReadAllApplications(ac)
	for _, a := range all {
		if a.Type == application.TypeApplication && a.Active && (readAll || ac.CanAccessApplication(string(a.ID))) {
			out.options = append(out.options, kit.Option{Value: string(a.ID), Label: a.Name + " (" + a.Code + ")"})
		}
	}
	slices.SortFunc(out.options, func(a, b kit.Option) int { return cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	return out
}

// ocOwners are the clients a portal client can be owned by (the SPA's
// clientsApi.list, which needs the client list: anchor with client:view).
type ocOwners struct {
	names   map[string]string
	options []kit.Option
	canPick bool
}

func (o ocOwners) label(id string) string {
	if n := o.names[id]; n != "" {
		return n
	}
	return id
}

func (h *OAuthClients) owners(ctx context.Context, ac *auth.AuthContext) ocOwners {
	if auth.CanReadClients(ac) != nil || h.ClientNames == nil {
		return ocOwners{}
	}
	all, err := h.ClientNames.FindAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ui: clients unavailable for oauth clients", "err", err)
		return ocOwners{}
	}
	o := ocOwners{canPick: true, names: make(map[string]string, len(all))}
	for _, c := range all {
		o.names[string(c.ID)] = c.Name
		o.options = append(o.options, kit.Option{Value: string(c.ID), Label: c.Name})
	}
	slices.SortFunc(o.options, func(a, b kit.Option) int { return cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	return o
}

// listView reads the list. A caller without the read permission gets an empty
// one (the forms and the shown-once dialogs of a create-only caller render the
// page around it); the list route itself refuses such a caller first.
func (h *OAuthClients) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (ocView, error) {
	if auth.CanReadOAuthClients(ac) != nil {
		return buildOCView(ac, q, nil, ocApps{}), nil
	}
	clients, err := h.Clients.FindAll(ctx)
	if err != nil {
		return ocView{}, fmt.Errorf("find oauth clients: %w", err)
	}
	return buildOCView(ac, q, clients, h.apps(ctx, ac)), nil
}

func (h *OAuthClients) renderPage(w http.ResponseWriter, r *http.Request, status int, view ocView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "OAuth Clients", Path: ocPath}, ocPage(view))
}

type ocRow struct {
	ID, Name, ClientID, Type string
	Apps                     []string
	MoreApps                 int
	Active                   bool
	Created, CreatedISO      string
	DetailHref, EditHref     string
	ActionBase               string
}

type ocView struct {
	Query       listQuery
	Rows        []ocRow
	Total       int
	Page, Pages int
	Perms       ocPerms
	Dialog      templ.Component
	Drawer      *ocDetail
}

// buildOCView applies the SPA's table filter (the quick search over name,
// client id and application names), sort and paging.
func buildOCView(ac *auth.AuthContext, q listQuery, clients []platformauth.OAuthClient, apps ocApps) ocView {
	v := ocView{Perms: ocPermsFor(ac)}
	needle := strings.ToLower(q.Q)
	shown := make([]platformauth.OAuthClient, 0, len(clients))
	for _, c := range clients {
		if needle != "" && !strings.Contains(strings.ToLower(c.ClientName+" "+c.ClientID+" "+strings.Join(ocAppNames(c, apps), " ")), needle) {
			continue
		}
		shown = append(shown, c)
	}
	slices.SortStableFunc(shown, func(a, b platformauth.OAuthClient) int {
		var c int
		switch q.Sort {
		case "clientId":
			c = cmp.Compare(a.ClientID, b.ClientID)
		case "type":
			c = cmp.Compare(a.ClientType, b.ClientType)
		case "status":
			c = cmp.Compare(ocBit(a.Active), ocBit(b.Active))
		case "created":
			c = a.CreatedAt.Compare(b.CreatedAt)
		default:
			c = cmp.Compare(strings.ToLower(a.ClientName), strings.ToLower(b.ClientName))
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
	for i := lo; i < hi; i++ {
		c := &shown[i]
		names := ocAppNames(*c, apps)
		row := ocRow{
			ID: c.ID, Name: c.ClientName, ClientID: c.ClientID, Type: string(c.ClientType), Active: c.Active,
			Created: c.CreatedAt.UTC().Format("2006-01-02"), CreatedISO: c.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			DetailHref: ocURL(c.ID), EditHref: ocURL(c.ID) + "/edit", ActionBase: ocURL(c.ID),
		}
		// The SPA shows the first three application tags and a count.
		row.Apps = names[:min(len(names), 3)]
		row.MoreApps = max(len(names)-3, 0)
		v.Rows = append(v.Rows, row)
	}
	return v
}

func ocBit(b bool) int {
	if b {
		return 1
	}
	return 0
}

func ocAppNames(c platformauth.OAuthClient, apps ocApps) []string {
	names := make([]string, len(c.ApplicationIDs))
	for i, id := range c.ApplicationIDs {
		names[i] = apps.label(id)
	}
	return names
}

func ocURL(id string) string { return ocPath + "/" + url.PathEscape(id) }

func ocTypeSeverity(t string) kit.TagSeverity {
	if t == string(platformauth.OAuthClientPublic) {
		return kit.TagInfo
	}
	return kit.TagWarn
}

// rowActions mirrors the SPA's row: Edit inline; View details, the toggle
// and Delete in the menu. Actions the user has no permission for are left
// out. Activation and deletion ask first (the SPA toggles at once).
func (v ocView) rowActions(row ocRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "oc-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	if v.Perms.Update {
		if row.Active {
			p.Menu = append(p.Menu, kit.RowAction{Icon: "ban", Label: "Deactivate…", Attrs: confirmTrigger(ocDeactivateDialog, row.ActionBase+"/deactivate")})
		} else {
			p.Menu = append(p.Menu, kit.RowAction{Icon: "circle-check", Label: "Activate…", Attrs: confirmTrigger(ocActivateDialog, row.ActionBase+"/activate")})
		}
	}
	if v.Perms.Delete {
		p.Menu = append(p.Menu, kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, Attrs: confirmTrigger(ocDeleteDialog, row.ActionBase+"/delete")})
	}
	return p
}

func (v ocView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(ocTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

// ── drawer ─────────────────────────────────────────────────────────────

type ocDetail struct {
	ID, Name, ClientID, Type string
	Active                   bool
	Redirects, PostLogout    []string
	Origins                  []string
	Grants, Scopes           []string
	PKCE                     bool
	Apps                     []string
	PortalOwner              string
	// PortalApp is the linked app's name and code, or its id when the app
	// cannot be read; empty for a legacy portal client with no app.
	PortalApp        string
	IsPortal         bool
	APIAccess        bool
	Created, Updated time.Time
	EditHref         string
	ActionBase       string
	Perms            ocPerms
	OverlapUntil     time.Time
	OverlapLastUsed  *time.Time
	HasOverlap       bool
	Confidential     bool
}

func (d ocDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: ocDrawerID, Signal: ocDrawerSig, Label: "OAuth client", Size: kit.DrawerWide, Title: d.Name, Subtitle: d.ClientID, Extras: ocHeaderTags(d.Type, d.Active)}
}

// loadClient reads a client with GET /api/oauth-clients/{id}'s checks.
func (h *OAuthClients) loadClient(ctx context.Context, ac *auth.AuthContext, id string) (*platformauth.OAuthClient, error) {
	if err := auth.CanReadOAuthClients(ac); err != nil {
		return nil, err
	}
	c, err := h.Clients.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil {
		return nil, httperror.NotFound("OAuthClient", id)
	}
	return c, nil
}

// detailOf builds the drawer with what the SPA loads for it: the client, the
// application names, the portal owner and the portal app. The owner and app
// names need the permissions of the lists the SPA reads them from; without
// them the ids are shown.
func (h *OAuthClients) detailOf(ctx context.Context, ac *auth.AuthContext, c *platformauth.OAuthClient) ocDetail {
	d := ocDetail{
		ID: c.ID, Name: c.ClientName, ClientID: c.ClientID, Type: string(c.ClientType), Active: c.Active,
		Redirects: c.RedirectURIs, PostLogout: c.PostLogoutRedirectURIs, Origins: c.AllowedOrigins,
		Grants: c.GrantTypes, Scopes: c.Scopes, PKCE: c.PKCERequired, APIAccess: c.APIAccess,
		Created: c.CreatedAt, Updated: c.UpdatedAt,
		EditHref: ocURL(c.ID) + "/edit", ActionBase: ocURL(c.ID), Perms: ocPermsFor(ac),
		Confidential: c.ClientType == platformauth.OAuthClientConfidential,
	}
	d.Apps = ocAppNames(*c, h.apps(ctx, ac))
	if c.PortalClientID != nil && *c.PortalClientID != "" {
		d.IsPortal = true
		d.PortalOwner = h.owners(ctx, ac).label(*c.PortalClientID)
		if id := deref(c.PortalAppID); id != "" {
			d.PortalApp = id
			if auth.CanReadPortalUsers(ac, *c.PortalClientID) == nil && h.Portal != nil {
				if apps, err := h.Portal.Apps(ctx, *c.PortalClientID); err == nil {
					for _, a := range apps {
						if a.ID == id {
							d.PortalApp = a.Name + " (" + a.Code + ")"
						}
					}
				}
			}
		}
	}
	// The overlap is shown only while it is open: an expired one is the same
	// as none (the API reports it that way).
	if c.UsablePreviousSecretRef() != nil {
		d.HasOverlap = true
		d.OverlapUntil = *c.PreviousSecretExpiresAt
		d.OverlapLastUsed = c.PreviousSecretLastUsedAt
	}
	return d
}

// overlapText says how long the previous secret is still accepted and
// whether anyone has used it since the rotation (never used is the safe state
// to revoke in).
func (d ocDetail) overlapText() string {
	s := "It stops working at " + d.OverlapUntil.UTC().Format(time.DateTime) + " UTC."
	if d.OverlapLastUsed == nil {
		return s + " It has not been used since the rotation."
	}
	return s + " It was last used at " + d.OverlapLastUsed.UTC().Format(time.DateTime) + " UTC."
}

func (h *OAuthClients) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ocID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d, err := h.drawerFor(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		if err := openOCDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: oauth client drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, ocList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *OAuthClients) drawerFor(ctx context.Context, ac *auth.AuthContext, id string) (ocDetail, error) {
	c, err := h.loadClient(ctx, ac, id)
	if err != nil {
		return ocDetail{}, err
	}
	return h.detailOf(ctx, ac, c), nil
}

func openOCDrawer(sse *ds.SSE, d ocDetail) error {
	return errors.Join(
		sse.Patch(ocDrawerContent(d), ds.Inner("#"+ocDrawerID)),
		sse.PatchSignals(map[string]any{ocDrawerSig: d.ID}),
		sse.Focus(ocDrawerID+"-title"),
	)
}

// afterChange re-renders what a change touched: the table the user is on and
// the drawer when it shows this client (closed after a delete). A caller who
// may not read OAuth clients (a create-only or rotate-only role) gets neither
// the table nor the drawer, only the outcome.
func (h *OAuthClients) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if r.PostFormValue("drawer") == id && deleted {
		if err := sse.PatchSignals(map[string]any{ocDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: oauth client drawer close not delivered", "err", err)
		}
	}
	if auth.CanReadOAuthClients(ac) != nil {
		return
	}
	if view, err := h.listView(ctx, ac, refererList(r, ocList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: oauth clients table patch not delivered", "err", err)
	}
	if deleted || r.PostFormValue("drawer") != id {
		return
	}
	d, err := h.drawerFor(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(ocDrawerContent(d), ds.Inner("#"+ocDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: oauth client drawer patch not delivered", "err", err)
	}
}

// ── actions ────────────────────────────────────────────────────────────

// done answers a successful change: the table, the drawer and a flash; a
// native post redirects to the list with the flash key.
func (h *OAuthClients) done(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id, flash string, deleted bool) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, ocPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, deleted)
	if err := flashKey(sse, flash); err != nil {
		slog.DebugContext(ctx, "ui: oauth client flash not delivered", "err", err)
	}
}

// ocAction is one of the plain state changes: the API's permission check and
// the use case. The check runs before anything is read, so a refused caller
// cannot tell a real id from an invented one.
type ocAction struct {
	check func(*auth.AuthContext) error
	run   func(OAuthClientCommands, context.Context, string) error
	flash string
	// deleted: the drawer closes if it shows the client.
	deleted bool
}

var (
	activateOCAction   = ocAction{check: auth.CanUpdateOAuthClients, run: OAuthClientCommands.Activate, flash: flashOCActivated}
	deactivateOCAction = ocAction{check: auth.CanUpdateOAuthClients, run: OAuthClientCommands.Deactivate, flash: flashOCDeactivated}
	deleteOCAction     = ocAction{check: auth.CanDeleteOAuthClients, run: OAuthClientCommands.Delete, flash: flashOCDeleted, deleted: true}
	revokeOCAction     = ocAction{check: auth.CanRotateOAuthClientSecrets, run: OAuthClientCommands.RevokePreviousSecret, flash: flashOCRevoked}
)

func (h *OAuthClients) activate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, activateOCAction)
}

func (h *OAuthClients) deactivate(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deactivateOCAction)
}

func (h *OAuthClients) delete(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, deleteOCAction)
}

func (h *OAuthClients) revokePreviousSecret(w http.ResponseWriter, r *http.Request) {
	h.runAction(w, r, revokeOCAction)
}

func (h *OAuthClients) runAction(w http.ResponseWriter, r *http.Request, a ocAction) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := ocID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := a.check(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := a.run(h.Ops, ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, a.flash, a.deleted)
}

func ocConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: ocPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(ocDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
