//go:build integration

package api

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/principal"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestListPrincipals_SQLFilteringSortingPaginationAndScoping(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: principal.NewRepository(pool)}

	cMine := "cli_sqlflt0001"
	cOther := "cli_sqlflt0002"
	prefix := "sqlflt-"
	t0 := time.Date(2025, 6, 1, 10, 0, 0, 0, time.UTC)

	seedRow := func(id, typ, scope string, cid *string, name string, active bool, email *string, created time.Time) {
		t.Helper()
		_, err := pool.Exec(ctx, `
			INSERT INTO iam_principals (id, type, scope, client_id, name, active, email, created_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $8)`,
			id, typ, scope, cid, name, active, email, created)
		require.NoError(t, err)
	}

	eAlice := prefix + "alice_100%@acme.example"
	eBob := prefix + "bob@acme.example"
	ePartner := prefix + "charlie@partner.example"
	eOther := prefix + "other@theirs.example"
	eAnchor := prefix + "anchor@platform.example"
	eSvc := prefix + "svc-hidden@acme.example" // SERVICE row with email column populated: must NOT match email search

	seedRow("prn_sqlflt0001", "USER", "CLIENT", &cMine, prefix+"alice_100%", true, &eAlice, t0.Add(2*time.Hour))
	seedRow("prn_sqlflt0002", "USER", "CLIENT", &cMine, prefix+"Bob", false, &eBob, t0)
	seedRow("prn_sqlflt0003", "USER", "PARTNER", nil, prefix+"Charlie", true, &ePartner, t0.Add(time.Hour))
	seedRow("prn_sqlflt0004", "USER", "CLIENT", &cOther, prefix+"Other", true, &eOther, t0.Add(3*time.Hour))
	seedRow("prn_sqlflt0005", "USER", "ANCHOR", nil, prefix+"Anchor", true, &eAnchor, t0.Add(4*time.Hour))
	seedRow("prn_sqlflt0006", "SERVICE", "CLIENT", &cMine, prefix+"BillingSvc", true, &eSvc, t0.Add(5*time.Hour))

	// Partner holds grants to both cMine and cOther.
	_, err := pool.Exec(ctx, `
		INSERT INTO iam_client_access_grants (id, principal_id, client_id, granted_by)
		VALUES ('cag_sqlflt0001', 'prn_sqlflt0003', $1, 'prn_0TESTPGADM1N0'),
		       ('cag_sqlflt0002', 'prn_sqlflt0003', $2, 'prn_0TESTPGADM1N0')`, cMine, cOther)
	require.NoError(t, err)

	// Roles: Alice -> app:admin, Bob -> app:viewer, Partner -> app:auditor.
	_, err = pool.Exec(ctx, `
		INSERT INTO iam_principal_roles (principal_id, role_name, assignment_source)
		VALUES ('prn_sqlflt0001', 'sqlflt:app:admin', 'MANUAL'),
		       ('prn_sqlflt0002', 'sqlflt:app:viewer', 'MANUAL'),
		       ('prn_sqlflt0003', 'sqlflt:app:auditor', 'MANUAL')`)
	require.NoError(t, err)

	anchor := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_sqlfltadm",
		Scope:       auth.ScopeAnchor,
		Permissions: []string{testPermUserView},
	})
	clientCaller := auth.WithContext(ctx, clientScopedCaller("prn_sqlfltcaller", cMine, testPermUserView))

	idsOf := func(res PrincipalListResponse) []string {
		out := make([]string, len(res.Principals))
		for i, p := range res.Principals {
			out[i] = p.ID
		}
		return out
	}

	// 1. Visibility + partner grant redaction for client-scoped caller.
	gotClient, err := s.ListPrincipals(clientCaller, ListQuery{Q: prefix, SortField: "name", SortOrder: "asc"})
	require.NoError(t, err)
	assert.Equal(t, 4, gotClient.Total)
	assert.Equal(t, []string{"prn_sqlflt0001", "prn_sqlflt0006", "prn_sqlflt0002", "prn_sqlflt0003"}, idsOf(gotClient),
		"case-insensitive name sort: alice_100%, BillingSvc, Bob, Charlie; Other and Anchor hidden")
	for _, p := range gotClient.Principals {
		if p.ID == "prn_sqlflt0003" {
			assert.Equal(t, []string{cMine}, p.GrantedClientIDs, "non-anchor sees only its own client grant on partner")
		}
	}

	// 2. Anchor sees all 6, and pagination returns pre-pagination Total + exact non-overlapping pages.
	p0, err := s.ListPrincipals(anchor, ListQuery{Q: prefix, SortField: "createdAt", SortOrder: "asc", Page: 0, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, 6, p0.Total)
	assert.Equal(t, []string{"prn_sqlflt0002", "prn_sqlflt0003"}, idsOf(p0))

	p1, err := s.ListPrincipals(anchor, ListQuery{Q: prefix, SortField: "createdAt", SortOrder: "asc", Page: 1, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, 6, p1.Total)
	assert.Equal(t, []string{"prn_sqlflt0001", "prn_sqlflt0004"}, idsOf(p1))

	pOut, err := s.ListPrincipals(anchor, ListQuery{Q: prefix, Page: 99, PageSize: 2})
	require.NoError(t, err)
	assert.Equal(t, 6, pOut.Total)
	assert.Empty(t, pOut.Principals)

	// 3. Literal _ and % characters in Q do NOT act as SQL LIKE wildcards.
	seedRow("prn_sqlflt0007", "USER", "CLIENT", &cMine, prefix+"aliceX1000", true, nil, t0)
	gotLiteral, err := s.ListPrincipals(anchor, ListQuery{Q: prefix + "alice_100%"})
	require.NoError(t, err)
	assert.Equal(t, []string{"prn_sqlflt0001"}, idsOf(gotLiteral))

	// 4. SERVICE row's raw DB email column is NOT matched as user email.
	gotSvcEmail, err := s.ListPrincipals(anchor, ListQuery{Q: "svc-hidden@"})
	require.NoError(t, err)
	assert.Empty(t, gotSvcEmail.Principals)

	// 5. ClientID filter matches both home client AND granted client (partner).
	gotByClient, err := s.ListPrincipals(anchor, ListQuery{Q: prefix, ClientID: cMine, Active: "true", Type: "USER", SortField: "name", SortOrder: "desc"})
	require.NoError(t, err)
	assert.Equal(t, []string{"prn_sqlflt0003", "prn_sqlflt0007", "prn_sqlflt0001"}, idsOf(gotByClient))

	// 6. Roles CSV uses OR semantics.
	gotRoles, err := s.ListPrincipals(anchor, ListQuery{Roles: "sqlflt:app:viewer, sqlflt:app:auditor", SortField: "name", SortOrder: "asc"})
	require.NoError(t, err)
	assert.Equal(t, []string{"prn_sqlflt0002", "prn_sqlflt0003"}, idsOf(gotRoles))
}

func TestListPrincipals_CorruptRowFailsLoudly(t *testing.T) {
	// Not t.Parallel(): WithConstraintDropped mutates table constraint state.
	ctx := context.Background()
	pool := testpg.Pool(t)
	s := &State{Repo: principal.NewRepository(pool)}

	const badID = "prn_sqlfltbad01"
	testpg.WithConstraintDropped(t, pool, "iam_principals", "chk_iam_principals_scope", func() {
		_, err := pool.Exec(ctx,
			`INSERT INTO iam_principals (id, type, scope, name, active)
			 VALUES ($1, 'USER', 'BOGUS_SCOPE', 'sqlflt-corrupt-row', TRUE)`, badID)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = pool.Exec(context.Background(), `DELETE FROM iam_principals WHERE id = $1`, badID)
		})
	})

	anchor := auth.WithContext(ctx, &auth.AuthContext{
		PrincipalID: "prn_sqlfltadm2",
		Scope:       auth.ScopeAnchor,
		Permissions: []string{testPermUserView},
	})
	_, err := s.ListPrincipals(anchor, ListQuery{Q: "sqlflt-corrupt-row"})
	require.Error(t, err)
}
