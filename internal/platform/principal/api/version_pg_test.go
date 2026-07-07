//go:build integration

package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/apicommon"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

// TestGetVersion_SelfAccessAllowedWithoutPermission pins that any
// authenticated principal can check its own version — no CanReadPrincipals
// permission required — since this is what an SDK's per-request revocation
// check calls using the end user's own token.
func TestGetVersion_SelfAccessAllowedWithoutPermission(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const pid = "prn_versionep0001"
	updatedAt := time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email, updated_at)
		 VALUES ($1, 'USER', 'CLIENT', 'Version EP', TRUE, 'version-ep1@example.com', $2)`,
		pid, updatedAt)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, &auth.AuthContext{PrincipalID: pid, Scope: auth.ScopeClient})
	out, err := s.getVersion(authCtx, &apicommon.IDInput{ID: pid})
	require.NoError(t, err, "a principal must be able to check its own version with no special permission")
	assert.True(t, out.Body.UpdatedAt.Underlying().Equal(updatedAt))
}

// TestGetVersion_CrossPrincipalDeniedWithoutPermission pins the authz floor:
// a non-anchor, non-admin caller cannot check ANOTHER principal's version.
func TestGetVersion_CrossPrincipalDeniedWithoutPermission(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const (
		callerPID = "prn_versionep0002"
		targetPID = "prn_versionep0003"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', 'Version EP Target', TRUE, 'version-ep3@example.com')`, targetPID)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, &auth.AuthContext{PrincipalID: callerPID, Scope: auth.ScopeClient})
	_, err = s.getVersion(authCtx, &apicommon.IDInput{ID: targetPID})
	require.Error(t, err, "a non-admin must not read another principal's version")
}

// TestGetVersion_AnchorCanCheckAnyPrincipal pins the admin escape hatch: an
// anchor (platform-level) caller can check any principal's version.
func TestGetVersion_AnchorCanCheckAnyPrincipal(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const (
		callerPID = "prn_versionep0004"
		targetPID = "prn_versionep0005"
	)
	updatedAt := time.Date(2026, 3, 3, 0, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email, updated_at)
		 VALUES ($1, 'USER', 'CLIENT', 'Version EP Target 2', TRUE, 'version-ep5@example.com', $2)`,
		targetPID, updatedAt)
	require.NoError(t, err)

	authCtx := auth.WithContext(ctx, &auth.AuthContext{PrincipalID: callerPID, Scope: auth.ScopeAnchor})
	out, err := s.getVersion(authCtx, &apicommon.IDInput{ID: targetPID})
	require.NoError(t, err, "an anchor must be able to check any principal's version")
	assert.True(t, out.Body.UpdatedAt.Underlying().Equal(updatedAt))
}

// TestGetVersion_NoAuthContextDeniedNotPanic pins a real bug found via live
// verification: a request whose auth middleware left NO AuthContext in the
// request context (e.g. the caller's own principal was just deactivated)
// must be denied cleanly, not panic on a nil-pointer field access.
func TestGetVersion_NoAuthContextDeniedNotPanic(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	s := &State{Repo: repo}

	const pid = "prn_versionep0006"
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', 'Version EP No Auth', TRUE, 'version-ep6@example.com')`, pid)
	require.NoError(t, err)

	_, err = s.getVersion(ctx, &apicommon.IDInput{ID: pid})
	require.Error(t, err, "a request with no AuthContext must be denied, not panic")
}
