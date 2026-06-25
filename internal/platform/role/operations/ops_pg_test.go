//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/seed"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

func ptr(s string) *string { return &s }

// runAuthorized drives op through the full use-case envelope (Validate →
// Authorize → Execute → atomic commit) as an anchor principal — the common
// case for these tests, which exercise validation, invariants, and
// persistence. Roles are global, so the operations carry no use-case-level
// authorization (the coarse CanWriteRoles/CanDeleteRoles permission lives on
// the controller); the Authorize phase here is Public. This mirrors how the
// HTTP handler runs the operation.
func runAuthorized[C any, E usecase.DomainEvent](
	uow *usecasepgx.UnitOfWork, op usecaseop.Operation[C, E], cmd C,
) (E, error) {
	return usecaseop.Run(testpg.AnchorCtx(), uow, op, cmd, testpg.TestEC())
}

// appAccessCtx is an all-applications anchor principal. SyncRoles authorizes
// against the target application (CanAccessApplication) — a bare AnchorCtx sets
// Scope=Anchor but NOT AllApplications, so it would be denied. App-scoped sync
// tests run under this principal so they can reach any application.
func appAccessCtx() context.Context {
	return testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_optestrunner1", Scope: auth.ScopeAnchor, AllApplications: true,
	})
}

// mustCreate seeds a DATABASE-sourced role through the public operation —
// the same path production uses. App codes are hand-unique per test: the
// fixture never truncates between tests, so tests own their rows and never
// assert table-wide.
func mustCreate(t *testing.T, repo *role.Repository, uow *usecasepgx.UnitOfWork, appCode, roleName, displayName string, perms ...string) operations.RoleCreated {
	t.Helper()
	ev, err := runAuthorized(uow, operations.CreateRole(repo), operations.CreateCommand{
		ApplicationCode: appCode,
		RoleName:        roleName,
		DisplayName:     displayName,
		Permissions:     perms,
	})
	require.NoError(t, err)
	return ev
}

// seedRawRole inserts an iam_roles row directly, mirroring the column set
// in repository.go's Persist. CODE-sourced roles are NOT creatable through
// the public ops (CreateRole always mints DATABASE), so the immutability
// guard can only be reached by seeding the row at the SQL level.
func seedRawRole(t *testing.T, id, name, displayName, source string, applicationID *string) {
	t.Helper()
	_, err := testpg.Pool(t).Exec(context.Background(),
		`INSERT INTO iam_roles (id, application_id, name, display_name, source, client_managed, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $5, FALSE, NOW(), NOW())`,
		id, applicationID, name, displayName, source)
	require.NoError(t, err)
}

// seedPrincipalWithRole creates a bare iam_principals row plus an
// iam_principal_roles assignment — the same seeding style as
// principal/repository_pg_test.go. The junction has no FK on role_name,
// so the role row itself can come from any source.
func seedPrincipalWithRole(t *testing.T, principalID, email, roleName string) {
	t.Helper()
	ctx := context.Background()
	pool := testpg.Pool(t)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email)
		 VALUES ($1, 'USER', 'PLATFORM', 'Role Ops Test User', TRUE, $2)`,
		principalID, email)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source)
		 VALUES ($1, $2, 'MANUAL')`, principalID, roleName)
	require.NoError(t, err)
}

// ── Create ────────────────────────────────────────────────────────────────

func TestCreateRole_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ev, err := runAuthorized(uow, operations.CreateRole(repo), operations.CreateCommand{
		ApplicationCode: "rolecrt",
		RoleName:        "editor",
		DisplayName:     "Document Editor",
		Description:     ptr("Edits documents"),
		Permissions:     []string{"rolecrt:doc:edit:*", "rolecrt:doc:read:*"},
		ClientManaged:   true,
	})
	require.NoError(t, err)

	assert.NotEmpty(t, ev.RoleID)
	assert.Equal(t, "rolecrt:editor", ev.Name, "full name = appCode:roleName")

	got, err := repo.FindByID(ctx, ev.RoleID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "rolecrt:editor", got.Name)
	assert.Equal(t, "Document Editor", got.DisplayName)
	assert.Equal(t, "rolecrt", got.ApplicationCode)
	require.NotNil(t, got.Description)
	assert.Equal(t, "Edits documents", *got.Description)
	assert.Equal(t, role.SourceDatabase, got.Source, "UI-created roles are DATABASE-sourced")
	assert.True(t, got.ClientManaged)
	assert.ElementsMatch(t, []string{"rolecrt:doc:edit:*", "rolecrt:doc:read:*"}, got.Permissions,
		"permissions granted via cmd must persist")
}

func TestCreateRole_Validation(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.CreateCommand
		code string
	}{
		{"missing application", operations.CreateCommand{RoleName: "x", DisplayName: "X"}, "APPLICATION_REQUIRED"},
		{"missing role name", operations.CreateCommand{ApplicationCode: "rolecrtval", DisplayName: "X"}, "ROLE_NAME_REQUIRED"},
		{"missing display name", operations.CreateCommand{ApplicationCode: "rolecrtval", RoleName: "x"}, "DISPLAY_NAME_REQUIRED"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.CreateRole(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, usecase.KindValidation, tc.code)
		})
	}
}

// Conflict is pinned by seeding through the operation itself: the first
// create IS the seed for the second.
func TestCreateRole_Duplicate_Conflict(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	mustCreate(t, repo, uow, "roledup", "editor", "First")

	_, err := runAuthorized(uow, operations.CreateRole(repo), operations.CreateCommand{
		ApplicationCode: "roledup", RoleName: "editor", DisplayName: "Second",
	})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "ROLE_EXISTS")
}

// ── Update ────────────────────────────────────────────────────────────────

func TestUpdateRole_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "roleupd", "viewer", "Before", "roleupd:doc:read:*")

	cm := true
	ev, err := runAuthorized(uow, operations.UpdateRole(repo), operations.UpdateCommand{
		ID:            seeded.RoleID,
		DisplayName:   ptr("  After  "),
		Description:   ptr("after"),
		Permissions:   []string{"roleupd:doc:read:*", "roleupd:doc:list:*"},
		ClientManaged: &cm,
	})
	require.NoError(t, err)
	assert.Equal(t, seeded.RoleID, ev.RoleID)
	assert.Equal(t, "roleupd:viewer", ev.Name)

	got, err := repo.FindByID(ctx, seeded.RoleID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "After", got.DisplayName, "display name is trimmed")
	require.NotNil(t, got.Description)
	assert.Equal(t, "after", *got.Description)
	assert.True(t, got.ClientManaged)
	assert.ElementsMatch(t, []string{"roleupd:doc:read:*", "roleupd:doc:list:*"}, got.Permissions)
	assert.Equal(t, "roleupd:viewer", got.Name, "name is immutable on update")
}

func TestUpdateRole_Errors(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.UpdateCommand
		kind usecase.Kind
		code string
	}{
		{"missing id", operations.UpdateCommand{DisplayName: ptr("X")}, usecase.KindValidation, "ID_REQUIRED"},
		{"blank display name", operations.UpdateCommand{ID: "rol_doesnotexist1", DisplayName: ptr("  ")}, usecase.KindValidation, "DISPLAY_NAME_REQUIRED"},
		{"unknown id", operations.UpdateCommand{ID: "rol_doesnotexist1", DisplayName: ptr("X")}, usecase.KindNotFound, "Role_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.UpdateRole(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

func TestUpdateRole_CodeRole_Immutable(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seedRawRole(t, "rol_immupdate0001", "roleimm:update-target", "Immutable Upd", "CODE", nil)

	_, err := runAuthorized(uow, operations.UpdateRole(repo), operations.UpdateCommand{
		ID: "rol_immupdate0001", DisplayName: ptr("Hacked"),
	})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_ROLE_IMMUTABLE")

	got, err := repo.FindByID(context.Background(), "rol_immupdate0001")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Immutable Upd", got.DisplayName, "refused update must not persist")
}

// ── Delete ────────────────────────────────────────────────────────────────

func TestDeleteRole_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seeded := mustCreate(t, repo, uow, "roledel", "doomed", "Doomed")

	ev, err := runAuthorized(uow, operations.DeleteRole(repo),
		operations.DeleteCommand{ID: seeded.RoleID})
	require.NoError(t, err)
	assert.Equal(t, seeded.RoleID, ev.RoleID)
	assert.Equal(t, "roledel:doomed", ev.Name)

	got, err := repo.FindByID(ctx, seeded.RoleID)
	require.NoError(t, err)
	assert.Nil(t, got, "deleted row must be gone")
}

func TestDeleteRole_Errors(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := runAuthorized(uow, operations.DeleteRole(repo), operations.DeleteCommand{})
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ID_REQUIRED")

	_, err = runAuthorized(uow, operations.DeleteRole(repo),
		operations.DeleteCommand{ID: "rol_doesnotexist1"})
	testpg.RequireUsecaseError(t, err, usecase.KindNotFound, "Role_NOT_FOUND")
}

func TestDeleteRole_CodeRole_Immutable(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	seedRawRole(t, "rol_immdelete0001", "roleimm:delete-target", "Immutable Del", "CODE", nil)

	_, err := runAuthorized(uow, operations.DeleteRole(repo),
		operations.DeleteCommand{ID: "rol_immdelete0001"})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "CODE_ROLE_IMMUTABLE")

	got, err := repo.FindByID(context.Background(), "rol_immdelete0001")
	require.NoError(t, err)
	assert.NotNil(t, got, "refused delete must leave the row in place")
}

// ── Grant / Revoke permission (lookup by NAME) ────────────────────────────

func TestGrantAndRevokePermission_HappyPath(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	mustCreate(t, repo, uow, "roleperm", "operator", "Operator", "roleperm:base:read:*")

	granted, err := runAuthorized(uow, operations.GrantPermission(repo), operations.GrantPermissionCommand{
		RoleName: "roleperm:operator", Permission: "roleperm:job:run:*",
	})
	require.NoError(t, err)
	assert.Equal(t, "roleperm:operator", granted.RoleName)
	assert.Equal(t, "roleperm:job:run:*", granted.Permission)

	got, err := repo.FindByName(ctx, "roleperm:operator")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.ElementsMatch(t, []string{"roleperm:base:read:*", "roleperm:job:run:*"}, got.Permissions,
		"granted permission must appear on reload by name")

	// Re-grant is idempotent at the data level (no duplicate row).
	_, err = runAuthorized(uow, operations.GrantPermission(repo), operations.GrantPermissionCommand{
		RoleName: "roleperm:operator", Permission: "roleperm:job:run:*",
	})
	require.NoError(t, err)
	got, err = repo.FindByName(ctx, "roleperm:operator")
	require.NoError(t, err)
	assert.Len(t, got.Permissions, 2, "re-grant must not duplicate the permission")

	revoked, err := runAuthorized(uow, operations.RevokePermission(repo), operations.RevokePermissionCommand{
		RoleName: "roleperm:operator", Permission: "roleperm:job:run:*",
	})
	require.NoError(t, err)
	assert.Equal(t, "roleperm:job:run:*", revoked.Permission)

	got, err = repo.FindByName(ctx, "roleperm:operator")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.ElementsMatch(t, []string{"roleperm:base:read:*"}, got.Permissions,
		"revoked permission must disappear on reload")
}

func TestGrantPermission_Errors(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.GrantPermissionCommand
		kind usecase.Kind
		code string
	}{
		{"missing role name", operations.GrantPermissionCommand{Permission: "a:b:c:d"}, usecase.KindValidation, "ROLE_NAME_REQUIRED"},
		{"missing permission", operations.GrantPermissionCommand{RoleName: "nope:nope"}, usecase.KindValidation, "PERMISSION_REQUIRED"},
		{"unknown role", operations.GrantPermissionCommand{RoleName: "rolepermerr:ghost", Permission: "a:b:c:d"}, usecase.KindNotFound, "Role_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.GrantPermission(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

func TestRevokePermission_Errors(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	cases := []struct {
		name string
		cmd  operations.RevokePermissionCommand
		kind usecase.Kind
		code string
	}{
		{"missing role name", operations.RevokePermissionCommand{Permission: "a:b:c:d"}, usecase.KindValidation, "ROLE_NAME_REQUIRED"},
		{"missing permission", operations.RevokePermissionCommand{RoleName: "nope:nope"}, usecase.KindValidation, "PERMISSION_REQUIRED"},
		{"unknown role", operations.RevokePermissionCommand{RoleName: "rolepermerr:ghost", Permission: "a:b:c:d"}, usecase.KindNotFound, "Role_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.RevokePermission(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

// ── SyncRoles (application-scoped SDK sync) ───────────────────────────────

func TestSyncRoles_Validation(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	_, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo),
		operations.SyncRolesCommand{}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "APPLICATION_CODE_REQUIRED")

	_, err = usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo),
		operations.SyncRolesCommand{ApplicationCode: "rolesyncval"}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "ROLES_REQUIRED")
}

// TestSyncRoles_RequiresAppAccess proves the use case's resource-level
// authorization: a principal without access to the target application is
// denied before any write (the coarse "may sync roles" permission is the
// controller's separate gate).
func TestSyncRoles_RequiresAppAccess(t *testing.T) {
	t.Parallel()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	noAccessCtx := testpg.WithAuth(context.Background(), &auth.AuthContext{
		PrincipalID: "prn_noappaccess", Scope: auth.ScopeClient, Applications: []string{"app_other"},
	})
	_, err := usecaseop.Run(noAccessCtx, uow, operations.SyncRoles(repo), operations.SyncRolesCommand{
		ApplicationCode: "rolesyncnoaccess",
		ApplicationID:   "app_rolesyncnoaccess",
		Roles:           []operations.SyncRoleInput{{Name: "Editor"}},
	}, testpg.TestEC())
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
}

// TestSyncRoles_UpsertPreserveAndRemoveUnlisted is the SDK-sync behavior
// pin: upsert counts, permission preservation on empty payload lists, and
// RemoveUnlisted touching only SDK-sourced rows. Scoped to a fresh app
// code + id (iam_roles.application_id has no FK), so it is fully hermetic
// and safe to run in parallel.
func TestSyncRoles_UpsertPreserveAndRemoveUnlisted(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()
	const appCode, appID = "rolesyncapp1", "app_rolesync00001"

	// Non-SDK row in the same application scope: sync must NEVER touch it,
	// even when its name appears in the payload.
	seedRawRole(t, "rol_syncmanual001", appCode+":manual", "Manual Row", "DATABASE", ptr(appID))

	first, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo), operations.SyncRolesCommand{
		ApplicationCode: appCode,
		ApplicationID:   appID,
		Roles: []operations.SyncRoleInput{
			{Name: "Editor", DisplayName: ptr("Doc Editor"), Permissions: []string{appCode + ":doc:edit:*"}},
			{Name: "Viewer"},
		},
	}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(2), first.Created)
	assert.Equal(t, uint32(0), first.Updated)
	assert.Equal(t, uint32(0), first.Removed)
	assert.Equal(t, uint32(2), first.Total)
	assert.Equal(t, appCode, first.ApplicationCode)
	assert.ElementsMatch(t, []string{appCode + ":editor", appCode + ":viewer"}, first.SyncedCodes,
		"canonical names are {appCode}:{name lowercased}")

	editor, err := repo.FindByName(ctx, appCode+":editor")
	require.NoError(t, err)
	require.NotNil(t, editor)
	assert.Equal(t, role.SourceSDK, editor.Source)
	require.NotNil(t, editor.ApplicationID)
	assert.Equal(t, appID, *editor.ApplicationID, "fresh SDK rows are stamped with the app id")
	assert.Equal(t, "Doc Editor", editor.DisplayName)
	assert.ElementsMatch(t, []string{appCode + ":doc:edit:*"}, editor.Permissions)

	// Re-sync with NO permissions on the editor row: stored permissions
	// must be preserved (apps declare role names; permissions are curated
	// in the UI — an empty list must not wipe them). DisplayName omitted
	// falls back to the raw payload name.
	second, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo), operations.SyncRolesCommand{
		ApplicationCode: appCode,
		ApplicationID:   appID,
		Roles:           []operations.SyncRoleInput{{Name: "Editor"}},
	}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), second.Created)
	assert.Equal(t, uint32(1), second.Updated)
	assert.Equal(t, uint32(0), second.Removed)

	editor, err = repo.FindByName(ctx, appCode+":editor")
	require.NoError(t, err)
	require.NotNil(t, editor)
	assert.ElementsMatch(t, []string{appCode + ":doc:edit:*"}, editor.Permissions,
		"empty permissions list on a sync row must PRESERVE stored permissions")
	assert.Equal(t, "Editor", editor.DisplayName, "omitted displayName falls back to the payload name")

	// RemoveUnlisted prunes the unlisted SDK row (viewer) but skips the
	// DATABASE row even though its name is in the payload.
	third, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo), operations.SyncRolesCommand{
		ApplicationCode: appCode,
		ApplicationID:   appID,
		Roles:           []operations.SyncRoleInput{{Name: "Editor"}, {Name: "Manual"}},
		RemoveUnlisted:  true,
	}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), third.Created, "existing non-SDK row must not be re-created")
	assert.Equal(t, uint32(1), third.Updated, "only the SDK row counts as updated")
	assert.Equal(t, uint32(1), third.Removed)

	gone, err := repo.FindByName(ctx, appCode+":viewer")
	require.NoError(t, err)
	assert.Nil(t, gone, "unlisted SDK row must be deleted")

	manual, err := repo.FindByName(ctx, appCode+":manual")
	require.NoError(t, err)
	require.NotNil(t, manual, "RemoveUnlisted must never touch non-SDK rows")
	assert.Equal(t, role.SourceDatabase, manual.Source)
	assert.Equal(t, "Manual Row", manual.DisplayName, "non-SDK row must not be updated either")
}

// TestSyncRoles_RemoveUnlisted_RoleHasAssignments pins the business rule:
// RemoveUnlisted REFUSES to drop a role that principals still hold —
// iam_principal_roles has no FK on role_name, so a silent delete would
// orphan the assignments. The whole sync aborts (nothing else persists).
func TestSyncRoles_RemoveUnlisted_RoleHasAssignments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()
	const appCode, appID = "rolesyncasgn1", "app_rolesyncasgn1"

	_, err := usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo), operations.SyncRolesCommand{
		ApplicationCode: appCode,
		ApplicationID:   appID,
		Roles:           []operations.SyncRoleInput{{Name: "Held"}},
	}, ec)
	require.NoError(t, err)

	seedPrincipalWithRole(t, "prn_rolesyncheld1", "rolesync-held@example.com", appCode+":held")

	_, err = usecaseop.Run(appAccessCtx(), uow, operations.SyncRoles(repo), operations.SyncRolesCommand{
		ApplicationCode: appCode,
		ApplicationID:   appID,
		Roles:           []operations.SyncRoleInput{{Name: "Other"}},
		RemoveUnlisted:  true,
	}, ec)
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "ROLE_HAS_ASSIGNMENTS")

	held, err := repo.FindByName(ctx, appCode+":held")
	require.NoError(t, err)
	assert.NotNil(t, held, "assigned role must survive the refused sync")

	other, err := repo.FindByName(ctx, appCode+":other")
	require.NoError(t, err)
	assert.Nil(t, other, "refused sync must be atomic — the new role must not be created")
}

// ── SyncPlatformRoles (static CODE catalogue) ─────────────────────────────

// TestSyncPlatformRoles_CatalogueLifecycle is deliberately SERIAL — no
// t.Parallel(). SyncPlatformRoles sweeps EVERY CODE-source role absent
// from the supplied catalogue, so it must not interleave with the
// parallel tests that raw-seed CODE rows (Go runs serial tests to
// completion before any parallel test body executes). Every call passes
// append(seed.PlatformRoles(), testRoles...) — the production catalogue
// (cf. shared/bff/roles.go syncPlatform) — so real catalogue rows are
// never swept.
func TestSyncPlatformRoles_CatalogueLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	catalogue := seed.PlatformRoles()
	n := len(catalogue)

	mkTest := func(roleName, displayName string) role.Role {
		r := role.New("roleplat", roleName, displayName)
		r.Source = role.SourceCode
		r.Description = ptr("platform-sync test role")
		r.Permissions = []string{"roleplat:thing:read:*"}
		return *r
	}
	withTest := func(extra ...role.Role) []role.Role {
		out := append([]role.Role{}, catalogue...)
		return append(out, extra...)
	}

	// 1. Fresh database (migrations seed no iam_roles): every catalogue
	//    role plus the two test roles is a CREATE.
	first, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.SyncPlatformRoles(repo,
		withTest(mkTest("sync-a", "Sync A"), mkTest("sync-b", "Sync B"))), operations.SyncPlatformRolesCommand{}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(n+2), first.Created)
	assert.Equal(t, uint32(0), first.Updated)
	assert.Equal(t, uint32(0), first.Removed)
	assert.Equal(t, uint32(n+2), first.Total)

	a, err := repo.FindByName(ctx, "roleplat:sync-a")
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, role.SourceCode, a.Source, "platform-synced rows are CODE-sourced")
	assert.ElementsMatch(t, []string{"roleplat:thing:read:*"}, a.Permissions)

	// 2. sync-b gains a live assignment; sync-a drifts in the catalogue.
	//    Re-sync without sync-b: existing catalogue roles are re-upserted
	//    (updated = n+1), the drifted display name is restored from the
	//    catalogue, and the stale-but-assigned sync-b is SKIPPED (warn,
	//    not an error) — removed stays 0.
	seedPrincipalWithRole(t, "prn_roleplatsync1", "roleplat-sync@example.com", "roleplat:sync-b")

	second, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.SyncPlatformRoles(repo,
		withTest(mkTest("sync-a", "Sync A v2"))), operations.SyncPlatformRolesCommand{}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(0), second.Created)
	assert.Equal(t, uint32(n+1), second.Updated, "every existing catalogue role is re-upserted")
	assert.Equal(t, uint32(0), second.Removed, "assigned stale CODE role is skipped, not deleted")

	a, err = repo.FindByName(ctx, "roleplat:sync-a")
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, "Sync A v2", a.DisplayName, "drifted rows are updated from the catalogue")

	b, err := repo.FindByName(ctx, "roleplat:sync-b")
	require.NoError(t, err)
	assert.NotNil(t, b, "stale CODE role with live assignments must survive the sweep")

	// 3. Drop the assignment: the stale row is now sweepable.
	_, err = testpg.Pool(t).Exec(ctx,
		`DELETE FROM iam_principal_roles WHERE principal_id = $1`, "prn_roleplatsync1")
	require.NoError(t, err)

	third, err := usecaseop.Run(testpg.AnchorCtx(), uow, operations.SyncPlatformRoles(repo,
		withTest(mkTest("sync-a", "Sync A v2"))), operations.SyncPlatformRolesCommand{}, ec)
	require.NoError(t, err)
	assert.Equal(t, uint32(1), third.Removed, "unassigned stale CODE role is swept")

	b, err = repo.FindByName(ctx, "roleplat:sync-b")
	require.NoError(t, err)
	assert.Nil(t, b)

	// Safety pin: the real platform catalogue is intact throughout.
	admin, err := repo.FindByName(ctx, "platform:admin")
	require.NoError(t, err)
	require.NotNil(t, admin, "production catalogue rows must never be swept")
	assert.Equal(t, role.SourceCode, admin.Source)
}
