//go:build integration

// Package lifecycletest states the dispatch-job lifecycle in one place: for
// every (transition, from-status) pair, the status the job ends in or
// "refused". The table below is the documentation. It lives in a package of its
// own because the stale-recovery and reaper transitions are database-wide
// sweeps, which must not run alongside other packages' parallel tests.
package lifecycletest

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatchjob"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
)

func TestMain(m *testing.M) { testpg.RunMain(m) }

// Every status the table's CHECK constraint admits.
var allStatuses = []string{
	"PENDING", "QUEUED", "PROCESSING", "IN_PROGRESS",
	"COMPLETED", "FAILED", "CANCELLED", "EXPIRED", "ERROR",
}

var (
	live        = []string{"PENDING", "QUEUED", "PROCESSING", "IN_PROGRESS"}
	queuedOrPro = []string{"QUEUED", "PROCESSING"}
)

type seeded struct {
	id        string
	createdAt time.Time
	updatedAt time.Time
}

type tcase struct {
	name string
	// from -> resulting status. Statuses absent from the map are REFUSED.
	allowed map[string]string
	// counted: a refusal increments fc_dispatch_job_transition_refused_total
	// (false for sweeps, whose normal outcome is matching nothing).
	counted bool
	// needsHead seeds a FAILED BLOCK_ON_ERROR head ahead of the job in its group.
	needsHead bool
	run       func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error
}

func to(status string, from ...string) map[string]string {
	m := map[string]string{}
	for _, f := range from {
		m[f] = status
	}
	return m
}

var cases = []tcase{
	{name: "retry", counted: true, allowed: to("PENDING", live...),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			msg := "boom"
			_, err := lc.Retry(ctx, j.id, j.createdAt, time.Now().Add(time.Minute), &msg)
			return err
		}},
	{name: "defer", counted: true, allowed: to("PENDING", live...),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.Defer(ctx, j.id, j.createdAt, time.Now().Add(time.Minute))
			return err
		}},
	{name: "hold", counted: true, allowed: to("PENDING", live...),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.Hold(ctx, j.id, j.createdAt, time.Now().Add(time.Minute))
			return err
		}},
	{name: "settle_acked", counted: true, allowed: to("PENDING", queuedOrPro...),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.SettleAcked(ctx, []string{j.id}, "test")
			return err
		}},
	{name: "sweep_stranded", needsHead: true, allowed: to("PENDING", queuedOrPro...),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.SweepStranded(ctx, time.Now().Add(time.Hour), "test")
			return err
		}},
	{name: "stale_queued", allowed: to("PENDING", "QUEUED"),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.RecoverStaleQueued(ctx, time.Now().Add(-30*time.Minute))
			return err
		}},
	{name: "stale_processing", allowed: to("PENDING", "PROCESSING"),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.RecoverStaleProcessing(ctx, time.Now().Add(-30*time.Minute))
			return err
		}},
	{name: "requeue", counted: true, allowed: to("PENDING", allStatuses...),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.Requeue(ctx, j.id, j.createdAt)
			return err
		}},
	{name: "mark_queued", counted: true, allowed: to("QUEUED", "PENDING"),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.MarkQueued(ctx, []string{j.id}, []time.Time{j.updatedAt}, j.createdAt, j.createdAt)
			return err
		}},
	{name: "claim_for_delivery", counted: true, allowed: to("PROCESSING", "PENDING", "QUEUED"),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.ClaimForDelivery(ctx, j.id, j.createdAt)
			return err
		}},
	{name: "reclaim_stale_delivery", counted: true, allowed: to("PROCESSING", "PROCESSING"),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.ReclaimStaleDelivery(ctx, j.id, j.createdAt, time.Now().Add(time.Hour))
			return err
		}},
	{name: "complete", counted: true, allowed: to("COMPLETED", live...),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.Complete(ctx, j.id, j.createdAt, 12)
			return err
		}},
	{name: "fail", counted: true, allowed: to("FAILED", live...),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			msg := "dead"
			_, err := lc.Fail(ctx, j.id, j.createdAt, &msg, 12)
			return err
		}},
	{name: "operator_cancel", counted: true, allowed: to("CANCELLED", "FAILED"),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.OperatorCancel(ctx, j.id, j.createdAt)
			return err
		}},
	{name: "operator_complete", counted: true, allowed: to("COMPLETED", "FAILED"),
		run: func(ctx context.Context, lc *dispatchjob.Lifecycle, j seeded) error {
			_, err := lc.OperatorComplete(ctx, j.id, j.createdAt)
			return err
		}},
}

func newJob(group string, mode common.DispatchMode, seq int32) dispatchjob.DispatchJob {
	return dispatchjob.DispatchJob{
		ID:                 tsid.GenerateUntyped(),
		Kind:               dispatchjob.KindEvent,
		Code:               "lifecycle:matrix",
		TargetURL:          "http://example.invalid/hook",
		Protocol:           dispatchjob.ProtocolHTTPWebhook,
		PayloadContentType: "application/json",
		Mode:               mode,
		MessageGroup:       &group,
		Sequence:           seq,
		TimeoutSeconds:     30,
		MaxRetries:         3,
		RetryStrategy:      dispatchjob.RetryExponentialBackoff,
		CreatedAt:          time.Now().UTC().Truncate(time.Microsecond),
	}
}

// seed creates a job through the lifecycle (always PENDING) and then puts it in
// `status` with a test-only write, its updated_at an hour in the past so a
// stamped updated_at is visible.
func seed(t *testing.T, pool *pgxpool.Pool, j dispatchjob.DispatchJob, status string) seeded {
	t.Helper()
	ctx := context.Background()
	rows, err := dispatchjob.NewLifecycle(pool).CreateBatch(ctx, []dispatchjob.DispatchJob{j})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	old := time.Now().Add(-time.Hour).UTC().Truncate(time.Microsecond)
	_, err = pool.Exec(ctx, `UPDATE msg_dispatch_jobs SET status = $2, updated_at = $3 WHERE id = $1`, j.ID, status, old)
	require.NoError(t, err)
	return seeded{id: j.ID, createdAt: j.CreatedAt, updatedAt: old}
}

func snapshot(t *testing.T, pool *pgxpool.Pool, id string) map[string]any {
	t.Helper()
	var raw []byte
	require.NoError(t, pool.QueryRow(context.Background(),
		`SELECT to_jsonb(j) FROM msg_dispatch_jobs j WHERE id = $1`, id).Scan(&raw))
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	return m
}

func refusedCount(t *testing.T, transition string) float64 {
	t.Helper()
	mfs, err := dispatchjob.MetricsRegistry.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != "fc_dispatch_job_transition_refused_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelValue(m, "transition") == transition {
				return m.GetCounter().GetValue()
			}
		}
	}
	t.Fatalf("no refused counter for transition %q", transition)
	return 0
}

func labelValue(m *dto.Metric, name string) string {
	for _, l := range m.GetLabel() {
		if l.GetName() == name {
			return l.GetValue()
		}
	}
	return ""
}

// TestLifecycleMatrix: for every transition and every status a job can be in,
// the job ends in the table's status — with updated_at moved forward — or is
// left byte-for-byte untouched and the refusal is counted.
func TestLifecycleMatrix(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)

	for _, tc := range cases {
		for _, from := range allStatuses {
			t.Run(fmt.Sprintf("%s/from_%s", tc.name, from), func(t *testing.T) {
				// Sequential and on a clean table: the sweeps are database-wide.
				_, err := pool.Exec(ctx, `DELETE FROM msg_dispatch_jobs`)
				require.NoError(t, err)

				group := "matrix-" + tsid.GenerateUntyped()
				mode := common.DispatchNextOnError
				if tc.needsHead {
					mode = common.DispatchBlockOnError
					seed(t, pool, newJob(group, mode, 1), "FAILED")
				}
				j := seed(t, pool, newJob(group, mode, 2), from)
				before := snapshot(t, pool, j.id)
				refusedBefore := refusedCount(t, tc.name)

				require.NoError(t, tc.run(ctx, lc, j))

				after := snapshot(t, pool, j.id)
				want, allowed := tc.allowed[from]
				if allowed {
					assert.Equal(t, want, after["status"], "status after %s from %s", tc.name, from)
					assert.NotEqual(t, before["updated_at"], after["updated_at"], "%s must stamp updated_at", tc.name)
					assert.Equal(t, refusedBefore, refusedCount(t, tc.name), "an allowed transition is not a refusal")
				} else {
					assert.Equal(t, before, after, "%s from %s must change nothing", tc.name, from)
					if tc.counted {
						assert.Equal(t, refusedBefore+1, refusedCount(t, tc.name), "%s from %s must count a refusal", tc.name, from)
					}
				}
			})
		}
	}
}

// TestLifecycleMatrix_AgreesWithTransitionTable keeps the table above and the
// lifecycle's own Transition table (which the SQL is built from) one statement.
func TestLifecycleMatrix_AgreesWithTransitionTable(t *testing.T) {
	byName := map[string]tcase{}
	for _, tc := range cases {
		byName[tc.name] = tc
	}
	for _, tr := range dispatchjob.Transitions() {
		if tr.Name == "create" {
			continue // an insert has no from-status; see TestCreate*
		}
		tc, ok := byName[tr.Name]
		require.True(t, ok, "transition %q has no row in the matrix", tr.Name)
		from := map[string]bool{}
		if tr.From == nil {
			for _, s := range allStatuses {
				from[s] = true
			}
		}
		for _, s := range tr.From {
			from[string(s)] = true
		}
		assert.Len(t, tc.allowed, len(from), "%s: from-set size", tr.Name)
		for s, res := range tc.allowed {
			assert.True(t, from[s], "%s: matrix allows from %s but the lifecycle does not", tr.Name, s)
			assert.Equal(t, string(tr.To), res, "%s: resulting status", tr.Name)
		}
	}
	assert.Len(t, byName, len(dispatchjob.Transitions())-1)
}

// A late callback never resurrects or overwrites a settled job, the retry
// budget survives (attempt_count is not bumped by a refused retry), and the
// stamped columns of an allowed outcome are as before.
func TestLifecycleCallbackOutcomeColumns(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)
	_, err := pool.Exec(ctx, `DELETE FROM msg_dispatch_jobs`)
	require.NoError(t, err)

	j := seed(t, pool, newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1), "PROCESSING")
	msg := "nope"
	ok, err := lc.Retry(ctx, j.id, j.createdAt, time.Now().Add(time.Minute), &msg)
	require.NoError(t, err)
	require.True(t, ok)
	s := snapshot(t, pool, j.id)
	assert.Equal(t, "PENDING", s["status"])
	assert.EqualValues(t, 1, s["attempt_count"])
	assert.Equal(t, "nope", s["last_error"])
	assert.NotNil(t, s["scheduled_for"])
	assert.NotNil(t, s["last_attempt_at"])

	ok, err = lc.Complete(ctx, j.id, j.createdAt, 5)
	require.NoError(t, err)
	require.True(t, ok, "PENDING -> COMPLETED: a terminal outcome may find the job PENDING")
	s = snapshot(t, pool, j.id)
	assert.Equal(t, "COMPLETED", s["status"])
	assert.EqualValues(t, 5, s["duration_millis"])
	assert.NotNil(t, s["completed_at"])

	// Settled now: a late retry / fail / defer leaves it COMPLETED.
	for name, f := range map[string]func() (bool, error){
		"retry": func() (bool, error) { return lc.Retry(ctx, j.id, j.createdAt, time.Now(), &msg) },
		"fail":  func() (bool, error) { return lc.Fail(ctx, j.id, j.createdAt, &msg, 1) },
		"defer": func() (bool, error) { return lc.Defer(ctx, j.id, j.createdAt, time.Now()) },
		"hold":  func() (bool, error) { return lc.Hold(ctx, j.id, j.createdAt, time.Now()) },
	} {
		ok, err := f()
		require.NoError(t, err, name)
		assert.False(t, ok, name)
	}
	s = snapshot(t, pool, j.id)
	assert.Equal(t, "COMPLETED", s["status"])
	assert.EqualValues(t, 1, s["attempt_count"])
}

// A job is born PENDING whatever the entity says; a duplicate id is skipped
// and counted.
func TestLifecycleCreate(t *testing.T) {
	pool := testpg.Pool(t)
	ctx := context.Background()
	lc := dispatchjob.NewLifecycle(pool)

	j := newJob("g-"+tsid.GenerateUntyped(), common.DispatchNextOnError, 1)
	j.Status = common.DispatchCompleted
	rows, err := lc.CreateBatch(ctx, []dispatchjob.DispatchJob{j})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "PENDING", rows[0].Status)
	assert.Equal(t, "PENDING", snapshot(t, pool, j.ID)["status"])

	before := refusedCount(t, "create")
	rows, err = lc.CreateBatch(ctx, []dispatchjob.DispatchJob{j})
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Equal(t, before+1, refusedCount(t, "create"))

	fo := dispatchjob.FanOutJob{
		ID: tsid.GenerateUntyped(), Code: "lifecycle:fanout", Source: "s", EventID: "ev",
		TargetURL: "http://example.invalid", Payload: "{}", SubscriptionID: "sub",
		Mode: "IMMEDIATE", Sequence: 99, TimeoutSeconds: 30, MaxRetries: 3,
		IdempotencyKey: "k", CreatedAt: time.Now().UTC().Truncate(time.Microsecond),
	}
	rows, err = lc.CreateFanOut(ctx, []dispatchjob.FanOutJob{fo})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "PENDING", rows[0].Status)
	assert.Equal(t, "PENDING", snapshot(t, pool, fo.ID)["status"])
}
