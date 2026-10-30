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

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/go-sdk/usecase"
)

// IdpRoles lists the roles a provider's role sync may confer (the SPA's
// rolesApi.list(): GET /api/roles).
type IdpRoles interface {
	FindAll(ctx context.Context) ([]role.Role, error)
}

// IdentityProviders is the identity providers page (/ui/identity-providers),
// mirroring the SPA's IdentityProviderListPage, IdentityProviderCreateDrawer
// and IdentityProviderDetailDrawer: the list, the drawer, the create and edit
// dialogs (OIDC settings, the write-only client secret, the domains routed to
// the provider with the scope of the mappings they create, role sync) and
// delete. Same structure as Connections (docs/ui/README.md, "Forms and
// actions"). Every identity provider permission is anchor-only
// (auth.anchorWith), as the API's.
type IdentityProviders struct {
	Store IdpStore
	Ops   IdpCommands
	Roles IdpRoles
	// Clients resolves the primary client a CLIENT-scoped domain is linked to
	// (its search and lookup); ClientNames names the client ids.
	Clients     ClientDirectory
	ClientNames ClientStore
	Shell       *shell.Shell
}

const (
	idpPath   = "/ui/identity-providers"
	idpFormID = "idp-list"

	idpDrawerID   = "idp-drawer"
	idpDrawerSig  = "selIdp"
	idpDialogSlot = "idp-dialog-slot"

	idpDeleteDialog = "idp-delete"

	// internalIdpCode is the seeded password-auth provider, which the use
	// case never deletes.
	internalIdpCode = "internal"
)

var idpList = &listSpec{
	path:        idpPath,
	sortKeys:    []string{"name", "code", "type", "created"},
	statusOK:    func(string) bool { return false },
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *IdentityProviders) Mount(r chi.Router) {
	r.Get("/identity-providers", h.list)
	r.Get("/identity-providers/new", h.createForm)
	r.Get("/identity-providers/{id}", h.detail)
	r.Get("/identity-providers/{id}/edit", h.editForm)
	post := r.With(parseForm)
	post.Post("/identity-providers/new", h.create)
	post.Post("/identity-providers/{id}/edit", h.update)
	post.Post("/identity-providers/{id}/delete", h.delete)
}

// idpPerms are the API handlers' checks: create CanCreateIdentityProviders,
// update CanUpdateIdentityProviders, delete CanDeleteIdentityProviders. The
// edit form also reads the provider (CanReadIdentityProviders, which the list
// needs anyway).
type idpPerms struct {
	Create, Update, Delete bool
}

func idpPermsFor(ac *auth.AuthContext) idpPerms {
	return idpPerms{
		Create: auth.CanCreateIdentityProviders(ac) == nil,
		Update: auth.CanUpdateIdentityProviders(ac) == nil,
		Delete: auth.CanDeleteIdentityProviders(ac) == nil,
	}
}

func idpURL(id string) string { return idpPath + "/" + url.PathEscape(id) }

// idpID reads and checks the {id} path parameter.
func idpID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("IdentityProvider", id)
	}
	return id, nil
}

// ── list ───────────────────────────────────────────────────────────────

func (h *IdentityProviders) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// Same check as GET /api/identity-providers.
	if err := auth.CanReadIdentityProviders(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	q := parseListQuery(r.URL.Query(), idpList)
	view, err := h.listView(ctx, ac, q)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patchTable(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: identity providers patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *IdentityProviders) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (idpView, error) {
	providers, err := h.Store.FindAll(ctx)
	if err != nil {
		return idpView{}, fmt.Errorf("find identity providers: %w", err)
	}
	return buildIdpView(ac, q, providers), nil
}

func (h *IdentityProviders) renderPage(w http.ResponseWriter, r *http.Request, status int, view idpView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Identity Providers", Path: idpPath}, idpPage(view))
}

type idpRow struct {
	ID, Code, Name, Type, Issuer string
	Domains                      []string
	Internal                     bool
	Created, CreatedISO          string
	DetailHref, EditHref         string
	ActionBase                   string
}

type idpView struct {
	Query       listQuery
	Rows        []idpRow
	Total       int
	Page, Pages int
	Perms       idpPerms
	Dialog      templ.Component
	Drawer      *idpDetail
}

func (v idpView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(idpTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func buildIdpView(ac *auth.AuthContext, q listQuery, providers []identityprovider.IdentityProvider) idpView {
	v := idpView{Query: q, Perms: idpPermsFor(ac)}
	needle := strings.ToLower(q.Q)
	matched := make([]identityprovider.IdentityProvider, 0, len(providers))
	for _, p := range providers {
		// The SPA's quick filter: name, code, type.
		hay := strings.ToLower(p.Name + " " + p.Code + " " + string(p.Type))
		if needle == "" || strings.Contains(hay, needle) {
			matched = append(matched, p)
		}
	}
	slices.SortStableFunc(matched, func(a, b identityprovider.IdentityProvider) int {
		var c int
		switch q.Sort {
		case "code":
			c = cmp.Compare(a.Code, b.Code)
		case "type":
			c = cmp.Compare(a.Type, b.Type)
		case "created":
			c = a.CreatedAt.Compare(b.CreatedAt)
		default:
			c = cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
		if q.Dir == kit.SortDesc {
			c = -c
		}
		return c
	})
	v.Total = len(matched)
	page, pages, lo, hi := q.pageBounds(len(matched))
	v.Page, v.Pages = page, pages
	for _, p := range matched[lo:hi] {
		v.Rows = append(v.Rows, toIdpRow(p))
	}
	return v
}

func toIdpRow(p identityprovider.IdentityProvider) idpRow {
	return idpRow{
		ID: p.ID, Code: p.Code, Name: p.Name, Type: string(p.Type), Issuer: deref(p.OIDCIssuerURL),
		Domains: p.AllowedEmailDomains, Internal: p.Code == internalIdpCode,
		Created: p.CreatedAt.UTC().Format("2006-01-02"), CreatedISO: p.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		DetailHref: idpURL(p.ID), EditHref: idpURL(p.ID) + "/edit", ActionBase: idpURL(p.ID),
	}
}

// idpTypeSeverity is the SPA's getTypeSeverity.
func idpTypeSeverity(t string) kit.TagSeverity {
	if t == string(identityprovider.TypeOIDC) {
		return kit.TagInfo
	}
	return kit.TagSecondary
}

const undeletableInternal = "The internal identity provider cannot be deleted."

// rowActions: Edit inline (the SPA's pencil); View details and Delete in the
// menu. Left out when the user has no permission; Delete is disabled for the
// internal provider, which the use case refuses.
func (v idpView) rowActions(row idpRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "idp-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	if v.Perms.Delete {
		del := kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, Attrs: confirmTrigger(idpDeleteDialog, row.ActionBase+"/delete")}
		if row.Internal {
			del = kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, DisabledReason: undeletableInternal}
		}
		p.Menu = append(p.Menu, del)
	}
	return p
}

// ── drawer ─────────────────────────────────────────────────────────────

type idpDetail struct {
	ID, Code, Name, Type string
	Created, Updated     string
	// The OIDC settings. The client secret reference is never read into the
	// view: only whether one is configured (hasClientSecret on the wire).
	Issuer, Pattern, ClientID string
	MultiTenant, HasSecret    bool
	Sync                      bool
	// Roles are the allowed roles' names (ids when roles cannot be read); an
	// empty list allows all mapped roles.
	Roles                []string
	Domains              []string
	Internal             bool
	EditHref, ActionBase string
	Perms                idpPerms
}

func (h *IdentityProviders) toDetail(ctx context.Context, ac *auth.AuthContext, p *identityprovider.IdentityProvider) idpDetail {
	d := idpDetail{
		ID: p.ID, Code: p.Code, Name: p.Name, Type: string(p.Type),
		Created: p.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), Updated: p.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		Issuer: deref(p.OIDCIssuerURL), Pattern: deref(p.OIDCIssuerPattern), ClientID: deref(p.OIDCClientID),
		MultiTenant: p.OIDCMultiTenant, HasSecret: p.HasClientSecret(), Sync: p.SyncRolesFromIDP,
		Domains: p.AllowedEmailDomains, Internal: p.Code == internalIdpCode,
		EditHref: idpURL(p.ID) + "/edit", ActionBase: idpURL(p.ID), Perms: idpPermsFor(ac),
	}
	names := map[string]string{}
	if len(p.AllowedRoleIDs) > 0 && h.Roles != nil && auth.CanReadAllRoles(ac) {
		if roles, err := h.Roles.FindAll(ctx); err != nil {
			slog.WarnContext(ctx, "ui: roles unavailable for the identity provider drawer", "err", err)
		} else {
			for _, rl := range roles {
				names[rl.ID] = cmpOrString(rl.DisplayName, rl.Name)
			}
		}
	}
	for _, id := range p.AllowedRoleIDs {
		d.Roles = append(d.Roles, cmpOrString(names[id], id))
	}
	return d
}

func (d idpDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: idpDrawerID, Signal: idpDrawerSig, Label: "Identity provider", Title: d.Name, Subtitle: d.Code, Extras: idpTypeTag(d.Type)}
}

func (h *IdentityProviders) load(ctx context.Context, id string) (*identityprovider.IdentityProvider, error) {
	p, err := h.Store.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("IdentityProvider", id)
	}
	return p, nil
}

func (h *IdentityProviders) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/identity-providers/{id}'s check.
	if err := auth.CanReadIdentityProviders(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := idpID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.load(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := h.toDetail(ctx, ac, p)
	if ds.IsDatastar(r) {
		if err := openIdpDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: identity provider drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, idpList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openIdpDrawer(sse *ds.SSE, d idpDetail) error {
	return errors.Join(
		sse.Patch(idpDrawerContent(d), ds.Inner("#"+idpDrawerID)),
		sse.PatchSignals(map[string]any{idpDrawerSig: d.ID}),
		sse.Focus(idpDrawerID+"-title"),
	)
}

// ── delete ─────────────────────────────────────────────────────────────

// delete removes a provider (DELETE /api/identity-providers/{id}). The use
// case refuses the internal provider and one that still routes email domains
// (409 DOMAINS_STILL_MAPPED), shown as an error flash.
func (h *IdentityProviders) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteIdentityProviders(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := idpID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Ops.Delete(ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashIdpDeleted)
		http.Redirect(w, r, idpPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, true)
	if err := flashKey(sse, flashIdpDeleted); err != nil {
		slog.DebugContext(ctx, "ui: identity provider flash not delivered", "err", err)
	}
}

// afterChange re-renders the table the user is on and the drawer when it
// shows this provider (closed after a delete).
func (h *IdentityProviders) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, idpList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: identity providers table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{idpDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: identity provider drawer close not delivered", "err", err)
		}
		return
	}
	p, err := h.load(ctx, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(idpDrawerContent(h.toDetail(ctx, ac, p)), ds.Inner("#"+idpDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: identity provider drawer patch not delivered", "err", err)
	}
}

// idpDeleteConfirm is the SPA's delete dialog; the use case blocks it while
// email domains still route to the provider.
func idpDeleteConfirm() kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: idpDeleteDialog, Action: idpPath, Title: "Delete Identity Provider",
		Message:      "Delete this identity provider? Deletion is blocked while email domains still route to it: move or delete those mappings first.",
		ConfirmLabel: "Delete", Tone: kit.ToneDanger,
		Hidden:    []kit.HiddenField{drawerField(idpDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}

// clientLabel names a client id for display: its name, else the id.
func clientLabel(names map[string]string, id string) string {
	if n := names[id]; n != "" {
		return n
	}
	return id
}

// clientNamesOf maps client ids to names for the pages that show a primary
// client (the SPA's clients list; without the permission or the store the
// map is empty and the id is shown).
func clientNamesOf(ctx context.Context, store ClientStore, ac *auth.AuthContext) map[string]string {
	if store == nil || auth.CanReadClients(ac) != nil {
		return nil
	}
	all, err := store.FindAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ui: clients unavailable for names", "err", err)
		return nil
	}
	names := make(map[string]string, len(all))
	for _, c := range all {
		names[string(c.ID)] = c.Name
	}
	return names
}

// clientIdentifierOf is the identifier the client select shows for a client
// id ("" when it cannot be read).
func clientIdentifierOf(ctx context.Context, store ClientStore, id string) string {
	if store == nil || id == "" {
		return ""
	}
	c, err := store.FindByID(ctx, id)
	if err != nil || c == nil {
		return ""
	}
	return c.Identifier
}
