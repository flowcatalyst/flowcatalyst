package sqlite_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/outbox"
	outboxsqlite "github.com/flowcatalyst/flowcatalyst-go/internal/outbox/sqlite"
	sdkoutbox "github.com/flowcatalyst/go-sdk/outbox"
	"github.com/flowcatalyst/go-sdk/outboxsql"
)

func openTestDB(t *testing.T) (*sql.DB, *outboxsqlite.Repository) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "outbox.db")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	_, err = db.ExecContext(t.Context(), `PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;`)
	require.NoError(t, err)

	repo := outboxsqlite.New(db)
	require.NoError(t, repo.InitSchema(t.Context()))
	return db, repo
}

func insertRow(t *testing.T, db *sql.DB, id, itemType, group string, created time.Time, status, retries int, msg string) {
	t.Helper()
	var g, m any
	if group != "" {
		g = group
	}
	if msg != "" {
		m = msg
	}
	_, err := db.ExecContext(t.Context(), `
INSERT INTO outbox_messages (id, type, message_group, payload, status, retry_count, error_message, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, itemType, g, fmt.Sprintf(`{"topic":"shop:a","type":"x","data":{},"pushId":%q}`, id),
		status, retries, m, created.UTC().Format(time.RFC3339Nano), created.UTC().Format(time.RFC3339Nano),
	)
	require.NoError(t, err)
}

type stored struct {
	status  int
	retries int
	msg     string
}

func readRow(t *testing.T, db *sql.DB, id string) (stored, bool) {
	t.Helper()
	var r stored
	var msg sql.NullString
	err := db.QueryRowContext(t.Context(),
		`SELECT status, retry_count, error_message FROM outbox_messages WHERE id = ?`, id,
	).Scan(&r.status, &r.retries, &msg)
	if err != nil {
		return stored{}, false
	}
	if msg.Valid {
		r.msg = msg.String
	}
	return r, true
}

func countRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	var n int
	require.NoError(t, db.QueryRowContext(t.Context(), `SELECT count(*) FROM outbox_messages`).Scan(&n))
	return n
}

func TestOpenFileAndMemory(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	memRepo, err := outboxsqlite.Open(ctx, ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = memRepo.Close() })
	assert.True(t, memRepo.Healthy(ctx))
	require.NoError(t, memRepo.InitSchema(ctx))

	fileRepo, err := outboxsqlite.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "app.sqlite"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = fileRepo.Close() })
	assert.True(t, fileRepo.Healthy(ctx))
	require.NoError(t, fileRepo.InitSchema(ctx))
}

func TestClaimFilterAndInProgressGroupBarrier(t *testing.T) {
	t.Parallel()
	db, repo := openTestDB(t)
	base := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

	// grp-busy already has an IN_PROGRESS row, so its later PENDING row must not be claimed.
	insertRow(t, db, "busy-1", "PUSH", "grp-busy", base, 9, 0, "")
	insertRow(t, db, "busy-2", "PUSH", "grp-busy", base.Add(time.Millisecond), 0, 0, "")

	// grp-skip is in skip.Groups; u-event is an ungrouped EVENT in skip.Types; u-skip-id is in skip.IDs.
	insertRow(t, db, "skip-g1", "PUSH", "grp-skip", base, 0, 0, "")
	insertRow(t, db, "u-event", "EVENT", "", base, 0, 0, "")
	// Grouped EVENT is NOT skipped by skip.Types (skip.Types applies only to ungrouped rows).
	insertRow(t, db, "g-event", "EVENT", "grp-ok", base, 0, 0, "")
	insertRow(t, db, "u-skip-id", "PUSH", "", base, 0, 0, "")
	insertRow(t, db, "u-ok", "PUSH", "", base.Add(2*time.Millisecond), 0, 0, "")

	claimed, err := repo.ClaimPending(t.Context(), 10, outbox.ClaimFilter{
		Groups: []string{"grp-skip"},
		Types:  []common.OutboxItemType{common.OutboxItemEvent},
		IDs:    []string{"u-skip-id"},
	})
	require.NoError(t, err)
	gotIDs := make([]string, len(claimed))
	for i, it := range claimed {
		gotIDs[i] = it.ID
		assert.Equal(t, common.OutboxInProgress, it.Status)
	}
	assert.Equal(t, []string{"g-event", "u-ok"}, gotIDs)
}

func TestMarkersLegacySweepRequeueAndRecoverStuck(t *testing.T) {
	t.Parallel()
	db, repo := openTestDB(t)
	ctx := t.Context()
	old := time.Now().UTC().Add(-10 * time.Minute)
	fresh := time.Now().UTC()

	// Legacy terminal failure without marker -> gets [skipped before upgrade]
	insertRow(t, db, "leg-1", "PUSH", "grp-leg", old, common.OutboxBadRequest.Code(), 1, "bad payload")
	// Blocking failures in grp-b (two rows: BlockingFailures must return the oldest)
	insertRow(t, db, "blk-1", "PUSH", "grp-b", old, common.OutboxForbidden.Code(), 1, outbox.BlockingMarker+"first")
	insertRow(t, db, "blk-2", "PUSH", "grp-b", old.Add(time.Second), common.OutboxBadRequest.Code(), 1, outbox.BlockingMarker+"second")
	// Retryable failure left by older processor -> swept back to PENDING
	insertRow(t, db, "ret-1", "PUSH", "grp-r", old, common.OutboxInternalError.Code(), 3, "500")
	// Stuck IN_PROGRESS row vs fresh IN_PROGRESS row
	insertRow(t, db, "stuck-old", "PUSH", "grp-s", old, 9, 0, "")
	insertRow(t, db, "stuck-fresh", "PUSH", "grp-f", fresh, 9, 0, "")

	marked, err := repo.MarkLegacyRefusals(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, marked)
	leg, ok := readRow(t, db, "leg-1")
	require.True(t, ok)
	assert.Equal(t, outbox.SkippedBeforeUpgradeMarker+"bad payload", leg.msg)

	// Idempotent on second run.
	marked, err = repo.MarkLegacyRefusals(ctx)
	require.NoError(t, err)
	assert.Zero(t, marked)

	swept, err := repo.RequeueRetryableFailures(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, swept)
	ret, ok := readRow(t, db, "ret-1")
	require.True(t, ok)
	assert.Equal(t, 0, ret.status)
	assert.Equal(t, 3, ret.retries)

	blocking, err := repo.BlockingFailures(ctx)
	require.NoError(t, err)
	require.Len(t, blocking, 1)
	assert.Equal(t, "blk-1", blocking[0].ID)

	// MarkSkipped replaces [blocking] with [skipped by operator] once.
	require.NoError(t, repo.MarkSkipped(ctx, []string{"blk-1"}))
	require.NoError(t, repo.MarkSkipped(ctx, []string{"blk-1"}))
	blk1, ok := readRow(t, db, "blk-1")
	require.True(t, ok)
	assert.Equal(t, outbox.SkippedMarker+"first", blk1.msg)

	// RecoverStuck resets only stuck-old, leaving stuck-fresh IN_PROGRESS.
	rec, err := repo.RecoverStuck(ctx, time.Minute)
	require.NoError(t, err)
	assert.Equal(t, 1, rec)
	sOld, _ := readRow(t, db, "stuck-old")
	sFresh, _ := readRow(t, db, "stuck-fresh")
	assert.Equal(t, 0, sOld.status)
	assert.Equal(t, 9, sFresh.status)

	// Release returns stuck-fresh to PENDING without penalty; Requeue resets blk-2 to fresh PENDING.
	require.NoError(t, repo.Release(ctx, []string{"stuck-fresh"}))
	sFresh, _ = readRow(t, db, "stuck-fresh")
	assert.Equal(t, 0, sFresh.status)

	require.NoError(t, repo.Requeue(ctx, []string{"blk-2"}))
	blk2, _ := readRow(t, db, "blk-2")
	assert.Equal(t, 0, blk2.status)
	assert.Equal(t, 0, blk2.retries)
	assert.Empty(t, blk2.msg)
}

func TestSDKOutboxSQLWriterCompatibility(t *testing.T) {
	t.Parallel()
	db, repo := openTestDB(t)
	ctx := t.Context()

	// Write rows through pkg/fcsdk/outboxsql.Writer (which binds Go time.Time
	// values via '?' placeholders) and ensure sqlite.Repository claims & parses them.
	writer, err := outboxsql.NewWriter(outboxsql.DialectMySQL, outboxsql.WriterConfig{
		ClientID:        "clt_123",
		ClientCode:      "acme",
		ApplicationCode: "orders",
	})
	require.NoError(t, err)

	p1 := sdkoutbox.NewPush("orders:live", "order.updated", map[string]any{"n": 1}).
		WithID("push_01").
		WithMessageGroup("grp-sdk")
	p2 := sdkoutbox.NewPush("orders:live", "order.updated", map[string]any{"n": 2}).
		WithID("push_02").
		WithMessageGroup("grp-sdk")
	require.NoError(t, writer.WritePush(ctx, db, p1))
	require.NoError(t, writer.WritePush(ctx, db, p2))

	items, err := repo.ClaimPending(ctx, 10, outbox.ClaimFilter{})
	require.NoError(t, err)
	require.Len(t, items, 2)
	assert.Equal(t, "push_01", items[0].ID)
	assert.Equal(t, "push_02", items[1].ID)
	assert.False(t, items[0].CreatedAt.IsZero())
	require.NoError(t, repo.MarkSuccess(ctx, []string{"push_01", "push_02"}))
	assert.Zero(t, countRows(t, db))
}

// End-to-end Processor test against SQLite: simulated platform outage + per-row
// retryable failures + concurrent group dispatch, verifying every row is
// delivered once and every message group arrives in exact written order.
func TestProcessorEndToEndOrderAndRecovery(t *testing.T) {
	t.Parallel()
	db, repo := openTestDB(t)
	at := time.Now().UTC().Add(-time.Minute)

	written := map[string][]string{}
	total := 0
	for _, g := range []string{"a", "b", "c"} {
		for i := range 8 {
			id := fmt.Sprintf("%s-%02d", g, i+1)
			// Mix sub-second and whole-second RFC3339 timestamps to verify unixepoch ordering.
			insertRow(t, db, id, "PUSH", "grp-"+g, at.Add(time.Duration(i)*time.Millisecond), 0, 0, "")
			written["grp-"+g] = append(written["grp-"+g], id)
			total++
		}
	}
	for i := range 5 {
		insertRow(t, db, fmt.Sprintf("u-%02d", i+1), "PUSH", "", at.Add(time.Duration(i)*time.Millisecond), 0, 0, "")
		total++
	}

	var (
		mu           sync.Mutex
		delivered    []string
		outageUntil  = time.Now().Add(250 * time.Millisecond)
		failuresLeft = map[string]int{"a-03": 2, "b-01": 2, "u-02": 1}
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Items []struct {
				PushID string `json:"pushId"`
			} `json:"items"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		mu.Lock()
		defer mu.Unlock()
		if time.Now().Before(outageUntil) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		out := make([]map[string]string, len(body.Items))
		for i, it := range body.Items {
			st := "SUCCESS"
			if failuresLeft[it.PushID] > 0 {
				failuresLeft[it.PushID]--
				st = "INTERNAL_ERROR"
			} else {
				delivered = append(delivered, it.PushID)
			}
			out[i] = map[string]string{"status": st}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": out})
	}))
	t.Cleanup(srv.Close)

	cfg := outbox.DefaultConfig()
	cfg.PlatformURL = srv.URL
	cfg.BatchSize = 8
	cfg.PollInterval = 15 * time.Millisecond
	cfg.RetryBackoffBase = 25 * time.Millisecond
	cfg.RetryBackoffMax = 100 * time.Millisecond

	proc := outbox.NewProcessor(cfg, repo)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { proc.Run(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	deadline := time.Now().Add(10 * time.Second)
	for countRows(t, db) != 0 {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for SQLite outbox to drain; remaining=%d", countRows(t, db))
		}
		time.Sleep(15 * time.Millisecond)
	}
	cancel()
	<-done

	mu.Lock()
	got := slices.Clone(delivered)
	mu.Unlock()

	require.Len(t, got, total, "every row delivered exactly once")
	for g, want := range written {
		var seq []string
		for _, id := range got {
			if "grp-"+id[:1] == g {
				seq = append(seq, id)
			}
		}
		assert.Equal(t, want, seq, "group %s delivered in order", g)
	}
}
