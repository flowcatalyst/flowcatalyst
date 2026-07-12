//go:build integration

package operations_test

import (
	"context"
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	roleops "github.com/flowcatalyst/flowcatalyst-go/internal/platform/role/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecasepgx"
)

// devRoleOnce/devRoleName ensure the shared "platform:developer" role
// catalog row is created exactly once, even though several tests in this
// file run t.Parallel() and all need the SAME role name
// (operations.developerRoleName is a fixed constant checked by literal
// string match, unlike the uniquely-per-test role names other tests in this
// package use — creating it twice concurrently would race on iam_roles'
// unique name index).
var (
	devRoleOnce sync.Once
	devRoleName string
)

func ensureDeveloperRole(t *testing.T, uow *usecasepgx.UnitOfWork) string {
	t.Helper()
	devRoleOnce.Do(func() {
		ev, err := usecaseop.Run(testpg.AnchorCtx(), uow,
			roleops.CreateRole(role.NewRepository(testpg.Pool(t))),
			roleops.CreateCommand{ApplicationCode: "platform", RoleName: "developer", DisplayName: "platform developer"},
			testpg.TestEC())
		require.NoError(t, err)
		devRoleName = ev.Name
	})
	require.NotEmpty(t, devRoleName, "ensureDeveloperRole must run before any test that depends on it")
	return devRoleName
}

// requireAppKey skips the test when FLOWCATALYST_APP_KEY isn't configured —
// SetDeveloperCredential fails closed (matches OAuth client secret minting)
// without an encryption service, so these tests need a real key.
func requireAppKey(t *testing.T) {
	t.Helper()
	if os.Getenv("FLOWCATALYST_APP_KEY") == "" {
		t.Skip("FLOWCATALYST_APP_KEY not set; developer-credential encryption tests need it")
	}
}

func TestSetDeveloperCredential_HappyPath(t *testing.T) {
	requireAppKey(t)
	t.Parallel()
	ctx := context.Background()
	repo := principal.NewRepository(testpg.Pool(t))
	roles := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	devRole := ensureDeveloperRole(t, uow)
	seeded := mustCreateUser(t, repo, uow, "prn-devcred-happy@example.com", "ANCHOR", nil)
	_, err := runAuthorized(uow, operations.AssignRoles(repo, roles),
		operations.AssignRolesCommand{UserID: seeded.UserID, Roles: []string{devRole}})
	require.NoError(t, err)

	ev, err := runAuthorized(uow, operations.SetDeveloperCredential(repo),
		operations.SetDeveloperCredentialCommand{PrincipalID: seeded.UserID})
	require.NoError(t, err)
	assert.Equal(t, seeded.UserID, ev.UserID)

	got, err := repo.FindByID(ctx, seeded.UserID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.HasDevClientSecret(), "secret ref must be persisted")
	require.NotNil(t, got.UserIdentity)
	require.NotNil(t, got.UserIdentity.DevClientSecretUpdatedAt)

	plaintext, ok := operations.PopDevClientSecret(seeded.UserID)
	require.True(t, ok, "plaintext must be stashed exactly once after a successful set")
	assert.NotEmpty(t, plaintext)
	_, ok = operations.PopDevClientSecret(seeded.UserID)
	assert.False(t, ok, "stash is one-shot — a second pop must find nothing")

	// Rotating replaces the ref (and stashes a new plaintext).
	firstRef := got.UserIdentity.DevClientSecretRef
	_, err = runAuthorized(uow, operations.SetDeveloperCredential(repo),
		operations.SetDeveloperCredentialCommand{PrincipalID: seeded.UserID})
	require.NoError(t, err)
	rotated, err := repo.FindByID(ctx, seeded.UserID)
	require.NoError(t, err)
	assert.NotEqual(t, *firstRef, *rotated.UserIdentity.DevClientSecretRef, "rotate must generate a fresh ref")
	_, ok = operations.PopDevClientSecret(seeded.UserID)
	assert.True(t, ok)
}

func TestSetDeveloperCredential_RequiresDeveloperRole(t *testing.T) {
	requireAppKey(t)
	t.Parallel()
	repo := principal.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	seeded := mustCreateUser(t, repo, uow, "prn-devcred-norole@example.com", "ANCHOR", nil)

	_, err := runAuthorized(uow, operations.SetDeveloperCredential(repo),
		operations.SetDeveloperCredentialCommand{PrincipalID: seeded.UserID})
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "NOT_A_DEVELOPER")
}

func TestSetDeveloperCredential_RevokedRoleBlocksFutureSets(t *testing.T) {
	requireAppKey(t)
	t.Parallel()
	repo := principal.NewRepository(testpg.Pool(t))
	roles := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	devRole := ensureDeveloperRole(t, uow)
	seeded := mustCreateUser(t, repo, uow, "prn-devcred-revoked@example.com", "ANCHOR", nil)
	_, err := runAuthorized(uow, operations.AssignRoles(repo, roles),
		operations.AssignRolesCommand{UserID: seeded.UserID, Roles: []string{devRole}})
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.SetDeveloperCredential(repo),
		operations.SetDeveloperCredentialCommand{PrincipalID: seeded.UserID})
	require.NoError(t, err)
	operations.PopDevClientSecret(seeded.UserID) // drain the stash

	// Revoking the role (full role-set replace with the role omitted) must
	// block any FUTURE SetDeveloperCredential call — the live re-check.
	_, err = runAuthorized(uow, operations.AssignRoles(repo, roles),
		operations.AssignRolesCommand{UserID: seeded.UserID, Roles: []string{}})
	require.NoError(t, err)

	_, err = runAuthorized(uow, operations.SetDeveloperCredential(repo),
		operations.SetDeveloperCredentialCommand{PrincipalID: seeded.UserID})
	testpg.RequireUsecaseError(t, err, usecase.KindBusinessRule, "NOT_A_DEVELOPER")
}

func TestSetDeveloperCredential_Errors(t *testing.T) {
	requireAppKey(t)
	t.Parallel()
	repo := principal.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	svc := seedServicePrincipal(t, "sa_devcrederr01", "prn-devcred-svc")

	cases := []struct {
		name string
		cmd  operations.SetDeveloperCredentialCommand
		kind usecase.Kind
		code string
	}{
		{"missing principal id", operations.SetDeveloperCredentialCommand{}, usecase.KindValidation, "PRINCIPAL_ID_REQUIRED"},
		{"unknown principal", operations.SetDeveloperCredentialCommand{PrincipalID: "prn_doesnotexist1"}, usecase.KindNotFound, "User_NOT_FOUND"},
		{"service principal", operations.SetDeveloperCredentialCommand{PrincipalID: svc.ID}, usecase.KindBusinessRule, "NOT_A_USER"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := runAuthorized(uow, operations.SetDeveloperCredential(repo), tc.cmd)
			testpg.RequireUsecaseError(t, err, tc.kind, tc.code)
		})
	}
}

func TestRevokeDeveloperCredential_HappyPath(t *testing.T) {
	requireAppKey(t)
	t.Parallel()
	ctx := context.Background()
	repo := principal.NewRepository(testpg.Pool(t))
	roles := role.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	devRole := ensureDeveloperRole(t, uow)
	seeded := mustCreateUser(t, repo, uow, "prn-devcred-revoke@example.com", "ANCHOR", nil)
	_, err := runAuthorized(uow, operations.AssignRoles(repo, roles),
		operations.AssignRolesCommand{UserID: seeded.UserID, Roles: []string{devRole}})
	require.NoError(t, err)
	_, err = runAuthorized(uow, operations.SetDeveloperCredential(repo),
		operations.SetDeveloperCredentialCommand{PrincipalID: seeded.UserID})
	require.NoError(t, err)
	operations.PopDevClientSecret(seeded.UserID)

	ev, err := runAuthorized(uow, operations.RevokeDeveloperCredential(repo),
		operations.RevokeDeveloperCredentialCommand{PrincipalID: seeded.UserID})
	require.NoError(t, err)
	assert.Equal(t, seeded.UserID, ev.UserID)

	got, err := repo.FindByID(ctx, seeded.UserID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.HasDevClientSecret(), "revoke must clear the secret ref")

	// The role itself is untouched — revoke only clears the secret.
	hasRole := false
	for _, ra := range got.Roles {
		if ra.Role == devRole {
			hasRole = true
		}
	}
	assert.True(t, hasRole, "revoking the credential must not touch role assignment")
}
