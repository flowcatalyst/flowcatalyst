//go:build integration

package sdksync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	principalops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// principalRoleSources reads a principal's role assignments as name → source.
func principalRoleSources(t *testing.T, id string) map[string]string {
	t.Helper()
	rows, err := testpg.Pool(t).Query(context.Background(),
		`SELECT role_name, COALESCE(assignment_source, '') FROM iam_principal_roles WHERE principal_id = $1`, id)
	require.NoError(t, err)
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var name, source string
		require.NoError(t, rows.Scan(&name, &source))
		out[name] = source
	}
	require.NoError(t, rows.Err())
	return out
}

// F13: an application's principal sync replaces only that application's
// SDK_SYNC roles on a user it lists (Java Principal.syncSourcedRoles, Rust
// is_own_sdk_role), as its removeUnlisted sweep already did (X-02(c)).
// Before the fix application A's sync dropped every SDK_SYNC role of the
// user, application B's included. A role the sync names that the user already
// holds from another source keeps that source (Java keeps the existing
// assignment), so a later sweep cannot strip an administrator's grant. The
// platform route (no application) still replaces every SDK_SYNC role.
func TestSyncPrincipals_AppRouteReplacesOnlyItsOwnSyncedRoles(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	for _, code := range []string{"ownra", "ownrb"} {
		_, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(application.NewRepository(pool)),
			appops.CreateCommand{Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
	}
	// The roles the application's sync names exist (a name that is not a role
	// is skipped, F15).
	for _, rn := range [][2]string{{"ownra", "new"}, {"ownra", "admin"}, {"ownrb", "viewer"}} {
		app, name := rn[0], rn[1]
		testpg.EnsureApplication(t, app)
		_, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(role.NewRepository(pool)),
			roleops.CreateCommand{ApplicationCode: app, RoleName: name, DisplayName: name}, testpg.TestEC())
		require.NoError(t, err)
	}
	const id, email = "prn_ownroles0001", "shared@ownroles.test"
	seedSyncUser(t, id, email, "CLIENT", "", "ownrb:viewer")
	seedSyncUser(t, "prn_ownroles0002", "other@ownroles.test", "CLIENT", "", "ownrb:viewer")
	for name, source := range map[string]string{"ownra:old": "SDK_SYNC", "ownra:admin": "ADMIN_ASSIGNED"} {
		_, err := pool.Exec(ctx, `INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ($1, $2, $3)`,
			id, name, source)
		require.NoError(t, err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source) VALUES ('prn_ownroles0002', 'ownra:old', 'SDK_SYNC')`)
	require.NoError(t, err)

	s := &State{Apps: application.NewRepository(pool), Principals: principal.NewRepository(pool), Roles: role.NewRepository(pool), UoW: uow}
	caller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "p_ownroles", Scope: auth.ScopeAnchor, Permissions: []string{"platform:*:*:*"}, AllApplications: true,
	})
	sync := func(appCode string, removeUnlisted bool, roles ...string) {
		t.Helper()
		_, err := s.syncPrincipals(caller, &syncPrincipalsInput{
			AppCode: appCode, RemoveUnlisted: removeUnlisted,
			Body: syncPrincipalsRequest{Principals: []syncPrincipalInputRequest{{Email: email, Name: "Shared", Roles: roles}}},
		})
		require.NoError(t, err)
	}

	sync("ownra", false, "ownra:new", "ownra:admin")
	assert.Equal(t, map[string]string{
		"ownrb:viewer": "SDK_SYNC",       // another application's synced role survives
		"ownra:new":    "SDK_SYNC",       // this application's set replaced ...
		"ownra:admin":  "ADMIN_ASSIGNED", // ... an administrator's grant keeps its source
	}, principalRoleSources(t, id))

	// The sweep strips only this application's synced roles from unlisted users.
	sync("ownra", true, "ownra:new")
	assert.Equal(t, map[string]string{"ownrb:viewer": "SDK_SYNC"}, principalRoleSources(t, "prn_ownroles0002"))
	assert.Equal(t, map[string]string{"ownrb:viewer": "SDK_SYNC", "ownra:new": "SDK_SYNC", "ownra:admin": "ADMIN_ASSIGNED"},
		principalRoleSources(t, id))

	// Application B's sync leaves application A's synced role in place.
	sync("ownrb", false)
	assert.Equal(t, map[string]string{"ownra:new": "SDK_SYNC", "ownra:admin": "ADMIN_ASSIGNED"}, principalRoleSources(t, id))

	// The platform route (no application) replaces every SDK_SYNC role, as before.
	_, err = usecaseop.Run(caller, uow, principalops.SyncPrincipals(s.Principals, s.Roles, nil), principalops.SyncPrincipalsCommand{
		Principals: []principalops.SyncPrincipalInput{{Email: email, Name: "Shared", Active: true, Roles: []string{"ownrb:viewer"}}},
	}, testpg.TestEC())
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"ownrb:viewer": "SDK_SYNC", "ownra:admin": "ADMIN_ASSIGNED"}, principalRoleSources(t, id))
}
