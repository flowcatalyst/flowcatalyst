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

// TestInsertBatch_DedupCollisionDropsOnlyThatRow pins the ON CONFLICT DO
// NOTHING behavior: a duplicate deduplication_id drops just the colliding
// row — the rest of the batch still lands and no error surfaces (previously
// one collision aborted the entire pipelined batch).
func TestInsertBatch_DedupCollisionDropsOnlyThatRow(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := event.NewRepository(pool)

	// The unique index is (deduplication_id, created_at), so colliding rows
	// must share the timestamp — as a same-payload retry does.
	now := time.Now().UTC()
	mk := func(id, dedupID string) event.Event {
		return event.Event{
			ID: id, SpecVersion: "1.0", Type: "dedup.test.event",
			Source: "test://dedup", Time: now, CreatedAt: now,
			Data: []byte(`{"k":1}`), DeduplicationID: dedupID,
		}
	}

	inserted, err := repo.InsertBatch(ctx, []event.Event{mk("evtdeduptest1", "dedup-collision-1")})
	require.NoError(t, err)
	require.Equal(t, 1, inserted)

	// Second batch: one collision + one fresh row.
	inserted, err = repo.InsertBatch(ctx, []event.Event{
		mk("evtdeduptest2", "dedup-collision-1"), // same dedup id → dropped
		mk("evtdeduptest3", "dedup-collision-2"), // fresh → lands
	})
	require.NoError(t, err, "a dedup collision must not fail the batch")
	assert.Equal(t, 1, inserted, "only the non-colliding row counts")

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM msg_events WHERE type = 'dedup.test.event'`).Scan(&count))
	assert.Equal(t, 2, count, "original + fresh row; collision dropped")
}

// TestInsertBatch_DedupAcrossRequests pins the real-world retry: the outbox
// re-sends the same deduplication_id in a later request, so created_at
// differs and the (deduplication_id, created_at) unique index cannot catch
// it. InsertBatch must drop it anyway, and must also drop a repeat inside
// one batch whose created_at differs (first occurrence wins).
func TestInsertBatch_DedupAcrossRequests(t *testing.T) {
	ctx := context.Background()
	pool := testpg.Pool(t)
	repo := event.NewRepository(pool)

	mk := func(id, dedupID string, at time.Time) event.Event {
		return event.Event{
			ID: id, SpecVersion: "1.0", Type: "dedup.xreq.event",
			Source: "test://dedup", Time: at, CreatedAt: at,
			Data: []byte(`{"k":1}`), DeduplicationID: dedupID,
		}
	}
	t0 := time.Now().UTC().Add(-time.Minute)

	inserted, err := repo.InsertBatch(ctx, []event.Event{mk("evtxreq1", "xreq-1", t0)})
	require.NoError(t, err)
	require.Equal(t, 1, inserted)

	// Later request, later created_at, same dedup id → dropped. A fresh id
	// repeated within the batch with different timestamps → one row.
	t1 := t0.Add(time.Second)
	inserted, err = repo.InsertBatch(ctx, []event.Event{
		mk("evtxreq2", "xreq-1", t1),
		mk("evtxreq3", "xreq-2", t1),
		mk("evtxreq4", "xreq-2", t1.Add(time.Millisecond)),
		mk("evtxreq5", "", t1), // no dedup id → always kept
	})
	require.NoError(t, err)
	assert.Equal(t, 2, inserted, "xreq-2 once + the id-less event")

	var count int
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM msg_events WHERE type = 'dedup.xreq.event'`).Scan(&count))
	assert.Equal(t, 3, count)
	var kept string
	require.NoError(t, pool.QueryRow(ctx,
		`SELECT id FROM msg_events WHERE deduplication_id = 'xreq-2'`).Scan(&kept))
	assert.Equal(t, "evtxreq3", kept, "first occurrence wins")
}
