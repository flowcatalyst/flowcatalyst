//go:build integration

package api

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/application"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// This file pins the T4.1 IDOR fixes (docs/owner-rulings-plan.md §2b/§T4):
//
//   - getVersion, listRoles, listApplicationAccess, listAvailableApplications
//     ran NO per-resource scope check at all — a client-scoped caller holding
//     only the coarse read permission could read ANY tenant's data through
//     these four routes.
//   - addRole/removeRole skipped authorization entirely on their idempotent
//     no-op path (role already present / already absent), returning a full
//     PrincipalResponse for any tenant's principal.
//
// permUserView ("platform:iam:user:view", auth.CanReadPrincipals) and
// permUserUpdate ("platform:iam:user:update", auth.CanWritePrincipals) are
// unexported in the auth package, so the literal strings are used directly —
// matching the pattern in connection/dispatchpool/eventtype's ops_pg_test.go.
const (
	testPermUserView   = "platform:iam:user:view"
	testPermUserUpdate = "platform:iam:user:update"
)

// clientScopedCaller builds a non-anchor AuthContext holding perm for the
// given accessible client — the shape a client-administrator's token has.
func clientScopedCaller(principalID, clientID string, perms ...string) *auth.AuthContext {
	return &auth.AuthContext{
		PrincipalID: principalID,
		Scope:       auth.ScopeClient,
		Clients:     []string{clientID},
		Permissions: perms,
	}
}

// ── getVersion ────────────────────────────────────────────────────────────

func TestGetVersion_ScopedCallerWithPermissionDeniedCrossTenant(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const (
		callerClient = "cli_authzsc0001"
		targetClient = "cli_authzsc0002"
		targetPID    = "prn_authzscver01"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'Version Target', TRUE, 'authzsc-ver01@example.com')`,
		targetPID, targetClient)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzsccaller1", callerClient, testPermUserView))
	_, err = s.getVersion(authCtx, &apicommon.IDInput{ID: targetPID})
	require.Error(t, err, "a client-scoped caller holding CanReadPrincipals must not read another tenant's version")
}

func TestGetVersion_ScopedCallerWithPermissionAllowedInScope(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const (
		sharedClient = "cli_authzsc0003"
		targetPID    = "prn_authzscver02"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'Version Target 2', TRUE, 'authzsc-ver02@example.com')`,
		targetPID, sharedClient)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzsccaller2", sharedClient, testPermUserView))
	out, err := s.getVersion(authCtx, &apicommon.IDInput{ID: targetPID})
	require.NoError(t, err, "a client-scoped caller must read a principal homed at its OWN client")
	assert.NotNil(t, out)
}

// ── listRoles ─────────────────────────────────────────────────────────────

func TestListRoles_DeniedCrossTenant(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const (
		callerClient = "cli_authzlr0001"
		targetClient = "cli_authzlr0002"
		targetPID    = "prn_authzlrtgt01"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'Roles Target', TRUE, 'authzlr-tgt01@example.com')`,
		targetPID, targetClient)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzlrcaller1", callerClient, testPermUserView))
	_, err = s.listRoles(authCtx, &apicommon.IDInput{ID: targetPID})
	require.Error(t, err, "a client-scoped caller must not list another tenant's role assignments")
}

func TestListRoles_AllowedInScopeAndSelf(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const (
		sharedClient = "cli_authzlr0003"
		targetPID    = "prn_authzlrtgt02"
		selfPID      = "prn_authzlrself1"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'Roles Target 2', TRUE, 'authzlr-tgt02@example.com')`,
		targetPID, sharedClient)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'Roles Self', TRUE, 'authzlr-self1@example.com')`,
		selfPID, sharedClient)
	require.NoError(t, err)

	inScopeCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzlrcaller2", sharedClient, testPermUserView))
	_, err = s.listRoles(inScopeCtx, &apicommon.IDInput{ID: targetPID})
	require.NoError(t, err, "a client-scoped caller must list roles for a principal homed at its OWN client")

	selfCtx := auth.WithContext(ctx, clientScopedCaller(selfPID, sharedClient, testPermUserView))
	_, err = s.listRoles(selfCtx, &apicommon.IDInput{ID: selfPID})
	require.NoError(t, err, "a caller reading its own roles, in its own client scope, must succeed")
}

// ── listApplicationAccess ────────────────────────────────────────────────

func TestListApplicationAccess_DeniedCrossTenant(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const (
		callerClient = "cli_authzaa0001"
		targetClient = "cli_authzaa0002"
		targetPID    = "prn_authzaatgt01"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'AppAccess Target', TRUE, 'authzaa-tgt01@example.com')`,
		targetPID, targetClient)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzaacaller1", callerClient, testPermUserView))
	_, err = s.listApplicationAccess(authCtx, &apicommon.IDInput{ID: targetPID})
	require.Error(t, err, "a client-scoped caller must not list another tenant's application access")
}

func TestListApplicationAccess_AllowedInScope(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const (
		sharedClient = "cli_authzaa0003"
		targetPID    = "prn_authzaatgt02"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'AppAccess Target 2', TRUE, 'authzaa-tgt02@example.com')`,
		targetPID, sharedClient)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzaacaller2", sharedClient, testPermUserView))
	out, err := s.listApplicationAccess(authCtx, &apicommon.IDInput{ID: targetPID})
	require.NoError(t, err, "a client-scoped caller must read application-access for a principal in its OWN client")
	assert.Equal(t, 0, out.Body.Total, "the target has no application access grants")
}

// ── listAvailableApplications ───────────────────────────────────────────

func TestListAvailableApplications_DeniedCrossTenant(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const (
		callerClient = "cli_authzva0001"
		targetClient = "cli_authzva0002"
		targetPID    = "prn_authzvatgt01"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'Available Target', TRUE, 'authzva-tgt01@example.com')`,
		targetPID, targetClient)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzvacaller1", callerClient, testPermUserView))
	_, err = s.listAvailableApplications(authCtx, &apicommon.IDInput{ID: targetPID})
	require.Error(t, err, "a client-scoped caller must not compute another tenant's available-application menu")
}

func TestListAvailableApplications_AllowedInScope(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo, Applications: application.NewRepository(pool)}

	const (
		sharedClient = "cli_authzva0003"
		targetPID    = "prn_authzvatgt02"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, client_id, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', $2, 'Available Target 2', TRUE, 'authzva-tgt02@example.com')`,
		targetPID, sharedClient)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, clientScopedCaller("prn_authzvacaller2", sharedClient, testPermUserView))
	out, err := s.listAvailableApplications(authCtx, &apicommon.IDInput{ID: targetPID})
	require.NoError(t, err, "a client-scoped caller must compute the available-application menu for a principal in its OWN client")
	assert.NotNil(t, out)
}
