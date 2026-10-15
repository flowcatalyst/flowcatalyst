//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

func mustRoleWith(t *testing.T, uow *usecasepgx.UnitOfWork, appCode, roleName string, perms ...string) string {
	t.Helper()
	testpg.EnsureApplication(t, appCode)
	ev, err := usecaseop.Run(testpg.AnchorCtx(), uow, roleops.CreateRole(role.NewRepository(testpg.Pool(t))),
		roleops.CreateCommand{ApplicationCode: appCode, RoleName: roleName, DisplayName: roleName, Permissions: perms},
		testpg.TestEC())
	require.NoError(t, err)
	return ev.Name
}

func requireRoleAboveCaller(t *testing.T, err error, label, named string) {
	t.Helper()
	ue := usecase.AsError(err)
	require.NotNil(t, ue, "%s: want a refusal, got %v", label, err)
	assert.Equal(t, usecase.KindAuthorization, ue.Kind, label)
	assert.Equal(t, "ROLE_ABOVE_CALLER", ue.Code, label)
	assert.Contains(t, ue.Message, named, label)
}

// The role ceiling (F8) on a user's roles: a holder of user:assign-roles adds
// or removes only roles whose platform permissions it holds. Before the fix it
// could give anyone, itself included, super-admin.
func TestUserRoles_BoundedByCallersPermissions(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, Roles: role.NewRepository(pool), UoW: uow, InviteEmailer: noopInviteEmailer{}}

	power := mustRoleWith(t, uow, "ceilusr", "power", "platform:iam:user:create", "platform:iam:user:view")
	viewer := mustRoleWith(t, uow, "ceilusr", "viewer", "platform:iam:user:view")
	appOnly := mustRoleWith(t, uow, "ceilusr", "app-only", "ceilusr:thing:view")
	const target = "prn_ceilusr_t001"
	mustSeedPrincipalWithRole(t, ctx, repo, target, "cli_ceilusr_0001", "")

	assigner := anchorHolding("USER", permUserAssignOnly, "platform:iam:user:view")
	subset := anchorHolding("USER", permUserAssignOnly, "platform:iam:user:view", "platform:iam:user:create")
	superAdmin := anchorHolding("USER", "platform:*:*:*")

	// Lacking a permission: replace, add are refused and nothing changes.
	_, err := s.assignRoles(assigner, &assignRolesInput{ID: target, Body: AssignPrincipalRolesRequest{Roles: []string{power}}})
	requireRoleAboveCaller(t, err, "replace roles", power)
	_, err = s.addRole(assigner, &addRoleInput{ID: target, Body: AddRoleRequest{Role: power}})
	requireRoleAboveCaller(t, err, "add role", power)
	assert.Empty(t, roleNamesOf(t, repo, target))

	// A subset caller can, application permissions aside.
	_, err = s.assignRoles(assigner, &assignRolesInput{ID: target, Body: AssignPrincipalRolesRequest{Roles: []string{viewer, appOnly}}})
	require.NoError(t, err)
	_, err = s.addRole(subset, &addRoleInput{ID: target, Body: AddRoleRequest{Role: power}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{viewer, appOnly, power}, roleNamesOf(t, repo, target))

	// Removal counts; kept roles are not checked.
	_, err = s.removeRole(assigner, &removeRoleInput{ID: target, Role: power})
	requireRoleAboveCaller(t, err, "remove role", power)
	_, err = s.removeRole(assigner, &removeRoleInput{ID: target, Role: appOnly})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{viewer, power}, roleNamesOf(t, repo, target))

	// The wildcard holder may assign anything.
	_, err = s.assignRoles(superAdmin, &assignRolesInput{ID: target, Body: AssignPrincipalRolesRequest{Roles: []string{appOnly}}})
	require.NoError(t, err)
	assert.Equal(t, []string{appOnly}, roleNamesOf(t, repo, target))

	// A bulk-import row's roles go through the same rule.
	out, err := s.bulkImport(anchorHolding("USER", permUserCreateOnly, permUserAssignOnly),
		&apicommon.In[BulkImportRequest]{Body: BulkImportRequest{
			ClientID: "cli_ceilusr_0001",
			Users:    []BulkImportUser{{Name: "Escalated", Email: "escalated@ceilusr.test", Roles: []string{power}}},
		}})
	require.NoError(t, err)
	require.Len(t, out.Body.Results, 1)
	assert.Contains(t, out.Body.Results[0].Message, "roles not applied")
	imported, err := repo.FindByEmail(ctx, "escalated@ceilusr.test")
	require.NoError(t, err)
	require.NotNil(t, imported)
	assert.Empty(t, roleNamesFrom(imported.Roles))
}
