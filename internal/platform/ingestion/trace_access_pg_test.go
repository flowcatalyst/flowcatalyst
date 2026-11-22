//go:build integration

package ingestion_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Who may post trace records for an ingestion (owner's decision D2): the jobs
// call's checks, each refusal with its code. A time base of its own: the
// runtime table is global.
var tTraceAccess0 = time.Date(2034, 2, 1, 12, 0, 0, 0, time.UTC)

// traceAccess answers the refusal codes WithTraceAccess gives insert.
func (e *env) traceAccess(t *testing.T, runtime string, now time.Time, ids ...string) map[string]string {
	t.Helper()
	var got map[string]string
	require.NoError(t, e.repo.WithTraceAccess(context.Background(), runtime, ids, now,
		func(_ context.Context, _ pgx.Tx, refused map[string]string) error {
			got = refused
			return nil
		}))
	return got
}

func TestWithTraceAccess_TheJobsCallsChecks(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	t0 := tTraceAccess0
	polled := e.live(t, "trace-polled", e.sftp(t))
	shared := e.live(t, "trace-shared", sqsDef(t, "Shared", "q-trace-shared"))
	paused := e.live(t, "trace-paused", sqsDef(t, "Paused", "q-trace-paused"))
	paused = e.mustFind(t, paused.ID)
	paused.Pause()
	e.mustSave(t, paused)
	holder, other := "rt-trace-h-"+unique(), "rt-trace-o-"+unique()
	require.Len(t, given(e.poll(t, holder, t0)), 2)
	require.Len(t, given(e.poll(t, other, t0)), 1, "the other runtime holds no lease")
	all := []string{polled.ID, shared.ID, paused.ID, "ing_unknown00000"}

	assert.Equal(t, map[string]string{paused.ID: "INGESTION_NOT_LIVE", "ing_unknown00000": "INGESTION_NOT_FOUND"},
		e.traceAccess(t, holder, t0.Add(time.Second), all...), "the lease holder may post for both it runs")
	assert.Equal(t, map[string]string{
		polled.ID: "LEASE_NOT_HELD", paused.ID: "INGESTION_NOT_LIVE", "ing_unknown00000": "INGESTION_NOT_FOUND",
	}, e.traceAccess(t, other, t0.Add(time.Second), all...), "another runtime only for the shared one")
	assert.Equal(t, map[string]string{polled.ID: "RUNTIME_KIND_NOT_CARRIED", shared.ID: "RUNTIME_KIND_NOT_CARRIED"},
		e.traceAccess(t, "rt-trace-never-"+unique(), t0.Add(time.Second), polled.ID, shared.ID), "a runtime that never polled carries nothing")

	// The lease is the one held now: once it has run out unrenewed, the holder
	// may no longer post for it; when it is renewed after a gap, it may again.
	assert.Equal(t, map[string]string{polled.ID: "LEASE_NOT_HELD"}, e.traceAccess(t, holder, t0.Add(lease+time.Second), polled.ID, shared.ID))
	require.Len(t, given(e.poll(t, holder, t0.Add(10*time.Minute))), 2)
	assert.Empty(t, e.traceAccess(t, holder, t0.Add(10*time.Minute+time.Second), polled.ID, shared.ID))

	// A suspended client's ingestions take no trace records.
	_, err := e.pool.Exec(ctx, `UPDATE tnt_clients SET status = 'SUSPENDED' WHERE id = $1`, e.client.ID)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{polled.ID: "CLIENT_NOT_ACTIVE", shared.ID: "CLIENT_NOT_ACTIVE"},
		e.traceAccess(t, holder, t0.Add(10*time.Minute+2*time.Second), polled.ID, shared.ID))
}
