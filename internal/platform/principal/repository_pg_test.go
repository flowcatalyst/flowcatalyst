//go:build integration

package principal_test

import (
	"context"
	"testing"

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
