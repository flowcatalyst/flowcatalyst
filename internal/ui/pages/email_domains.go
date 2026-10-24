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
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/emaildomainmapping"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/identityprovider"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// EmailDomains is the email domain mappings page (/ui/email-domains),
// mirroring the SPA's EmailDomainMappingListPage, EmailDomainMappingCreateDrawer
// and EmailDomainMappingDetailDrawer: the list, the drawer, the create and edit
// dialogs (scope, primary client, the tenant a multi-tenant provider requires,
// two-factor policy), the provider move behind its confirmation, and delete.
// Same structure as Connections (docs/ui/README.md, "Forms and actions").
// Every mapping permission is anchor-only (auth.anchorWith), as the API's.
type EmailDomains struct {
	Store EmailDomainStore
	Ops   EmailDomainCommands
	// IdPs names the providers and says which are OIDC or multi-tenant.
	IdPs IdpStore
	// Clients resolves the primary client (its search and lookup);
	// ClientNames names the client ids.
	Clients     ClientDirectory
	ClientNames ClientStore
	Shell       *shell.Shell
}

const (
	edmPath   = "/ui/email-domains"
	edmFormID = "edm-list"

	edmDrawerID   = "edm-drawer"
	edmDrawerSig  = "selEdm"
	edmDialogSlot = "edm-dialog-slot"

	edmDeleteDialog = "edm-delete"

	// edmIdpFacet is the Identity Provider filter: the SPA's text, matched
	// against the provider's name.
	edmIdpFacet = "idp"
)

var edmList = &listSpec{
	path:        edmPath,
	sortKeys:    []string{"domain", "idp", "scope", "created"},
	statusOK:    func(s string) bool { _, ok := emaildomainmapping.ParseScopeType(s); return ok },
	facets:      []string{edmIdpFacet},
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *EmailDomains) Mount(r chi.Router) {
	r.Get("/email-domains", h.list)
	r.Get("/email-domains/new", h.createForm)
	r.Get("/email-domains/{id}", h.detail)
	r.Get("/email-domains/{id}/edit", h.editForm)
	post := r.With(parseForm)
	post.Post("/email-domains/new", h.create)
	post.Post("/email-domains/{id}/edit", h.update)
	post.Post("/email-domains/{id}/delete", h.delete)
}

// edmPerms are the API handlers' checks: create CanCreateEmailDomainMappings;
// update and the provider move CanUpdateEmailDomainMappings; delete
// CanDeleteEmailDomainMappings. The edit form also reads the mapping.
type edmPerms struct {
	Create, Update, Delete bool
}

func edmPermsFor(ac *auth.AuthContext) edmPerms {
	return edmPerms{
		Create: auth.CanCreateEmailDomainMappings(ac) == nil,
		Update: auth.CanUpdateEmailDomainMappings(ac) == nil,
		Delete: auth.CanDeleteEmailDomainMappings(ac) == nil,
	}
}

func edmURL(id string) string { return edmPath + "/" + url.PathEscape(id) }

// edmID reads and checks the {id} path parameter.
func edmID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("EmailDomainMapping", id)
	}
	return id, nil
}

// edmScopeSeverity is the SPA's getScopeTypeSeverity.
func edmScopeSeverity(scope string) kit.TagSeverity {
	switch scope {
	case "ANCHOR":
		return kit.TagDanger
	case "PARTNER":
		return kit.TagWarn
	case "CLIENT":
		return kit.TagInfo
	}
	return kit.TagSecondary
}

// edmProviders reads the providers once for a request: by id, and as the
// options of the provider select.
type edmProviders struct {
	byID    map[string]identityprovider.IdentityProvider
	options []kit.Option
}

func (h *EmailDomains) providers(ctx context.Context) (edmProviders, error) {
	all, err := h.IdPs.FindAll(ctx)
	if err != nil {
		return edmProviders{}, usecase.Internal("REPO", "find_all failed", err)
	}
	p := edmProviders{byID: make(map[string]identityprovider.IdentityProvider, len(all))}
	for _, ip := range all {
		p.byID[ip.ID] = ip
		p.options = append(p.options, kit.Option{Value: ip.ID, Label: ip.Name + " (" + ip.Code + ")"})
	}
	slices.SortFunc(p.options, func(a, b kit.Option) int { return cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	return p, nil
}

func (p edmProviders) name(id string) string {
	if ip, ok := p.byID[id]; ok {
		return ip.Name
	}
	return "Unknown"
}

// ── list ───────────────────────────────────────────────────────────────

func (h *EmailDomains) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// Same check as GET /api/email-domain-mappings.
	if err := auth.CanReadEmailDomainMappings(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), edmList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patchTable(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: email domains patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *EmailDomains) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (edmView, error) {
	mappings, err := h.Store.FindAll(ctx)
	if err != nil {
		return edmView{}, fmt.Errorf("find email domain mappings: %w", err)
	}
	providers, err := h.providers(ctx)
	if err != nil {
		return edmView{}, err
	}
	return buildEdmView(ac, q, mappings, providers, clientNamesOf(ctx, h.ClientNames, ac)), nil
}

func (h *EmailDomains) renderPage(w http.ResponseWriter, r *http.Request, status int, view edmView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Email Domains", Path: edmPath}, edmPage(view))
}

type edmRow struct {
	ID, Domain, Provider, Scope, Client string
	Created, CreatedISO                 string
	DetailHref, EditHref, ActionBase    string
}

type edmView struct {
	Query       listQuery
	Rows        []edmRow
	Total       int
	Page, Pages int
	Perms       edmPerms
	Dialog      templ.Component
	Drawer      *edmDetail
}

func (v edmView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(edmTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

func (v edmView) idpFilter() string {
	if vals := v.Query.facet(edmIdpFacet); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

func buildEdmView(ac *auth.AuthContext, q listQuery, mappings []emaildomainmapping.EmailDomainMapping, providers edmProviders, clients map[string]string) edmView {
	v := edmView{Query: q, Perms: edmPermsFor(ac)}
	needle := strings.ToLower(q.Q)
	idpNeedle := strings.ToLower(v.idpFilter())
	rows := make([]edmRow, 0, len(mappings))
	for _, m := range mappings {
		provider := providers.name(m.IdentityProviderID)
		// The SPA's quick filter: domain, scope, provider name; the Identity
		// Provider filter is a contains match on the provider's name.
		hay := strings.ToLower(m.EmailDomain + " " + string(m.ScopeType) + " " + provider)
		if (needle != "" && !strings.Contains(hay, needle)) || (q.Status != "" && string(m.ScopeType) != q.Status) ||
			(idpNeedle != "" && !strings.Contains(strings.ToLower(provider), idpNeedle)) {
			continue
		}
		row := edmRow{
			ID: m.ID, Domain: m.EmailDomain, Provider: provider, Scope: string(m.ScopeType),
			Created: m.CreatedAt.UTC().Format("2006-01-02"), CreatedISO: m.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			DetailHref: edmURL(m.ID), EditHref: edmURL(m.ID) + "/edit", ActionBase: edmURL(m.ID),
		}
		if id := ids.StringPtr(m.PrimaryClientID); id != nil {
			row.Client = clientLabel(clients, *id)
		}
		rows = append(rows, row)
	}
	slices.SortStableFunc(rows, func(a, b edmRow) int {
		var c int
		switch q.Sort {
		case "idp":
			c = cmp.Compare(strings.ToLower(a.Provider), strings.ToLower(b.Provider))
		case "scope":
			c = cmp.Compare(a.Scope, b.Scope)
		case "created":
			c = cmp.Compare(a.CreatedISO, b.CreatedISO)
		default:
			c = cmp.Compare(a.Domain, b.Domain)
		}
		if q.Dir == kit.SortDesc {
			c = -c
		}
		return c
	})
	v.Total = len(rows)
	page, pages, lo, hi := q.pageBounds(len(rows))
	v.Page, v.Pages = page, pages
	v.Rows = rows[lo:hi]
	return v
}

// rowActions: Edit inline (the SPA's pencil); View details and Delete in the
// menu. Left out when the user has no permission.
func (v edmView) rowActions(row edmRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "edm-" + row.ID + "-actions", RowLabel: row.Domain}
	if v.Perms.Update {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	if v.Perms.Delete {
		p.Menu = append(p.Menu, kit.RowAction{Icon: "trash-2", Label: "Delete…", Destructive: true, Attrs: confirmTrigger(edmDeleteDialog, row.ActionBase+"/delete")})
	}
	return p
}

// ── drawer ─────────────────────────────────────────────────────────────

type edmDetail struct {
	ID, Domain, Scope, Provider string
	Created, Updated            string
	// Client is the primary client's name (the id when it cannot be named);
	// shown for a CLIENT-scoped mapping, as the SPA's.
	Client string
	// OIDC reports the provider is external, MultiTenant that its issuer
	// varies per tenant (the tenant is then shown).
	OIDC, MultiTenant bool
	Tenant            string
	Require2FA        bool
	Methods           []string
	Remember          bool
	RememberDays      int
	EditHref          string
	ActionBase        string
	Perms             edmPerms
}

func (h *EmailDomains) toDetail(ctx context.Context, ac *auth.AuthContext, m *emaildomainmapping.EmailDomainMapping) edmDetail {
	d := edmDetail{
		ID: m.ID, Domain: m.EmailDomain, Scope: string(m.ScopeType), Provider: "Unknown",
		Created: m.CreatedAt.UTC().Format("2006-01-02 15:04:05 UTC"), Updated: m.UpdatedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		Tenant: deref(m.RequiredOIDCTenantID), Require2FA: m.Require2FA, Methods: m.Allowed2FAMethods,
		Remember: m.RememberDeviceEnabled, RememberDays: m.RememberDeviceDays,
		EditHref: edmURL(m.ID) + "/edit", ActionBase: edmURL(m.ID), Perms: edmPermsFor(ac),
	}
	if ip, err := h.IdPs.FindByID(ctx, m.IdentityProviderID); err != nil {
		slog.WarnContext(ctx, "ui: identity provider unavailable for the mapping drawer", "err", err)
	} else if ip != nil {
		d.Provider, d.OIDC, d.MultiTenant = ip.Name, ip.Type == identityprovider.TypeOIDC, ip.OIDCMultiTenant
	}
	if id := ids.StringPtr(m.PrimaryClientID); id != nil {
		d.Client = clientLabel(clientNamesOf(ctx, h.ClientNames, ac), *id)
		if d.Client == "" {
			d.Client = "Unknown"
		}
	}
	return d
}

func (d edmDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: edmDrawerID, Signal: edmDrawerSig, Label: "Email domain mapping", Title: d.Domain, Subtitle: "Identity Provider: " + d.Provider, Extras: edmScopeTag(d.Scope)}
}

// show2FA is the SPA's rule: two-factor applies to internal-auth domains
// only, so an OIDC-linked mapping shows none.
func (d edmDetail) show2FA() bool { return !d.OIDC }

func (d edmDetail) rememberText() string {
	if d.Remember {
		return "Allowed (" + strconv.Itoa(d.RememberDays) + " days)"
	}
	return "Off"
}

func edmMethodLabel(m string) string {
	if m == "TOTP" {
		return "Authenticator app"
	}
	return "Email code"
}

func (h *EmailDomains) load(ctx context.Context, id string) (*emaildomainmapping.EmailDomainMapping, error) {
	m, err := h.Store.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if m == nil {
		return nil, httperror.NotFound("EmailDomainMapping", id)
	}
	return m, nil
}

func (h *EmailDomains) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/email-domain-mappings/{id}'s check.
	if err := auth.CanReadEmailDomainMappings(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := edmID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	m, err := h.load(ctx, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	d := h.toDetail(ctx, ac, m)
	if ds.IsDatastar(r) {
		if err := openEdmDrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: email domain drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, edmList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func openEdmDrawer(sse *ds.SSE, d edmDetail) error {
	return errors.Join(
		sse.Patch(edmDrawerContent(d), ds.Inner("#"+edmDrawerID)),
		sse.PatchSignals(map[string]any{edmDrawerSig: d.ID}),
		sse.Focus(edmDrawerID+"-title"),
	)
}

// ── delete ─────────────────────────────────────────────────────────────

// delete removes a mapping (DELETE /api/email-domain-mappings/{id}).
func (h *EmailDomains) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanDeleteEmailDomainMappings(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := edmID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Ops.Delete(ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flashEdmDeleted)
		http.Redirect(w, r, edmPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, true)
	if err := flashKey(sse, flashEdmDeleted); err != nil {
		slog.DebugContext(ctx, "ui: email domain flash not delivered", "err", err)
	}
}

// afterChange re-renders the table the user is on and the drawer when it
// shows this mapping (closed after a delete).
func (h *EmailDomains) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, edmList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: email domains table patch not delivered", "err", err)
	}
	if r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{edmDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: email domain drawer close not delivered", "err", err)
		}
		return
	}
	m, err := h.load(ctx, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(edmDrawerContent(h.toDetail(ctx, ac, m)), ds.Inner("#"+edmDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: email domain drawer patch not delivered", "err", err)
	}
}

// edmDeleteConfirm is the SPA's delete dialog.
func edmDeleteConfirm() kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: edmDeleteDialog, Action: edmPath, Title: "Delete Email Domain Mapping",
		Message:      "Delete this mapping? Users from this domain will no longer be able to authenticate.",
		ConfirmLabel: "Delete", Tone: kit.ToneDanger,
		Hidden:    []kit.HiddenField{drawerField(edmDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
