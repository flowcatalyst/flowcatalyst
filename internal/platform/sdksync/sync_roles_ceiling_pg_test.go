//go:build integration

package sdksync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// The permission ceiling (F10) on POST /api/applications/{appCode}/roles/sync:
// an application's own permissions sync for any caller, but a synced role
// cannot gain or lose a platform permission the caller lacks. The route wrote
// whatever permissions the body carried, so an application's service account
// could put platform:*:*:* on a role its users hold.
func TestSyncRoles_PermissionsBoundedByCallersPermissions(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	roles := role.NewRepository(pool)

	const appCode = "ceilrolesync"
	app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(application.NewRepository(pool)),
		appops.CreateCommand{Code: appCode, Name: "Ceiling role sync"}, testpg.TestEC())
	require.NoError(t, err)

	s := &State{Apps: application.NewRepository(pool), Roles: roles, UoW: uow}
	syncAs := func(perms []string, removeUnlisted bool, rs ...syncRoleInputRequest) error {
		c := auth.WithContext(ctx, &auth.AuthContext{
			PrincipalID: "prn_ceilrolesync1", Scope: auth.ScopeAnchor,
			Applications: []string{app.ApplicationID},
			Permissions:  perms,
		})
		_, err := s.syncRoles(c, &syncRolesInput{AppCode: appCode, RemoveUnlisted: removeUnlisted, Body: syncRolesRequest{Roles: rs}})
		return err
	}
	sdk := []string{"platform:application-service:role:create", "platform:application-service:role:update"}
	refused := func(label string, err error, named string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
		assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
		assert.Equal(t, "PERMISSION_ABOVE_CALLER", ue.Code, label)
		assert.Contains(t, ue.Message, named, label)
	}

	// The application's own permissions: synced for any caller.
	require.NoError(t, syncAs(sdk, false, syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view"}}))

	// A platform permission the caller lacks: refused on create and update, nothing written.
	refused("new role", syncAs(sdk, false,
		syncRoleInputRequest{Name: "escalated", Permissions: []string{"platform:*:*:*"}}), "platform:*:*:*")
	refused("existing role", syncAs(sdk, false,
		syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view", "platform:*:*:*"}}), "platform:*:*:*")
	none, err := roles.FindByName(ctx, appCode+":escalated")
	require.NoError(t, err)
	assert.Nil(t, none)
	clerk, err := roles.FindByName(ctx, appCode+":clerk")
	require.NoError(t, err)
	require.NotNil(t, clerk)
	assert.Equal(t, []string{appCode + ":order:view"}, clerk.Permissions)

	// The wildcard holder may; then removal counts, by a replacing list or by
	// removeUnlisted deleting the role.
	require.NoError(t, syncAs([]string{"platform:*:*:*"}, false,
		syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view", "platform:iam:user:view"}}))
	refused("removing", syncAs(sdk, false,
		syncRoleInputRequest{Name: "clerk", Permissions: []string{appCode + ":order:view"}}), "platform:iam:user:view")
	refused("removeUnlisted", syncAs(sdk, true,
		syncRoleInputRequest{Name: "other", Permissions: []string{appCode + ":order:view"}}), "platform:iam:user:view")
	clerk, err = roles.FindByName(ctx, appCode+":clerk")
	require.NoError(t, err)
	require.NotNil(t, clerk)
	assert.ElementsMatch(t, []string{appCode + ":order:view", "platform:iam:user:view"}, clerk.Permissions)

	// Unchanged permissions are not checked: a re-sync of the same list, or of
	// none (which keeps the stored list), passes.
	require.NoError(t, syncAs(sdk, false,
		syncRoleInputRequest{Name: "clerk", Permissions: []string{"platform:iam:user:view", appCode + ":order:view"}}))
	require.NoError(t, syncAs(sdk, false, syncRoleInputRequest{Name: "clerk"}))
}
