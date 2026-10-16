//go:build integration

package api

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
)

const (
	appSvcRoleView = "platform:application-service:role:view"
	appSvcPermView = "platform:application-service:permission:view"
	iamRoleView    = "platform:iam:role:view"
)

// F21: GET /api/roles and its siblings must not hand an application service
// account every application's roles.
type roleReadFixture struct {
	s                  *State
	ownID, otherID     string
	own, other         string
	ownRole, otherRole string // by application id (+ code)
	ownLegacy          string // NULL application_id, own code
	mislinked          string // application_id of the other application, own code: the id wins
	otherLegacy        string // NULL application_id, other code
	ownRoleID          string
	otherRoleID        string
	ownPerm, otherPerm string
}

func newRoleReadFixture(t *testing.T) *roleReadFixture {
	t.Helper()
	pool := testpg.Pool(t)
	ctx := context.Background()
	sfx := strings.ToLower(tsid.GenerateUntyped())[5:]
	f := &roleReadFixture{
		s:       &State{Repo: role.NewRepository(pool), Permissions: role.NewPermissionRepo(pool), Apps: application.NewRepository(pool)},
		own:     "own" + sfx,
		other:   "oth" + sfx,
		ownID:   tsid.Generate(tsid.Application),
		otherID: tsid.Generate(tsid.Application),
	}
	for _, a := range [][2]string{{f.ownID, f.own}, {f.otherID, f.other}} {
		_, err := pool.Exec(ctx, `INSERT INTO app_applications (id, code, name) VALUES ($1, $2, $2)`, a[0], a[1])
		require.NoError(t, err)
	}
	add := func(id string, appID *string, app, short, perm string) string {
		name := app + ":" + short
		_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, application_id, name, display_name, description, application_code, source)
			VALUES ($1, $2, $3, $4, $5, $6, 'SDK')`, id, appID, name, "Display "+name, "secret-description-"+name, app)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO iam_role_permissions (role_id, permission) VALUES ($1, $2)`, id, perm)
		require.NoError(t, err)
		return name
	}
	f.ownPerm = f.own + ":ctx:agg:act"
	f.otherPerm = f.other + ":ctx:agg:act"
	f.ownRoleID, f.otherRoleID = "rol_f21own"+sfx[:6], "rol_f21oth"+sfx[:6]
	f.ownRole = add(f.ownRoleID, &f.ownID, f.own, "clerk", f.ownPerm)
	f.otherRole = add(f.otherRoleID, &f.otherID, f.other, "clerk", f.otherPerm)
	f.ownLegacy = add("rol_f21lgo"+sfx[:6], nil, f.own, "legacy", f.ownPerm)
	f.otherLegacy = add("rol_f21lgt"+sfx[:6], nil, f.other, "legacy", f.otherPerm)
	// A row linked to the other application's id but carrying the own code.
	f.mislinked = f.own + ":mislinked"
	_, err := pool.Exec(ctx, `INSERT INTO iam_roles (id, application_id, name, display_name, application_code, source)
		VALUES ($1, $2, $3, 'Mislinked', $4, 'SDK')`, "rol_f21mis"+sfx[:6], f.otherID, f.mislinked, f.own)
	require.NoError(t, err)

	for _, p := range []string{f.ownPerm, f.otherPerm, "platform:ctx:agg:act"} {
		require.NoError(t, f.s.Permissions.Upsert(ctx, role.Permission{Permission: p}))
	}
	return f
}

func (f *roleReadFixture) ctx(ac *auth.AuthContext) context.Context {
	return auth.WithContext(context.Background(), ac)
}

// svc is an application service account bound to the own application.
func (f *roleReadFixture) svc(perms ...string) *auth.AuthContext {
	return &auth.AuthContext{PrincipalID: "prn_svc", Scope: auth.ScopeAnchor, Permissions: perms, Applications: []string{f.ownID}}
}

func (f *roleReadFixture) ownRoles() []string { return []string{f.ownRole, f.ownLegacy} }

func names(rs []RoleResponse) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Name)
	}
	return out
}

func requireForbidden(t *testing.T, err error, label string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
}

func requireNotFound(t *testing.T, err error, label string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%s: want a not-found, got %v", label, err)
	assert.Equal(t, usecase.KindNotFound, ue.Kind, label)
}

// notFoundMessage is the answer for a role that does not exist, to prove the
// confined answer is byte-identical (no existence leak).
func notFoundMessage(t *testing.T, f *roleReadFixture, call func(name string) error, name string) string {
	t.Helper()
	err := call(name)
	requireNotFound(t, err, "missing "+name)
	return usecase.AsError(err).Message
}

func TestRoleRead_ApplicationConfinement(t *testing.T) {
	f := newRoleReadFixture(t)
	svc := f.ctx(f.svc(appSvcRoleView))

	t.Run("list: own application only, legacy rows by code, nothing leaks", func(t *testing.T) {
		out, err := f.s.list(svc, &apicommon.Empty{})
		require.NoError(t, err)
		assert.ElementsMatch(t, f.ownRoles(), names(out.Body.Roles),
			"exactly the bound application's roles (id-linked and legacy NULL-id), no other application's")
		assert.Equal(t, 2, out.Body.Total)
	})

	t.Run("by id and by name: own ok, other is the same 404 as a missing role", func(t *testing.T) {
		out, err := f.s.getByID(svc, &getInput{ID: f.ownRoleID})
		require.NoError(t, err)
		assert.Equal(t, f.ownRole, out.Body.Name)
		out, err = f.s.getByID(svc, &getInput{ID: f.ownRole}) // by name
		require.NoError(t, err)
		assert.Equal(t, f.ownRoleID, out.Body.ID)
		out, err = f.s.getByID(svc, &getInput{ID: f.ownLegacy})
		require.NoError(t, err)
		assert.Equal(t, f.ownLegacy, out.Body.Name)

		call := func(key string) error { _, err := f.s.getByID(svc, &getInput{ID: key}); return err }
		for _, key := range []string{f.otherRoleID, f.otherRole, f.otherLegacy, f.mislinked} {
			err := call(key)
			requireNotFound(t, err, key)
			missing := notFoundMessage(t, f, call, strings.Replace(key, f.other, f.own+"zz", 1))
			assert.Equal(t, strings.Replace(missing, f.own+"zz", f.other, 1), usecase.AsError(err).Message, "indistinguishable from missing")
		}
	})

	t.Run("by-code", func(t *testing.T) {
		out, err := f.s.byCode(svc, &byCodeInput{Code: f.ownRole})
		require.NoError(t, err)
		assert.Equal(t, f.ownRole, out.Body.Name)
		for _, c := range []string{f.otherRole, f.otherLegacy} {
			_, err = f.s.byCode(svc, &byCodeInput{Code: c})
			requireNotFound(t, err, c)
		}
	})

	t.Run("by-source", func(t *testing.T) {
		out, err := f.s.bySource(svc, &bySourceInput{Source: "SDK"})
		require.NoError(t, err)
		assert.ElementsMatch(t, f.ownRoles(), names(out.Body))
	})

	t.Run("by-application: own ok, foreign and unknown id are one 403", func(t *testing.T) {
		out, err := f.s.byApplication(svc, &byApplicationInput{ApplicationID: f.ownID})
		require.NoError(t, err)
		assert.Equal(t, []string{f.ownRole}, names(out.Body), "by application_id (legacy NULL-id rows are not linked to an id)")
		_, err = f.s.byApplication(svc, &byApplicationInput{ApplicationID: f.otherID})
		requireForbidden(t, err, "foreign application")
		_, err = f.s.byApplication(svc, &byApplicationInput{ApplicationID: "app_doesnotexist"})
		requireForbidden(t, err, "unknown application")
	})

	t.Run("filters/applications: only the bound code", func(t *testing.T) {
		out, err := f.s.applicationFilters(svc, &apicommon.Empty{})
		require.NoError(t, err)
		assert.Equal(t, []string{f.own}, out.Body.ApplicationCodes)
	})

	t.Run("role permissions", func(t *testing.T) {
		out, err := f.s.listRolePermissions(svc, &roleNameInput{RoleName: f.ownRole})
		require.NoError(t, err)
		assert.Equal(t, []string{f.ownPerm}, out.Body.Permissions)
		out, err = f.s.listRolePermissions(svc, &roleNameInput{RoleName: f.ownLegacy})
		require.NoError(t, err)
		assert.Equal(t, []string{f.ownPerm}, out.Body.Permissions)
		for _, n := range []string{f.otherRole, f.otherLegacy} {
			_, err = f.s.listRolePermissions(svc, &roleNameInput{RoleName: n})
			requireNotFound(t, err, n)
		}
	})

	t.Run("permission catalog: own application's permissions only", func(t *testing.T) {
		svcP := f.ctx(f.svc(appSvcPermView))
		out, err := f.s.listPermissions(svcP, &apicommon.Empty{})
		require.NoError(t, err)
		var got []string
		for _, p := range out.Body.Permissions {
			got = append(got, p.Permission)
		}
		assert.Equal(t, []string{f.ownPerm}, got)
		assert.Equal(t, 1, out.Body.Total)
		one, err := f.s.getPermission(svcP, &permissionPathInput{Permission: f.ownPerm})
		require.NoError(t, err)
		assert.Equal(t, f.ownPerm, one.Body.Permission)
		for _, p := range []string{f.otherPerm, "platform:ctx:agg:act"} {
			_, err = f.s.getPermission(svcP, &permissionPathInput{Permission: p})
			requireNotFound(t, err, p)
		}
		// role:view alone does not admit the catalog, nor permission:view the roles.
		_, err = f.s.listPermissions(svc, &apicommon.Empty{})
		requireForbidden(t, err, "role:view on the catalog")
		_, err = f.s.list(svcP, &apicommon.Empty{})
		requireForbidden(t, err, "permission:view on roles")
	})

	t.Run("unbound credential is refused on every read, not answered empty", func(t *testing.T) {
		unbound := &auth.AuthContext{PrincipalID: "prn_unb", Scope: auth.ScopeAnchor, Permissions: []string{appSvcRoleView, appSvcPermView}}
		ghost := &auth.AuthContext{PrincipalID: "prn_ghost", Scope: auth.ScopeAnchor, Permissions: []string{appSvcRoleView, appSvcPermView}, Applications: []string{"app_doesnotexist"}}
		for label, ac := range map[string]*auth.AuthContext{"unbound": unbound, "dangling": ghost} {
			c := f.ctx(ac)
			_, err := f.s.list(c, &apicommon.Empty{})
			requireForbidden(t, err, label+" list")
			_, err = f.s.getByID(c, &getInput{ID: f.ownRoleID})
			requireForbidden(t, err, label+" get")
			_, err = f.s.byCode(c, &byCodeInput{Code: f.ownRole})
			requireForbidden(t, err, label+" by-code")
			_, err = f.s.bySource(c, &bySourceInput{Source: "SDK"})
			requireForbidden(t, err, label+" by-source")
			_, err = f.s.byApplication(c, &byApplicationInput{ApplicationID: f.ownID})
			requireForbidden(t, err, label+" by-application")
			_, err = f.s.applicationFilters(c, &apicommon.Empty{})
			requireForbidden(t, err, label+" filters")
			_, err = f.s.listRolePermissions(c, &roleNameInput{RoleName: f.ownRole})
			requireForbidden(t, err, label+" role permissions")
			_, err = f.s.listPermissions(c, &apicommon.Empty{})
			requireForbidden(t, err, label+" catalog")
			_, err = f.s.getPermission(c, &permissionPathInput{Permission: f.ownPerm})
			requireForbidden(t, err, label+" catalog get")
		}
	})

	t.Run("a dangling id beside a real one still confines to the real one", func(t *testing.T) {
		ac := f.svc(appSvcRoleView)
		ac.Applications = []string{"app_doesnotexist", f.ownID}
		out, err := f.s.list(f.ctx(ac), &apicommon.Empty{})
		require.NoError(t, err)
		assert.ElementsMatch(t, f.ownRoles(), names(out.Body.Roles))
	})

	t.Run("bound to the other application sees only that one", func(t *testing.T) {
		ac := f.svc(appSvcRoleView)
		ac.Applications = []string{f.otherID}
		out, err := f.s.list(f.ctx(ac), &apicommon.Empty{})
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{f.otherRole, f.otherLegacy, f.mislinked}, names(out.Body.Roles))
	})

	t.Run("bound to both sees both", func(t *testing.T) {
		ac := f.svc(appSvcRoleView)
		ac.Applications = []string{f.ownID, f.otherID}
		out, err := f.s.list(f.ctx(ac), &apicommon.Empty{})
		require.NoError(t, err)
		assert.ElementsMatch(t, append(f.ownRoles(), f.otherRole, f.otherLegacy, f.mislinked), names(out.Body.Roles))
	})

	t.Run("client-scoped application caller: client reach does not widen application reach", func(t *testing.T) {
		ac := f.svc(appSvcRoleView)
		ac.Scope = auth.ScopeClient
		ac.Clients = []string{"clt_x"}
		out, err := f.s.list(f.ctx(ac), &apicommon.Empty{})
		require.NoError(t, err)
		assert.ElementsMatch(t, f.ownRoles(), names(out.Body.Roles))
	})

	t.Run("all-applications caller holding only the application-service permission is not narrowed", func(t *testing.T) {
		ac := f.svc(appSvcRoleView)
		ac.AllApplications = true
		ac.Applications = nil
		out, err := f.s.list(f.ctx(ac), &apicommon.Empty{})
		require.NoError(t, err)
		got := names(out.Body.Roles)
		assert.Subset(t, got, append(f.ownRoles(), f.otherRole, f.otherLegacy))
		_, err = f.s.getByID(f.ctx(ac), &getInput{ID: f.otherRoleID})
		require.NoError(t, err)
	})

	t.Run("console permission unchanged: sees everything, even when application-bound", func(t *testing.T) {
		ac := f.svc(iamRoleView)
		out, err := f.s.list(f.ctx(ac), &apicommon.Empty{})
		require.NoError(t, err)
		assert.Subset(t, names(out.Body.Roles), append(f.ownRoles(), f.otherRole, f.otherLegacy))
		_, err = f.s.getByID(f.ctx(ac), &getInput{ID: f.otherRoleID})
		require.NoError(t, err)
		pc, err := f.s.listPermissions(f.ctx(ac), &apicommon.Empty{})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, pc.Body.Total, 3)
		af, err := f.s.applicationFilters(f.ctx(ac), &apicommon.Empty{})
		require.NoError(t, err)
		assert.Subset(t, af.Body.ApplicationCodes, []string{f.own, f.other})
		// No unbound refusal for the console permission.
		unbound := &auth.AuthContext{PrincipalID: "prn_admin", Scope: auth.ScopeAnchor, AllApplications: true, Permissions: []string{iamRoleView}}
		_, err = f.s.list(f.ctx(unbound), &apicommon.Empty{})
		require.NoError(t, err)
	})

	t.Run("no permission, no read", func(t *testing.T) {
		none := f.svc("platform:application-service:event:view")
		_, err := f.s.list(f.ctx(none), &apicommon.Empty{})
		requireForbidden(t, err, "list")
		_, err = f.s.getByID(f.ctx(none), &getInput{ID: f.ownRoleID})
		requireForbidden(t, err, "get")
	})

	t.Run("wire shape is unchanged", func(t *testing.T) {
		out, err := f.s.getByID(svc, &getInput{ID: f.ownRoleID})
		require.NoError(t, err)
		b, err := json.Marshal(out.Body)
		require.NoError(t, err)
		var m map[string]any
		require.NoError(t, json.Unmarshal(b, &m))
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		assert.ElementsMatch(t, []string{"id", "applicationId", "name", "displayName", "description", "applicationCode", "permissions", "source", "clientManaged", "createdAt", "updatedAt"}, keys)
	})
}
