//go:build integration

package appusers

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

const (
	pUserView   = "platform:application-service:user:view"
	pUserCreate = "platform:application-service:user:create"
	pInvView    = "platform:application-service:invitation:view"
	pInvManage  = "platform:application-service:invitation:manage"
)

// fakeInvites records the deliveries and mints a recognisable fake link.
type fakeInvites struct {
	mu    sync.Mutex
	sent  []string // emails sent
	links []string // principal ids a link was minted for
	pool  *pgxpool.Pool
}

func (f *fakeInvites) token(ctx context.Context, p *principal.Principal, redirect *string) string {
	// Behave like the real emailer: replace the principal's outstanding tokens.
	_, _ = f.pool.Exec(ctx, `DELETE FROM iam_password_reset_tokens WHERE principal_id = $1`, string(p.ID))
	raw := "rawtoken-" + tsid.GenerateUntyped()
	_, err := f.pool.Exec(ctx, `INSERT INTO iam_password_reset_tokens (id, principal_id, token_hash, purpose, expires_at, created_at)
		VALUES ($1, $2, $3, 'invite', NOW() + interval '72 hours', NOW())`,
		tsid.Generate(tsid.PasswordResetToken), string(p.ID), "h"+tsid.GenerateUntyped())
	if err != nil {
		panic(err)
	}
	return raw
}

func (f *fakeInvites) SendInviteRedirect(ctx context.Context, p *principal.Principal, redirect *string) error {
	f.token(ctx, p, redirect)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, p.UserIdentity.Email)
	return nil
}

func (f *fakeInvites) InviteLink(ctx context.Context, p *principal.Principal, redirect *string) (string, error) {
	raw := f.token(ctx, p, redirect)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links = append(f.links, string(p.ID))
	return "https://platform.example/auth/set-password?token=" + raw, nil
}

type fixture struct {
	t              *testing.T
	pool           *pgxpool.Pool
	s              *State
	inv            *fakeInvites
	own, other     string // application codes
	ownID, otherID string
	c1, c2, c3, c4 string // c1,c2 enabled for own; c3 not; c4 config disabled
	u              map[string]string
	platformRole   string
	emailOf        map[string]string
}

func setup(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	sfx := strings.ToLower(tsid.GenerateUntyped())[4:]
	f := &fixture{t: t, pool: pool, own: "ua" + sfx, other: "ub" + sfx, u: map[string]string{}, emailOf: map[string]string{}}

	apps := application.NewRepository(pool)
	roles := role.NewRepository(pool)
	mkApp := func(code string) string {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps), appops.CreateCommand{Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		return ev.ApplicationID
	}
	f.ownID, f.otherID = mkApp(f.own), mkApp(f.other)
	mkRole := func(app, name string) {
		_, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: app, RoleName: name, DisplayName: name, Permissions: []string{app + ":thing:view"}}, testpg.TestEC())
		require.NoError(t, err)
	}
	mkRole(f.own, "clerk")
	mkRole(f.own, "admin")
	mkRole(f.other, "clerk")
	testpg.EnsureApplication(t, "platform")
	f.platformRole = "platform:ua" + sfx
	_, err0 := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{ApplicationCode: "platform", RoleName: "ua" + sfx, DisplayName: "ua"}, testpg.TestEC())
	require.NoError(t, err0)

	mkClient := func(tag string, cfgFor string, enabled bool) string {
		id := "clt_" + tag + sfx[:6]
		_, err := pool.Exec(ctx, `INSERT INTO tnt_clients (id, name, identifier) VALUES ($1, $1, $1)`, id)
		require.NoError(t, err)
		if cfgFor != "" {
			_, err = pool.Exec(ctx, `INSERT INTO app_client_configs (id, application_id, client_id, enabled) VALUES ($1, $2, $3, $4)`,
				tsid.Generate(tsid.AppClientConfig), cfgFor, id, enabled)
			require.NoError(t, err)
		}
		return id
	}
	f.c1, f.c2 = mkClient("c1", f.ownID, true), mkClient("c2", f.ownID, true)
	f.c3 = mkClient("c3", f.otherID, true)
	f.c4 = mkClient("c4", f.ownID, false)

	// users: key → (client, scope, roles, active, passwordHash, idp)
	mkUser := func(key string, clientID *string, scope string, rolesHeld []string, active bool, hash *string, idp string) {
		id := tsid.Generate(tsid.Principal)
		email := key + "-" + sfx + "@example.test"
		_, err := pool.Exec(ctx, `INSERT INTO iam_principals (id, type, scope, client_id, name, active, email, email_domain, idp_type, password_hash)
			VALUES ($1, 'USER', $2, $3, $4, $5, $6, 'example.test', $7, $8)`, id, scope, clientID, "User "+key, active, email, idp, hash)
		require.NoError(t, err)
		for _, r := range rolesHeld {
			_, err = pool.Exec(ctx, `INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1, $2, 'ADMIN_ASSIGNED')`, id, r)
			require.NoError(t, err)
		}
		f.u[key], f.emailOf[key] = id, email
	}
	hash := "$argon2id$v=19$m=65536,t=3,p=4$SECRETSALT$SECRETHASHVALUE"
	c1, c2, c3, c4 := f.c1, f.c2, f.c3, f.c4
	clerk, adminRole, otherClerk := f.own+":clerk", f.own+":admin", f.other+":clerk"
	mkUser("pend", &c1, "CLIENT", []string{clerk, otherClerk}, true, nil, "INTERNAL") // invited, never set a password; also holds the other app's role
	mkUser("done", &c1, "CLIENT", []string{adminRole}, true, &hash, "INTERNAL")       // has a password
	mkUser("off", &c2, "CLIENT", []string{clerk}, false, nil, "INTERNAL")             // inactive
	mkUser("fed", &c2, "CLIENT", []string{clerk}, true, nil, "OIDC")                  // federated
	mkUser("c2u", &c2, "CLIENT", []string{clerk}, true, &hash, "INTERNAL")            // another enabled client
	// Not the application's:
	mkUser("c3u", &c3, "CLIENT", []string{clerk}, true, &hash, "INTERNAL")         // client not enabled for the app
	mkUser("c4u", &c4, "CLIENT", []string{clerk}, true, &hash, "INTERNAL")         // client config disabled
	mkUser("noRole", &c1, "CLIENT", []string{otherClerk}, true, &hash, "INTERNAL") // holds none of the app's roles
	mkUser("anchor", nil, "ANCHOR", []string{clerk}, true, &hash, "INTERNAL")      // platform principal
	mkUser("partner", &c1, "PARTNER", []string{clerk}, true, &hash, "INTERNAL")    // not CLIENT scope

	// "pend" has a live invite token.
	_, err := pool.Exec(ctx, `INSERT INTO iam_password_reset_tokens (id, principal_id, token_hash, purpose, expires_at, created_at)
		VALUES ($1, $2, $3, 'invite', NOW() + interval '24 hours', NOW())`, tsid.Generate(tsid.PasswordResetToken), f.u["pend"], "hpend"+sfx)
	require.NoError(t, err)
	// "c2u" is not awaiting an invitation but an ordinary reset token must not matter.
	_, err = pool.Exec(ctx, `INSERT INTO iam_password_reset_tokens (id, principal_id, token_hash, purpose, expires_at, created_at)
		VALUES ($1, $2, $3, 'reset', NOW() + interval '1 hours', NOW())`, tsid.Generate(tsid.PasswordResetToken), f.u["c2u"], "hreset"+sfx)
	require.NoError(t, err)

	f.inv = &fakeInvites{pool: pool}
	f.s = &State{
		Repo: NewRepository(pool), Principals: principal.NewRepository(pool), Roles: roles, Apps: apps,
		ClientConfigs: application.NewClientConfigRepo(pool), Clients: client.NewRepository(pool),
		UoW: uow, Invites: f.inv,
	}
	return f
}

func (f *fixture) ctx(ac *auth.AuthContext) context.Context {
	return auth.WithContext(context.Background(), ac)
}

// svc is the application's own service account.
func (f *fixture) svc(perms ...string) context.Context {
	return f.ctx(&auth.AuthContext{PrincipalID: "prn_svc", Scope: auth.ScopeAnchor, Permissions: perms, Applications: []string{f.ownID}})
}

func (f *fixture) allPerms() []string { return []string{pUserView, pUserCreate, pInvView, pInvManage} }

func userIDs(us []ApplicationUser) []string {
	out := make([]string, 0, len(us))
	for _, u := range us {
		out = append(out, u.ID)
	}
	return out
}

func (f *fixture) visibleKeys() []string { return []string{"pend", "done", "off", "fed", "c2u"} }

func (f *fixture) idsOf(keys ...string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, f.u[k])
	}
	return out
}

func forbidden(t *testing.T, err error, app string) {
	t.Helper()
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
	assert.Contains(t, err.Error(), "Not authorised for application '"+app+"'")
}

// The users visible to an application: CLIENT users at its enabled clients
// holding its roles; nobody else; and only its own roles on them.
func TestAppUsers_ListVisibleSet(t *testing.T) {
	f := setup(t)
	out, err := f.s.list(f.svc(pUserView), &listInput{AppCode: f.own, PageSize: 200})
	require.NoError(t, err)
	assert.ElementsMatch(t, f.idsOf(f.visibleKeys()...), userIDs(out.Body.Users))
	assert.Equal(t, len(f.visibleKeys()), out.Body.Total)
	for _, u := range out.Body.Users {
		for _, r := range u.Roles {
			assert.True(t, strings.HasPrefix(r, f.own+":"), "only the application's own roles are shown, got %s", r)
		}
		if u.ID == f.u["pend"] {
			assert.Equal(t, []string{f.own + ":clerk"}, u.Roles, "the other application's role on the same user is not shown")
			assert.Equal(t, "PENDING", u.Invitation)
		}
		if u.ID == f.u["done"] {
			assert.Empty(t, u.Invitation)
		}
	}

	t.Run("filters", func(t *testing.T) {
		list := func(in listInput) []string {
			in.AppCode, in.PageSize = f.own, 200
			o, err := f.s.list(f.svc(pUserView), &in)
			require.NoError(t, err)
			return userIDs(o.Body.Users)
		}
		assert.ElementsMatch(t, f.idsOf("done", "pend"), list(listInput{ClientID: f.c1}))
		assert.ElementsMatch(t, f.idsOf("off"), list(listInput{Active: "false"}))
		assert.ElementsMatch(t, f.idsOf("done"), list(listInput{Roles: f.own + ":admin"}))
		assert.ElementsMatch(t, f.idsOf("done"), list(listInput{Roles: "admin"}), "short role names are the application's")
		assert.Empty(t, list(listInput{Roles: f.other + ":clerk"}), "another application's role matches nothing")
		assert.ElementsMatch(t, f.idsOf("pend"), list(listInput{Q: "PEND-"}))
		assert.Empty(t, list(listInput{Q: "%"}), "LIKE metacharacters are literal")
		// client c3's user cannot be reached by naming the client
		assert.Empty(t, list(listInput{ClientID: f.c3}))
	})

	t.Run("pagination", func(t *testing.T) {
		page := func(p, size int) (*ApplicationUserList, []string) {
			o, err := f.s.list(f.svc(pUserView), &listInput{AppCode: f.own, Page: p, PageSize: size, SortField: "email"})
			require.NoError(t, err)
			return &o.Body, userIDs(o.Body.Users)
		}
		first, a := page(0, 2)
		assert.Len(t, a, 2)
		assert.Equal(t, 5, first.Total)
		second, b := page(1, 2)
		assert.Len(t, b, 2)
		_, c := page(2, 2)
		assert.Len(t, c, 1)
		assert.ElementsMatch(t, f.idsOf(f.visibleKeys()...), append(append(a, b...), c...), "pages tile the set without overlap")
		assert.Equal(t, 5, second.Total)
		clamped, _ := page(0, 100000)
		assert.Equal(t, maxPageSize, clamped.PageSize)
		def, _ := page(0, 0)
		assert.Equal(t, defaultPageSize, def.PageSize)
	})
}

func TestAppUsers_Get(t *testing.T) {
	f := setup(t)
	ctx := f.svc(pUserView)
	got, err := f.s.get(ctx, &userInput{AppCode: f.own, ID: f.u["pend"]})
	require.NoError(t, err)
	assert.Equal(t, f.emailOf["pend"], got.Body.Email)

	_, errMissing := f.s.get(ctx, &userInput{AppCode: f.own, ID: "prn_doesnotexist"})
	testpg.RequireUsecaseError(t, errMissing, usecase.KindNotFound, "User_NOT_FOUND")
	for _, hidden := range []string{"c3u", "c4u", "noRole", "anchor", "partner"} {
		_, err := f.s.get(ctx, &userInput{AppCode: f.own, ID: f.u[hidden]})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "User_NOT_FOUND")
		assert.Equal(t, strings.ReplaceAll(errMissing.Error(), "prn_doesnotexist", "X"),
			strings.ReplaceAll(err.Error(), f.u[hidden], "X"), "%s: not distinguishable from a missing id", hidden)
	}
}

// The application binding: the SDK syncs' rule and error shape.
func TestAppUsers_ApplicationBinding(t *testing.T) {
	f := setup(t)
	otherSvc := f.ctx(&auth.AuthContext{PrincipalID: "prn_o", Scope: auth.ScopeAnchor, Permissions: f.allPerms(), Applications: []string{f.otherID}})
	unbound := f.ctx(&auth.AuthContext{PrincipalID: "prn_u", Scope: auth.ScopeAnchor, Permissions: f.allPerms()})

	calls := map[string]func(ctx context.Context, app string) error{
		"list": func(ctx context.Context, app string) error {
			_, err := f.s.list(ctx, &listInput{AppCode: app})
			return err
		},
		"get": func(ctx context.Context, app string) error {
			_, err := f.s.get(ctx, &userInput{AppCode: app, ID: f.u["pend"]})
			return err
		},
		"invite": func(ctx context.Context, app string) error {
			_, err := f.s.invite(ctx, &inviteInput{AppCode: app, Body: InviteApplicationUserRequest{Email: "x@example.test", Name: "X", ClientID: f.c1, Roles: []string{f.own + ":clerk"}}})
			return err
		},
		"invitations": func(ctx context.Context, app string) error {
			_, err := f.s.invitations(ctx, &invitationsInput{AppCode: app})
			return err
		},
		"resend": func(ctx context.Context, app string) error {
			_, err := f.s.resend(ctx, &resendInput{AppCode: app, ID: f.u["pend"]})
			return err
		},
		"revoke": func(ctx context.Context, app string) error {
			_, err := f.s.revoke(ctx, &userInput{AppCode: app, ID: f.u["pend"]})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			forbidden(t, call(otherSvc, f.own), f.own)
			forbidden(t, call(unbound, f.own), f.own)
			// An application that does not exist answers a scoped caller the
			// same 403, so it cannot probe which applications exist.
			forbidden(t, call(otherSvc, "nosuchapp"), "nosuchapp")
			// A caller reaching every application gets the 404.
			admin := f.ctx(&auth.AuthContext{PrincipalID: "prn_a", Scope: auth.ScopeAnchor, Permissions: f.allPerms(), AllApplications: true})
			testpg.RequireUsecaseError(t, call(admin, "nosuchapp"), usecase.KindNotFound, "Application_NOT_FOUND")
		})
	}
	// nothing was changed by any of the refused calls
	var active bool
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT active FROM iam_principals WHERE id = $1`, f.u["pend"]).Scan(&active))
	assert.True(t, active)
	var n int
	require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT count(*) FROM iam_principals WHERE email = 'x@example.test'`).Scan(&n))
	assert.Zero(t, n)
}

func TestAppUsers_AllApplicationsAdminSeesAny(t *testing.T) {
	f := setup(t)
	admin := f.ctx(&auth.AuthContext{PrincipalID: "prn_a", Scope: auth.ScopeAnchor, Permissions: []string{"platform:iam:user:view"}, AllApplications: true})
	for _, app := range []string{f.own, f.other} {
		out, err := f.s.list(admin, &listInput{AppCode: app, PageSize: 200})
		require.NoError(t, err)
		if app == f.own {
			assert.ElementsMatch(t, f.idsOf(f.visibleKeys()...), userIDs(out.Body.Users))
		} else {
			assert.Empty(t, out.Body.Users, "the other application has no users of its own at its enabled clients")
		}
	}
}

func TestAppUsers_ClientScopedCallerKeepsItsReach(t *testing.T) {
	f := setup(t)
	scoped := f.ctx(&auth.AuthContext{
		PrincipalID: "prn_cs", Scope: auth.ScopeClient, Clients: []string{f.c1},
		Permissions: f.allPerms(), Applications: []string{f.ownID},
	})
	out, err := f.s.list(scoped, &listInput{AppCode: f.own, PageSize: 200})
	require.NoError(t, err)
	assert.ElementsMatch(t, f.idsOf("pend", "done"), userIDs(out.Body.Users), "only client c1's users")

	_, err = f.s.get(scoped, &userInput{AppCode: f.own, ID: f.u["c2u"]})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "User_NOT_FOUND")

	// inviting into a client outside its reach, or one the application is not
	// enabled for, is one and the same refusal
	for _, c := range []string{f.c2, f.c3, f.c4, "clt_nosuchclient", "nosuchidentifier"} {
		_, err = f.s.invite(scoped, &inviteInput{AppCode: f.own, Body: InviteApplicationUserRequest{
			Email: "scoped-" + c + "@example.test", Name: "S", ClientID: c, Roles: []string{f.own + ":clerk"},
		}})
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
		assert.Contains(t, err.Error(), clientRefused)
	}
	// revoke / resend of a user at another client: not found
	_, err = f.s.resend(scoped, &resendInput{AppCode: f.own, ID: f.u["off"]})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Invitation_NOT_FOUND")
}

func TestAppUsers_PermissionsRequired(t *testing.T) {
	f := setup(t)
	deny := func(label string, err error) {
		t.Helper()
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")
		_ = label
	}
	// only invitation:view: lists invitations, nothing else
	view := f.svc(pInvView)
	_, err := f.s.invitations(view, &invitationsInput{AppCode: f.own})
	require.NoError(t, err)
	_, err = f.s.list(view, &listInput{AppCode: f.own})
	deny("users", err)
	_, err = f.s.resend(view, &resendInput{AppCode: f.own, ID: f.u["pend"]})
	deny("resend", err)
	_, err = f.s.revoke(view, &userInput{AppCode: f.own, ID: f.u["pend"]})
	deny("revoke", err)
	// user:view alone: reads users, cannot invite
	uv := f.svc(pUserView)
	_, err = f.s.list(uv, &listInput{AppCode: f.own})
	require.NoError(t, err)
	_, err = f.s.invite(uv, &inviteInput{AppCode: f.own, Body: InviteApplicationUserRequest{Email: "a@example.test", Name: "A", ClientID: f.c1, Roles: []string{f.own + ":clerk"}}})
	deny("invite", err)
	_, err = f.s.invitations(uv, &invitationsInput{AppCode: f.own})
	deny("invitations", err)
	// invitation:manage alone can manage but not read users
	mg := f.svc(pInvManage)
	_, err = f.s.list(mg, &listInput{AppCode: f.own})
	deny("users", err)
	// nothing at all, and no principal
	_, err = f.s.list(f.svc("platform:application-service:event:view"), &listInput{AppCode: f.own})
	deny("none", err)
	_, err = f.s.list(f.ctx(nil), &listInput{AppCode: f.own})
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "UNAUTHENTICATED")
}

// No password hash, token or link ever appears in a read.
func TestAppUsers_NoSensitiveFields(t *testing.T) {
	f := setup(t)
	ctx := f.svc(f.allPerms()...)
	users, err := f.s.list(ctx, &listInput{AppCode: f.own, PageSize: 200})
	require.NoError(t, err)
	one, err := f.s.get(ctx, &userInput{AppCode: f.own, ID: f.u["done"]})
	require.NoError(t, err)
	invs, err := f.s.invitations(ctx, &invitationsInput{AppCode: f.own})
	require.NoError(t, err)
	for _, v := range []any{users.Body, one.Body, invs.Body} {
		raw, err := json.Marshal(v)
		require.NoError(t, err)
		s := strings.ToLower(string(raw))
		for _, banned := range []string{"password", "hash", "secret", "token", "inviteLink", "argon2", "mfa", "externalidp"} {
			assert.NotContains(t, s, strings.ToLower(banned))
		}
	}
	// and the shape is exactly the documented one
	var m map[string]any
	raw, _ := json.Marshal(one.Body)
	require.NoError(t, json.Unmarshal(raw, &m))
	keys := make([]string, 0)
	for k := range m {
		keys = append(keys, k)
	}
	assert.ElementsMatch(t, []string{"id", "email", "name", "active", "clientId", "idpType", "roles", "createdAt"}, keys)
}

func TestAppUsers_Invite(t *testing.T) {
	f := setup(t)
	ctx := f.svc(f.allPerms()...)
	yes, no := true, false
	invite := func(c context.Context, email, clientID string, roles []string, mut func(*InviteApplicationUserRequest)) (*InviteApplicationUserResponse, error) {
		body := InviteApplicationUserRequest{Email: email, Name: "New Person", ClientID: clientID, Roles: roles}
		if mut != nil {
			mut(&body)
		}
		out, err := f.s.invite(c, &inviteInput{AppCode: f.own, Body: body})
		if err != nil {
			return nil, err
		}
		return &out.Body, nil
	}

	t.Run("creates a visible CLIENT user with the application's role and emails the invitation", func(t *testing.T) {
		resp, err := invite(ctx, "Fresh@Example.test", f.c1, []string{"clerk"}, nil)
		require.NoError(t, err)
		assert.True(t, resp.InvitationSent)
		assert.Nil(t, resp.InviteLink)
		assert.Equal(t, []string{"fresh@example.test"}, f.inv.sent)
		got, err := f.s.get(ctx, &userInput{AppCode: f.own, ID: resp.ID})
		require.NoError(t, err)
		assert.Equal(t, []string{f.own + ":clerk"}, got.Body.Roles)
		assert.Equal(t, f.c1, got.Body.ClientID)
		assert.Equal(t, "PENDING", got.Body.Invitation)
		var scope, src string
		require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT p.scope, r.assignment_source FROM iam_principals p JOIN iam_principal_roles r ON r.principal_id = p.id WHERE p.id = $1`, resp.ID).Scan(&scope, &src))
		assert.Equal(t, "CLIENT", scope)
		assert.Equal(t, "ADMIN_ASSIGNED", src, "an SDK sync sweep must not strip it")
	})

	t.Run("returnInviteLink returns the link and sends nothing", func(t *testing.T) {
		before := len(f.inv.sent)
		resp, err := invite(ctx, "link@example.test", f.c2, []string{f.own + ":admin"}, func(r *InviteApplicationUserRequest) {
			r.ReturnInviteLink = &yes
			r.InviteRedirectURI = new("https://app.example/welcome")
		})
		require.NoError(t, err)
		require.NotNil(t, resp.InviteLink)
		assert.Contains(t, *resp.InviteLink, "set-password?token=")
		assert.False(t, resp.InvitationSent)
		assert.Len(t, f.inv.sent, before)
	})

	t.Run("sendInvitation false delivers nothing", func(t *testing.T) {
		before := len(f.inv.sent)
		resp, err := invite(ctx, "quiet@example.test", f.c1, []string{"clerk"}, func(r *InviteApplicationUserRequest) { r.SendInvitation = &no })
		require.NoError(t, err)
		assert.False(t, resp.InvitationSent)
		assert.Nil(t, resp.InviteLink)
		assert.Len(t, f.inv.sent, before)
	})

	t.Run("a bad redirect writes nothing", func(t *testing.T) {
		_, err := invite(ctx, "badredirect@example.test", f.c1, []string{"clerk"}, func(r *InviteApplicationUserRequest) { r.InviteRedirectURI = new("javascript:alert(1)") })
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVITE_REDIRECT_URI_INVALID")
		assert.Zero(t, f.countUsers("badredirect@example.test"))
	})

	t.Run("roles: another application's, unknown, none", func(t *testing.T) {
		_, err := invite(ctx, "r1@example.test", f.c1, []string{f.other + ":clerk"}, nil)
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "ROLE_APP_FORBIDDEN")
		_, err = invite(ctx, "r2@example.test", f.c1, []string{f.own + ":clerk", f.platformRole}, nil)
		testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "ROLE_APP_FORBIDDEN")
		_, err = invite(ctx, "r3@example.test", f.c1, []string{"nosuchrole"}, nil)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "UNKNOWN_ROLE")
		_, err = invite(ctx, "r4@example.test", f.c1, nil, nil)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ROLES_REQUIRED")
		for _, e := range []string{"r1", "r2", "r3", "r4"} {
			assert.Zero(t, f.countUsers(e+"@example.test"), e)
		}
	})

	t.Run("clients: not enabled, disabled config, unknown, by identifier", func(t *testing.T) {
		for _, c := range []string{f.c3, f.c4, "clt_nosuchclient"} {
			_, err := invite(ctx, "c-"+c+"@example.test", c, []string{"clerk"}, nil)
			testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
			assert.Contains(t, err.Error(), clientRefused, c)
		}
		resp, err := invite(ctx, "byident@example.test", strings.ToUpper(f.c1), []string{"clerk"}, nil) // identifier == id here, case-insensitive
		require.NoError(t, err)
		assert.NotEmpty(t, resp.ID)
	})

	t.Run("existing email conflicts", func(t *testing.T) {
		_, err := invite(ctx, f.emailOf["c3u"], f.c1, []string{"clerk"}, nil)
		testpg.RequireUsecaseError(t, err, usecase.KindConflict, "EMAIL_EXISTS")
	})

	t.Run("a user of the other application cannot be invited here by an other-bound caller", func(t *testing.T) {
		otherSvc := f.ctx(&auth.AuthContext{PrincipalID: "prn_o", Scope: auth.ScopeAnchor, Permissions: f.allPerms(), Applications: []string{f.otherID}})
		_, err := f.s.invite(otherSvc, &inviteInput{AppCode: f.own, Body: InviteApplicationUserRequest{Email: "x2@example.test", Name: "X", ClientID: f.c1, Roles: []string{f.own + ":clerk"}}})
		forbidden(t, err, f.own)
		assert.Zero(t, f.countUsers("x2@example.test"))
	})
}

func (f *fixture) countUsers(email string) int {
	var n int
	require.NoError(f.t, f.pool.QueryRow(context.Background(), `SELECT count(*) FROM iam_principals WHERE email = $1`, email).Scan(&n))
	return n
}

func TestAppUsers_Invitations(t *testing.T) {
	f := setup(t)
	ctx := f.svc(f.allPerms()...)
	list := func(status string) []ApplicationInvitation {
		out, err := f.s.invitations(ctx, &invitationsInput{AppCode: f.own, Status: status, PageSize: 200})
		require.NoError(t, err)
		return out.Body.Invitations
	}
	uid := func(is []ApplicationInvitation) []string {
		out := []string{}
		for _, i := range is {
			out = append(out, i.UserID)
		}
		return out
	}
	// "pend" (live token) is PENDING; the inactive, the password-holding and the
	// federated users are not invitations; the other app's and unlinked users
	// never appear.
	assert.ElementsMatch(t, f.idsOf("pend"), uid(list("")))
	assert.ElementsMatch(t, f.idsOf("pend"), uid(list("PENDING")))
	assert.Empty(t, list("EXPIRED"))
	p := list("")[0]
	assert.Equal(t, "PENDING", p.Status)
	assert.NotNil(t, p.ExpiresAt)
	assert.Equal(t, []string{f.own + ":clerk"}, p.Roles)

	// the token lapses: the invitation is EXPIRED, still listed, so it can be resent
	_, err := f.pool.Exec(context.Background(), `UPDATE iam_password_reset_tokens SET expires_at = NOW() - interval '1 hour' WHERE principal_id = $1`, f.u["pend"])
	require.NoError(t, err)
	assert.ElementsMatch(t, f.idsOf("pend"), uid(list("EXPIRED")))
	assert.Empty(t, list("PENDING"))
	// the janitor purges it: still an invitation (no token, no password)
	_, err = f.pool.Exec(context.Background(), `DELETE FROM iam_password_reset_tokens WHERE principal_id = $1`, f.u["pend"])
	require.NoError(t, err)
	e := list("EXPIRED")
	require.Len(t, e, 1)
	assert.Nil(t, e[0].ExpiresAt)

	_, err = f.s.invitations(ctx, &invitationsInput{AppCode: f.own, Status: "bogus"})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVALID_STATUS")

	t.Run("pagination", func(t *testing.T) {
		for i := range 3 {
			_, err := f.s.invite(ctx, &inviteInput{AppCode: f.own, Body: InviteApplicationUserRequest{
				Email: strings.Repeat("p", i+1) + "@example.test", Name: "P", ClientID: f.c1, Roles: []string{"clerk"},
			}})
			require.NoError(t, err)
		}
		all, err := f.s.invitations(ctx, &invitationsInput{AppCode: f.own, PageSize: 200})
		require.NoError(t, err)
		assert.Equal(t, 4, all.Body.Total)
		p1, err := f.s.invitations(ctx, &invitationsInput{AppCode: f.own, Page: 0, PageSize: 3})
		require.NoError(t, err)
		p2, err := f.s.invitations(ctx, &invitationsInput{AppCode: f.own, Page: 1, PageSize: 3})
		require.NoError(t, err)
		assert.Len(t, p1.Body.Invitations, 3)
		assert.Len(t, p2.Body.Invitations, 1)
		assert.Equal(t, 4, p2.Body.Total)
	})
}

func TestAppUsers_ResendAndRevoke(t *testing.T) {
	f := setup(t)
	ctx := f.svc(f.allPerms()...)
	yes, no := true, false

	t.Run("resend emails a fresh invitation and replaces the old token", func(t *testing.T) {
		var oldHash string
		require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT token_hash FROM iam_password_reset_tokens WHERE principal_id = $1`, f.u["pend"]).Scan(&oldHash))
		out, err := f.s.resend(ctx, &resendInput{AppCode: f.own, ID: f.u["pend"]})
		require.NoError(t, err)
		assert.Nil(t, out.Body.InviteLink)
		assert.NotNil(t, out.Body.ExpiresAt)
		assert.Contains(t, f.inv.sent, f.emailOf["pend"])
		var n int
		require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT count(*) FROM iam_password_reset_tokens WHERE principal_id = $1 AND token_hash = $2`, f.u["pend"], oldHash).Scan(&n))
		assert.Zero(t, n, "the earlier link no longer works")
	})

	t.Run("resend can return the link instead", func(t *testing.T) {
		out, err := f.s.resend(ctx, &resendInput{AppCode: f.own, ID: f.u["pend"], Body: ResendInvitationRequest{ReturnInviteLink: &yes}})
		require.NoError(t, err)
		require.NotNil(t, out.Body.InviteLink)
	})

	t.Run("nothing to deliver is refused", func(t *testing.T) {
		_, err := f.s.resend(ctx, &resendInput{AppCode: f.own, ID: f.u["pend"], Body: ResendInvitationRequest{SendInvitation: &no}})
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, "INVITATION_DELIVERY_REQUIRED")
	})

	t.Run("not an invitee, not visible, not a user: not found", func(t *testing.T) {
		for _, k := range []string{"done", "off", "fed", "c3u", "anchor", "noRole"} {
			_, err := f.s.resend(ctx, &resendInput{AppCode: f.own, ID: f.u[k]})
			testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Invitation_NOT_FOUND")
			_, err = f.s.revoke(ctx, &userInput{AppCode: f.own, ID: f.u[k]})
			testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Invitation_NOT_FOUND")
		}
		_, err := f.s.resend(ctx, &resendInput{AppCode: f.own, ID: "prn_nosuchuser"})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Invitation_NOT_FOUND")
		// and none of them was touched
		for _, k := range []string{"done", "c3u", "anchor", "noRole"} {
			var active bool
			require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT active FROM iam_principals WHERE id = $1`, f.u[k]).Scan(&active))
			assert.True(t, active, k)
		}
	})

	t.Run("revoke removes the link and deactivates the invitee", func(t *testing.T) {
		_, err := f.s.revoke(ctx, &userInput{AppCode: f.own, ID: f.u["pend"]})
		require.NoError(t, err)
		var tokens int
		require.NoError(t, f.pool.QueryRow(context.Background(), `SELECT count(*) FROM iam_password_reset_tokens WHERE principal_id = $1 AND purpose = 'invite'`, f.u["pend"]).Scan(&tokens))
		assert.Zero(t, tokens)
		got, err := f.s.get(ctx, &userInput{AppCode: f.own, ID: f.u["pend"]})
		require.NoError(t, err)
		assert.False(t, got.Body.Active)
		assert.Empty(t, got.Body.Invitation)
		inv, err := f.s.invitations(ctx, &invitationsInput{AppCode: f.own})
		require.NoError(t, err)
		assert.Empty(t, inv.Body.Invitations)
		// a second revoke: it is no longer an invitation
		_, err = f.s.revoke(ctx, &userInput{AppCode: f.own, ID: f.u["pend"]})
		testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Invitation_NOT_FOUND")
	})
}
