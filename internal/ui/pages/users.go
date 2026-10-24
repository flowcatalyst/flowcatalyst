package pages

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/a-h/templ"
	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/kit"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/shell"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// UserService is the principal API the users page runs: each method is the
// route's handler (principalapi.State), called in-process with the caller on
// ctx, so the permission checks, the client scoping, a client administrator's
// bounds, the role ceiling and the last-administrator guard are the API's.
type UserService interface {
	ListPrincipals(ctx context.Context, q principalapi.ListQuery) (principalapi.PrincipalListResponse, error)
	GetPrincipal(ctx context.Context, id string) (principalapi.PrincipalResponse, error)
	CheckEmailDomain(ctx context.Context, email string) (principalapi.CheckEmailDomainResponse, error)
	CreateDomainUser(ctx context.Context, body principalapi.CreateUserRequest) (principalapi.PrincipalResponse, error)
	CreatePrincipal(ctx context.Context, body principalapi.CreatePrincipalRequest) (principalapi.CreatePrincipalResponse, error)
	UpdatePrincipal(ctx context.Context, id string, body principalapi.UpdatePrincipalRequest) (principalapi.PrincipalResponse, error)
	ActivatePrincipal(ctx context.Context, id string) error
	DeactivatePrincipal(ctx context.Context, id string) error
	ResetPrincipalPassword(ctx context.Context, id string, body principalapi.ResetPasswordRequest) error
	SendPrincipalPasswordReset(ctx context.Context, id string) error
	ResetPrincipalTwoFactor(ctx context.Context, id string) error
	DeletePrincipal(ctx context.Context, id string) error
	ListPrincipalRoles(ctx context.Context, id string) ([]principalapi.PrincipalRoleAssignmentDTO, error)
	AssignPrincipalRoles(ctx context.Context, id string, roles []string) (principalapi.RolesAssignedResponse, error)
	ListPrincipalApplicationAccess(ctx context.Context, id string) (principalapi.ApplicationAccessListResponse, error)
	ListPrincipalAvailableApplications(ctx context.Context, id string) ([]principalapi.PrincipalAvailableApplication, error)
	AssignPrincipalApplicationAccess(ctx context.Context, id string, body principalapi.AssignApplicationAccessRequest) (principalapi.SetApplicationAccessResponse, error)
	ListPrincipalClientAccess(ctx context.Context, id string) ([]principalapi.ClientAccessGrantResponse, error)
	GrantPrincipalClientAccess(ctx context.Context, id, clientID string) error
	RevokePrincipalClientAccess(ctx context.Context, id, clientID string) error
	SetPrincipalClientAssociation(ctx context.Context, id string, body principalapi.ClientAssociationRequest) (principalapi.PrincipalResponse, error)
	BulkImportPrincipals(ctx context.Context, body principalapi.BulkImportRequest) (principalapi.BulkImportResponse, error)
}

// UserRoles lists the roles the role picker offers (GET /bff/roles).
type UserRoles interface {
	FindAll(ctx context.Context) ([]role.Role, error)
}

// Users is the users page (/ui/users), mirroring the SPA's UserListPage,
// UserCreateDrawer, UserDetailDrawer/UserDetailBody and, for a client
// administrator, ClientUsersPage and its drawers: the list, create and CSV
// import, and the record drawer with its roles, application access, client
// access and account actions. A platform administrator (anchor) manages every
// user; anyone else the CLIENT-scope users of the clients it can access, with
// the platform-only controls left out. Every action is the API handler's
// (UserService) and is checked there again: a control that is not shown is
// not the guard. No response carries a password or a stored credential; a
// sign-in link is in the response that minted it, only when asked for.
type Users struct {
	Users UserService
	// Roles is optional: without it the role picker and the roles filter are
	// not offered.
	Roles   UserRoles
	Clients ClientStore
	Shell   *shell.Shell
}

const (
	usersPath   = "/ui/users"
	usrFormID   = "usr-list"
	usrDrawerID = "usr-drawer"
	// usrDrawerSig holds the user the drawer shows.
	usrDrawerSig  = "selUsr"
	usrDialogSlot = "usr-dialog-slot"
	// usrSecretSlot receives the one-time sign-in link and the import report.
	usrSecretSlot = "usr-secret-slot"

	usrLiveGet = "@get('/ui/users', {contentType: 'form'})"

	usrFacetClient = "client"
	usrFacetRoles  = "roles"

	usrSendResetDialog = "usr-send-reset"
	usrReset2FADialog  = "usr-reset-2fa"
	usrDeleteDialog    = "usr-delete"
	usrRevokeDialog    = "usr-revoke"
	usrSelfDialog      = "usr-self-restrict"
)

// usrList is the list's URL state: the SPA's filters (search, client, status,
// roles) and sorts (name, email, created), 100 rows by default.
var usrList = &listSpec{
	path:        usersPath,
	sortKeys:    []string{"created", "name", "email"},
	statusOK:    func(s string) bool { return s == "active" || s == "inactive" },
	facets:      []string{usrFacetClient, usrFacetRoles},
	rows:        []int{50, 100, 250, 500},
	defaultRows: 100,
}

// Mount registers the page's routes (POSTs behind parseForm). GET routes
// never change state: dialogs are fetched with GET, written with POST.
func (h *Users) Mount(r chi.Router) {
	r.Get("/users", h.list)
	r.Get("/users/new", h.createForm)
	r.Get("/users/check-email", h.checkEmail)
	r.Get("/users/import", h.importForm)
	r.Get("/users/import/template.csv", h.importTemplate)
	r.Get("/users/{id}", h.detail)
	r.Get("/users/{id}/edit", h.editForm)
	r.Get("/users/{id}/roles", h.rolesForm)
	r.Get("/users/{id}/applications", h.appsForm)
	r.Get("/users/{id}/clients/new", h.grantForm)
	r.Get("/users/{id}/reset-password", h.passwordForm)
	post := r.With(parseForm)
	post.Post("/users/new", h.create)
	post.Post("/users/import", h.importCSV)
	post.Post("/users/{id}/edit", h.update)
	post.Post("/users/{id}/activate", h.activate)
	post.Post("/users/{id}/deactivate", h.deactivate)
	post.Post("/users/{id}/delete", h.delete)
	post.Post("/users/{id}/send-reset", h.sendReset)
	post.Post("/users/{id}/reset-password", h.resetPassword)
	post.Post("/users/{id}/reset-2fa", h.resetTwoFactor)
	post.Post("/users/{id}/roles", h.saveRoles)
	post.Post("/users/{id}/applications", h.saveApps)
	post.Post("/users/{id}/all-applications/grant", h.grantAllApps)
	post.Post("/users/{id}/all-applications/restrict", h.restrictAllApps)
	post.Post("/users/{id}/clients", h.grantClient)
	post.Post("/users/{id}/clients/{clientId}/revoke", h.revokeClient)
}

// ── permissions ────────────────────────────────────────────────────────

// usrPerms are what the viewer is offered; the API decides again on every
// post. Anchor stands for the platform administrator's controls (scope and
// client association, client access, all-applications access, delete): the
// SPA shows them on its platform page and leaves them off the client
// administrator's.
type usrPerms struct {
	Anchor                               bool
	Create, Update, Delete, AssignRoles  bool
	ReadRoles, ReadClients, GrantAllApps bool
	ClientAdmin                          bool
}

func usrPermsFor(ac *auth.AuthContext) usrPerms {
	return usrPerms{
		Anchor:      ac.IsAnchor(),
		ClientAdmin: !ac.IsAnchor(),
		Create:      auth.CanCreatePrincipals(ac) == nil,
		Update:      auth.CanUpdatePrincipals(ac) == nil,
		Delete:      auth.CanDeletePrincipals(ac) == nil,
		AssignRoles: auth.CanAssignPrincipalRoles(ac) == nil,
		ReadRoles:   auth.CanReadRoles(ac) == nil,
		ReadClients: auth.CanReadClients(ac) == nil,
		// Only an assigner holding all-applications access may grant it.
		GrantAllApps: ac != nil && ac.AllApplications,
	}
}

// ── client names ───────────────────────────────────────────────────────

// usrClients are the clients by id (the list's and the drawer's labels).
type usrClients map[string]client.Client

func (h *Users) clientIndex(ctx context.Context) (usrClients, error) {
	n := usrClients{}
	if h.Clients == nil {
		return n, nil
	}
	cs, err := h.Clients.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find clients failed", err)
	}
	for i := range cs {
		n[string(cs[i].ID)] = cs[i]
	}
	return n, nil
}

// label is the SPA's getClientLabel: the client's name, its id when unknown.
func (n usrClients) label(id string) string { return cmp.Or(n[id].Name, id) }

// clientChoices are the active clients the caller can access (the SPA's
// filter options and the create form's picker), by name.
func (h *Users) clientChoices(ctx context.Context, ac *auth.AuthContext) ([]client.Client, error) {
	if h.Clients == nil {
		return nil, nil
	}
	all, err := h.Clients.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "find clients failed", err)
	}
	var out []client.Client
	for i := range all {
		if all[i].Status == client.StatusActive && (ac.IsAnchor() || ac.CanAccessClient(string(all[i].ID))) {
			out = append(out, all[i])
		}
	}
	slices.SortFunc(out, func(a, b client.Client) int { return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) })
	return out, nil
}

func clientOptionsOf(cs []client.Client) []kit.Option {
	opts := make([]kit.Option, 0, len(cs))
	for i := range cs {
		opts = append(opts, kit.Option{Value: string(cs[i].ID), Label: cs[i].Name + " (" + cs[i].Identifier + ")"})
	}
	return opts
}

// ── list ───────────────────────────────────────────────────────────────

func (h *Users) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ac := auth.FromContext(ctx)
	// GET /api/principals's check.
	if err := auth.CanReadPrincipals(ac); err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	view, err := h.listView(ctx, ac, parseListQuery(r.URL.Query(), usrList))
	if err != nil {
		h.Shell.Fail(w, r, err)
		return
	}
	if ds.IsDatastar(r) {
		sse := ds.NewSSE(w, r)
		if err := errors.Join(view.patch(sse), sse.Patch(kit.TableToolbarState(view.toolbar().State())), sse.ReplaceURL(view.Query.URL())); err != nil {
			slog.DebugContext(ctx, "ui: users patch not delivered", "err", err)
		}
		return
	}
	h.renderPage(w, r, http.StatusOK, view)
}

func (h *Users) renderPage(w http.ResponseWriter, r *http.Request, status int, view usrView) {
	title := "Users"
	if view.Perms.ClientAdmin {
		title = "User Management"
	}
	h.Shell.RenderStatus(w, r, status, shell.Page{Title: title, Path: usersPath}, usrPage(view))
}

// usrCell is the SPA's client column: every client, a named client with the
// number of others, or none.
type usrCell struct {
	All  bool
	Name string
	More int
}

type usrRow struct {
	ID, Name, Email string
	// TypeLabel is the SPA's getUserType; TypeTip its tooltip.
	TypeLabel string
	TypeBadge kit.BadgeVariant
	TypeTip   string
	Anchor    bool
	Client    usrCell
	Active    bool
	// Roles are the first two role short names; MoreRoles the rest.
	Roles                []string
	MoreRoles            int
	Created, CreatedISO  string
	DetailHref, EditHref string
}

type usrView struct {
	Query       listQuery
	Rows        []usrRow
	Total       int
	Page, Pages int
	// Clients and Roles are the filters' options.
	Clients, Roles []kit.Option
	Perms          usrPerms
	// Dialog is a form dialog rendered with the page (a navigation to a
	// dialog's address, or a native post re-rendered with errors); Secret a
	// shown-once answer (the sign-in link, the import report); Drawer the
	// record the drawer opens with.
	Dialog templ.Component
	Secret templ.Component
	Drawer *usrDetail
}

// usrSortField maps the URL's sort key to the API's sortField.
func usrSortField(key string) string {
	if key == "created" {
		return "createdAt"
	}
	return key
}

// rolesFilter is the roles the filter offers (the SPA's rolesApi.list()), by
// display name.
func (h *Users) roleOptions(ctx context.Context, ac *auth.AuthContext) ([]kit.Option, error) {
	if h.Roles == nil || auth.CanReadRoles(ac) != nil {
		return nil, nil
	}
	rs, err := h.Roles.FindAll(ctx)
	if err != nil {
		return nil, usecase.Internal("REPO", "list roles failed", err)
	}
	opts := make([]kit.Option, 0, len(rs))
	for i := range rs {
		opts = append(opts, kit.Option{Value: rs[i].Name, Label: cmp.Or(rs[i].DisplayName, rs[i].Name)})
	}
	slices.SortFunc(opts, func(a, b kit.Option) int { return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) })
	return opts, nil
}

func (h *Users) listView(ctx context.Context, ac *auth.AuthContext, q listQuery) (usrView, error) {
	v := usrView{Perms: usrPermsFor(ac)}
	clients, err := h.clientChoices(ctx, ac)
	if err != nil {
		return usrView{}, err
	}
	v.Clients = make([]kit.Option, 0, len(clients))
	for i := range clients {
		v.Clients = append(v.Clients, kit.Option{Value: string(clients[i].ID), Label: clients[i].Name})
	}
	if v.Roles, err = h.roleOptions(ctx, ac); err != nil {
		return usrView{}, err
	}
	q = q.withFacet(usrFacetClient, prune(q.facet(usrFacetClient), v.Clients)).withFacet(usrFacetRoles, prune(q.facet(usrFacetRoles), v.Roles))
	query := principalapi.ListQuery{
		Type: "USER", Q: q.Q, Roles: strings.Join(q.facet(usrFacetRoles), ","),
		SortField: usrSortField(q.Sort), SortOrder: string(q.Dir), PageSize: q.Rows,
	}
	if c := q.facet(usrFacetClient); len(c) > 0 {
		query.ClientID = c[0]
	}
	switch q.Status {
	case "active":
		query.Active = "true"
	case "inactive":
		query.Active = "false"
	}
	query.Page = q.Page - 1
	res, err := h.Users.ListPrincipals(ctx, query)
	if err != nil {
		return usrView{}, err
	}
	// A page past the end shows the last one.
	var pages int
	q.Page, pages, _, _ = q.pageBounds(res.Total)
	if query.Page != q.Page-1 {
		query.Page = q.Page - 1
		if res, err = h.Users.ListPrincipals(ctx, query); err != nil {
			return usrView{}, err
		}
	}
	names, err := h.clientIndex(ctx)
	if err != nil {
		return usrView{}, err
	}
	v.Total, v.Pages, v.Page, v.Query = res.Total, pages, q.Page, q
	for i := range res.Principals {
		v.Rows = append(v.Rows, toUsrRow(&res.Principals[i], names))
	}
	return v, nil
}

func usrURL(id string) string { return usersPath + "/" + url.PathEscape(id) }

// usrType is the SPA's getUserType for a list row.
func usrType(u *principalapi.PrincipalResponse, names usrClients) (label string, badge kit.BadgeVariant, tip string) {
	if u.IsAnchorUser {
		return "Anchor", kit.BadgeWarn, "Has access to all clients via anchor domain"
	}
	granted := len(u.GrantedClientIDs)
	if granted > 0 || (u.ClientID == nil && granted == 0) {
		if u.ClientID != nil {
			return "Partner", kit.BadgeInfo, "Home: " + names.label(*u.ClientID) + ", +" + strconv.Itoa(granted) + " granted"
		}
		return "Partner", kit.BadgeInfo, "Access to " + strconv.Itoa(granted) + " client(s)"
	}
	return "Client", kit.BadgeSecondary, "Home client: " + names.label(deref(u.ClientID))
}

// usrClientCell is the SPA's client column.
func usrClientCell(u *principalapi.PrincipalResponse, names usrClients) usrCell {
	granted := u.GrantedClientIDs
	switch {
	case u.IsAnchorUser:
		return usrCell{All: true}
	case u.ClientID != nil:
		return usrCell{Name: names.label(*u.ClientID), More: len(granted)}
	case len(granted) > 0:
		return usrCell{Name: names.label(granted[0]), More: len(granted) - 1}
	}
	return usrCell{}
}

// roleShort is the part of a role name after its last colon (the SPA's
// role.split(':').pop()).
func roleShort(name string) string {
	if _, short, ok := strings.CutLast(name, ":"); ok {
		return short
	}
	return name
}

func toUsrRow(u *principalapi.PrincipalResponse, names usrClients) usrRow {
	row := usrRow{
		ID: u.ID, Name: u.Name, Email: deref(u.Email), Anchor: u.IsAnchorUser, Client: usrClientCell(u, names), Active: u.Active,
		Created: u.CreatedAt.Underlying().UTC().Format("2006-01-02"), CreatedISO: u.CreatedAt.Underlying().UTC().Format("2006-01-02T15:04:05Z"),
		DetailHref: usrURL(u.ID), EditHref: usrURL(u.ID) + "/edit",
	}
	row.TypeLabel, row.TypeBadge, row.TypeTip = usrType(u, names)
	for i, r := range u.Roles {
		if i == 2 {
			row.MoreRoles = len(u.Roles) - 2
			break
		}
		row.Roles = append(row.Roles, roleShort(r))
	}
	return row
}

// rowActions: Edit inline for whoever may edit (the SPA's pencil), View
// details in the menu.
func (v usrView) rowActions(row usrRow) kit.RowActionsProps {
	p := kit.RowActionsProps{ID: "usr-" + row.ID + "-actions", RowLabel: row.Name}
	if v.Perms.Update || v.Perms.Anchor {
		p.Actions = append(p.Actions, kit.RowAction{Icon: "pencil", Label: "Edit", Href: row.EditHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink, "aria-haspopup": "dialog"}})
	}
	p.Menu = append(p.Menu, kit.RowAction{Icon: "eye", Label: "View details", Href: row.DetailHref, Attrs: templ.Attributes{"data-on:click__prevent": liveLink}})
	return p
}

func (v usrView) patch(sse *ds.SSE) error {
	return errors.Join(sse.Patch(usrTable(v)), sse.Patch(kit.Paginator(v.pager())))
}

// usrID reads and checks the {id} path parameter: an id that could not be an
// element id is not a user (and must never reach an expression).
func usrID(r *http.Request) (string, error) {
	id := chi.URLParam(r, "id")
	if !kit.ValidID(id) {
		return "", httperror.NotFound("Principal", id)
	}
	return id, nil
}
