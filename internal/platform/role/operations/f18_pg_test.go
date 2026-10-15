//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// F18: a role name is rejected as sent when it is malformed, never trimmed.
func TestCreateRole_RejectsMalformedNamesAndPermissions(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	testpg.EnsureApplication(t, "f18create")

	for _, c := range []operations.CreateCommand{
		{ApplicationCode: "f18create", RoleName: " dispatcher", DisplayName: "d"},
		{ApplicationCode: "f18create", RoleName: "dis patcher", DisplayName: "d"},
		{ApplicationCode: "f18create ", RoleName: "dispatcher", DisplayName: "d"},
		{ApplicationCode: "f18create", RoleName: "dispatcher\n", DisplayName: "d"},
		{ApplicationCode: "f18create", RoleName: "a:", DisplayName: "d"},
	} {
		_, err := runAuthorized(uow, operations.CreateRole(repo), c)
		testpg.RequireUsecaseError(t, err, usecase.KindValidation, role.InvalidNameCode)
	}
	_, err := runAuthorized(uow, operations.CreateRole(repo), operations.CreateCommand{
		ApplicationCode: "f18create", RoleName: "ok", DisplayName: "d", Permissions: []string{"f18create: thing:view"}})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, role.InvalidPermissionCode)

	// Nothing was stored under any of those spellings.
	n, err := repo.FindByName(context.Background(), "f18create: dispatcher")
	require.NoError(t, err)
	assert.Nil(t, n)
}

// F18: a role is written with the id of the application that owns it, and a
// role for an application that does not exist is refused.
func TestCreateRole_SetsApplicationIDAndRefusesUnknownApplication(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	appID := testpg.EnsureApplication(t, "f18appid")

	ev := mustCreate(t, repo, uow, "f18appid", "dispatcher", "Dispatcher")
	got, err := repo.FindByName(ctx, ev.Name)
	require.NoError(t, err)
	require.NotNil(t, got)
	require.NotNil(t, got.ApplicationID, "application_id must not be null")
	assert.Equal(t, appID, string(*got.ApplicationID))

	_, err = runAuthorized(uow, operations.CreateRole(repo), operations.CreateCommand{
		ApplicationCode: "f18nosuchapp", RoleName: "x", DisplayName: "x"})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "APPLICATION_NOT_FOUND")
}

func TestGrantPermission_RejectsMalformedPermission(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	testpg.EnsureApplication(t, "f18grant")
	ev := mustCreate(t, repo, uow, "f18grant", "r", "R")
	_, err := runAuthorized(uow, operations.GrantPermission(repo),
		operations.GrantPermissionCommand{RoleName: ev.Name, Permission: "f18grant:thing: view"})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, role.InvalidPermissionCode)
}

// F18: the SDK role sync refuses a malformed role name or permission before
// any write, and adopts a role of the application that was stored without its
// id instead of colliding with it on the unique name.
func TestSyncRoles_RejectsMalformedAndAdoptsIdlessRole(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := role.NewRepository(pool)
	uow := testpg.NewUoW(t)
	appID := testpg.EnsureApplication(t, "f18sync")

	cmd := operations.SyncRolesCommand{ApplicationCode: "f18sync", ApplicationID: appID,
		Roles: []operations.SyncRoleInput{{Name: "good"}, {Name: "bad name"}}}
	_, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo), cmd, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, role.InvalidNameCode)
	g, err := repo.FindByName(ctx, "f18sync:good")
	require.NoError(t, err)
	assert.Nil(t, g, "a malformed entry refuses the whole sync")

	_, err = usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo), operations.SyncRolesCommand{
		ApplicationCode: "f18sync", ApplicationID: appID,
		Roles: []operations.SyncRoleInput{{Name: "good", Permissions: []string{"f18sync: x"}}}}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, role.InvalidPermissionCode)

	// A legacy row of the application with a null application_id.
	_, err = pool.Exec(ctx, `INSERT INTO iam_roles (id, application_id, name, display_name, application_code, source)
		VALUES ('rol_f18idless001', NULL, 'f18sync:legacy', 'Legacy', 'f18sync', 'SDK')`)
	require.NoError(t, err)
	_, err = usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo), operations.SyncRolesCommand{
		ApplicationCode: "f18sync", ApplicationID: appID,
		Roles: []operations.SyncRoleInput{{Name: "legacy"}}}, testpg.TestEC())
	require.NoError(t, err)
	l, err := repo.FindByName(ctx, "f18sync:legacy")
	require.NoError(t, err)
	require.NotNil(t, l)
	require.NotNil(t, l.ApplicationID, "the sync gives the adopted role its application id")
	assert.Equal(t, appID, string(*l.ApplicationID))
}
