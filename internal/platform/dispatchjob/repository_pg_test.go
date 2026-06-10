//go:build integration

package dispatchjob_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindWithFilters_TenantScoping pins the SQL-side AccessibleClientIDs
// enforcement on the dispatch-job list — same contract as the event repo: a
// non-anchor's caller-controlled clientId filters narrow within its own
// tenants (plus platform-scoped rows), never across.
func TestFindWithFilters_TenantScoping(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := dispatchjob.NewRepository(pool)

	const (
		code    = "scopetest:jobs:list" // unique code so assertions see only our rows
		clientA = "clt_scopejob0001"
		clientB = "clt_scopejob0002"
	)
	seed := func(id string, clientID *string) {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_dispatch_jobs (id, code, target_url, client_id)
			 VALUES ($1, $2, 'http://example.invalid/hook', $3)`,
			id, code, clientID)
		require.NoError(t, err)
	}
	a, b := clientA, clientB
	seed("djscopetest01", &a)  // tenant A
	seed("djscopetest02", &b)  // tenant B
	seed("djscopetest03", nil) // platform-scoped

	ids := func(rows []dispatchjob.DispatchJob) []string {
		out := make([]string, 0, len(rows))
		for i := range rows {
			out = append(out, rows[i].ID)
		}
		return out
	}

	// Anchor (no scoping): all three.
	rows, err := repo.FindWithFilters(ctx, dispatchjob.FilterParams{Codes: []string{code}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"djscopetest01", "djscopetest02", "djscopetest03"}, ids(rows))

	// Non-anchor with access to A: own tenant + platform-scoped, never B.
	accessible := []string{clientA}
	rows, err = repo.FindWithFilters(ctx, dispatchjob.FilterParams{
		Codes: []string{code}, AccessibleClientIDs: &accessible,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"djscopetest01", "djscopetest03"}, ids(rows))

	// The attack shape: explicit filter for the OTHER tenant yields nothing.
	rows, err = repo.FindWithFilters(ctx, dispatchjob.FilterParams{
		Codes: []string{code}, ClientIDs: []string{clientB}, AccessibleClientIDs: &accessible,
	})
	require.NoError(t, err)
	assert.Empty(t, ids(rows), "cross-tenant filter must not leak another tenant's jobs")
}
