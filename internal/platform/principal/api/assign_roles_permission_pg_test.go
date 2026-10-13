//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/ids"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// Owner ruling 2026-10-11: user:assign-roles, not user:update, governs a
// user's roles. These pin it end to end on a real database: each permission
// alone, against each role route, the profile edit and the bulk import.

const (
	permUserUpdateOnly = "platform:iam:user:update"
	permUserAssignOnly = "platform:iam:user:assign-roles"
	permUserCreateOnly = "platform:iam:user:create"
)

func mustRole(t *testing.T, uow *usecasepgx.UnitOfWork, appCode, roleName string) string {
	t.Helper()
	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(role.NewRepository(testpg.Pool(t))),
		roleops.CreateCommand{ApplicationCode: appCode, RoleName: roleName, DisplayName: appCode + " " + roleName},
		testpg.TestEC())
	require.NoError(t, err)
	return ev.Name
}

func roleNamesOf(t *testing.T, repo *principal.Repository, id string) []string {
	t.Helper()
	p, err := repo.FindByID(context.Background(), ids.PrincipalID(id))
	require.NoError(t, err)
	require.NotNil(t, p)
	return roleNamesFrom(p.Roles)
}

func requirePermissionRequired(t *testing.T, err error, label string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%s: want a refusal", label)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
	assert.Equal(t, "PERMISSION_REQUIRED", ue.Code, label)
}

// user:update alone no longer changes roles: replace, add and remove are
// refused and the roles stay as they were.
func TestRoleChanges_RefusedWithUpdateOnly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, Roles: role.NewRepository(pool), UoW: uow}

	kept := mustRole(t, uow, "asgnperm1", "kept")
	other := mustRole(t, uow, "asgnperm1", "other")
	const target = "prn_asgnperm_u01"
	mustSeedPrincipalWithRole(t, ctx, repo, target, "cli_asgnperm_u01", kept)

	caller := anchorHolding("USER", permUserUpdateOnly)
	_, err := s.assignRoles(caller, &assignRolesInput{ID: target, Body: AssignPrincipalRolesRequest{Roles: []string{other}}})
	requirePermissionRequired(t, err, "replace roles")
	_, err = s.addRole(caller, &addRoleInput{ID: target, Body: AddRoleRequest{Role: other}})
	requirePermissionRequired(t, err, "add role")
	_, err = s.removeRole(caller, &removeRoleInput{ID: target, Role: kept})
	requirePermissionRequired(t, err, "remove role")

	assert.Equal(t, []string{kept}, roleNamesOf(t, repo, target), "no role change went through")
}

// user:assign-roles alone changes roles (replace, add, remove) but edits no
// profile.
func TestRoleChanges_AllowedWithAssignRolesOnly(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, Roles: role.NewRepository(pool), UoW: uow}

	first := mustRole(t, uow, "asgnperm2", "first")
	second := mustRole(t, uow, "asgnperm2", "second")
	const target = "prn_asgnperm_a01"
	mustSeedPrincipalWithRole(t, ctx, repo, target, "cli_asgnperm_a01", "")

	caller := anchorHolding("USER", permUserAssignOnly)
	_, err := s.assignRoles(caller, &assignRolesInput{ID: target, Body: AssignPrincipalRolesRequest{Roles: []string{first}}})
	require.NoError(t, err, "replace roles")
	assert.Equal(t, []string{first}, roleNamesOf(t, repo, target))

	_, err = s.addRole(caller, &addRoleInput{ID: target, Body: AddRoleRequest{Role: second}})
	require.NoError(t, err, "add role")
	assert.ElementsMatch(t, []string{first, second}, roleNamesOf(t, repo, target))

	_, err = s.removeRole(caller, &removeRoleInput{ID: target, Role: first})
	require.NoError(t, err, "remove role")
	assert.Equal(t, []string{second}, roleNamesOf(t, repo, target))

	name := "Renamed"
	_, err = s.update(caller, &updateInput{ID: target, Body: UpdatePrincipalRequest{Name: &name}})
	requirePermissionRequired(t, err, "profile edit")
	p, err := repo.FindByID(ctx, target)
	require.NoError(t, err)
	assert.NotEqual(t, name, p.Name, "the profile is unchanged")
}

// The bulk import creates users with user:create; a row's roles need
// user:assign-roles as well. Without it the user is created and the row says
// its roles were not applied; a row without roles needs nothing more.
func TestBulkImport_RowRolesNeedAssignRoles(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, Roles: role.NewRepository(pool), UoW: uow, InviteEmailer: noopInviteEmailer{}}
	imported := mustRole(t, uow, "asgnperm3", "imported")
	const client = "cli_asgnperm_b01"

	byEmail := func(out *apicommon.Out[BulkImportResponse]) map[string]BulkImportResult {
		m := map[string]BulkImportResult{}
		for _, r := range out.Body.Results {
			m[r.Email] = r
		}
		return m
	}

	createOnly := anchorHolding("USER", permUserCreateOnly)
	out, err := s.bulkImport(createOnly, &apicommon.In[BulkImportRequest]{Body: BulkImportRequest{
		ClientID: client,
		Users: []BulkImportUser{
			{Name: "With Roles", Email: "with-roles@asgnperm.test", Roles: []string{imported}},
			{Name: "No Roles", Email: "no-roles@asgnperm.test"},
		},
	}})
	require.NoError(t, err)
	rows := byEmail(out)
	assert.Equal(t, "created", rows["with-roles@asgnperm.test"].Status)
	assert.Contains(t, rows["with-roles@asgnperm.test"].Message, "roles not applied")
	assert.Equal(t, "created", rows["no-roles@asgnperm.test"].Status)
	assert.Empty(t, rows["no-roles@asgnperm.test"].Message)
	withRoles, err := repo.FindByEmail(ctx, "with-roles@asgnperm.test")
	require.NoError(t, err)
	require.NotNil(t, withRoles)
	assert.Empty(t, roleNamesFrom(withRoles.Roles), "the roles were not assigned")

	both := anchorHolding("USER", permUserCreateOnly, permUserAssignOnly)
	out, err = s.bulkImport(both, &apicommon.In[BulkImportRequest]{Body: BulkImportRequest{
		ClientID: client,
		Users:    []BulkImportUser{{Name: "Assigned", Email: "assigned@asgnperm.test", Roles: []string{imported}}},
	}})
	require.NoError(t, err)
	rows = byEmail(out)
	assert.Equal(t, "created", rows["assigned@asgnperm.test"].Status)
	assert.Empty(t, rows["assigned@asgnperm.test"].Message)
	assigned, err := repo.FindByEmail(ctx, "assigned@asgnperm.test")
	require.NoError(t, err)
	require.NotNil(t, assigned)
	assert.Equal(t, []string{imported}, roleNamesFrom(assigned.Roles))
}
