//go:build integration

package ingestion_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

// The retention sweep prunes trace records past FC_INGESTION_TRACE_RETENTION_DAYS
// (7 by default) by the time the platform stored them, never the runtime's own
// `at`, and the windows that ended before that; a window still inside the
// retention, live or ended, stays.
func TestPrune_TraceLogAndWindows(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	now := time.Now().UTC()
	old, fresh := now.Add(-8*24*time.Hour), now.Add(-time.Hour)
	ing := e.live(t, "trace-prune", e.sftp(t))
	gone := e.live(t, "trace-prune-gone", e.sftp(t))
	recent := e.live(t, "trace-prune-recent", e.sftp(t))

	// Each record's runtime time is the opposite of its server time, so a sweep
	// that read the wrong column keeps the wrong one.
	for _, c := range []struct {
		id        string
		at, saved time.Time
	}{{"old", fresh, old}, {"fresh", old, fresh}} {
		require.NoError(t, e.repo.InsertTraceRecords(ctx, []ingestion.TraceLogRecord{{
			ID: "itr_" + c.id + unique(), IngestionID: ing.ID, Version: 1, WindowID: "itw_p", CaptureID: "c", RuntimeID: "rt",
			Kind: "poll", Level: "info", Message: c.id, At: c.at, CreatedAt: c.saved,
		}}))
	}
	exec := func(q string, args ...any) {
		t.Helper()
		_, err := e.pool.Exec(ctx, q, args...)
		require.NoError(t, err)
	}
	window := func(id string, hardStop time.Time) {
		exec(`INSERT INTO ing_trace_windows (ingestion_id, window_id, window_secs, sample_every_secs, hard_stop, enabled_at, updated_at)
			VALUES ($1, 'itw_p', 60, 5, $2, $2, $2)`, id, hardStop)
	}
	window(ing.ID, now.Add(time.Minute)) // live
	window(gone.ID, old)                 // ended a week ago
	window(recent.ID, fresh)             // ended an hour ago

	pruned, err := e.repo.Prune(ctx, now)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, pruned.TraceLog, int64(1))
	assert.GreaterOrEqual(t, pruned.TraceWindows, int64(1))

	assert.Equal(t, []string{"fresh"}, e.strings(t, `SELECT message FROM ing_trace_log WHERE ingestion_id = $1`, ing.ID), "kept by the platform's time")
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM ing_trace_windows WHERE ingestion_id = $1`, gone.ID), "an ended window past the retention goes")
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_trace_windows WHERE ingestion_id = $1`, ing.ID), "a live window stays")
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_trace_windows WHERE ingestion_id = $1`, recent.ID), "a window that ended within the retention stays")
}
