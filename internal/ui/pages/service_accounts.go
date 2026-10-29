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

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	platformauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/connection"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount"
	saops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/serviceaccount/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// Flash keys of this area (shell.SetFlash after a native post). The SPA's
// roles and applications toasts count what changed; a flash key carries no
// request data.
const (
	flashSACreated           = "service-account-created"
	flashSAUpdated           = "service-account-updated"
	flashSADeleted           = "service-account-deleted"
	flashSARoles             = "service-account-roles-updated"
	flashSAApps              = "service-account-applications-updated"
	flashSAAllApps           = "service-account-all-applications"
	flashSARestricted        = "service-account-applications-restricted"
	flashSATokenRegenerated  = "service-account-token-regenerated"
	flashSASecretRegenerated = "service-account-secret-regenerated"
)

func init() {
	maps.Copy(FlashMessages, map[string]kit.FlashMessageProps{
		flashSACreated:           success("Service account created successfully"),
		flashSAUpdated:           success("Service account updated successfully"),
		flashSADeleted:           success("Service account deleted successfully"),
		flashSARoles:             success("Roles updated"),
		flashSAApps:              success("Application access updated"),
		flashSAAllApps:           success("Granted access to all applications"),
		flashSARestricted:        success("Restricted to specific applications"),
		flashSATokenRegenerated:  success("Auth token regenerated"),
		flashSASecretRegenerated: success("Signing secret regenerated"),
	})
}

// ServiceAccountStore is the slice of the service-account repository the page
// reads (GET /api/service-accounts, GET /api/service-accounts/{id}).
type ServiceAccountStore interface {
	FindAll(ctx context.Context) ([]serviceaccount.ServiceAccount, error)
	FindByID(ctx context.Context, id string) (*serviceaccount.ServiceAccount, error)
}

// ServicePrincipals finds the SERVICE principal linked to an account: it
// owns the account's roles and application access.
type ServicePrincipals interface {
	FindByServiceAccount(ctx context.Context, serviceAccountID string) (*principal.Principal, error)
}

// ServiceOAuthClients lists the OAuth clients of a principal (the drawer
// shows the public client id GET /api/service-accounts/{id} reports).
type ServiceOAuthClients interface {
	FindByPrincipalID(ctx context.Context, principalID string) ([]platformauth.OAuthClient, error)
}

// ServiceAccountRoles lists the roles an account can be given (GET /api/roles).
type ServiceAccountRoles interface {
	FindAll(ctx context.Context) ([]role.Role, error)
}

// ServiceAccounts is the service accounts page (/ui/service-accounts),
// mirroring the SPA's ServiceAccountListPage, ServiceAccountCreateDrawer and
// ServiceAccountDetailDrawer: the list, create with the shown-once
// credentials, and the drawer with edit, webhook credential regeneration,
// the admin token mint, roles, application access, connections and delete.
type ServiceAccounts struct {
	Accounts      ServiceAccountStore
	Ops           ServiceAccountCommands
	Principals    ServicePrincipals
	OAuthClients  ServiceOAuthClients
	Roles         ServiceAccountRoles
	Clients       ClientStore
	Apps          ApplicationStore
	Connections   ConnectionStore
	ConnectionOps ConnectionCommands
	Shell         *shell.Shell
}

const (
	saPath   = "/ui/service-accounts"
	saFormID = "sa-list"

	saDrawerID   = "sa-drawer"
	saDrawerSig  = "selSa"
	saDialogSlot = "sa-dialog-slot"

	saRegenTokenDialog  = "sa-regen-token"
	saRegenSecretDialog = "sa-regen-secret"
	saMintDialog        = "sa-mint"
	saDeleteDialog      = "sa-delete"
	saPauseConnDialog   = "sa-conn-pause"

	saLiveGet = "@get('/ui/service-accounts', {contentType: 'form'})"

	// saClientFacet is the SPA's single Client filter.
	saClientFacet = "client"
)

var saList = &listSpec{
	path:        saPath,
	sortKeys:    []string{"name", "code", "created"},
	statusOK:    func(s string) bool { return s == "ACTIVE" || s == "INACTIVE" },
	facets:      []string{saClientFacet},
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm).
func (h *ServiceAccounts) Mount(r chi.Router) {
	r.Get("/service-accounts", h.list)
	r.Get("/service-accounts/new", h.createForm)
	r.Get("/service-accounts/{id}", h.detail)
	r.Get("/service-accounts/{id}/edit", h.editForm)
	r.Get("/service-accounts/{id}/export", h.export)
	r.Get("/service-accounts/{id}/export.json", h.exportFile)
	r.Get("/service-accounts/{id}/roles", h.rolesForm)
	post := r.With(parseForm)
	post.Post("/service-accounts/new", h.create)
	post.Post("/service-accounts/new/import", h.importDefinition)
	post.Post("/service-accounts/{id}/edit", h.update)
	post.Post("/service-accounts/{id}/roles", h.saveRoles)
	post.Post("/service-accounts/{id}/applications", h.saveApplications)
	post.Post("/service-accounts/{id}/all-applications", h.toggleAllApplications)
	post.Post("/service-accounts/{id}/regenerate-token", h.regenerateToken)
	post.Post("/service-accounts/{id}/regenerate-secret", h.regenerateSecret)
	post.Post("/service-accounts/{id}/token", h.mintToken)
	post.Post("/service-accounts/{id}/delete", h.delete)
	post.Post("/service-accounts/{id}/connections/{connectionId}/pause", h.pauseConnection)
	post.Post("/service-accounts/{id}/connections/{connectionId}/activate", h.activateConnection)
}

// ── permissions ────────────────────────────────────────────────────────

// saPerms are the API's checks. Every write to a service account needs the
// permission for the operation AND anchor scope (owner decision #19), and an
// application-scoped caller only reaches the accounts of its applications
// (saops.AuthorizeWrite); create is the same without an account. Roles also
// need the role list (GET /api/roles: role:view), application access the
// principal API's permissions (user:view to read, user:update to write) and
// connections theirs.
type saPerms struct {
	Create, Update, Delete bool
	Roles                  bool
	AppsRead, AppsWrite    bool
	// GrantAll: only a caller holding all-applications access may grant it.
	GrantAll                            bool
	ConnsRead, ConnsUpdate, ConnsCreate bool
	ClientsRead                         bool
}

// saListPerms are the list-level permissions (no account in hand): what
// decides the shared confirmation dialogs; an account's own reach is
// saPermsFor's.
func saListPerms(ac *auth.AuthContext) saPerms {
	return saPerms{
		Create:      saops.AuthorizeCreate(ac, false, nil) == nil,
		Update:      saops.RequireAnchorWriter(ac, auth.CanUpdateServiceAccounts) == nil,
		Delete:      saops.RequireAnchorWriter(ac, auth.CanDeleteServiceAccounts) == nil,
		ConnsUpdate: auth.CanUpdateConnections(ac) == nil,
		ClientsRead: auth.CanReadClients(ac) == nil,
		GrantAll:    ac.AllApplications,
	}
}

// saPermsFor are the permissions on one account (its application decides
// whether an application-scoped caller reaches it).
func saPermsFor(ac *auth.AuthContext, sa *serviceaccount.ServiceAccount) saPerms {
	app := ids.StringPtr(sa.ApplicationID)
	p := saListPerms(ac)
	p.Update = saops.AuthorizeWrite(ac, app, auth.CanUpdateServiceAccounts) == nil
	p.Delete = saops.AuthorizeWrite(ac, app, auth.CanDeleteServiceAccounts) == nil
	p.Roles = p.Update && auth.CanReadAllRoles(ac)
	p.AppsRead = auth.CanReadPrincipals(ac) == nil
	p.AppsWrite = saops.AuthorizeWrite(ac, app, auth.CanUpdatePrincipals) == nil
	p.ConnsRead = auth.CanReadConnections(ac) == nil
	p.ConnsCreate = auth.CanCreateConnections(ac) == nil
	return p
}

// writable loads account id for a write: the permission and anchor gate
// first (before anything is read, so an unauthorised caller cannot tell a
// real id from an invented one), then the account and the application
// confinement.
func (h *ServiceAccounts) writable(ctx context.Context, ac *auth.AuthContext, id string, can func(*auth.AuthContext) error) (*serviceaccount.ServiceAccount, error) {
	if err := saops.RequireAnchorWriter(ac, can); err != nil {
		return nil, err
	}
	sa, err := h.Accounts.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return nil, httperror.NotFound("ServiceAccount", id)
	}
	if err := saops.RequireApplicationReach(ac, ids.StringPtr(sa.ApplicationID)); err != nil {
		return nil, err
	}
	return sa, nil
}

func saID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("ServiceAccount", id)
	}
	return id, nil
}

// ── list ───────────────────────────────────────────────────────────────

func (h *ServiceAccounts) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/service-accounts's check.
	if err := auth.CanReadServiceAccounts(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), saList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patchTable(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: service accounts patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

// saClients are the client names the page shows (the SPA's useClientOptions,
// which needs the client list: anchor with client:view); without it the
// columns show client ids and the Client filter is left out.
type saClients struct {
	names   map[string]string
	options []kit.Option
	// identifiers maps a client id to its identifier, byIdentifier the way
	// back (the definition export and import write clients by identifier).
	identifiers  map[string]string
	byIdentifier map[string]string
}

func (c saClients) label(id string) string {
	if n := c.names[id]; n != "" {
		return n
	}
	return id
}

func (h *ServiceAccounts) clients(ctx context.Context, ac *auth.AuthContext) saClients {
	if auth.CanReadClients(ac) != nil || h.Clients == nil {
		return saClients{}
	}
	all, err := h.Clients.FindAll(ctx)
	if err != nil {
		slog.WarnContext(ctx, "ui: clients unavailable for service accounts", "err", err)
		return saClients{}
	}
	c := saClients{names: make(map[string]string, len(all)), identifiers: make(map[string]string, len(all)), byIdentifier: make(map[string]string, len(all))}
	for _, cl := range all {
		c.names[string(cl.ID)] = cl.Name
		c.identifiers[string(cl.ID)] = cl.Identifier
		c.byIdentifier[cl.Identifier] = string(cl.ID)
		c.options = append(c.options, kit.Option{Value: string(cl.ID), Label: cl.Name})
	}
	slices.SortFunc(c.options, func(a, b kit.Option) int { return cmp.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	return c
}

func (h *ServiceAccounts) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (saView, error) {
	accounts, err := h.Accounts.FindAll(ctx)
	if err != nil {
		return saView{}, fmt.Errorf("find service accounts: %w", err)
	}
	return buildSAView(ac, q, accounts, h.clients(ctx, ac)), nil
}

func (h *ServiceAccounts) renderPage(w http.ResponseWriter, r *http.Request, status int, view saView) {
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: "Service Accounts", Path: saPath}, saPage(view))
}

type saRow struct {
	ID, Code, Name, AuthType string
	ClientsText              string
	Active                   bool
	Created, CreatedISO      string
	DetailHref, EditHref     string
	CanEdit                  bool
}

type saView struct {
	Query       listQuery
	Rows        []saRow
	Total       int
	Page, Pages int
	Perms       saPerms
	Clients     saClients
	Dialog      templ.Component
	Drawer      *saDetail
}

// clientsText is the SPA's Clients column: none = "All", one or two by name,
// more as the first and a count.
func (c saClients) text(clientIDs []string) string {
	switch len(clientIDs) {
	case 0:
		return "All"
	case 1, 2:
		names := make([]string, len(clientIDs))
		for i, id := range clientIDs {
			names[i] = c.label(id)
		}
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s +%d more", c.label(clientIDs[0]), len(clientIDs)-1)
}

func saStatus(sa serviceaccount.ServiceAccount) string {
	if sa.Active {
		return "ACTIVE"
	}
	return "INACTIVE"
}

// saAuthType is the SPA's Auth Type tag (`authType || 'BEARER'`).
func saAuthType(sa serviceaccount.ServiceAccount) string {
	return cmp.Or(string(sa.WebhookCredentials.AuthType), "BEARER")
}

// buildSAView applies the SPA's table filters (the quick search over name and
// code, the single Client and the Status filters), sort and paging.
func buildSAView(ac *auth.AuthContext, q listQuery, accounts []serviceaccount.ServiceAccount, clients saClients) saView {
	v := saView{Perms: saListPerms(ac), Clients: clients}
	// The Client filter exists only when the clients are known; a value that
	// is not one of them is dropped from the canonical URL.
	clientID := ""
	if vals := q.facet(saClientFacet); len(vals) > 0 && clients.names[vals[0]] != "" {
		clientID = vals[0]
	}
	q = q.withFacet(saClientFacet, nil)
	if clientID != "" {
		q = q.withFacet(saClientFacet, []string{clientID})
	}
	needle := strings.ToLower(q.Q)
	shown := make([]serviceaccount.ServiceAccount, 0, len(accounts))
	for _, sa := range accounts {
		if clientID != "" && !slices.Contains(sa.ClientIDs, clientID) {
			continue
		}
		if q.Status != "" && saStatus(sa) != q.Status {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(sa.Name+" "+sa.Code), needle) {
			continue
		}
		shown = append(shown, sa)
	}
	slices.SortStableFunc(shown, func(a, b serviceaccount.ServiceAccount) int {
		var c int
		switch q.Sort {
		case "code":
			c = cmp.Compare(a.Code, b.Code)
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
	v.Total = len(shown)
	var lo, hi int
	q.Page, v.Pages, lo, hi = q.pageBounds(len(shown))
	v.Page, v.Query = q.Page, q
	for i := lo; i < hi; i++ {
		sa := &shown[i]
		v.Rows = append(v.Rows, saRow{
			ID: sa.ID, Code: sa.Code, Name: sa.Name, AuthType: saAuthType(*sa), ClientsText: clients.text(sa.ClientIDs), Active: sa.Active,
			Created: sa.CreatedAt.UTC().Format("2006-01-02"), CreatedISO: sa.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			DetailHref: saURL(sa.ID), EditHref: saURL(sa.ID) + "/edit", CanEdit: saPermsFor(ac, sa).Update,
		})
	}
	return v
}

func saURL(id string) string { return saPath + "/" + url.PathEscape(id) }

func (v saView) rowActions(row saRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "sa-" + row.ID + "-actions", RowLabel: row.Name}
	if row.CanEdit {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Actions = append(p.Actions, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	return p
}

func (v saView) patchTable(sse *ds.SSE) error {
	return errors.Join(sse.Patch(saTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

// ── drawer ─────────────────────────────────────────────────────────────

type saRoleRow struct {
	Short, Full, Source, Assigned string
}

type saAppRow struct{ Name, Code string }

type saConnRow struct {
	ID, Code, Name, Status string
	Active                 bool
	Href, ActionBase       string
}

type saDetail struct {
	ID, Code, Name, Description string
	Active                      bool
	PrincipalID, OAuthClientID  string
	Scope                       string
	ScopeSeverity               kit.TagSeverity
	ClientAccess                string
	ShowClientAccess            bool
	AuthType                    string
	Created, LastUsed           string
	EditHref, ActionBase        string
	Perms                       saPerms

	Roles []saRoleRow
	// RolesNote says why the roles cannot be managed (empty when they can).
	RolesNote string

	// HasPrincipal: the account has a linked SERVICE principal, which owns
	// its application access.
	HasPrincipal bool
	AppsVisible  bool
	// AppsNote says why the section is not shown (empty when it is).
	AppsNote        string
	AllApplications bool
	Apps            []saAppRow
	AppPicker       kit.PickListProps
	AppPickerOn     bool

	ConnsVisible bool
	Conns        []saConnRow
}

func (d saDetail) drawerProps() kit.DrawerProps {
	return kit.DrawerProps{ID: saDrawerID, Signal: saDrawerSig, Label: "Service account", Size: kit.DrawerWide, Title: d.Name, Subtitle: d.Code, Extras: saActiveTag(d.Active)}
}

// loadAccount reads an account with GET /api/service-accounts/{id}'s checks.
func (h *ServiceAccounts) loadAccount(ctx context.Context, ac *auth.AuthContext, id string) (*serviceaccount.ServiceAccount, error) {
	if err := auth.CanReadServiceAccounts(ac); err != nil {
		return nil, err
	}
	sa, err := h.Accounts.FindByID(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_id failed", err)
	}
	if sa == nil {
		return nil, httperror.NotFound("ServiceAccount", id)
	}
	return sa, nil
}

// scopeOf is the account's scope as the SPA shows it: the stored value, or
// "N/A" when none was stored.
func saScopeSeverity(scope string) kit.TagSeverity {
	switch scope {
	case "ANCHOR":
		return kit.TagSuccess
	case "PARTNER":
		return kit.TagInfo
	}
	return kit.TagWarn
}

// detailOf builds the drawer with what the SPA loads for it: the account, its
// roles, application access and connections. A section the user's
// permissions cannot read is left out with a note, as the SPA's failed loads
// leave it empty.
func (h *ServiceAccounts) detailOf(ctx context.Context, ac *auth.AuthContext, sa *serviceaccount.ServiceAccount) (saDetail, error) {
	d := saDetail{
		ID: sa.ID, Code: sa.Code, Name: sa.Name, Description: deref(sa.Description), Active: sa.Active,
		Scope: cmp.Or(deref(sa.Scope), "N/A"), AuthType: saAuthType(*sa),
		Created: sa.CreatedAt.UTC().Format("2006-01-02"), LastUsed: "—",
		EditHref: saURL(sa.ID) + "/edit", ActionBase: saURL(sa.ID), Perms: saPermsFor(ac, sa),
	}
	d.ScopeSeverity = saScopeSeverity(deref(sa.Scope))
	if sa.LastUsedAt != nil {
		d.LastUsed = sa.LastUsedAt.UTC().Format("2006-01-02")
	}
	clients := h.clients(ctx, ac)
	d.ShowClientAccess = deref(sa.Scope) != "ANCHOR"
	if len(sa.ClientIDs) == 0 {
		d.ClientAccess = "All clients (no restriction)"
	} else {
		labels := make([]string, len(sa.ClientIDs))
		for i, id := range sa.ClientIDs {
			labels[i] = clients.label(id)
		}
		d.ClientAccess = strings.Join(labels, ", ")
	}

	p, err := h.Principals.FindByServiceAccount(ctx, sa.ID)
	if err != nil {
		return saDetail{}, usecase.Internal("REPO", "find_by_service_account failed", err)
	}
	if p != nil {
		d.HasPrincipal = true
		d.PrincipalID = string(p.ID)
		if ocs, err := h.OAuthClients.FindByPrincipalID(ctx, string(p.ID)); err == nil && len(ocs) > 0 {
			d.OAuthClientID = ocs[0].ClientID
		}
		for _, ra := range p.Roles {
			d.Roles = append(d.Roles, saRoleRow{
				Short: saLastSegment(ra.Role.String()), Full: ra.Role.String(), Source: deref(ra.AssignmentSource),
				Assigned: ra.AssignedAt.UTC().Format("2006-01-02"),
			})
		}
		if err := h.appAccess(ctx, ac, &d, p); err != nil {
			return saDetail{}, err
		}
	}
	if !d.Perms.Roles {
		switch {
		case !d.Perms.Update:
			d.RolesNote = "Changing roles needs the service-account update permission and anchor scope."
		default:
			d.RolesNote = "Changing roles needs permission to view roles."
		}
	}

	if d.Perms.ConnsRead && h.Connections != nil {
		conns, err := h.Connections.FindWithFilters(ctx, nil, nil)
		if err != nil {
			slog.WarnContext(ctx, "ui: connections unavailable for service account", "err", err)
		} else {
			d.ConnsVisible = true
			visible := auth.FilterClientScoped(ac, conns, func(c *connection.Connection) *string { return ids.StringPtr(c.ClientID) })
			for _, c := range visible {
				if c.ServiceAccountID != sa.ID {
					continue
				}
				d.Conns = append(d.Conns, saConnRow{
					ID: c.ID, Code: c.Code, Name: c.Name, Status: string(c.Status), Active: c.Status == connection.StatusActive,
					Href: connsPath + "/" + url.PathEscape(c.ID), ActionBase: d.ActionBase + "/connections/" + url.PathEscape(c.ID),
				})
			}
		}
	}
	return d, nil
}

// appAccess fills the Application Access section from the linked principal
// (the SPA reads GET /api/principals/{id}/application-access and
// .../available-applications: user:view).
func (h *ServiceAccounts) appAccess(ctx context.Context, ac *auth.AuthContext, d *saDetail, p *principal.Principal) error {
	if !d.Perms.AppsRead || h.Apps == nil {
		d.AppsNote = "Viewing application access needs permission to view users."
		return nil
	}
	// The principal API's client-scope check on its application-access reads.
	if p.ClientID != nil && !ac.CanAccessClient(string(*p.ClientID)) {
		d.AppsNote = "This service account's application access belongs to a client you cannot access."
		return nil
	}
	d.AppsVisible = true
	d.AllApplications = p.AllApplications
	all, err := h.Apps.FindWithFilters(ctx, nil, nil)
	if err != nil {
		return usecase.Internal("REPO", "find applications failed", err)
	}
	byID := make(map[string]application.Application, len(all))
	for _, a := range all {
		byID[string(a.ID)] = a
	}
	granted := ids.Strings(p.AccessibleApplicationIDs)
	for _, id := range granted {
		// An id that no longer resolves is skipped, as the API does.
		if a, ok := byID[id]; ok {
			d.Apps = append(d.Apps, saAppRow{Name: a.Name, Code: a.Code})
		}
	}
	if !d.Perms.AppsWrite || d.AllApplications {
		return nil
	}
	items := make([]kit.PickItem, 0, len(all))
	for _, a := range all {
		if a.Active {
			items = append(items, kit.PickItem{Value: string(a.ID), Name: a.Name, Code: a.Code, Checked: slices.Contains(granted, string(a.ID))})
		}
	}
	slices.SortFunc(items, func(a, b kit.PickItem) int { return cmp.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	d.AppPickerOn = true
	d.AppPicker = kit.PickListProps{
		ID: "sa-apps", Signal: "saApps", FilterSignal: "saAppsQ", Name: "applicationIds", ItemIDPrefix: "sa-app-",
		Items: items, AvailableTitle: "Available (" + fmt.Sprint(len(items)) + ")", SelectedTitle: "Selected",
		FilterPlaceholder: "Filter applications", EmptyText: "No applications selected", SaveLabel: "Save Application Access",
		Action: d.ActionBase + "/applications", FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitForm},
	}
	return nil
}

func saLastSegment(name string) string {
	if _, last, found := strings.CutLast(name, ":"); found {
		return last
	}
	return name
}

func (h *ServiceAccounts) detail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := saID(r)
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
		if err := openSADrawer(ds.NewSSE(w, r), d); err != nil {
			slog.DebugContext(ctx, "ui: service account drawer patch not delivered", "err", err)
		}
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(nil, saList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view.Drawer = &d
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *ServiceAccounts) drawerFor(ctx context.Context, ac *auth.AuthContext, id string) (saDetail, error) {
	sa, err := h.loadAccount(ctx, ac, id)
	if err != nil {
		return saDetail{}, err
	}
	return h.detailOf(ctx, ac, sa)
}

func openSADrawer(sse *ds.SSE, d saDetail) error {
	return errors.Join(
		sse.Patch(saDrawerContent(d), ds.Inner("#"+saDrawerID)),
		sse.PatchSignals(map[string]any{saDrawerSig: d.ID}),
		sse.Focus(saDrawerID+"-title"),
	)
}

// refreshDrawer re-renders the drawer for id.
func (h *ServiceAccounts) refreshDrawer(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string) {
	ctx := r.Context()
	d, err := h.drawerFor(ctx, ac, id)
	if err != nil {
		h.Shell.FailSSE(sse, r, err)
		return
	}
	if err := sse.Patch(saDrawerContent(d), ds.Inner("#"+saDrawerID)); err != nil {
		slog.DebugContext(ctx, "ui: service account drawer patch not delivered", "err", err)
	}
}

// afterChange re-renders what a change touched: the table the user is on and
// the drawer when it shows this account (closed after a delete). refresh
// forces the drawer's refresh for forms that are only ever in the drawer.
func (h *ServiceAccounts) afterChange(sse *ds.SSE, r *http.Request, ac *auth.AuthContext, id string, deleted, refresh bool) {
	ctx := r.Context()
	if view, err := h.listView(ctx, ac, refererList(r, saList)); err != nil {
		h.Shell.FailSSE(sse, r, err)
	} else if err := view.patchTable(sse); err != nil {
		slog.DebugContext(ctx, "ui: service accounts table patch not delivered", "err", err)
	}
	if !refresh && r.PostFormValue("drawer") != id {
		return
	}
	if deleted {
		if err := sse.PatchSignals(map[string]any{saDrawerSig: ""}); err != nil {
			slog.DebugContext(ctx, "ui: service account drawer close not delivered", "err", err)
		}
		return
	}
	h.refreshDrawer(sse, r, ac, id)
}

// ── actions ────────────────────────────────────────────────────────────

// done answers a successful change: the table, the drawer and a flash; a
// native post redirects to the list with the flash key.
func (h *ServiceAccounts) done(w http.ResponseWriter, r *http.Request, ac *auth.AuthContext, id, flash string, deleted, refresh bool) {
	ctx := r.Context()
	if !ds.IsDatastar(r) {
		h.Shell.SetFlash(w, flash)
		http.Redirect(w, r, saPath, http.StatusSeeOther)
		return
	}
	sse := ds.NewSSE(w, r)
	h.afterChange(sse, r, ac, id, deleted, refresh)
	if err := flashKey(sse, flash); err != nil {
		slog.DebugContext(ctx, "ui: service account flash not delivered", "err", err)
	}
}

func (h *ServiceAccounts) delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := saID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if _, err := h.writable(ctx, ac, id, auth.CanDeleteServiceAccounts); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Ops.Delete(ctx, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, flashSADeleted, true, false)
}

// ── application access ────────────────────────────────────────────────

// appAccessTarget gates a write to the account's application access, which
// the SPA drives through the principal API: user:update, and here also the
// account gate (anchor reach and application confinement; the SPA's drawer
// is the account's, so a client administrator cannot reach it).
func (h *ServiceAccounts) appAccessTarget(ctx context.Context, ac *auth.AuthContext, id string) (*principal.Principal, error) {
	if _, err := h.writable(ctx, ac, id, auth.CanUpdatePrincipals); err != nil {
		return nil, err
	}
	p, err := h.Principals.FindByServiceAccount(ctx, id)
	if err != nil {
		return nil, usecase.Internal("REPO", "find_by_service_account failed", err)
	}
	if p == nil {
		return nil, httperror.NotFound("Principal", id)
	}
	return p, nil
}

// saveApplications replaces the account's applications (the pick list's Save:
// PUT /api/principals/{id}/application-access without allApplications).
func (h *ServiceAccounts) saveApplications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := saID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.appAccessTarget(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := h.Ops.SetApplicationAccess(ctx, string(p.ID), saPosted(r.PostForm["applicationIds"]), nil); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, flashSAApps, false, true)
}

// toggleAllApplications sets the account's all-applications flag, keeping its
// explicit grants so switching back restores them (the SPA's switch). The
// principal API's rules: only a caller holding all-applications access may
// grant it, and removing one's own needs a confirmation the SPA's switch
// never sends, so it is refused.
func (h *ServiceAccounts) toggleAllApplications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	id, err := saID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	p, err := h.appAccessTarget(ctx, ac, id)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	on := r.PostFormValue("allApplications") == "true"
	if on && !ac.AllApplications {
		h.Shell.Fail(w, r, httperror.Forbidden("Only an all-applications administrator may grant all-applications access"))
		return
	}
	if !on && ac.PrincipalID == string(p.ID) && p.AllApplications {
		h.Shell.Fail(w, r, usecase.Conflict("SELF_RESTRICTION_REQUIRES_CONFIRMATION",
			"You are removing your own all-applications access: you will no longer be able to grant "+
				"all-applications access to anyone (including yourself)."))
		return
	}
	if err := h.Ops.SetApplicationAccess(ctx, string(p.ID), ids.Strings(p.AccessibleApplicationIDs), &on); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	flash := flashSARestricted
	if on {
		flash = flashSAAllApps
	}
	h.done(w, r, ac, id, flash, false, true)
}

// saPosted is a multi-valued field as posted: trimmed, without blanks and
// repeats.
func saPosted(raw []string) []string {
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

// ── connections ───────────────────────────────────────────────────────

// connectionOf is the account's connection connectionId (any other id is
// not found: the route must not act on a connection of another account).
func (h *ServiceAccounts) connectionOf(ctx context.Context, sa string, r *http.Request) (string, error) {
	cid := chi.URLParam(r, "connectionId")
	if !kit.ValidID(cid) {
		return "", httperror.NotFound("Connection", cid)
	}
	c, err := h.Connections.FindByID(ctx, cid)
	if err != nil {
		return "", usecase.Internal("REPO", "find_by_id failed", err)
	}
	if c == nil || c.ServiceAccountID != sa {
		return "", httperror.NotFound("Connection", cid)
	}
	return cid, nil
}

func (h *ServiceAccounts) pauseConnection(w http.ResponseWriter, r *http.Request) {
	h.connectionAction(w, r, flashConnPaused, ConnectionCommands.Pause)
}

func (h *ServiceAccounts) activateConnection(w http.ResponseWriter, r *http.Request) {
	h.connectionAction(w, r, flashConnActivated, ConnectionCommands.Activate)
}

// connectionAction pauses or activates one of the account's connections: the
// connections API's check (connection:update), then the connection use case.
func (h *ServiceAccounts) connectionAction(w http.ResponseWriter, r *http.Request, flash string, run func(ConnectionCommands, context.Context, string) error) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	if err := auth.CanUpdateConnections(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	id, err := saID(r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if _, err := h.loadAccount(ctx, ac, id); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	cid, err := h.connectionOf(ctx, id, r)
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if err := run(h.ConnectionOps, ctx, cid); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	h.done(w, r, ac, id, flash, false, true)
}

func saConfirm(id, title, message, label string, tone kit.ConfirmTone) kit.ConfirmDialogProps {
	return kit.ConfirmDialogProps{
		ID: id, Action: saPath, Title: title, Message: message, ConfirmLabel: label, Tone: tone,
		Hidden:    []kit.HiddenField{drawerField(saDrawerSig)},
		FormAttrs: templ.Attributes{"data-on:submit": kit.SubmitConfirm},
	}
}
