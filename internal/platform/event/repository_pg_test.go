//go:build integration

package event_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/event"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// TestFindWithFilters_TenantScoping pins the SQL-side AccessibleClientIDs
// enforcement: a non-anchor's caller-controlled clientId filters may only
// narrow within its own tenants (plus platform-scoped rows) — never reach
// into another tenant's events.
func TestFindWithFilters_TenantScoping(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := event.NewRepository(pool)

	const (
		typ     = "scope.test.event" // unique type so assertions see only our rows
		clientA = "clt_scopeevt0001"
		clientB = "clt_scopeevt0002"
	)
	seed := func(id string, clientID *string) {
		t.Helper()
		_, err := pool.Exec(ctx,
			`INSERT INTO msg_events_read (id, type, source, time, client_id, created_at)
			 VALUES ($1, $2, 'test://scoping', NOW(), $3, $4)`,
			id, typ, clientID, time.Now().UTC())
		require.NoError(t, err)
	}
	a, b := clientA, clientB
	seed("evtscopetest1", &a)  // tenant A
	seed("evtscopetest2", &b)  // tenant B
	seed("evtscopetest3", nil) // platform-scoped

	ids := func(rows []event.Event) []string {
		out := make([]string, 0, len(rows))
		for i := range rows {
			out = append(out, rows[i].ID)
		}
		return out
	}

	// Anchor (no scoping): all three.
	rows, err := repo.FindWithFilters(ctx, event.FilterParams{Types: []string{typ}})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"evtscopetest1", "evtscopetest2", "evtscopetest3"}, ids(rows))

	// Non-anchor with access to A: own tenant + platform-scoped, never B.
	accessible := []string{clientA}
	rows, err = repo.FindWithFilters(ctx, event.FilterParams{
		Types: []string{typ}, AccessibleClientIDs: &accessible,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"evtscopetest1", "evtscopetest3"}, ids(rows))

	// The attack shape: non-anchor (access to A) explicitly filters for
	// tenant B's events. The scoping must intersect, yielding nothing.
	rows, err = repo.FindWithFilters(ctx, event.FilterParams{
		Types: []string{typ}, ClientIDs: []string{clientB}, AccessibleClientIDs: &accessible,
	})
	require.NoError(t, err)
	assert.Empty(t, ids(rows), "cross-tenant filter must not leak another tenant's events")

	// Filtering for both tenants narrows to the accessible one.
	rows, err = repo.FindWithFilters(ctx, event.FilterParams{
		Types: []string{typ}, ClientIDs: []string{clientA, clientB}, AccessibleClientIDs: &accessible,
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"evtscopetest1"}, ids(rows))
}
