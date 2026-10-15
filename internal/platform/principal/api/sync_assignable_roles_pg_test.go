//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	appops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/application/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// Application confinement (F12) on POST /api/principals/sync: a non-anchor
// caller may sync only the roles it could assign through
// PUT /api/principals/{id}/roles, application roles of an application the
// target's home client is entitled to (a new user has no client, so no role at
// all): 400 UNKNOWN_ROLE, 403 PLATFORM_ROLE_FORBIDDEN or ROLE_APP_FORBIDDEN, as
// Java's requireSyncableRoles. Before the fix the route handed a client
// administrator's users any role the F8 ceiling let through. An anchor may
// sync any role, as before.
func TestUserSync_NonAnchorSyncsOnlyAssignableRoles(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	uow := testpg.NewUoW(t)
	repo := principal.NewRepository(pool)
	roles := role.NewRepository(pool)
	apps := application.NewRepository(pool)
	s := &State{Repo: repo, Roles: roles, ClientConfigs: application.NewClientConfigRepo(pool), UoW: uow}

	// Two applications with an SDK-synced role each (application roles carry
	// their application id); the caller's client is entitled to the first.
	sdkRole := func(code string) (string, string) {
		t.Helper()
		app, err := usecaseop.Run(testpg.AnchorCtx(), uow, appops.CreateApplication(apps),
			appops.CreateCommand{Code: code, Name: code}, testpg.TestEC())
		require.NoError(t, err)
		allApps := auth.WithContext(ctx, &auth.AuthContext{
			PrincipalID: "prn_assignsync_s1", Scope: auth.ScopeAnchor, AllApplications: true,
			Permissions: []string{"platform:*:*:*"},
		})
		_, err = usecaseop.Run(allApps, uow, roleops.SyncRoles(roles), roleops.SyncRolesCommand{
			ApplicationCode: code, ApplicationID: app.ApplicationID,
			Roles: []roleops.SyncRoleInput{{Name: "clerk", Permissions: []string{code + ":order:view"}}},
		}, testpg.TestEC())
		require.NoError(t, err)
		return app.ApplicationID, code + ":clerk"
	}
	entitledID, entitled := sdkRole("assignsync")
	_, notEntitled := sdkRole("assignsyncx")
	platformRole := mustRoleWith(t, uow, "platform", "assignsync-empty")

	const mine = "cli_assignsync001"
	_, err := pool.Exec(ctx, `INSERT INTO app_client_configs (id, application_id, client_id, enabled)
		VALUES ('acc_assignsync01', $1, $2, TRUE)`, entitledID, mine)
	require.NoError(t, err)
	seedSyncTarget(t, "prn_assignsync_u1", "user@assignsync.test", "CLIENT", mine)

	clientAdmin := auth.WithContext(ctx, clientScopedCaller("prn_assignsync_a1", mine,
		"platform:iam:user:update", "platform:iam:user:create"))
	sync := func(c context.Context, email string, rs ...string) error {
		_, err := s.syncUsers(c, &apicommon.In[SyncUsersRequest]{Body: SyncUsersRequest{
			Principals: []SyncUserInput{{Email: email, Name: email, Roles: rs}},
		}})
		return err
	}
	refused := func(label string, err error, kind usecase.Kind, code, message string) {
		t.Helper()
		ue := usecase.AsError(err)
		require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
		assert.Equal(t, kind, ue.Kind, label)
		assert.Equal(t, code, ue.Code, label)
		assert.Equal(t, message, ue.Message, label)
	}
	rolesOf := func(email string) []string {
		t.Helper()
		p, err := repo.FindByEmail(ctx, email)
		require.NoError(t, err)
		if p == nil {
			return nil
		}
		return roleNamesFrom(p.Roles)
	}

	refused("an application the client is not entitled to", sync(clientAdmin, "user@assignsync.test", notEntitled),
		usecase.KindAuthorization, "ROLE_APP_FORBIDDEN", "role belongs to an application the client cannot access")
	refused("a platform role", sync(clientAdmin, "user@assignsync.test", platformRole),
		usecase.KindAuthorization, "PLATFORM_ROLE_FORBIDDEN", "client administrators cannot assign platform roles")
	refused("an unknown role", sync(clientAdmin, "user@assignsync.test", "ghost:role"),
		usecase.KindValidation, "UNKNOWN_ROLE", "role not found: ghost:role")
	refused("a new user has no client, so no role", sync(clientAdmin, "new@assignsync.test", entitled),
		usecase.KindAuthorization, "ROLE_APP_FORBIDDEN", "role belongs to an application the client cannot access")
	assert.Empty(t, rolesOf("user@assignsync.test"), "nothing written")
	assert.Nil(t, rolesOf("new@assignsync.test"), "nothing written")

	// The legitimate flows: an entitled application's role on the client's
	// user; a new user without roles; an anchor with any role.
	require.NoError(t, sync(clientAdmin, "user@assignsync.test", entitled))
	assert.Equal(t, []string{entitled}, rolesOf("user@assignsync.test"))
	require.NoError(t, sync(clientAdmin, "new@assignsync.test"))
	anchor := anchorHolding("USER", "platform:iam:user:update")
	require.NoError(t, sync(anchor, "new@assignsync.test", notEntitled, platformRole, "ghost:role"))
	// "ghost:role" is no role: skipped like on the application route (F15,
	// F18); it used to be stored and then minted into the user's tokens.
	assert.ElementsMatch(t, []string{notEntitled, platformRole}, rolesOf("new@assignsync.test"))
}
