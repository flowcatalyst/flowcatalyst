//go:build integration

package principal_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindAll_HydratesRolesAndClientGrants pins the bulk-hydration fix:
// FindAll used to skip the role/grant junctions entirely, so every list row
// serialized roles:[] and the ?roles= filter could never match.
func TestFindAll_HydratesRolesAndClientGrants(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)

	const (
		pid     = "prn_hydratetest1"
		grantID = "icg_hydratetest1"
		client  = "clt_hydratetest1"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', 'Hydrate Test', TRUE, 'hydrate-test@example.com')`, pid)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source)
		 VALUES ($1, 'hydrate:role-a', 'MANUAL'), ($1, 'hydrate:role-b', 'MANUAL')`, pid)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_client_access_grants (id, principal_id, client_id, granted_by)
		 VALUES ($1, $2, $3, $2)`, grantID, pid, client)
	require.NoError(t, err)

	all, err := repo.FindAll(ctx)
	require.NoError(t, err)

	var got *principal.Principal
	for i := range all {
		if all[i].ID == pid {
			got = &all[i]
			break
		}
	}
	require.NotNil(t, got, "seeded principal must appear in FindAll")

	roleNames := make([]string, 0, len(got.Roles))
	for _, ra := range got.Roles {
		roleNames = append(roleNames, ra.Role)
	}
	assert.ElementsMatch(t, []string{"hydrate:role-a", "hydrate:role-b"}, roleNames,
		"FindAll must hydrate role assignments")
	assert.ElementsMatch(t, []string{client}, got.AssignedClients,
		"FindAll must hydrate granted-client access")

	// Parity with the single-row path: FindByID returns the same view.
	one, err := repo.FindByID(ctx, pid)
	require.NoError(t, err)
	require.NotNil(t, one)
	oneRoles := make([]string, 0, len(one.Roles))
	for _, ra := range one.Roles {
		oneRoles = append(oneRoles, ra.Role)
	}
	assert.ElementsMatch(t, roleNames, oneRoles)
	assert.ElementsMatch(t, got.AssignedClients, one.AssignedClients)
}

// TestAllApplications_RoundTrip pins the application-scope wiring an application
// service account relies on: a principal with all_applications=false and a
// single application-access row reads back as scoped to exactly that app (the
// CanAccessApplication input), while a principal without an access row but
// all_applications=true reads back as unrestricted.
func TestAllApplications_RoundTrip(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)

	// Scoped service account: all_applications=false + one access row.
	const (
		saPID = "prn_appscoped01"
		appID = "app_scoped00001"
	)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, all_applications)
		 VALUES ($1, 'SERVICE', 'ANCHOR', 'Scoped SA', TRUE, FALSE)`, saPID)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principal_application_access (principal_id, application_id)
		 VALUES ($1, $2)`, saPID, appID)
	require.NoError(t, err)

	sa, err := repo.FindByID(ctx, saPID)
	require.NoError(t, err)
	require.NotNil(t, sa)
	assert.False(t, sa.AllApplications, "scoped SA must read all_applications=false")
	assert.ElementsMatch(t, []string{appID}, sa.AccessibleApplicationIDs)

	// Unrestricted principal: all_applications=true, no access rows.
	const adminPID = "prn_allapps00001"
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, all_applications)
		 VALUES ($1, 'USER', 'ANCHOR', 'All Apps Admin', TRUE, TRUE)`, adminPID)
	require.NoError(t, err)

	admin, err := repo.FindByID(ctx, adminPID)
	require.NoError(t, err)
	require.NotNil(t, admin)
	assert.True(t, admin.AllApplications, "admin must read all_applications=true")
	assert.Empty(t, admin.AccessibleApplicationIDs)
}

// TestLookupVersion_UsesPrincipalUpdatedAtWhenNoRoles pins the base case: a
// principal with no role assignments reports its own updated_at.
func TestLookupVersion_UsesPrincipalUpdatedAtWhenNoRoles(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)

	const pid = "prn_versiontest1"
	principalUpdatedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email, updated_at)
		 VALUES ($1, 'USER', 'CLIENT', 'Version Test', TRUE, 'version-test1@example.com', $2)`,
		pid, principalUpdatedAt)
	require.NoError(t, err)

	got, err := repo.LookupVersion(ctx, pid)
	require.NoError(t, err)
	assert.True(t, got.Equal(principalUpdatedAt), "got %v, want %v", got, principalUpdatedAt)
}

// TestLookupVersion_PrefersLaterRoleUpdatedAt pins the GREATEST logic: when an
// assigned role changed more recently than the principal row itself (e.g. a
// role's permissions were edited), LookupVersion reports the role's
// updated_at — this is what lets a revocation-check catch a role-permission
// change without the principal row being touched at all.
func TestLookupVersion_PrefersLaterRoleUpdatedAt(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)

	const (
		pid    = "prn_versiontest2"
		roleID = "rol_versiontest2"
	)
	principalUpdatedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	roleUpdatedAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) // later
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email, updated_at)
		 VALUES ($1, 'USER', 'CLIENT', 'Version Test 2', TRUE, 'version-test2@example.com', $2)`,
		pid, principalUpdatedAt)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_roles (id, name, display_name, updated_at)
		 VALUES ($1, 'version-test:role', 'Version Test Role', $2)`,
		roleID, roleUpdatedAt)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source)
		 VALUES ($1, 'version-test:role', 'MANUAL')`, pid)
	require.NoError(t, err)

	got, err := repo.LookupVersion(ctx, pid)
	require.NoError(t, err)
	assert.True(t, got.Equal(roleUpdatedAt), "got %v, want the later role updated_at %v", got, roleUpdatedAt)
}

// TestLookupVersion_PrefersLaterPrincipalUpdatedAt is the mirror case: the
// principal row changed more recently than any role it holds, so the
// principal's own updated_at wins.
func TestLookupVersion_PrefersLaterPrincipalUpdatedAt(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)

	const (
		pid    = "prn_versiontest3"
		roleID = "rol_versiontest3"
	)
	roleUpdatedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	principalUpdatedAt := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC) // later
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_roles (id, name, display_name, updated_at)
		 VALUES ($1, 'version-test:role3', 'Version Test Role 3', $2)`,
		roleID, roleUpdatedAt)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email, updated_at)
		 VALUES ($1, 'USER', 'CLIENT', 'Version Test 3', TRUE, 'version-test3@example.com', $2)`,
		pid, principalUpdatedAt)
	require.NoError(t, err)
	_, err = pool.Exec(ctx,
		`INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source)
		 VALUES ($1, 'version-test:role3', 'MANUAL')`, pid)
	require.NoError(t, err)

	got, err := repo.LookupVersion(ctx, pid)
	require.NoError(t, err)
	assert.True(t, got.Equal(principalUpdatedAt), "got %v, want the later principal updated_at %v", got, principalUpdatedAt)
}

// TestPersist_BumpsVersionCache pins the write-path hook: any Persist call
// (the base case every principal write goes through) bumps the configured
// VersionCache with the new updated_at.
func TestPersist_BumpsVersionCache(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := principal.NewRepository(pool)
	cache := &recordingStore{}
	repo.VersionCache = cache

	const pid = "prn_versiontest4"
	_, err := pool.Exec(ctx,
		`INSERT INTO iam_principals (id, type, scope, name, active, email)
		 VALUES ($1, 'USER', 'CLIENT', 'Version Test 4', TRUE, 'version-test4@example.com')`, pid)
	require.NoError(t, err)

	err = repo.UpdatePasswordHash(ctx, pid, "some-hash")
	require.NoError(t, err)

	require.Len(t, cache.bumps, 1)
	assert.Equal(t, pid, cache.bumps[0])
}

// recordingStore is a minimal versioncache.Store test double that just
// records which principal IDs were bumped.
type recordingStore struct {
	bumps []string
}

func (s *recordingStore) Bump(_ context.Context, principalID string, _ time.Time) {
	s.bumps = append(s.bumps, principalID)
}

func (s *recordingStore) Get(context.Context, string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}
