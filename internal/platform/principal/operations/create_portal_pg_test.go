//go:build integration

package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
)

// TestCreatePortalUser_InertShape pins the security contract of a portal
// identity (docs/portal-identity-plan.md): the persisted principal must be
// inert on every authority axis — CLIENT scope with NO client_id (so
// CanAccessClient is always false), no roles, AllApplications=false, no app
// access, no password hash — while still being a real, active, authenticable
// USER.
func TestCreatePortalUser_InertShape(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := principal.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	ev, err := runAuthorized(uow, operations.CreatePortalUser(repo), operations.CreatePortalUserCommand{
		Email:    "  Portal-Inert@Tigerbrands.COM  ",
		Name:     ptr("  Sipho Dlamini  "),
		Provider: ptr("OIDC"),
	})
	require.NoError(t, err)
	assert.Equal(t, "portal-inert@tigerbrands.com", ev.Email, "email is lower-cased + trimmed")

	got, err := repo.FindByID(ctx, ev.UserID)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, principal.TypeUser, got.Type)
	assert.True(t, got.Active)
	assert.Equal(t, "Sipho Dlamini", got.Name)

	// The inert-authority contract.
	assert.Equal(t, principal.ScopeClient, got.Scope)
	assert.Nil(t, got.ClientID, "portal identities must have NO home client")
	assert.Empty(t, got.Roles)
	assert.Empty(t, got.AssignedClients)
	assert.Empty(t, got.AccessibleApplicationIDs)
	assert.False(t, got.AllApplications, "portal identities must not pass application-axis checks")
	assert.False(t, got.Scope.CanAccessClient("clt_anything", got.ClientID, got.AssignedClients))

	require.NotNil(t, got.UserIdentity)
	assert.Nil(t, got.UserIdentity.PasswordHash, "SSO-provisioned portal identities carry no password")
	require.NotNil(t, got.UserIdentity.Provider)
	assert.Equal(t, "OIDC", *got.UserIdentity.Provider)
}

// TestCreatePortalUser_DuplicateEmail_Conflict: global email uniqueness makes
// "add membership, don't create" the caller's job — the op must surface the
// existing-principal case as a CONFLICT, exactly like CreateUser.
func TestCreatePortalUser_DuplicateEmail_Conflict(t *testing.T) {
	t.Parallel()
	repo := principal.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	const email = "prn-portal-dup@example.com"
	mustCreateUser(t, repo, uow, email, "ANCHOR", nil)

	_, err := runAuthorized(uow, operations.CreatePortalUser(repo),
		operations.CreatePortalUserCommand{Email: email})
	testpg.RequireUsecaseError(t, err, usecase.KindConflict, "EMAIL_EXISTS")
}

// TestCreatePortalUser_Validation: bad emails are rejected before Execute.
func TestCreatePortalUser_Validation(t *testing.T) {
	t.Parallel()
	repo := principal.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)

	for _, email := range []string{"", "   ", "not-an-email", "missing@tld"} {
		_, err := runAuthorized(uow, operations.CreatePortalUser(repo),
			operations.CreatePortalUserCommand{Email: email})
		assert.Error(t, err, "email %q must be rejected", email)
	}
}
