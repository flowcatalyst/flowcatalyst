package pages

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/jsontime"
	"github.com/flowcatalyst/flowcatalyst-go/internal/ui/ds"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

// ── fakes ──────────────────────────────────────────────────────────────

const (
	usrPermView   = "platform:iam:user:view"
	usrPermCreate = "platform:iam:user:create"
	usrPermUpdate = "platform:iam:user:update"
	usrPermDelete = "platform:iam:user:delete"
	usrPermAssign = "platform:iam:user:assign-roles"
	usrPermRoles  = "platform:iam:role:view"
	usrPermClient = "platform:admin:client:view"
	// usrPermManageAccess changes a user's type and client access.
	usrPermManageAccess = "platform:iam:user:manage-access"
)

// usrInviteLink and usrPasswordValue are values no response may carry unless
// the test says so: the link only in the answer that minted it, a password in
// none.
const (
	usrInviteLink     = "https://flow.example.test/auth/set-password?token=SENTINEL-INVITE-TOKEN"
	usrPasswordValue  = "Sentinel-Passw0rd-xyz!"
	usrPasswordValue2 = "Another-Sentinel-1!"
)

// fakeUsers is the principal API as the page sees it. It keeps no rules of its
// own: the API's gates are the integration tests'. It records what the page
// asked for.
type fakeUsers struct {
	users     []principalapi.PrincipalResponse
	roles     map[string][]principalapi.PrincipalRoleAssignmentDTO
	access    map[string]principalapi.ApplicationAccessListResponse
	available []principalapi.PrincipalAvailableApplication
	grants    map[string][]principalapi.ClientAccessGrantResponse
	// domains are the email-domain checks by domain; any other domain is an
	// unmapped one (internal, CLIENT, needs a client).
	domains map[string]principalapi.CheckEmailDomainResponse

	calls    []string
	err      error
	lastList principalapi.ListQuery

	createdDomain  principalapi.CreateUserRequest
	createdClient  principalapi.CreatePrincipalRequest
	updated        principalapi.UpdatePrincipalRequest
	assoc          principalapi.ClientAssociationRequest
	assignedRoles  []string
	appBody        principalapi.AssignApplicationAccessRequest
	passwordReset  principalapi.ResetPasswordRequest
	imported       principalapi.BulkImportRequest
	importResponse principalapi.BulkImportResponse
	// link is the sign-in link a create returns when asked for one.
	link string
}

func (f *fakeUsers) rec(call string) { f.calls = append(f.calls, call) }

func (f *fakeUsers) find(id string) *principalapi.PrincipalResponse {
	for i := range f.users {
		if f.users[i].ID == id {
			return &f.users[i]
		}
	}
	return nil
}

func (f *fakeUsers) ListPrincipals(_ context.Context, q principalapi.ListQuery) (principalapi.PrincipalListResponse, error) {
	f.lastList = q
	if f.err != nil {
		return principalapi.PrincipalListResponse{}, f.err
	}
	var matched []principalapi.PrincipalResponse
	for _, u := range f.users {
		switch {
		case q.Active == "true" && !u.Active, q.Active == "false" && u.Active:
		case q.ClientID != "" && deref(u.ClientID) != q.ClientID && !slices.Contains(u.GrantedClientIDs, q.ClientID):
		case q.Q != "" && !strings.Contains(strings.ToLower(u.Name+" "+deref(u.Email)), strings.ToLower(q.Q)):
		case q.Roles != "" && !slices.ContainsFunc(strings.Split(q.Roles, ","), func(r string) bool { return slices.Contains(u.Roles, r) }):
		default:
			matched = append(matched, u)
		}
	}
	slices.SortStableFunc(matched, func(a, b principalapi.PrincipalResponse) int {
		c := 0
		switch q.SortField {
		case "name":
			c = strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		case "email":
			c = strings.Compare(deref(a.Email), deref(b.Email))
		default:
			c = a.CreatedAt.Underlying().Compare(b.CreatedAt.Underlying())
		}
		if q.SortOrder == "desc" {
			c = -c
		}
		return c
	})
	total := len(matched)
	if q.PageSize > 0 {
		lo := min(q.Page*q.PageSize, total)
		matched = matched[lo:min(lo+q.PageSize, total)]
	}
	return principalapi.PrincipalListResponse{Principals: matched, Total: total}, nil
}

func (f *fakeUsers) GetPrincipal(_ context.Context, id string) (principalapi.PrincipalResponse, error) {
	if f.err != nil {
		return principalapi.PrincipalResponse{}, f.err
	}
	u := f.find(id)
	if u == nil {
		return principalapi.PrincipalResponse{}, usecase.NotFound("NOT_FOUND", "Principal not found: "+id)
	}
	return *u, nil
}

func (f *fakeUsers) CheckEmailDomain(_ context.Context, email string) (principalapi.CheckEmailDomainResponse, error) {
	f.rec("check:" + email)
	if f.err != nil {
		return principalapi.CheckEmailDomainResponse{}, f.err
	}
	domain := email[strings.IndexByte(email, '@')+1:]
	res, ok := f.domains[domain]
	if !ok {
		res = principalapi.CheckEmailDomainResponse{AuthMethod: "internal", Domain: domain, AuthProvider: "INTERNAL", DerivedScope: "CLIENT", RequiresClientID: true, AllowedClientIDs: []string{}}
	}
	for _, u := range f.users {
		if strings.EqualFold(deref(u.Email), email) {
			res.EmailExists = true
			w := "A user with this email address already exists."
			res.Warning = &w
		}
	}
	return res, nil
}

func (f *fakeUsers) CreateDomainUser(_ context.Context, body principalapi.CreateUserRequest) (principalapi.PrincipalResponse, error) {
	f.rec("create-user")
	f.createdDomain = body
	if f.err != nil {
		return principalapi.PrincipalResponse{}, f.err
	}
	u := principalapi.PrincipalResponse{ID: "prn_new", Name: body.Name, Email: &body.Email, Scope: deref(body.Scope), Active: true, IdpType: new("INTERNAL"), Roles: []string{}, GrantedClientIDs: []string{}, CreatedAt: jsontime.New(time.Now())}
	f.users = append(f.users, u)
	if body.ReturnInviteLink != nil && *body.ReturnInviteLink {
		u.InviteLink = &f.link
	}
	return u, nil
}

func (f *fakeUsers) CreatePrincipal(_ context.Context, body principalapi.CreatePrincipalRequest) (principalapi.CreatePrincipalResponse, error) {
	f.rec("create-principal")
	f.createdClient = body
	if f.err != nil {
		return principalapi.CreatePrincipalResponse{}, f.err
	}
	res := principalapi.CreatePrincipalResponse{ID: "prn_new"}
	f.users = append(f.users, principalapi.PrincipalResponse{ID: "prn_new", Name: deref(body.Name), Email: &body.Email, Scope: "CLIENT", ClientID: body.ClientID, Active: true, IdpType: new("INTERNAL"), Roles: []string{}, GrantedClientIDs: []string{}, CreatedAt: jsontime.New(time.Now())})
	if body.ReturnInviteLink != nil && *body.ReturnInviteLink {
		res.InviteLink = &f.link
	}
	return res, nil
}

func (f *fakeUsers) UpdatePrincipal(_ context.Context, id string, body principalapi.UpdatePrincipalRequest) (principalapi.PrincipalResponse, error) {
	f.rec("update:" + id)
	f.updated = body
	return principalapi.PrincipalResponse{}, f.err
}

func (f *fakeUsers) ActivatePrincipal(_ context.Context, id string) error {
	f.rec("activate:" + id)
	return f.err
}

func (f *fakeUsers) DeactivatePrincipal(_ context.Context, id string) error {
	f.rec("deactivate:" + id)
	return f.err
}

func (f *fakeUsers) ResetPrincipalPassword(_ context.Context, id string, body principalapi.ResetPasswordRequest) error {
	f.rec("reset-password:" + id)
	f.passwordReset = body
	return f.err
}

func (f *fakeUsers) SendPrincipalPasswordReset(_ context.Context, id string) error {
	f.rec("send-reset:" + id)
	return f.err
}

func (f *fakeUsers) ResetPrincipalTwoFactor(_ context.Context, id string) error {
	f.rec("reset-2fa:" + id)
	return f.err
}

func (f *fakeUsers) DeletePrincipal(_ context.Context, id string) error {
	f.rec("delete:" + id)
	return f.err
}

func (f *fakeUsers) ListPrincipalRoles(_ context.Context, id string) ([]principalapi.PrincipalRoleAssignmentDTO, error) {
	return f.roles[id], f.err
}

func (f *fakeUsers) AssignPrincipalRoles(_ context.Context, id string, roles []string) (principalapi.RolesAssignedResponse, error) {
	f.rec("assign-roles:" + id)
	f.assignedRoles = roles
	return principalapi.RolesAssignedResponse{Added: roles}, f.err
}

func (f *fakeUsers) ListPrincipalApplicationAccess(_ context.Context, id string) (principalapi.ApplicationAccessListResponse, error) {
	return f.access[id], f.err
}

func (f *fakeUsers) ListPrincipalAvailableApplications(context.Context, string) ([]principalapi.PrincipalAvailableApplication, error) {
	return f.available, f.err
}

func (f *fakeUsers) AssignPrincipalApplicationAccess(_ context.Context, id string, body principalapi.AssignApplicationAccessRequest) (principalapi.SetApplicationAccessResponse, error) {
	f.rec("assign-apps:" + id)
	f.appBody = body
	return principalapi.SetApplicationAccessResponse{Added: len(body.ApplicationIDs)}, f.err
}

func (f *fakeUsers) ListPrincipalClientAccess(_ context.Context, id string) ([]principalapi.ClientAccessGrantResponse, error) {
	return f.grants[id], f.err
}

func (f *fakeUsers) GrantPrincipalClientAccess(_ context.Context, id, clientID string) error {
	f.rec("grant:" + id + ":" + clientID)
	return f.err
}

func (f *fakeUsers) RevokePrincipalClientAccess(_ context.Context, id, clientID string) error {
	f.rec("revoke:" + id + ":" + clientID)
	return f.err
}

func (f *fakeUsers) SetPrincipalClientAssociation(_ context.Context, id string, body principalapi.ClientAssociationRequest) (principalapi.PrincipalResponse, error) {
	f.rec("associate:" + id)
	f.assoc = body
	return principalapi.PrincipalResponse{}, f.err
}

func (f *fakeUsers) BulkImportPrincipals(_ context.Context, body principalapi.BulkImportRequest) (principalapi.BulkImportResponse, error) {
	f.rec("import")
	f.imported = body
	return f.importResponse, f.err
}

type fakeUsrRoles struct{ roles []role.Role }

func (f fakeUsrRoles) FindAll(context.Context) ([]role.Role, error) { return f.roles, nil }

type fakeUsrClients struct{ clients []client.Client }

func (f fakeUsrClients) FindAll(context.Context) ([]client.Client, error) {
	return append([]client.Client(nil), f.clients...), nil
}

func (f fakeUsrClients) FindByID(_ context.Context, id string) (*client.Client, error) {
	for i := range f.clients {
		if string(f.clients[i].ID) == id {
			c := f.clients[i]
			return &c, nil
		}
	}
	return nil, nil
}

// ── fixtures ───────────────────────────────────────────────────────────

func usrFixture() *fakeUsers {
	day := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	mk := func(id, name, email, scope, idp string, clientID *string, granted []string, active bool, roles []string, n int) principalapi.PrincipalResponse {
		return principalapi.PrincipalResponse{
			ID: id, Type: "USER", Scope: scope, ClientID: clientID, Name: name, Active: active, Email: &email, IdpType: &idp,
			Roles: roles, IsAnchorUser: scope == "ANCHOR", GrantedClientIDs: granted, CreatedAt: jsontime.New(day.Add(time.Duration(n) * time.Hour)),
		}
	}
	acme, globex := "clt_acme", "clt_globex"
	f := &fakeUsers{
		users: []principalapi.PrincipalResponse{
			mk("prn_1", "Ada Admin", "ada@flowcatalyst.local", "ANCHOR", "INTERNAL", nil, []string{}, true, []string{"platform:super-admin"}, 0),
			mk("prn_2", "Bob <Builder>", "bob@acme.test", "CLIENT", "INTERNAL", &acme, []string{}, true, []string{"orders:viewer", "orders:editor", "orders:approver"}, 1),
			mk("prn_3", "Cara Partner", "cara@partner.test", "PARTNER", "OIDC", nil, []string{"clt_acme", "clt_globex"}, true, []string{}, 2),
			mk("prn_4", "Dan Inactive", "dan@globex.test", "CLIENT", "INTERNAL", &globex, []string{}, false, []string{"orders:viewer"}, 3),
		},
		roles: map[string][]principalapi.PrincipalRoleAssignmentDTO{
			"prn_2": {
				{ID: "r1", RoleName: "orders:viewer", AssignmentSource: "MANUAL", AssignedAt: jsontime.New(day)},
				{ID: "r2", RoleName: "orders:editor", AssignmentSource: "IDP_SYNC", AssignedAt: jsontime.New(day)},
				{ID: "r3", RoleName: "orders:approver", AssignmentSource: "MANUAL", AssignedAt: jsontime.New(day)},
			},
			"prn_1": {{ID: "r4", RoleName: "platform:super-admin", AssignmentSource: "MANUAL", AssignedAt: jsontime.New(day)}},
		},
		access: map[string]principalapi.ApplicationAccessListResponse{
			"prn_1": {AllApplications: true, Applications: []principalapi.ApplicationAccessResponse{}},
			"prn_2": {Applications: []principalapi.ApplicationAccessResponse{{ApplicationID: "app_orders", ApplicationCode: "orders", ApplicationName: "Orders <app>"}}},
		},
		available: []principalapi.PrincipalAvailableApplication{{ID: "app_orders", Code: "orders", Name: "Orders <app>"}, {ID: "app_billing", Code: "billing", Name: "Billing"}},
		grants: map[string][]principalapi.ClientAccessGrantResponse{
			"prn_3": {{ID: "g1", ClientID: "clt_acme", GrantedAt: jsontime.New(day)}, {ID: "g2", ClientID: "clt_globex", GrantedAt: jsontime.New(day)}},
		},
		domains: map[string]principalapi.CheckEmailDomainResponse{
			"flowcatalyst.local": {AuthMethod: "internal", Domain: "flowcatalyst.local", AuthProvider: "INTERNAL", IsAnchorDomain: true, DerivedScope: "ANCHOR", AllowedClientIDs: []string{}},
			"sso.test":           {AuthMethod: "external", Domain: "sso.test", AuthProvider: "OIDC", HasIDPConfig: true, DerivedScope: "CLIENT", RequiresClientID: true, AllowedClientIDs: []string{"clt_globex"}},
			"partner.test":       {AuthMethod: "internal", Domain: "partner.test", AuthProvider: "INTERNAL", DerivedScope: "PARTNER", RequiresClientID: true, AllowedClientIDs: []string{"clt_acme", "clt_globex"}},
		},
		link: usrInviteLink,
	}
	f.users[1].TwoFactorMethods = []string{"TOTP", "EMAIL_PIN"}
	return f
}

func usrRoleFixture() fakeUsrRoles {
	return fakeUsrRoles{roles: []role.Role{
		{Name: "orders:viewer", DisplayName: "Orders Viewer", ApplicationCode: "orders", Permissions: []string{"orders:order:view"}},
		{Name: "orders:editor", DisplayName: "Orders Editor", ApplicationCode: "orders", Permissions: []string{"orders:order:edit"}},
		{Name: "orders:approver", DisplayName: "Orders Approver", ApplicationCode: "orders", Permissions: []string{"orders:order:approve"}},
		{Name: "billing:clerk", DisplayName: "Billing Clerk", ApplicationCode: "billing", Permissions: []string{"billing:invoice:view"}},
		{Name: "platform:super-admin", DisplayName: "Super Admin", ApplicationCode: "platform", Permissions: []string{"platform:*:*:*"}},
		{Name: "platform:user-admin", DisplayName: "User Admin", ApplicationCode: "platform", Permissions: []string{"platform:iam:user:delete"}},
	}}
}

type usrHarness struct {
	users *fakeUsers
}

func newUsrHarness() *usrHarness { return &usrHarness{users: usrFixture()} }

func (h *usrHarness) page() *Users {
	return &Users{
		Users: h.users, Roles: usrRoleFixture(),
		Clients: fakeUsrClients{clients: []client.Client{
			{ID: "clt_acme", Name: "Acme Corp", Identifier: "acme", Status: client.StatusActive},
			{ID: "clt_globex", Name: "Globex <Inc>", Identifier: "globex", Status: client.StatusActive},
			{ID: "clt_old", Name: "Old Co", Identifier: "old", Status: client.StatusInactive},
		}},
		Shell: shellForTest(),
	}
}

func (h *usrHarness) do(t *testing.T, ac *auth.AuthContext, q req) *httptest.ResponseRecorder {
	t.Helper()
	return serveMounted(t, ac, q, h.page())
}

// usrAnchor is an anchor user holding exactly the given permissions.
func usrAnchor(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_9", Email: "ops@flowcatalyst.local", Scope: auth.ScopeAnchor, Roles: []string{"platform:ops"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

// usrClientAdmin is a client administrator of Acme holding the given
// permissions.
func usrClientAdmin(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_8", Email: "boss@acme.test", Scope: auth.ScopeClient, Clients: []string{"clt_acme"}, Roles: []string{"acme:admin"}, Permissions: perms, PrincipalType: auth.PrincipalTypeUser}
}

var usrAllPerms = []string{usrPermView, usrPermCreate, usrPermUpdate, usrPermDelete, usrPermAssign, usrPermRoles, usrPermClient, usrPermManageAccess}

func usrWithout(perm string) *auth.AuthContext {
	return usrAnchor(slices.DeleteFunc(slices.Clone(usrAllPerms), func(p string) bool { return p == perm })...)
}

func usrBody(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// ── list ───────────────────────────────────────────────────────────────

func TestUsersListRendersTheSPAsColumns(t *testing.T) {
	h := newUsrHarness()
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users"}))
	mustContain(t, body,
		"Manage platform users and their access", "Search by name or email...",
		`href="/ui/users/prn_2"`, "Bob &lt;Builder&gt;", "bob@acme.test",
		// The SPA's type tags, client cell and role tags.
		"Anchor", "Partner", "All Clients", "Acme Corp", "approver", "+1 more",
		"Inactive", "2026-10-01",
		// The header actions and the row actions.
		`href="/ui/users/new"`, `href="/ui/users/import"`, `href="/ui/users/prn_2/edit"`,
		// The filters: client, status, roles.
		`name="client"`, `name="status"`, `name="roles"`,
	)
	mustNotContain(t, body, "Bob <Builder>")
	if got := h.users.lastList; got.Type != "USER" || got.PageSize != 100 || got.SortField != "createdAt" || got.SortOrder != "asc" || got.Page != 0 {
		t.Errorf("default query = %+v", got)
	}
}

func TestUsersListFiltersAreTheURL(t *testing.T) {
	h := newUsrHarness()
	target := "/ui/users?q=bob&status=active&client=clt_acme&roles=orders:viewer&roles=orders:editor&sort=name&dir=desc&rows=50&page=1"
	rec := h.do(t, anchorAdmin(), req{target: target, datastar: true})
	got := h.users.lastList
	if got.Q != "bob" || got.Active != "true" || got.ClientID != "clt_acme" || got.Roles != "orders:viewer,orders:editor" ||
		got.SortField != "name" || got.SortOrder != "desc" || got.PageSize != 50 {
		t.Errorf("query = %+v", got)
	}
	body := usrBody(t, rec)
	mustContain(t, body, `href="/ui/users/prn_2"`)
	mustNotContain(t, body, `id="usr-prn_1"`, `id="usr-prn_4"`)
	// The canonical URL is kept in the address bar.
	mustContain(t, body, "client=clt_acme", "status=active")
	// A hand-edited address still shows a list: unknown values are dropped.
	usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users?status=banana&client=clt_nope&roles=nope&sort=x&rows=7&page=-4"}))
	if got := h.users.lastList; got.ClientID != "" || got.Roles != "" || got.Active != "" || got.PageSize != 100 {
		t.Errorf("tolerated query = %+v", got)
	}
}

func TestUsersListPermissionAndScope(t *testing.T) {
	h := newUsrHarness()
	// No view permission: refused, nothing read.
	rec := h.do(t, usrAnchor(), req{target: "/ui/users"})
	if rec.Code != http.StatusForbidden {
		t.Errorf("status %d without user:view", rec.Code)
	}
	// A view-only anchor sees no create controls.
	body := usrBody(t, h.do(t, usrAnchor(usrPermView), req{target: "/ui/users"}))
	mustNotContain(t, body, `href="/ui/users/new"`, `href="/ui/users/import"`)
	mustContain(t, body, `href="/ui/users/prn_2"`)
	// A client administrator gets the client page: no type column, its title,
	// no client filter with one client.
	ca := usrClientAdmin(usrPermView, usrPermCreate, usrPermUpdate)
	body = usrBody(t, h.do(t, ca, req{target: "/ui/users"}))
	mustContain(t, body, "User Management", "Manage users for your client", `href="/ui/users/new"`, `href="/ui/users/import"`)
	mustNotContain(t, body, ">Type<", `name="client"`, "Manage platform users and their access")
}

// ── drawer ─────────────────────────────────────────────────────────────

func TestUserDrawerContent(t *testing.T) {
	h := newUsrHarness()
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_2", datastar: true}))
	mustContain(t, body,
		"User Information", "Bob &lt;Builder&gt;", "bob@acme.test", "Internal", "Acme Corp",
		"Client Access", "Roles", "Application Access", "Account Actions",
		"Orders &lt;app&gt;", "IDP_SYNC", "orders:approver",
		`href="/ui/users/prn_2/edit"`, `href="/ui/users/prn_2/roles"`, `href="/ui/users/prn_2/applications"`,
		`href="/ui/users/prn_2/reset-password"`, `data-action="/ui/users/prn_2/send-reset"`, `data-action="/ui/users/prn_2/reset-2fa"`,
		`action="/ui/users/prn_2/deactivate"`, `data-action="/ui/users/prn_2/delete"`,
		`href="/ui/users/prn_2/clients/new"`, `action="/ui/users/prn_2/all-applications/grant"`,
	)
	mustNotContain(t, body, "Bob <Builder>")
	// An inactive user is offered Activate.
	body = usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_4", datastar: true}))
	mustContain(t, body, `action="/ui/users/prn_4/activate"`)
	mustNotContain(t, body, `/ui/users/prn_4/deactivate`)
}

func TestUserDrawerKindsOfUsers(t *testing.T) {
	h := newUsrHarness()
	// An anchor user: the anchor notice, no client to add, all applications.
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_1", datastar: true}))
	mustContain(t, body, "automatically has access to all clients", "This user has access to every application.", `action="/ui/users/prn_1/all-applications/restrict"`)
	mustNotContain(t, body, `/ui/users/prn_1/clients/new`, `/ui/users/prn_1/applications"`)
	// Restricting your own all-applications access asks first.
	self := anchorAdmin()
	self.PrincipalID = "prn_1"
	body = usrBody(t, h.do(t, self, req{target: "/ui/users/prn_1"}))
	mustContain(t, body, `data-action="/ui/users/prn_1/all-applications/restrict"`, `name="confirmSelf" value="1"`, "Restrict your own access")
	// A partner: the granted clients with their revoke actions; an OIDC user has
	// no password actions.
	body = usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_3", datastar: true}))
	mustContain(t, body, "Granted Access", "Acme Corp", "Globex &lt;Inc&gt;", `data-action="/ui/users/prn_3/clients/clt_acme/revoke"`)
	mustNotContain(t, body, "Send Password Reset", "Reset Password", "Two-Factor Authentication")
}

func TestUserDrawerPermissionMatrix(t *testing.T) {
	h := newUsrHarness()
	marks := map[string]string{
		"edit":        `href="/ui/users/prn_2/edit"`,
		"roles":       `href="/ui/users/prn_2/roles"`,
		"apps":        `href="/ui/users/prn_2/applications"`,
		"all apps":    `action="/ui/users/prn_2/all-applications/grant"`,
		"send reset":  `data-action="/ui/users/prn_2/send-reset"`,
		"password":    `href="/ui/users/prn_2/reset-password"`,
		"2fa":         `data-name`,
		"deactivate":  `action="/ui/users/prn_2/deactivate"`,
		"delete":      `data-action="/ui/users/prn_2/delete"`,
		"add client":  `href="/ui/users/prn_2/clients/new"`,
		"actions box": "Account Actions",
	}
	delete(marks, "2fa")
	cases := []struct {
		name  string
		perms []string
		want  []string
	}{
		// Changing a user's type and client, and client access, needs
		// user:manage-access (owner ruling 2026-10-25): anchor tier alone no
		// longer offers Edit or Add Client.
		{"view only", []string{usrPermView}, nil},
		{"update", []string{usrPermView, usrPermUpdate}, []string{"edit", "apps", "all apps", "send reset", "password", "deactivate", "actions box"}},
		{"assign roles", []string{usrPermView, usrPermAssign, usrPermRoles}, []string{"roles"}},
		{"assign roles without role:view", []string{usrPermView, usrPermAssign}, nil},
		{"delete", []string{usrPermView, usrPermDelete}, []string{"delete", "actions box"}},
		{"manage access", []string{usrPermView, usrPermManageAccess}, []string{"edit", "add client"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			body := usrBody(t, h.do(t, usrAnchor(c.perms...), req{target: "/ui/users/prn_2", datastar: true}))
			for name, mark := range marks {
				if has := strings.Contains(body, mark); has != slices.Contains(c.want, name) {
					t.Errorf("%q present = %v, want %v", name, has, !has)
				}
			}
		})
	}
	t.Run("a client administrator", func(t *testing.T) {
		ca := usrClientAdmin(usrPermView, usrPermUpdate, usrPermDelete, usrPermAssign, usrPermRoles)
		// Without user:update the client administrator is offered no edit.
		viewOnly := usrBody(t, h.do(t, usrClientAdmin(usrPermView), req{target: "/ui/users/prn_2", datastar: true}))
		mustNotContain(t, viewOnly, `href="/ui/users/prn_2/edit"`, "Account Actions")
		body := usrBody(t, h.do(t, ca, req{target: "/ui/users/prn_2", datastar: true}))
		// Left off the client administrator's drawer, as in the SPA: client
		// access, the all-applications switch and delete.
		mustNotContain(t, body, "Client Access", `/ui/users/prn_2/clients/new`, `/ui/users/prn_2/all-applications`, `data-action="/ui/users/prn_2/delete"`)
		mustContain(t, body, `href="/ui/users/prn_2/edit"`, `href="/ui/users/prn_2/roles"`, `href="/ui/users/prn_2/applications"`, `href="/ui/users/prn_2/reset-password"`)
	})
	t.Run("a drawer needs the read permission", func(t *testing.T) {
		rec := h.do(t, usrAnchor(usrPermUpdate), req{target: "/ui/users/prn_2", datastar: true})
		mustContain(t, usrBody(t, rec), "permission required")
		mustNotContain(t, rec.Body.String(), "bob@acme.test")
	})
	t.Run("an id that could not be one", func(t *testing.T) {
		mustContain(t, usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/not$valid", datastar: true})), "Not Found")
	})
}

func TestUserDrawerDeepLink(t *testing.T) {
	h := newUsrHarness()
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_2"}))
	mustContain(t, body, "selUsr: &#39;prn_2&#39;", "Bob &lt;Builder&gt;", `id="usr-table"`)
}

// ── create ─────────────────────────────────────────────────────────────

func TestUserCreateDialogAndDomainCheck(t *testing.T) {
	h := newUsrHarness()
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/new", datastar: true}))
	mustContain(t, body, `id="usr-form"`, "Add User", "Full Name", "Email Address", `id="usr-form-domain"`)
	// The email is checked when the field is left.
	mustContain(t, body, "check-email")

	// An unmapped domain: internal, needs a client, the sign-in link is offered.
	rec := h.do(t, anchorAdmin(), req{target: "/ui/users/check-email?email=new@unmapped.test&name=N", datastar: true})
	body = usrBody(t, rec)
	mustContain(t, body, `id="usr-form-domain"`, `id="usr-form-client-id-field"`, "Acme Corp (acme)", "Globex &lt;Inc&gt; (globex)",
		"one-time sign-in link", "Scope: CLIENT.", `name="returnLink"`)
	mustNotContain(t, body, "Old Co") // inactive clients are not offered
	// A partner domain allows its clients only; a single allowed client is picked.
	body = usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/check-email?email=x@partner.test", datastar: true}))
	mustContain(t, body, "This partner domain restricts users to specific clients.", "Scope: PARTNER.")
	body = usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/check-email?email=x@sso.test", datastar: true}))
	mustContain(t, body, `value="clt_globex" selected`, "identity provider (OIDC)")
	mustNotContain(t, body, "Acme Corp (acme)", `name="returnLink"`) // an SSO user has no link
	// An anchor domain needs no client.
	body = usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/check-email?email=x@flowcatalyst.local", datastar: true}))
	mustContain(t, body, "Scope: ANCHOR.")
	mustNotContain(t, body, `id="usr-form-client-id-field"`)
	// An existing address is a warning, with no link switch.
	body = usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/check-email?email=bob@acme.test", datastar: true}))
	mustContain(t, body, "A user with this email address already exists.")
	mustNotContain(t, body, `name="returnLink"`)
	// A malformed address is the SPA's message on the field.
	body = usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/check-email?email=nonsense", datastar: true}))
	mustContain(t, body, "Please enter a valid email address.")
	// The check needs the permissions the form does.
	for _, ac := range []*auth.AuthContext{usrWithout(usrPermView), usrWithout(usrPermCreate)} {
		mustContain(t, usrBody(t, h.do(t, ac, req{target: "/ui/users/check-email?email=a@b.test", datastar: true})), "permission required")
	}
}

func TestUserCreateSendsTheDerivedScope(t *testing.T) {
	h := newUsrHarness()
	rec := h.do(t, anchorAdmin(), req{
		method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true,
		form: url.Values{"name": {"New Person"}, "email": {"new@partner.test"}, "clientId": {"clt_acme"}},
	})
	body := usrBody(t, rec)
	got := h.users.createdDomain
	if got.Email != "new@partner.test" || got.Name != "New Person" || got.Scope == nil || *got.Scope != "PARTNER" || got.ClientID == nil || *got.ClientID != "clt_acme" {
		t.Errorf("request = %+v", got)
	}
	if got.ReturnInviteLink != nil {
		t.Errorf("a link was asked for without the switch")
	}
	// As the SPA: the new user's drawer opens, with the toast.
	mustContain(t, body, "selUsr", "prn_new", "User created", "We&#39;ve emailed them a one-time sign-in link", "usr-form")
	mustNotContain(t, body, "usr-link-dialog", "SENTINEL-INVITE-TOKEN")
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q", cc)
	}

	// An anchor domain carries no client even if one is posted.
	h = newUsrHarness()
	h.do(t, anchorAdmin(), req{
		method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true,
		form: url.Values{"name": {"Anchor Person"}, "email": {"a@flowcatalyst.local"}, "clientId": {"clt_acme"}},
	})
	if got := h.users.createdDomain; got.Scope == nil || *got.Scope != "ANCHOR" || got.ClientID != nil {
		t.Errorf("anchor request = %+v", got)
	}
}

func TestUserCreateValidation(t *testing.T) {
	h := newUsrHarness()
	cases := []struct {
		name string
		form url.Values
		want string
	}{
		{"no name", url.Values{"email": {"a@b.test"}, "clientId": {"clt_acme"}}, "Name is required."},
		{"no email", url.Values{"name": {"N"}}, "Email is required."},
		{"bad email", url.Values{"name": {"N"}, "email": {"nope"}}, "Please enter a valid email address."},
		{"existing email", url.Values{"name": {"N"}, "email": {"bob@acme.test"}, "clientId": {"clt_acme"}}, "A user with this email address already exists."},
		{"no client", url.Values{"name": {"N"}, "email": {"a@unmapped.test"}}, "Select a client."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true, form: c.form})
			mustContain(t, usrBody(t, rec), c.want)
			if slices.Contains(h.users.calls, "create-user") {
				t.Fatalf("created despite %q", c.want)
			}
		})
	}
	// A use-case code lands on its field.
	h.users.err = usecase.Conflict("EMAIL_EXISTS", "User with email 'x@unmapped.test' already exists")
	body := usrBody(t, h.do(t, anchorAdmin(), req{
		method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true,
		form: url.Values{"name": {"N"}, "email": {"x@unmapped.test"}, "clientId": {"clt_acme"}},
	}))
	mustContain(t, body, "User with email &#39;x@unmapped.test&#39; already exists", `id="usr-form-email-field"`)
}

// TestUserSignInLinkOnRequestOnly: the link is minted only when the switch is
// on, shown in the answer that created the user and nowhere else, and no
// answer is cacheable.
func TestUserSignInLinkOnRequestOnly(t *testing.T) {
	h := newUsrHarness()
	form := url.Values{"name": {"Linked Person"}, "email": {"linked@unmapped.test"}, "clientId": {"clt_acme"}, "returnLink": {"1"}}
	rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true, form: form})
	body := usrBody(t, rec)
	if rl := h.users.createdDomain.ReturnInviteLink; rl == nil || !*rl {
		t.Fatalf("the link was not asked for")
	}
	mustContain(t, body, "usr-link-dialog", usrInviteLink, "Give them the sign-in link", `data-on:cancel="evt.preventDefault(); el.remove()"`)
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Errorf("Cache-Control = %q", cc)
	}
	// Not in a signal, an attribute of the page or a flash: only the dialog's text.
	if n := strings.Count(body, "SENTINEL-INVITE-TOKEN"); n != 1 {
		t.Errorf("the link appears %d times", n)
	}
	// Nothing else the page serves carries it.
	for _, target := range []string{"/ui/users", "/ui/users/prn_new", "/ui/users/new", "/ui/users/prn_new/edit"} {
		for _, datastar := range []bool{false, true} {
			mustNotContain(t, usrBody(t, h.do(t, anchorAdmin(), req{target: target, datastar: datastar})), "SENTINEL-INVITE-TOKEN")
		}
	}
	// A native post cannot redirect (the link would be lost): the page answers.
	h = newUsrHarness()
	rec = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/new", form: form})
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), usrInviteLink) || !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
		t.Errorf("native: status %d, cache %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	// An SSO user has no link to give: the switch is ignored.
	h = newUsrHarness()
	form = url.Values{"name": {"Sso Person"}, "email": {"sso@sso.test"}, "clientId": {"clt_globex"}, "returnLink": {"1"}}
	body = usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true, form: form}))
	if h.users.createdDomain.ReturnInviteLink != nil {
		t.Errorf("a link was asked for an SSO user")
	}
	mustNotContain(t, body, "usr-link-dialog")
}

func TestClientAdministratorCreate(t *testing.T) {
	h := newUsrHarness()
	ca := usrClientAdmin(usrPermView, usrPermCreate)
	body := usrBody(t, h.do(t, ca, req{target: "/ui/users/new", datastar: true}))
	mustContain(t, body, "Create a user for your client", `name="password"`, "Leave blank to require the user to set it via a reset email.", `name="returnLink"`)
	mustNotContain(t, body, `id="usr-form-client-id-field"`, "check-email") // one client is implicit; no domain check
	rec := h.do(t, ca, req{
		method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true,
		form: url.Values{"name": {"Pat"}, "email": {"pat@acme.test"}, "password": {usrPasswordValue}, "clientId": {"clt_acme"}},
	})
	body = usrBody(t, rec)
	got := h.users.createdClient
	if got.Scope != "CLIENT" || got.ClientID == nil || *got.ClientID != "clt_acme" || got.Password == nil || *got.Password != usrPasswordValue || got.ReturnInviteLink != nil {
		t.Errorf("request = %+v", got)
	}
	mustContain(t, body, "Pat was added")
	// The password is in the request and in no answer.
	mustNotContain(t, body, usrPasswordValue)

	// A weak password is the field's error and is not echoed.
	h = newUsrHarness()
	h.users.err = usecase.Validation("PASSWORD_TOO_COMMON", "Password is too common")
	rec = h.do(t, ca, req{
		method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true,
		form: url.Values{"name": {"Pat"}, "email": {"pat@acme.test"}, "password": {usrPasswordValue}, "clientId": {"clt_acme"}},
	})
	body = usrBody(t, rec)
	mustContain(t, body, "Password is too common", `id="usr-form-password-field"`)
	mustNotContain(t, body, usrPasswordValue)
	// With no password the link may be asked for.
	h = newUsrHarness()
	h.do(t, ca, req{
		method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true,
		form: url.Values{"name": {"Pat"}, "email": {"pat@acme.test"}, "returnLink": {"1"}, "clientId": {"clt_acme"}},
	})
	if rl := h.users.createdClient.ReturnInviteLink; rl == nil || !*rl {
		t.Errorf("the link was not asked for")
	}
	// One client is implicit; with several the choice is asked for, and a client
	// the administrator cannot reach is not one of them.
	h = newUsrHarness()
	h.do(t, ca, req{method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true, form: url.Values{"name": {"Pat"}, "email": {"pat@acme.test"}}})
	if got := h.users.createdClient; got.ClientID == nil || *got.ClientID != "clt_acme" {
		t.Errorf("implicit client = %+v", got.ClientID)
	}
	h = newUsrHarness()
	two := usrClientAdmin(usrPermView, usrPermCreate)
	two.Clients = []string{"clt_acme", "clt_globex"}
	mustContain(t, usrBody(t, h.do(t, two, req{
		method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true,
		form: url.Values{"name": {"Pat"}, "email": {"pat@acme.test"}},
	})), "Select a client.")
	h.users.calls = nil
	mustContain(t, usrBody(t, h.do(t, ca, req{
		method: http.MethodPost, target: "/ui/users/new", datastar: true, multipart: true,
		form: url.Values{"name": {"Pat"}, "email": {"pat@acme.test"}, "clientId": {"clt_globex"}},
	})), "You cannot add users to that client.")
	if len(h.users.calls) != 0 {
		t.Errorf("a client out of reach was used: %v", h.users.calls)
	}
}

// ── edit ───────────────────────────────────────────────────────────────

func TestUserEditDialogAndAssociation(t *testing.T) {
	h := newUsrHarness()
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_2/edit", datastar: true}))
	mustContain(t, body, "Edit User", `name="scope"`, `name="clientId"`, "Acme Corp (acme)", `value="Bob &lt;Builder&gt;"`, "usrScope: &#39;CLIENT&#39;")
	// A client administrator edits the name only.
	body = usrBody(t, h.do(t, usrClientAdmin(usrPermView, usrPermUpdate), req{target: "/ui/users/prn_2/edit", datastar: true}))
	mustContain(t, body, `name="name"`)
	mustNotContain(t, body, `name="scope"`, `name="clientId"`)

	post := func(form url.Values) *httptest.ResponseRecorder {
		h.users.calls = nil
		return h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_2/edit", datastar: true, multipart: true, form: form})
	}
	// A new name only.
	usrBody(t, post(url.Values{"name": {"Robert"}, "scope": {"CLIENT"}, "clientId": {"clt_acme"}}))
	if !slices.Equal(h.users.calls, []string{"update:prn_2"}) || h.users.updated.Name == nil || *h.users.updated.Name != "Robert" {
		t.Errorf("calls = %v, update = %+v", h.users.calls, h.users.updated)
	}
	// The SPA's mapping: ANCHOR is "*", CLIENT changes the home client, PARTNER
	// promotes keeping the old one.
	usrBody(t, post(url.Values{"name": {"Bob <Builder>"}, "scope": {"ANCHOR"}, "clientId": {""}}))
	if h.users.assoc.ClientID != "*" || h.users.assoc.Mode != nil || !slices.Equal(h.users.calls, []string{"associate:prn_2"}) {
		t.Errorf("anchor: %+v %v", h.users.assoc, h.users.calls)
	}
	usrBody(t, post(url.Values{"name": {"Bob <Builder>"}, "scope": {"CLIENT"}, "clientId": {"clt_globex"}}))
	if h.users.assoc.ClientID != "clt_globex" || h.users.assoc.Mode == nil || *h.users.assoc.Mode != "CHANGE_CLIENT" {
		t.Errorf("client: %+v", h.users.assoc)
	}
	usrBody(t, post(url.Values{"name": {"Bob <Builder>"}, "scope": {"PARTNER"}, "clientId": {"clt_acme"}}))
	if h.users.assoc.ClientID != "clt_acme" || h.users.assoc.Mode == nil || *h.users.assoc.Mode != "TO_PARTNER" {
		t.Errorf("partner: %+v", h.users.assoc)
	}
	// CLIENT or PARTNER with no client is refused before anything is written,
	// and so is a blank name.
	body = usrBody(t, post(url.Values{"name": {"Renamed"}, "scope": {"PARTNER"}, "clientId": {""}}))
	mustContain(t, body, "Select a client for CLIENT/PARTNER scope.")
	body = usrBody(t, post(url.Values{"name": {"  "}, "scope": {"CLIENT"}, "clientId": {"clt_acme"}}))
	mustContain(t, body, "Name is required.")
	if len(h.users.calls) != 0 {
		t.Errorf("wrote %v", h.users.calls)
	}
	// A client administrator's post cannot carry an association: the API is the
	// guard, and the page does not offer or send one.
	h.users.calls = nil
	h.do(t, usrClientAdmin(usrPermView, usrPermUpdate), req{
		method: http.MethodPost, target: "/ui/users/prn_2/edit", datastar: true, multipart: true,
		form: url.Values{"name": {"Robert"}, "scope": {"ANCHOR"}},
	})
	if slices.Contains(h.users.calls, "associate:prn_2") {
		t.Errorf("a client administrator set an association: %v", h.users.calls)
	}
}

// ── actions ────────────────────────────────────────────────────────────

func TestUserActions(t *testing.T) {
	cases := []struct {
		name, target, call, flash string
		form                      url.Values
	}{
		{"activate", "/ui/users/prn_4/activate", "activate:prn_4", "User activated", nil},
		{"deactivate", "/ui/users/prn_2/deactivate", "deactivate:prn_2", "User deactivated", nil},
		{"send reset", "/ui/users/prn_2/send-reset", "send-reset:prn_2", "Reset email sent", nil},
		{"reset 2fa", "/ui/users/prn_2/reset-2fa", "reset-2fa:prn_2", "2FA reset", nil},
		{"delete", "/ui/users/prn_2/delete", "delete:prn_2", `User &#34;Bob &lt;Builder&gt;&#34; deleted`, url.Values{"drawer": {"prn_2"}}},
		{"grant", "/ui/users/prn_3/clients", "grant:prn_3:clt_acme", "Client access granted", url.Values{"clientId": {"clt_acme"}}},
		{"revoke", "/ui/users/prn_3/clients/clt_acme/revoke", "revoke:prn_3:clt_acme", "Client access revoked", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newUsrHarness()
			form := c.form
			if form == nil {
				form = url.Values{}
			}
			body := usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: c.target, form: form, datastar: true, multipart: true}))
			if !slices.Contains(h.users.calls, c.call) {
				t.Fatalf("calls = %v, want %s", h.users.calls, c.call)
			}
			mustContain(t, body, c.flash)
		})
	}
	t.Run("a native post redirects with the flash key", func(t *testing.T) {
		h := newUsrHarness()
		rec := h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_4/activate", form: url.Values{}})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/users/prn_4" {
			t.Errorf("status %d, location %q", rec.Code, rec.Header().Get("Location"))
		}
		if c := rec.Result().Cookies(); len(c) != 1 || c[0].Value != flashUsrActivated {
			t.Errorf("cookies = %v", c)
		}
		rec = h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_4/delete", form: url.Values{}})
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/ui/users" {
			t.Errorf("delete: status %d, location %q", rec.Code, rec.Header().Get("Location"))
		}
	})
	t.Run("a refusal is the use case's message", func(t *testing.T) {
		h := newUsrHarness()
		h.users.err = usecase.BusinessRule("LAST_ALL_APPLICATIONS_ADMIN", "Cannot delete the last all-applications administrator")
		body := usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_1/delete", form: url.Values{}, datastar: true, multipart: true}))
		mustContain(t, body, "Cannot delete the last all-applications administrator")
		mustNotContain(t, body, "deleted")
	})
	t.Run("an unknown user is not found", func(t *testing.T) {
		h := newUsrHarness()
		h.users.err = usecase.NotFound("NOT_FOUND", "Principal not found")
		mustContain(t, usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_x/activate", form: url.Values{}, datastar: true, multipart: true})), "Principal not found")
	})
}

func TestUserForgedPostsAreRefused(t *testing.T) {
	cases := []struct {
		name, need, target string
		form               url.Values
		call               string
		// asClient posts as a client administrator instead of an anchor.
		anchorOnly bool
	}{
		{"create", usrPermCreate, "/ui/users/new", url.Values{"name": {"N"}, "email": {"n@unmapped.test"}, "clientId": {"clt_acme"}}, "create-user", false},
		{"edit", usrPermUpdate, "/ui/users/prn_2/edit", url.Values{"name": {"Robert"}}, "update:prn_2", false},
		{"activate", usrPermUpdate, "/ui/users/prn_4/activate", nil, "activate:prn_4", false},
		{"deactivate", usrPermUpdate, "/ui/users/prn_2/deactivate", nil, "deactivate:prn_2", false},
		{"delete", usrPermDelete, "/ui/users/prn_2/delete", nil, "delete:prn_2", false},
		{"send reset", usrPermUpdate, "/ui/users/prn_2/send-reset", nil, "send-reset:prn_2", false},
		{"reset password", usrPermUpdate, "/ui/users/prn_2/reset-password", url.Values{"newPassword": {usrPasswordValue}, "confirmPassword": {usrPasswordValue}}, "reset-password:prn_2", false},
		{"reset 2fa", usrPermUpdate, "/ui/users/prn_2/reset-2fa", nil, "reset-2fa:prn_2", false},
		{"roles", usrPermAssign, "/ui/users/prn_2/roles", url.Values{"roles": {"orders:viewer"}}, "assign-roles:prn_2", false},
		{"applications", usrPermUpdate, "/ui/users/prn_2/applications", url.Values{"applicationIds": {"app_orders"}}, "assign-apps:prn_2", false},
		{"all applications", usrPermUpdate, "/ui/users/prn_2/all-applications/grant", nil, "assign-apps:prn_2", false},
		{"restrict", usrPermUpdate, "/ui/users/prn_1/all-applications/restrict", nil, "assign-apps:prn_1", false},
		{"import", usrPermCreate, "/ui/users/import", url.Values{"clientId": {"clt_acme"}}, "import", false},
	}
	for _, c := range cases {
		for _, datastar := range []bool{true, false} {
			t.Run(c.name+map[bool]string{true: " datastar", false: " native"}[datastar], func(t *testing.T) {
				h := newUsrHarness()
				form := c.form
				if form == nil {
					form = url.Values{}
				}
				rec := h.do(t, usrWithout(c.need), req{method: http.MethodPost, target: c.target, form: form, datastar: datastar, multipart: datastar})
				for _, call := range h.users.calls {
					if !strings.HasPrefix(call, "check:") {
						t.Fatalf("a caller without %s ran %v", c.need, h.users.calls)
					}
				}
				if !datastar && rec.Code != http.StatusForbidden && rec.Code != http.StatusUnprocessableEntity {
					t.Errorf("native status %d", rec.Code)
				}
				if !strings.Contains(strings.ToLower(rec.Body.String()), "permission required") {
					t.Errorf("no refusal in the answer: %s", rec.Body.String())
				}
			})
		}
	}
	// The anchor-only routes: a client administrator holding every permission
	// is refused, and nothing runs.
	ca := usrClientAdmin(usrAllPerms...)
	for _, c := range []struct {
		name, target string
		form         url.Values
	}{
		{"grant client access", "/ui/users/prn_2/clients", url.Values{"clientId": {"clt_globex"}}},
		{"revoke client access", "/ui/users/prn_3/clients/clt_acme/revoke", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := newUsrHarness()
			form := c.form
			if form == nil {
				form = url.Values{}
			}
			rec := h.do(t, ca, req{method: http.MethodPost, target: c.target, form: form, datastar: true, multipart: true})
			if len(h.users.calls) != 0 {
				t.Fatalf("a client administrator ran %v", h.users.calls)
			}
			if !strings.Contains(strings.ToLower(rec.Body.String()), "anchor scope required") {
				t.Errorf("no refusal in the answer: %s", rec.Body.String())
			}
		})
	}
}

func TestUserDialogsAreRefusedWithTheSamePermission(t *testing.T) {
	for _, c := range []struct{ need, target string }{
		{usrPermView, "/ui/users/prn_2/edit"},
		{usrPermCreate, "/ui/users/new"},
		{usrPermCreate, "/ui/users/import"},
		{usrPermUpdate, "/ui/users/prn_2/reset-password"},
		{usrPermUpdate, "/ui/users/prn_2/applications"},
		{usrPermAssign, "/ui/users/prn_2/roles"},
		{usrPermRoles, "/ui/users/prn_2/roles"},
	} {
		t.Run(c.target+" without "+c.need, func(t *testing.T) {
			h := newUsrHarness()
			rec := h.do(t, usrWithout(c.need), req{target: c.target, datastar: true})
			mustContain(t, usrBody(t, rec), "permission required")
			mustNotContain(t, rec.Body.String(), "<dialog")
		})
	}
	// The client-access dialog is anchor-only.
	h := newUsrHarness()
	rec := h.do(t, usrClientAdmin(usrAllPerms...), req{target: "/ui/users/prn_2/clients/new", datastar: true})
	mustContain(t, usrBody(t, rec), "anchor scope required")
}

// ── password ───────────────────────────────────────────────────────────

func TestUserResetPasswordNeverEchoesThePassword(t *testing.T) {
	h := newUsrHarness()
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_2/reset-password", datastar: true}))
	mustContain(t, body, "Reset Password", "Bob &lt;Builder&gt; (bob@acme.test)", "New password", "Confirm password", `autocomplete="new-password"`, "can&#39;t receive the password-reset email")
	post := func(form url.Values) string {
		return usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_2/reset-password", datastar: true, multipart: true, form: form}))
	}
	// A mismatch and an empty password are the SPA's messages on the fields.
	b := post(url.Values{"newPassword": {usrPasswordValue}, "confirmPassword": {usrPasswordValue2}})
	mustContain(t, b, "Passwords do not match.")
	mustNotContain(t, b, usrPasswordValue, usrPasswordValue2)
	mustContain(t, post(url.Values{"newPassword": {""}, "confirmPassword": {""}}), "Password is required.")
	if len(h.users.calls) != 0 {
		t.Fatalf("ran %v", h.users.calls)
	}
	// The policy is the use case's, on the field, and the value is not echoed.
	h.users.err = usecase.Validation("PASSWORD_CONTAINS_IDENTITY", "Password must not contain your name or email")
	b = post(url.Values{"newPassword": {usrPasswordValue}, "confirmPassword": {usrPasswordValue}})
	mustContain(t, b, "Password must not contain your name or email", `id="usr-pwd-form-new-password-field"`)
	mustNotContain(t, b, usrPasswordValue)
	// A good one is set, reported without the value.
	h.users.err = nil
	b = post(url.Values{"newPassword": {usrPasswordValue}, "confirmPassword": {usrPasswordValue}})
	if h.users.passwordReset.NewPassword != usrPasswordValue {
		t.Errorf("request = %+v", h.users.passwordReset)
	}
	mustContain(t, b, "Password reset")
	mustNotContain(t, b, usrPasswordValue)
}

// ── roles ──────────────────────────────────────────────────────────────

func TestUserRolesDialogCeilingAndBounds(t *testing.T) {
	h := newUsrHarness()
	// An anchor user lacking user-admin power: the super-admin role the target
	// holds is above the caller, so it is kept, not offered.
	h.users.roles["prn_2"] = append(h.users.roles["prn_2"], principalapi.PrincipalRoleAssignmentDTO{ID: "r9", RoleName: "platform:super-admin", AssignmentSource: "MANUAL"})
	ac := usrAnchor(usrPermView, usrPermAssign, usrPermRoles, "platform:iam:user:delete")
	body := usrBody(t, h.do(t, ac, req{target: "/ui/users/prn_2/roles", datastar: true}))
	mustContain(t, body, "Manage Roles", `value="orders:viewer" checked`, `value="orders:editor" checked`, "Orders Approver", "orders:approver",
		// Roles of an application the user cannot access are left out, with a note.
		"2 role(s) hidden because their application isn't enabled",
		// The role above the caller is locked and kept as it is.
		"Above your access", `<input type="hidden" name="roles" value="platform:super-admin">`)
	mustNotContain(t, body, "billing:clerk", `value="platform:user-admin"`) // user-admin is within the caller's delete permission... but not assigned
	// The user-admin role (delete only) is within the caller: offered unchecked
	// to an anchor user, whose roles are never filtered by application.
	h.users.users[1].Scope = "ANCHOR"
	body = usrBody(t, h.do(t, ac, req{target: "/ui/users/prn_2/roles", datastar: true}))
	mustContain(t, body, "billing:clerk", "platform:user-admin")
	mustNotContain(t, body, "role(s) hidden")

	// The post is the chosen set plus the kept roles; the API decides.
	rec := h.do(t, ac, req{
		method: http.MethodPost, target: "/ui/users/prn_2/roles", datastar: true, multipart: true,
		form: url.Values{"roles": {"orders:viewer", "platform:super-admin", "orders:viewer"}, "drawer": {"prn_2"}},
	})
	body = usrBody(t, rec)
	if !slices.Equal(h.users.assignedRoles, []string{"orders:viewer", "platform:super-admin"}) {
		t.Errorf("roles = %v", h.users.assignedRoles)
	}
	mustContain(t, body, "Added 2 role(s)")
	// A refusal is the dialog's error, with the API's message.
	h.users.err = usecase.Authorization("ROLE_ABOVE_CALLER", "You may only assign or remove roles whose permissions you hold yourself: platform:super-admin")
	body = usrBody(t, h.do(t, ac, req{method: http.MethodPost, target: "/ui/users/prn_2/roles", datastar: true, multipart: true, form: url.Values{"roles": {"orders:viewer"}}}))
	mustContain(t, body, "You may only assign or remove roles whose permissions you hold yourself")
}

// ── application access ─────────────────────────────────────────────────

func TestUserApplicationAccess(t *testing.T) {
	h := newUsrHarness()
	// A grant outside the offered applications (an inactive one) is kept for an
	// anchor user.
	h.users.access["prn_2"] = principalapi.ApplicationAccessListResponse{Applications: []principalapi.ApplicationAccessResponse{
		{ApplicationID: "app_orders", ApplicationCode: "orders", ApplicationName: "Orders"},
		{ApplicationID: "app_gone", ApplicationCode: "gone", ApplicationName: "Gone"},
	}}
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_2/applications", datastar: true}))
	mustContain(t, body, "Manage Application Access", `value="app_orders" checked`, "Billing (billing)", `<input type="hidden" name="applicationIds" value="app_gone">`, "Kept as they are")
	// A client administrator is offered the bounded list and keeps nothing: the
	// API preserves what is out of reach.
	body = usrBody(t, h.do(t, usrClientAdmin(usrPermView, usrPermUpdate), req{target: "/ui/users/prn_2/applications", datastar: true}))
	mustNotContain(t, body, `value="app_gone"`, "Kept as they are")

	rec := h.do(t, anchorAdmin(), req{
		method: http.MethodPost, target: "/ui/users/prn_2/applications", datastar: true, multipart: true,
		form: url.Values{"applicationIds": {"app_orders", "app_gone"}, "drawer": {"prn_2"}},
	})
	mustContain(t, usrBody(t, rec), "Added 2 app(s)")
	if !slices.Equal(h.users.appBody.ApplicationIDs, []string{"app_orders", "app_gone"}) || h.users.appBody.AllApplications != nil {
		t.Errorf("body = %+v", h.users.appBody)
	}
}

func TestUserAllApplicationsSwitch(t *testing.T) {
	h := newUsrHarness()
	h.users.access["prn_2"] = principalapi.ApplicationAccessListResponse{Applications: []principalapi.ApplicationAccessResponse{{ApplicationID: "app_orders"}}}
	// Granting keeps the explicit grants; the flag is set.
	usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_2/all-applications/grant", datastar: true, multipart: true, form: url.Values{}}))
	if b := h.users.appBody; b.AllApplications == nil || !*b.AllApplications || !slices.Equal(b.ApplicationIDs, []string{"app_orders"}) {
		t.Errorf("grant body = %+v", b)
	}
	// Restricting someone else sends no self-restriction confirmation; your own
	// does, from the confirm dialog's field.
	usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_1/all-applications/restrict", datastar: true, multipart: true, form: url.Values{}}))
	if b := h.users.appBody; b.AllApplications == nil || *b.AllApplications || b.ConfirmSelfRestriction != nil {
		t.Errorf("restrict body = %+v", b)
	}
	usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_1/all-applications/restrict", datastar: true, multipart: true, form: url.Values{"confirmSelf": {"1"}}}))
	if b := h.users.appBody; b.ConfirmSelfRestriction == nil || !*b.ConfirmSelfRestriction {
		t.Errorf("self restrict body = %+v", b)
	}
	// Only an all-applications administrator is offered the grant.
	noAll := anchorAdmin()
	noAll.AllApplications = false
	body := usrBody(t, h.do(t, noAll, req{target: "/ui/users/prn_2", datastar: true}))
	mustContain(t, body, "Only an all-applications administrator can grant this.")
}

// ── client access ──────────────────────────────────────────────────────

func TestUserGrantClientDialog(t *testing.T) {
	h := newUsrHarness()
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/prn_3/clients/new", datastar: true}))
	// Cara is a partner of Acme and Globex already; only the others are offered.
	mustContain(t, body, "Grant Client Access", "Old Co (old)")
	mustNotContain(t, body, "Acme Corp (acme)", "Globex")
	// Not a valid client: the field's error, nothing called.
	body = usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_3/clients", datastar: true, multipart: true, form: url.Values{"clientId": {""}}}))
	mustContain(t, body, "Select a client.")
	// The API's refusals are the dialog's error.
	h.users.err = usecase.BusinessRule("NOT_PARTNER_SCOPE", "Client access can only be granted to partner users")
	body = usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/prn_2/clients", datastar: true, multipart: true, form: url.Values{"clientId": {"clt_globex"}}}))
	mustContain(t, body, "Client access can only be granted to partner users")
}

// ── import ─────────────────────────────────────────────────────────────

func TestParseUsrCSV(t *testing.T) {
	rows, err := parseUsrCSV(strings.NewReader("Full Name,Email,Roles (| separated)\n\"Doe, Jane\",jane@acme.test,orders:viewer|orders:editor\nJohn,john@acme.test,\n,,\nSolo,solo@acme.test\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []principalapi.BulkImportUser{
		{Name: "Doe, Jane", Email: "jane@acme.test", Roles: []string{"orders:viewer", "orders:editor"}},
		{Name: "John", Email: "john@acme.test"},
		{Name: "Solo", Email: "solo@acme.test"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %+v", rows)
	}
	for i := range want {
		if rows[i].Name != want[i].Name || rows[i].Email != want[i].Email || !slices.Equal(rows[i].Roles, want[i].Roles) {
			t.Errorf("row %d = %+v, want %+v", i, rows[i], want[i])
		}
	}
	// No header row is needed.
	rows, _ = parseUsrCSV(strings.NewReader("Jane,jane@acme.test,r1\n"))
	if len(rows) != 1 || rows[0].Name != "Jane" {
		t.Errorf("rows = %+v", rows)
	}
}

func TestUserImportDialogAndReport(t *testing.T) {
	h := newUsrHarness()
	body := usrBody(t, h.do(t, anchorAdmin(), req{target: "/ui/users/import", datastar: true}))
	mustContain(t, body, "Import users from CSV", `enctype="multipart/form-data"`, `type="file"`, "Download template", "Acme Corp (acme)")
	// A client administrator with one client has it implicit, and without
	// user:assign-roles is told the roles are not applied.
	body = usrBody(t, h.do(t, usrClientAdmin(usrPermView, usrPermCreate), req{target: "/ui/users/import", datastar: true}))
	mustContain(t, body, `<input type="hidden" name="clientId" value="clt_acme">`, "You cannot assign roles")
	mustNotContain(t, body, "Globex")

	rec := h.do(t, anchorAdmin(), req{target: "/ui/users/import/template.csv"})
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Type"), "text/csv") || !strings.Contains(rec.Body.String(), "Full Name,Email,Roles") {
		t.Errorf("template: %d %q", rec.Code, rec.Header().Get("Content-Type"))
	}

	h.users.importResponse = principalapi.BulkImportResponse{Created: 1, Skipped: 1, Failed: 1, Results: []principalapi.BulkImportResult{
		{Row: 1, Email: "jane@acme.test", Status: "created"},
		{Row: 2, Email: "bob@acme.test", Status: "exists", Message: "already exists — skipped"},
		{Row: 3, Email: "x@acme.test", Status: "error", Message: "roles not applied: nope <b>"},
	}}
	body = usrBody(t, h.postFile(t, anchorAdmin(), "/ui/users/import", url.Values{"clientId": {"clt_acme"}}, "users.csv", "Full Name,Email,Roles\nJane,jane@acme.test,orders:viewer\n"))
	if h.users.imported.ClientID != "clt_acme" || len(h.users.imported.Users) != 1 || h.users.imported.Users[0].Roles[0] != "orders:viewer" {
		t.Errorf("import = %+v", h.users.imported)
	}
	mustContain(t, body, "Import complete", "1 created, 1 skipped, 1 failed", "already exists — skipped", "roles not applied: nope &lt;b&gt;", "bob@acme.test")
	mustNotContain(t, body, "jane@acme.test</td>") // created rows are not listed
	// No file, an empty file and a missing client are the fields' errors.
	h.users.calls = nil
	mustContain(t, usrBody(t, h.do(t, anchorAdmin(), req{method: http.MethodPost, target: "/ui/users/import", datastar: true, multipart: true, form: url.Values{"clientId": {"clt_acme"}}})), "Choose a CSV file with at least one user.")
	mustContain(t, usrBody(t, h.postFile(t, anchorAdmin(), "/ui/users/import", url.Values{"clientId": {"clt_acme"}}, "e.csv", "Full Name,Email,Roles\n")), "No user rows found in the file.")
	mustContain(t, usrBody(t, h.postFile(t, anchorAdmin(), "/ui/users/import", url.Values{}, "e.csv", "A,a@b.test\n")), "Select a client.")
	if len(h.users.calls) != 0 {
		t.Errorf("ran %v", h.users.calls)
	}
}

// ── errors and the shared plumbing ─────────────────────────────────────

func TestUserPagesNeverCarryCredentials(t *testing.T) {
	h := newUsrHarness()
	for _, target := range []string{
		"/ui/users", "/ui/users/prn_2", "/ui/users/prn_2/edit", "/ui/users/prn_2/roles", "/ui/users/prn_2/applications",
		"/ui/users/prn_2/reset-password", "/ui/users/new", "/ui/users/import", "/ui/users/prn_3/clients/new",
	} {
		for _, datastar := range []bool{false, true} {
			body := usrBody(t, h.do(t, anchorAdmin(), req{target: target, datastar: datastar}))
			mustNotContain(t, body, usrPasswordValue, usrInviteLink, "password_hash", "passwordHash")
		}
	}
}

func TestUserListFailureIsTheUseCasesMessage(t *testing.T) {
	h := newUsrHarness()
	h.users.err = errors.New("db down")
	rec := h.do(t, anchorAdmin(), req{target: "/ui/users", datastar: true})
	mustContain(t, usrBody(t, rec), "Something went wrong")
	mustNotContain(t, rec.Body.String(), "db down")
}

// postFile posts a multipart form with one file as Datastar does.
func (h *usrHarness) postFile(t *testing.T, ac *auth.AuthContext, target string, form url.Values, filename, content string) *httptest.ResponseRecorder {
	t.Helper()
	var b strings.Builder
	mw := multipart.NewWriter(&b)
	for k, vs := range form {
		for _, v := range vs {
			if err := mw.WriteField(k, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	fw, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(fw, content); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	mux := chi.NewRouter()
	mux.Route("/ui", func(r chi.Router) { h.page().Mount(r) })
	r := httptest.NewRequest(http.MethodPost, target, strings.NewReader(b.String()))
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.Header.Set(ds.RequestHeader, "true")
	r.Header.Set("Accept", "text/event-stream, text/html, application/json")
	r = r.WithContext(auth.WithContext(r.Context(), ac))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}
