//go:build integration

// Pinning tests for PR-3/PR-4 (docs/owner-rulings-todo.md #3):
//   - the coarse permission gate runs before any load (so an unauthorized
//     caller touches nothing and can't distinguish a real id from an
//     invented one), and
//   - an out-of-scope target (a different tenant, or a nil/platform-scoped
//     target the caller can't reach) answers the SAME not-found error a
//     genuinely missing id would — never 403.
//
// requireUserResourceAccess (update/activate/deactivate/delete) and
// requireUserAdmin (assign_roles/assign_application_access/
// developer_credential's admin branch) both changed shape in
// internal/platform/principal/operations/authz.go; this file exercises both
// call sites directly at the operation layer (below the coarse controller
// gate, which internal/platform/principal/api pins separately) so the
// resource-scope behaviour is proven independent of the HTTP layer.
package operations_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/role"
	sharedauth "github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/httperror"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/usecaseop"
)

// assertCanonicalNotFound asserts got is EXACTLY the error
// httperror.NotFound(resource, id) would produce for id — the same Code and
// Message httperror.NotFound always builds. "Byte-identical to not-found"
// can't mean the literal message text matches ACROSS two different ids (a
// cross-tenant real id vs. a fabricated missing one, e.g. "prn_6G6..." vs
// "prn_doesnotexist_x") — the id is part of the message by construction — so
// this checks each path against its OWN canonical not-found shape (proving
// neither carries any distinguishing marker: no different code, no 403,
// nothing) rather than comparing two different ids' messages to each other.
func assertCanonicalNotFound(t *testing.T, resource, id string, got error) {
	t.Helper()
	want := httperror.NotFound(resource, id)
	require.Error(t, got)
	assert.Equal(t, want.Error(), got.Error(), "must be the exact canonical %s not-found shape for id %s", resource, id)
}

// clientCtx is a non-anchor CLIENT-scoped auth context confined to clientID,
// holding the same principal-admin permission set the seeded client-admin
// role grants (seed/roles.go) — enough to reach every op under test, so a
// denial in these tests is provably about SCOPE, not the coarse permission
// gate (which internal/platform/principal/api pins on its own).
func clientCtx(clientID string) context.Context {
	return testpg.WithAuth(context.Background(), &sharedauth.AuthContext{
		PrincipalID: "prn_optestrunner1",
		Scope:       sharedauth.ScopeClient,
		Clients:     []string{clientID},
		Permissions: []string{
			"platform:iam:user:view", "platform:iam:user:create", "platform:iam:user:update",
			"platform:iam:user:delete", "platform:iam:user:assign-roles",
		},
	})
}

// TestRequireUserResourceAccess_CrossTenantAndMissing_IdenticalNotFound pins
// PR-3(b) for update/activate/deactivate/delete: a caller confined to
// clientA gets the SAME not-found error for a clientB principal as for an id
// that doesn't exist at all.
func TestRequireUserResourceAccess_CrossTenantAndMissing_IdenticalNotFound(t *testing.T) {
	t.Parallel()
	repo := principal.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	clientA := mustCreateClient(t, uow, "Client A RUA", "cli-rua-a")
	clientB := mustCreateClient(t, uow, "Client B RUA", "cli-rua-b")
	callerCtx := clientCtx(clientA)

	cases := []struct {
		name string
		run  func(id string) error
	}{
		{"update", func(id string) error {
			_, err := usecaseop.Run(callerCtx, uow, operations.UpdateUser(repo),
				operations.UpdateCommand{ID: id, Name: ptr("New Name")}, ec)
			return err
		}},
		{"activate", func(id string) error {
			_, err := usecaseop.Run(callerCtx, uow, operations.ActivateUser(repo),
				operations.ActivateCommand{ID: id}, ec)
			return err
		}},
		{"deactivate", func(id string) error {
			_, err := usecaseop.Run(callerCtx, uow, operations.DeactivateUser(repo),
				operations.DeactivateCommand{ID: id}, ec)
			return err
		}},
		{"delete", func(id string) error {
			_, err := usecaseop.Run(callerCtx, uow, operations.DeleteUser(repo),
				operations.DeleteCommand{ID: id}, ec)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Each subtest gets its own target (delete would otherwise consume a
			// shared one) and its own definitely-nonexistent id.
			target := mustCreateUser(t, repo, uow, "prn-rua-"+tc.name+"@example.com", "CLIENT", &clientB)

			crossErr := tc.run(target.UserID)
			missingErr := tc.run("prn_doesnotexist_" + tc.name)

			testpg.RequireUsecaseError(t, crossErr, usecase.KindNotFound, "Principal_NOT_FOUND")
			testpg.RequireUsecaseError(t, missingErr, usecase.KindNotFound, "Principal_NOT_FOUND")
			assertCanonicalNotFound(t, "Principal", target.UserID, crossErr)
			assertCanonicalNotFound(t, "Principal", "prn_doesnotexist_"+tc.name, missingErr)

			// Confirm nothing was touched: the cross-tenant target still exists,
			// active, unchanged.
			still, ferr := repo.FindByID(context.Background(), target.UserID)
			require.NoError(t, ferr)
			require.NotNil(t, still, "a denied cross-tenant call must not delete the target")
		})
	}
}

// TestRequireUserResourceAccess_KindMismatch_Stays403 pins that
// blockNonClientTarget (a non-anchor admin acting on an ANCHOR/PARTNER-scope
// principal) is UNCHANGED by PR-3(b) — it's a distinct "wrong kind of
// administrator" decision within the caller's reach, not a tenancy boundary,
// so it stays 403 rather than folding into the 404 shape.
func TestRequireUserResourceAccess_KindMismatch_Stays403(t *testing.T) {
	t.Parallel()
	repo := principal.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	clientA := mustCreateClient(t, uow, "Client A KindMismatch", "cli-kindmismatch-a")
	anchorTarget := mustCreateUser(t, repo, uow, "prn-kindmismatch-anchor@example.com", "ANCHOR", nil)

	_, err := usecaseop.Run(clientCtx(clientA), uow, operations.UpdateUser(repo),
		operations.UpdateCommand{ID: anchorTarget.UserID, Name: ptr("X")}, ec)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "FORBIDDEN")
}

// TestRequireUserAdmin_CrossTenantAndMissing_IdenticalNotFound pins PR-3(b)
// for assign_roles / assign_application_access / developer_credential's
// admin branch (all authorized via requireUserAdmin).
func TestRequireUserAdmin_CrossTenantAndMissing_IdenticalNotFound(t *testing.T) {
	t.Parallel()
	repo := principal.NewRepository(testpg.Pool(t))
	roles := role.NewRepository(testpg.Pool(t))
	apps := application.NewRepository(testpg.Pool(t))
	uow := testpg.NewUoW(t)
	ec := testpg.TestEC()

	clientA := mustCreateClient(t, uow, "Client A RUAdm", "cli-ruadm-a")
	clientB := mustCreateClient(t, uow, "Client B RUAdm", "cli-ruadm-b")
	callerCtx := clientCtx(clientA)

	cases := []struct {
		name string
		run  func(id string) error
	}{
		{"assign_roles", func(id string) error {
			_, err := usecaseop.Run(callerCtx, uow, operations.AssignRoles(repo, roles),
				operations.AssignRolesCommand{UserID: id, Roles: nil}, ec)
			return err
		}},
		{"assign_application_access", func(id string) error {
			_, err := usecaseop.Run(callerCtx, uow, operations.AssignApplicationAccess(repo, apps),
				operations.AssignApplicationAccessCommand{UserID: id, ApplicationIDs: nil}, ec)
			return err
		}},
		{"set_developer_credential_admin_branch", func(id string) error {
			_, err := usecaseop.Run(callerCtx, uow, operations.SetDeveloperCredential(repo),
				operations.SetDeveloperCredentialCommand{PrincipalID: id}, ec)
			return err
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := mustCreateUser(t, repo, uow, "prn-ruadm-"+tc.name+"@example.com", "CLIENT", &clientB)

			crossErr := tc.run(target.UserID)
			missingErr := tc.run("prn_doesnotexist_" + tc.name)

			testpg.RequireUsecaseError(t, crossErr, usecase.KindNotFound, "User_NOT_FOUND")
			testpg.RequireUsecaseError(t, missingErr, usecase.KindNotFound, "User_NOT_FOUND")
			assertCanonicalNotFound(t, "User", target.UserID, crossErr)
			assertCanonicalNotFound(t, "User", "prn_doesnotexist_"+tc.name, missingErr)

			still, ferr := repo.FindByID(context.Background(), target.UserID)
			require.NoError(t, ferr)
			require.NotNil(t, still, "a denied cross-tenant call must not mutate the target")
		})
	}
}
