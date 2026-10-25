//go:build integration

package pages

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	clientops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/client/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalapi "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/api"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/platformsink"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// usrMail records the invitations the API mints or sends.
type usrMail struct{ sent, minted []string }

func (m *usrMail) SendInviteRedirect(_ context.Context, p *principal.Principal, _ *string) error {
	m.sent = append(m.sent, p.UserIdentity.Email)
	return nil
}

func (m *usrMail) InviteLink(_ context.Context, p *principal.Principal, _ *string) (string, error) {
	m.minted = append(m.minted, p.UserIdentity.Email)
	return usrInviteLink, nil
}

// usrPG is the users page on Postgres: the handlers are the real principal
// API's, so the gates, scoping, ceiling and last-administrator guard are the API's.
type usrPG struct {
	t              *testing.T
	repo           *principal.Repository
	uow            *usecasepgx.UnitOfWork
	page           *Users
	mail           *usrMail
	clientA        string
	clientB        string
	appID          string
	bodies         []string
	clerk, platAdm string
}

func newUsrPG(t *testing.T) *usrPG {
	t.Helper()
	pool := testpg.ScratchDB(t, "uiusers_pg")
	uow := usecasepgx.New(pool, platformsink.New())
	repo := principal.NewRepository(pool)
	clients := client.NewRepository(pool)
	roles := role.NewRepository(pool)
	apps := application.NewRepository(pool)
	mail := &usrMail{}
	state := &principalapi.State{
		Repo: repo, GrantRepo: principal.NewClientAccessGrantRepo(pool), Roles: roles, Applications: apps,
		ClientConfigs: application.NewClientConfigRepo(pool), Clients: clients, UoW: uow, InviteEmailer: mail,
	}
	p := &usrPG{t: t, repo: repo, uow: uow, mail: mail, page: &Users{Users: state, Roles: roles, Clients: clients, Shell: shellForTest()}}
	run := func(ev any, err error) {
		t.Helper()
		require.NoError(t, err)
	}
	a, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: "Users A", Identifier: "uiusr-a"}, testpg.TestEC())
	run(a, err)
	b, err := usecaseop.Run(testpg.AnchorCtx(), uow, clientops.CreateClient(clients), clientops.CreateCommand{Name: "Users B", Identifier: "uiusr-b"}, testpg.TestEC())
	run(b, err)
	p.clientA, p.clientB = a.ClientID, b.ClientID
	app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps), appops.CreateCommand{Code: "uiusrapp", Name: "Users App"}, testpg.TestEC())
	run(app, err)
	p.appID = app.ApplicationID
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, appops.EnableApplicationForClient(apps, clients, application.NewClientConfigRepo(pool)),
		appops.EnableForClientCommand{ApplicationID: p.appID, ClientID: p.clientA}, testpg.TestEC())
	require.NoError(t, err)
	plat, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps), appops.CreateCommand{Code: "uiusrplat", Name: "Platform"}, testpg.TestEC())
	run(plat, err)
	mk := func(app, name string, perms ...string) string {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: app, RoleName: name, DisplayName: name, Permissions: perms}, testpg.TestEC())
		require.NoError(t, err)
		return ev.Name
	}
	p.clerk = mk("uiusrapp", "clerk", "uiusrapp:order:view")
	p.platAdm = mk("uiusrplat", "deleter", "platform:iam:user:delete")
	return p
}

func (p *usrPG) seed(email, name, scope, clientID string) string {
	p.t.Helper()
	cmd := operations.CreateCommand{Email: email, Name: &name, Scope: scope}
	if clientID != "" {
		cmd.ClientID = &clientID
	}
	ev, err := usecaseop.Run(testpg.AnchorCtx(), p.uow, operations.CreateUser(p.repo), cmd, testpg.TestEC())
	require.NoError(p.t, err)
	return string(ev.UserID)
}

// seedAdmin makes an active all-applications administrator.
func (p *usrPG) seedAdmin(email string) string {
	p.t.Helper()
	id := p.seed(email, "Admin "+email, "ANCHOR", "")
	yes := true
	_, err := usecaseop.Run(testpg.AnchorCtx(), p.uow, operations.AssignApplicationAccess(p.repo, application.NewRepository(testpg.Pool(p.t))),
		operations.AssignApplicationAccessCommand{UserID: ids.PrincipalID(id), AllApplications: &yes}, testpg.TestEC())
	_ = err
	return id
}

func (p *usrPG) do(ac *auth.AuthContext, method, target string, form url.Values) string {
	p.t.Helper()
	rec := serveMounted(p.t, ac, req{method: method, target: target, form: form, datastar: true, multipart: form != nil}, p.page)
	body := rec.Body.String()
	p.bodies = append(p.bodies, body)
	return body
}

func (p *usrPG) post(ac *auth.AuthContext, target string, form url.Values) string {
	p.t.Helper()
	if form == nil {
		form = url.Values{}
	}
	return p.do(ac, http.MethodPost, target, form)
}

func (p *usrPG) get(ac *auth.AuthContext, target string) string {
	return p.do(ac, http.MethodGet, target, nil)
}

func (p *usrPG) user(id string) *principal.Principal {
	p.t.Helper()
	u, err := p.repo.FindByID(context.Background(), ids.PrincipalID(id))
	require.NoError(p.t, err)
	return u
}

func (p *usrPG) byEmail(email string) *principal.Principal {
	p.t.Helper()
	u, err := p.repo.FindByEmail(context.Background(), email)
	require.NoError(p.t, err)
	return u
}

func usrRoleNames(u *principal.Principal) []string {
	var out []string
	for _, r := range u.Roles {
		out = append(out, r.Role.String())
	}
	return out
}

func usrSuper() *auth.AuthContext {
	ac := anchorAdmin()
	ac.AllApplications = true
	return ac
}

func TestUsersPageAgainstPostgres(t *testing.T) {
	p := newUsrPG(t)
	admin := usrSuper()

	// Create: the platform sends the invitation, and no link is shown.
	body := p.post(admin, "/ui/users/new", url.Values{"name": {"Una Unit"}, "email": {"una@uiusr.test"}, "clientId": {p.clientA}})
	assert.Contains(t, body, "Una Unit")
	u := p.byEmail("una@uiusr.test")
	require.NotNil(t, u)
	assert.Equal(t, principal.ScopeClient, u.Scope)
	assert.Equal(t, []string{"una@uiusr.test"}, p.mail.sent)
	assert.Empty(t, p.mail.minted)
	assert.NotContains(t, body, "usr-link-dialog")

	// Asked for, the link is minted, nothing is emailed, and it is in that answer only.
	body = p.post(admin, "/ui/users/new", url.Values{"name": {"Lin Link"}, "email": {"lin@uiusr.test"}, "clientId": {p.clientA}, "returnLink": {"1"}})
	assert.Contains(t, body, usrInviteLink)
	assert.Equal(t, []string{"lin@uiusr.test"}, p.mail.minted)
	assert.Len(t, p.mail.sent, 1)
	linID := string(p.byEmail("lin@uiusr.test").ID)
	for _, target := range []string{"/ui/users", "/ui/users/" + linID, "/ui/users/" + linID + "/edit"} {
		assert.NotContains(t, p.get(admin, target), "SENTINEL-INVITE-TOKEN", target)
	}

	// A duplicate address is the email field's error.
	body = p.post(admin, "/ui/users/new", url.Values{"name": {"Dup"}, "email": {"una@uiusr.test"}, "clientId": {p.clientA}})
	assert.Contains(t, strings.ToLower(body), "already exists")

	// Edit: name, then the association through the API's modes.
	uid := string(u.ID)
	p.post(admin, "/ui/users/"+uid+"/edit", url.Values{"name": {"Una Renamed"}, "scope": {"CLIENT"}, "clientId": {p.clientA}})
	assert.Equal(t, "Una Renamed", p.user(uid).Name)
	p.post(admin, "/ui/users/"+uid+"/edit", url.Values{"name": {"Una Renamed"}, "scope": {"PARTNER"}, "clientId": {p.clientB}})
	assert.Equal(t, principal.ScopePartner, p.user(uid).Scope)
	p.post(admin, "/ui/users/"+uid+"/edit", url.Values{"name": {"Una Renamed"}, "scope": {"ANCHOR"}})
	assert.Equal(t, principal.ScopeAnchor, p.user(uid).Scope)

	// Roles and application access.
	cid := string(p.byEmail("lin@uiusr.test").ID)
	p.post(admin, "/ui/users/"+cid+"/roles", url.Values{"roles": {p.clerk}})
	assert.Equal(t, []string{p.clerk}, usrRoleNames(p.user(cid)))
	p.post(admin, "/ui/users/"+cid+"/applications", url.Values{"applicationIds": {p.appID}})
	assert.Equal(t, []string{p.appID}, ids.Strings(p.user(cid).AccessibleApplicationIDs))
	p.post(admin, "/ui/users/"+cid+"/all-applications/grant", nil)
	assert.True(t, p.user(cid).AllApplications)
	p.post(admin, "/ui/users/"+cid+"/all-applications/restrict", nil)
	assert.False(t, p.user(cid).AllApplications)

	// Password: a weak one is refused on the field, a good one is set; neither
	// is echoed and the stored hash is in no answer.
	weak := p.post(admin, "/ui/users/"+cid+"/reset-password", url.Values{"newPassword": {"password"}, "confirmPassword": {"password"}})
	assert.Contains(t, strings.ToLower(weak), "password")
	assert.Nil(t, p.user(cid).UserIdentity.PasswordHash)
	good := p.post(admin, "/ui/users/"+cid+"/reset-password", url.Values{"newPassword": {usrPasswordValue}, "confirmPassword": {usrPasswordValue}})
	assert.NotContains(t, good, usrPasswordValue)
	require.NotNil(t, p.user(cid).UserIdentity.PasswordHash)
	hash := *p.user(cid).UserIdentity.PasswordHash
	p.get(admin, "/ui/users/"+cid)

	// State.
	p.post(admin, "/ui/users/"+cid+"/deactivate", nil)
	assert.False(t, p.user(cid).Active)
	p.post(admin, "/ui/users/"+cid+"/activate", nil)
	assert.True(t, p.user(cid).Active)
	p.post(admin, "/ui/users/"+cid+"/delete", nil)
	assert.Nil(t, p.user(cid))

	// The last all-applications administrator cannot be deleted. Una, made an
	// anchor above, is one too: with Yan beside her she can go, then Yan is the last.
	y := p.seedAdmin("y@uiusr.test")
	p.post(admin, "/ui/users/"+uid+"/delete", nil)
	require.Nil(t, p.user(uid))
	body = p.post(admin, "/ui/users/"+y+"/delete", nil)
	assert.Contains(t, body, "last active all-applications administrator")
	assert.NotNil(t, p.user(y))
	x := p.seedAdmin("x@uiusr.test")
	p.post(admin, "/ui/users/"+y+"/delete", nil)
	assert.Nil(t, p.user(y))
	assert.NotNil(t, p.user(x))

	// Secrets are in no answer, the sign-in link only in the one that minted it.
	linkAnswers := 0
	for _, b := range p.bodies {
		assert.NotContains(t, b, usrPasswordValue)
		assert.NotContains(t, b, hash)
		if strings.Contains(b, "SENTINEL-INVITE-TOKEN") {
			linkAnswers++
		}
	}
	assert.Equal(t, 1, linkAnswers)
}

func TestUsersPageClientAdministratorAgainstPostgres(t *testing.T) {
	p := newUsrPG(t)
	inA := p.seed("a1@uiusr.test", "In A", "CLIENT", p.clientA)
	inB := p.seed("b1@uiusr.test", "In B", "CLIENT", p.clientB)
	partner := p.seed("p1@uiusr.test", "Partner", "PARTNER", p.clientA)
	ca := usrClientAdmin(usrPermView, usrPermCreate, usrPermUpdate, usrPermAssign, usrPermRoles)
	ca.Clients = []string{p.clientA}

	// Only the clients' own users are listed, and another client's user is not found.
	list := p.get(ca, "/ui/users")
	assert.Contains(t, list, "In A")
	assert.NotContains(t, list, "In B")
	assert.Contains(t, p.get(ca, "/ui/users/"+inB), "not found")

	// Forged posts against another client's user, a partner and the anchor-only routes.
	p.post(ca, "/ui/users/"+inB+"/deactivate", nil)
	assert.True(t, p.user(inB).Active)
	p.post(ca, "/ui/users/"+inB+"/edit", url.Values{"name": {"Hijacked"}})
	assert.Equal(t, "In B", p.user(inB).Name)
	assert.Contains(t, p.post(ca, "/ui/users/"+partner+"/deactivate", nil), "client-scope")
	assert.True(t, p.user(partner).Active)
	p.post(ca, "/ui/users/"+inA+"/edit", url.Values{"name": {"In A"}, "scope": {"ANCHOR"}}) // the page sends no association for a client administrator
	assert.Equal(t, principal.ScopeClient, p.user(inA).Scope)
	assert.Contains(t, p.post(ca, "/ui/users/"+inA+"/clients", url.Values{"clientId": {p.clientB}}), "Anchor scope required")
	assert.Empty(t, p.user(inA).AssignedClients)

	// Create: in its client; never in another, never an anchor.
	p.post(ca, "/ui/users/new", url.Values{"name": {"New A"}, "email": {"new@uiusr.test"}, "clientId": {p.clientA}})
	created := p.byEmail("new@uiusr.test")
	require.NotNil(t, created)
	assert.Equal(t, principal.ScopeClient, created.Scope)
	p.post(ca, "/ui/users/new", url.Values{"name": {"New B"}, "email": {"newb@uiusr.test"}, "clientId": {p.clientB}})
	assert.Nil(t, p.byEmail("newb@uiusr.test"))

	// Roles: an application role of an entitled application, never a platform role.
	p.post(ca, "/ui/users/"+inA+"/roles", url.Values{"roles": {p.clerk}})
	assert.Equal(t, []string{p.clerk}, usrRoleNames(p.user(inA)))
	body := p.post(ca, "/ui/users/"+inA+"/roles", url.Values{"roles": {p.clerk, p.platAdm}})
	assert.Contains(t, body, "cannot access")
	assert.Equal(t, []string{p.clerk}, usrRoleNames(p.user(inA)))
}

func TestUsersPageForgedPostsAgainstPostgres(t *testing.T) {
	p := newUsrPG(t)
	target := p.seed("t@uiusr.test", "Target", "CLIENT", p.clientA)
	viewOnly := usrAnchor(usrPermView)
	id := "/ui/users/" + target

	p.post(viewOnly, id+"/edit", url.Values{"name": {"Changed"}})
	p.post(viewOnly, id+"/deactivate", nil)
	p.post(viewOnly, id+"/roles", url.Values{"roles": {p.clerk}})
	p.post(viewOnly, id+"/applications", url.Values{"applicationIds": {p.appID}})
	p.post(viewOnly, id+"/reset-password", url.Values{"newPassword": {usrPasswordValue}, "confirmPassword": {usrPasswordValue}})
	p.post(viewOnly, id+"/delete", nil)
	p.post(viewOnly, "/ui/users/new", url.Values{"name": {"X"}, "email": {"x@uiusr.test"}, "clientId": {p.clientA}})
	u := p.user(target)
	require.NotNil(t, u)
	assert.Equal(t, "Target", u.Name)
	assert.True(t, u.Active)
	assert.Empty(t, usrRoleNames(u))
	assert.Empty(t, u.AccessibleApplicationIDs)
	assert.Nil(t, u.UserIdentity.PasswordHash)
	assert.Nil(t, p.byEmail("x@uiusr.test"))

	// user:update does not change roles; user:assign-roles does not update the name.
	p.post(usrAnchor(usrPermView, usrPermUpdate), id+"/roles", url.Values{"roles": {p.clerk}})
	assert.Empty(t, usrRoleNames(p.user(target)))
	p.post(usrAnchor(usrPermView, usrPermAssign, usrPermRoles), id+"/edit", url.Values{"name": {"Changed"}})
	assert.Equal(t, "Target", p.user(target).Name)

	// The role ceiling: a caller lacking the permissions of a role can neither
	// add it nor take it off, and the dialog keeps it as it is.
	assigner := usrAnchor(usrPermView, usrPermAssign, usrPermRoles)
	body := p.post(assigner, id+"/roles", url.Values{"roles": {p.platAdm}})
	assert.Contains(t, body, "You may only assign or remove roles")
	assert.Empty(t, usrRoleNames(p.user(target)))
	p.post(usrSuper(), id+"/roles", url.Values{"roles": {p.platAdm}})
	require.Equal(t, []string{p.platAdm}, usrRoleNames(p.user(target)))
	dialog := p.get(assigner, id+"/roles")
	assert.Contains(t, dialog, "Above your access")
	assert.Contains(t, dialog, `name="roles" value="`+p.platAdm+`"`)
	body = p.post(assigner, id+"/roles", url.Values{"roles": {p.clerk}})
	assert.Contains(t, body, "You may only assign or remove roles")
	assert.Equal(t, []string{p.platAdm}, usrRoleNames(p.user(target)))
}
