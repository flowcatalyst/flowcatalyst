//go:build integration

package sdksync

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// F15 (owner ruling 2026-10-13) on POST /api/applications/{appCode}/principals/sync:
// a role name that is not a role is skipped, never assigned, and reported in
// skippedRoles; the rest of the sync applies. Before, such a name was stored
// as a role assignment (an SDK_SYNC row naming no role, which became live
// authority the moment a role of that name was created), or, when its prefix
// named another application, refused the whole sync.
func TestSyncPrincipals_AppRouteSkipsUnknownRoles(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(pool)
	principals := principal.NewRepository(pool)

	const appCode, otherCode = "unkrolesdk", "unkroleother"
	apps := application.NewRepository(pool)
	app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
		appops.CreateCommand{Code: appCode, Name: "Unknown roles SDK"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
		appops.CreateCommand{Code: otherCode, Name: "Unknown roles other"}, testpg.TestEC())
	require.NoError(t, err)
	mkRole := func(app, name string, perms ...string) string {
		t.Helper()
		testpg.EnsureApplication(t, app)
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
			roleops.CreateCommand{ApplicationCode: app, RoleName: name, DisplayName: name, Permissions: perms},
			testpg.TestEC())
		require.NoError(t, err)
		return ev.Name
	}
	clerk := mkRole(appCode, "clerk", appCode+":order:view")
	foreign := mkRole(otherCode, "clerk", otherCode+":order:view")

	s := &State{Apps: apps, Principals: principals, Roles: roles, UoW: uow}
	caller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_unkrolesdk_c1", Scope: auth.ScopeAnchor,
		Applications: []string{app.ApplicationID},
		Permissions:  []string{"platform:iam:user:update"},
	})
	sync := func(entries ...syncPrincipalInputRequest) (SyncResultResponse, error) {
		out, err := s.syncPrincipals(caller, &syncPrincipalsInput{
			AppCode: appCode,
			Body:    syncPrincipalsRequest{Principals: entries},
		})
		if err != nil {
			return SyncResultResponse{}, err
		}
		return out.Body, nil
	}
	rolesOf := func(email string) []string {
		t.Helper()
		p, err := principals.FindByEmail(ctx, email)
		require.NoError(t, err)
		require.NotNil(t, p, email)
		return roleNamesOfAssignments(p)
	}
	const pending = appCode + ":not-yet-synced"

	// Unknown names, own-prefixed, unprefixed and another application's
	// prefix alike, are skipped; the known role is assigned; the sync is not
	// refused. Reported lower-cased, each once, in payload order.
	got, err := sync(
		syncPrincipalInputRequest{
			Email: "a@unkrolesdk.test", Name: "A",
			Roles: []string{clerk, pending, "Unprefixed", "ghostapp:admin"},
		},
		syncPrincipalInputRequest{
			Email: "b@unkrolesdk.test", Name: "B",
			Roles: []string{"UNKROLESDK:Not-Yet-Synced", "unprefixed"},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, uint32(2), got.Created)
	assert.Equal(t, []string{"a@unkrolesdk.test", "b@unkrolesdk.test"}, got.SyncedCodes)
	assert.Equal(t, []string{pending, "unprefixed", "ghostapp:admin"}, got.SkippedRoles)
	assert.Equal(t, []string{clerk}, rolesOf("a@unkrolesdk.test"), "no assignment for a name that is not a role")
	assert.Empty(t, rolesOf("b@unkrolesdk.test"), "every name skipped: the user is still synced, with no role")

	// Once the role exists, the same sync assigns it and reports nothing.
	admin := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_unkrolesdk_a1", Scope: auth.ScopeAnchor, AllApplications: true,
		Permissions: []string{"platform:*:*:*"},
	})
	_, err = s.syncRoles(admin, &syncRolesInput{
		AppCode: appCode,
		Body:    syncRolesRequest{Roles: []syncRoleInputRequest{{Name: "not-yet-synced"}}},
	})
	require.NoError(t, err)
	got, err = sync(syncPrincipalInputRequest{Email: "a@unkrolesdk.test", Name: "A", Roles: []string{clerk, pending}})
	require.NoError(t, err)
	assert.Nil(t, got.SkippedRoles)
	assert.ElementsMatch(t, []string{clerk, pending}, rolesOf("a@unkrolesdk.test"))
	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "skippedRoles", "omitted when nothing was skipped")

	// A known role of another application is still refused, the whole sync
	// with it (F12, unchanged), even beside an unknown name.
	_, err = sync(syncPrincipalInputRequest{
		Email: "c@unkrolesdk.test", Name: "C",
		Roles: []string{"ghostapp:admin", foreign},
	})
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "want a refusal, got %v", err)
	assert.Equal(t, "ROLE_APP_FORBIDDEN", ue.Code)
	p, err := principals.FindByEmail(ctx, "c@unkrolesdk.test")
	require.NoError(t, err)
	assert.Nil(t, p, "a refused sync writes nothing")
}

// F18: a role name with whitespace in it is not a role. The SDK principal sync
// treats it as it treats any unknown name (F15): skipped and reported, never
// stored, so it can never reach a token.
func TestSyncPrincipals_MalformedRoleNameIsSkippedNotStored(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(pool)
	principals := principal.NewRepository(pool)
	apps := application.NewRepository(pool)

	const appCode = "f18malformed"
	app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
		appops.CreateCommand{Code: appCode, Name: "F18 malformed"}, testpg.TestEC())
	require.NoError(t, err)
	_, err = usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(roles),
		roleops.CreateCommand{ApplicationCode: appCode, RoleName: "dispatcher", DisplayName: "d"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Apps: apps, Principals: principals, Roles: roles, UoW: uow}
	caller := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_f18malform_c1", Scope: auth.ScopeAnchor,
		Applications: []string{app.ApplicationID},
		Permissions:  []string{"platform:iam:user:update"},
	})
	out, err := s.syncPrincipals(caller, &syncPrincipalsInput{
		AppCode: appCode,
		Body: syncPrincipalsRequest{Principals: []syncPrincipalInputRequest{{
			Email: "a@f18malformed.test", Name: "A",
			Roles: []string{appCode + ": dispatcher", appCode + ":dispatcher"},
		}}},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{appCode + ": dispatcher"}, out.Body.SkippedRoles)
	p, err := principals.FindByEmail(ctx, "a@f18malformed.test")
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, []string{appCode + ":dispatcher"}, roleNamesOfAssignments(p))
}
