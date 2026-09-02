//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// This file pins the addRole/removeRole idempotent-path authorization hole
// (docs/owner-rulings-plan.md §2b): both handlers only reached AssignRoles'
// post-load authorization check when the requested role actually changed
// membership. When the role was already present (addRole) or already absent
// (removeRole), the handler skipped straight to `return fromEntity(p)` with
// NO authorization at all beyond the coarse permission — handing back a full
// PrincipalResponse for any tenant's principal.

// mustSeedPrincipalWithRole inserts a minimal USER principal at clientID,
// with roleName already assigned via a direct row in the junction table (no
// iam_roles FK exists, so this doesn't need a real role definition).
func mustSeedPrincipalWithRole(t *testing.T, ctx context.Context, repo *principal.Repository, id, clientID, roleName string) {
	t.Helper()
	pool := testpg.Pool(t)
	// email is computed in Go and passed as its OWN parameter ($3) rather than
	// reused inline as `$1 || '@example.com'`: reusing $1 in two different
	// type contexts (a VARCHAR column value, then concatenated with a text
	// literal) makes Postgres's extended-query-protocol type inference for
	// that parameter ambiguous — "inconsistent types deduced for parameter
	// $1... text versus character varying" — which fails every call site
	// (root cause of the TestAddRole_*/TestRemoveRole_* failures reported
	// against this helper; not an authorization regression).
	email := id + "@example.com"
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'Role Idem Target', TRUE, $3)`,
		id, clientID, email)
	require.NoError(t, err)
	if roleName != "" {
		_, err = pool.Exec(ctx,
			`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source)
			 VALUES ($1, $2, 'ADMIN_ASSIGNED')`, id, roleName)
		require.NoError(t, err)
	}
}

// ── addRole ───────────────────────────────────────────────────────────────

// TestAddRole_AlreadyPresentRefusedCrossTenant is the key regression: a
// client-scoped caller holding the coarse write permission must NOT be able
// to read back another tenant's PrincipalResponse by "adding" a role the
// target already holds.
func TestAddRole_AlreadyPresentRefusedCrossTenant(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, UoW: uow}

	const (
		callerClient = "cli_authzar0001"
		targetClient = "cli_authzar0002"
		targetPID    = "prn_authzartgt01"
		roleName     = "someapp:already-had"
	)
	mustSeedPrincipalWithRole(t, ctx, repo, targetPID, targetClient, roleName)

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzarcaller1", callerClient, testPermUserUpdate))
	_, err := s.addRole(authCtx, &addRoleInput{ID: targetPID, Body: AddRoleRequest{Role: roleName}})
	require.Error(t, err, "a cross-tenant caller must be refused even when the role is already present")
}

// TestAddRole_NoPermissionDeniedEvenForOwnClient pins the coarse gate: a
// caller with NO write permission at all must be refused before any load —
// addRole previously had no coarse check whatsoever.
func TestAddRole_NoPermissionDeniedEvenForOwnClient(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, UoW: uow}

	const (
		sharedClient = "cli_authzar0003"
		targetPID    = "prn_authzartgt02"
		roleName     = "someapp:already-had"
	)
	mustSeedPrincipalWithRole(t, ctx, repo, targetPID, sharedClient, roleName)

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzarcaller2", sharedClient /* no perms */))
	_, err := s.addRole(authCtx, &addRoleInput{ID: targetPID, Body: AddRoleRequest{Role: roleName}})
	require.Error(t, err, "a caller with no user-write permission must be refused, even in its own client")
}

// TestAddRole_AlreadyPresentAllowedInScope pins that the fix doesn't break
// the legitimate idempotent case: an anchor re-adding an already-present role
// still succeeds and returns the principal unchanged.
func TestAddRole_AlreadyPresentAllowedInScope(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, UoW: uow}

	const (
		targetClient = "cli_authzar0004"
		targetPID    = "prn_authzartgt03"
		roleName     = "someapp:already-had"
	)
	mustSeedPrincipalWithRole(t, ctx, repo, targetPID, targetClient, roleName)

	out, err := s.addRole(testpg.AnchorCtx(), &addRoleInput{ID: targetPID, Body: AddRoleRequest{Role: roleName}})
	require.NoError(t, err, "an anchor re-adding an already-present role must still succeed idempotently")
	assert.Contains(t, out.Body.Roles, roleName)
}

// ── removeRole ────────────────────────────────────────────────────────────

// TestRemoveRole_NotPresentRefusedCrossTenant is the mirror regression: a
// client-scoped caller must NOT be able to read back another tenant's
// PrincipalResponse by "removing" a role the target never had.
func TestRemoveRole_NotPresentRefusedCrossTenant(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, UoW: uow}

	const (
		callerClient = "cli_authzrr0001"
		targetClient = "cli_authzrr0002"
		targetPID    = "prn_authzrrtgt01"
	)
	mustSeedPrincipalWithRole(t, ctx, repo, targetPID, targetClient, "")

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzrrcaller1", callerClient, testPermUserUpdate))
	_, err := s.removeRole(authCtx, &removeRoleInput{ID: targetPID, Role: "someapp:never-had"})
	require.Error(t, err, "a cross-tenant caller must be refused even when the role is already absent")
}

// TestRemoveRole_NoPermissionDeniedEvenForOwnClient mirrors the addRole
// coarse-gate pin.
func TestRemoveRole_NoPermissionDeniedEvenForOwnClient(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, UoW: uow}

	const (
		sharedClient = "cli_authzrr0003"
		targetPID    = "prn_authzrrtgt02"
	)
	mustSeedPrincipalWithRole(t, ctx, repo, targetPID, sharedClient, "")

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzrrcaller2", sharedClient /* no perms */))
	_, err := s.removeRole(authCtx, &removeRoleInput{ID: targetPID, Role: "someapp:never-had"})
	require.Error(t, err, "a caller with no user-write permission must be refused, even in its own client")
}

// TestRemoveRole_NotPresentAllowedInScope pins that the fix doesn't break the
// legitimate idempotent case: an anchor removing an absent role still
// succeeds.
func TestRemoveRole_NotPresentAllowedInScope(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	uow := testpg.NewUoW(t)
	s := &State{Repo: repo, UoW: uow}

	const (
		targetClient = "cli_authzrr0004"
		targetPID    = "prn_authzrrtgt03"
	)
	mustSeedPrincipalWithRole(t, ctx, repo, targetPID, targetClient, "")

	out, err := s.removeRole(testpg.AnchorCtx(), &removeRoleInput{ID: targetPID, Role: "someapp:never-had"})
	require.NoError(t, err, "an anchor removing an absent role must still succeed idempotently")
	assert.NotNil(t, out)
}
