//go:build integration

package ingestion_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/client"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/tsid"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecasepgx"
)

// pollAs polls as a runtime that declares builtin (nil: the v2 baseline) and
// reports the extension kinds.
func (e *env) pollAs(t *testing.T, runtime string, now time.Time, builtin []string, kinds ...ingestion.RuntimeKind) []string {
	t.Helper()
	req := e.pollRequest(runtime, now, kinds)
	req.BuiltinKinds = builtin
	res, err := e.repo.Poll(context.Background(), req)
	require.NoError(t, err)
	return given(res)
}

// The fair share is per kind (C2): a mixed fleet where one runtime alone
// carries a kind takes all of that kind, whatever the others hold of other
// kinds. Before, the share was counted over every polled ingestion and every
// live runtime, so the only carrier of a kind was capped below what the kind
// needed and the rest starved.
func TestFairShare_PerKindInAMixedFleet(t *testing.T) {
	e := newEnv(t)
	tf := day(20)
	kind := "mix" + unique()
	var files, feeds []string
	for n := range 2 {
		files = append(files, e.live(t, fmt.Sprintf("file-%d", n), e.sftp(t)).ID)
	}
	for n := range 6 {
		feeds = append(feeds, e.live(t, fmt.Sprintf("feed-%d", n), e.custom(t, kind)).ID)
	}
	mix := ingestion.RuntimeKind{Kind: kind}
	sftpOnly, mixOnly, both := "fair-sftp-"+unique(), "fair-mix-"+unique(), "fair-both-"+unique()

	// Two runtimes, one kind each: each holds all of its kind. (The old share,
	// ceil(8/2) = 4, left two feeds with no holder.)
	e.pollAs(t, sftpOnly, tf, []string{"sftp"})
	c := e.pollAs(t, mixOnly, tf.Add(time.Second), []string{}, mix)
	a := e.pollAs(t, sftpOnly, tf.Add(2*time.Second), []string{"sftp"})
	assert.ElementsMatch(t, files, a)
	assert.ElementsMatch(t, feeds, c, "the only runtime that carries the kind runs all of it")

	// A runtime that carries both joins: each kind is split over its carriers,
	// mix over two (3 each) and sftp over two (1 each), once the surplus
	// leases have run out.
	// Polls a minute apart, as runtimes poll, so all three stay live.
	held := map[string][]string{}
	now := tf.Add(time.Minute)
	for range 10 {
		held[sftpOnly] = e.pollAs(t, sftpOnly, now, []string{"sftp"})
		held[mixOnly] = e.pollAs(t, mixOnly, now.Add(time.Second), []string{}, mix)
		held[both] = e.pollAs(t, both, now.Add(2*time.Second), nil, mix)
		now = now.Add(time.Minute)
	}
	var all []string
	for _, ids := range held {
		all = append(all, ids...)
	}
	assert.ElementsMatch(t, append(append([]string{}, files...), feeds...), all, "every ingestion held by exactly one runtime")
	assert.Len(t, held[sftpOnly], 1, "sftp over two carriers")
	assert.Len(t, held[mixOnly], 3, "mix over two carriers")
	assert.Len(t, held[both], 4, "one sftp and three mix")
	assert.Empty(t, intersect(held[sftpOnly], feeds), "a runtime never holds a kind it does not carry")
}

// A runtime that declares builtinKinds (C1) is listed only ingestions of those
// kinds and of its extension kinds.
func TestPoll_BuiltinKindsDecideWhatIsListed(t *testing.T) {
	e := newEnv(t)
	tf := day(21)
	files := e.live(t, "files", e.sftp(t))
	queue := e.live(t, "queue", sqsDef(t, "Queue", "q-builtin"))
	assert.ElementsMatch(t, []string{queue.ID}, e.pollAs(t, "rt-sqs-"+unique(), tf, []string{"sqs"}))
	assert.Empty(t, e.pollAs(t, "rt-none-"+unique(), tf, []string{}), "an extension-only runtime runs no built-in kind")
	assert.ElementsMatch(t, []string{files.ID, queue.ID}, e.pollAs(t, "rt-all-"+unique(), tf, nil), "undeclared: the v2 baseline")
}

// Concurrent polls of runtimes that each hold leases and claim others take the
// lease section one at a time (C2): before, two polls renewing and claiming in
// different row orders could deadlock and fail.
func TestPoll_ConcurrentPollsDoNotDeadlock(t *testing.T) {
	e := newEnv(t)
	tf := day(22)
	var ids []string
	for n := range 16 {
		ids = append(ids, e.live(t, fmt.Sprintf("dl-%02d", n), e.sftp(t)).ID)
	}
	runtimes := []string{"dl-a-" + unique(), "dl-b-" + unique(), "dl-c-" + unique(), "dl-d-" + unique(), "dl-e-" + unique()}
	now := tf
	var last [][]string
	for round := range 8 {
		start := make(chan struct{})
		results := make([][]string, len(runtimes))
		errs := make([]error, len(runtimes))
		var wg sync.WaitGroup
		for n, rt := range runtimes {
			wg.Go(func() {
				<-start
				res, err := e.repo.Poll(context.Background(), e.pollRequest(rt, now.Add(time.Duration(n)*time.Millisecond), nil))
				errs[n] = err
				if err == nil {
					results[n] = given(res)
				}
			})
		}
		close(start)
		wg.Wait()
		for n, err := range errs {
			require.NoError(t, err, "round %d, runtime %d", round, n)
		}
		last = results
		// Half a lease, then past it: renewals, releases and claims of expired
		// leases all interleave.
		if round%2 == 0 {
			now = now.Add(lease / 2)
		} else {
			now = now.Add(lease + time.Second)
		}
	}
	holders := map[string]int{}
	for _, held := range last {
		for _, id := range held {
			holders[id]++
		}
	}
	for id, n := range holders {
		assert.Equal(t, 1, n, "ingestion %s held once", id)
	}
	assert.LessOrEqual(t, len(holders), len(ids))
}

// The deadlock the lease lock removes, set up on purpose: two live runtimes
// each hold one lease and have room for more. Each poll renews its own lease
// (locking that row) and then tries the others in id order, which includes the
// other runtime's row; without one poll at a time, A waits on B's row while B
// waits on A's, and Postgres aborts one of them.
func TestPoll_RenewThenClaimDoesNotDeadlock(t *testing.T) {
	ctx := context.Background()
	for attempt := range 12 {
		e := newEnv(t)
		now := day(30 + attempt)
		var ids []string
		for n := range 4 {
			ids = append(ids, e.live(t, fmt.Sprintf("x-%d", n), e.sftp(t)).ID)
		}
		a, b := "dlx-a-"+unique(), "dlx-b-"+unique()
		exec := func(q string, args ...any) {
			t.Helper()
			_, err := e.pool.Exec(ctx, q, args...)
			require.NoError(t, err)
		}
		for _, rt := range []string{a, b} {
			exec(`INSERT INTO ing_runtimes (runtime_id, contract_version, first_seen_at, last_seen_at) VALUES ($1, 2, $2, $2)`, rt, now.Add(-10*time.Second))
		}
		// b holds the lower id, a the higher: each renews its own, then reaches
		// the other's in id order.
		exec(`INSERT INTO ing_leases (ingestion_id, runtime_id, acquired_at, expires_at) VALUES ($1, $2, $3, $4)`, ids[0], b, now.Add(-time.Minute), now.Add(2*time.Minute))
		exec(`INSERT INTO ing_leases (ingestion_id, runtime_id, acquired_at, expires_at) VALUES ($1, $2, $3, $4)`, ids[1], a, now.Add(-time.Minute), now.Add(2*time.Minute))

		start := make(chan struct{})
		errs := make([]error, 2)
		var wg sync.WaitGroup
		for n, rt := range []string{a, b} {
			wg.Go(func() {
				<-start
				_, errs[n] = e.repo.Poll(ctx, e.pollRequest(rt, now, nil))
			})
		}
		close(start)
		wg.Wait()
		require.NoError(t, errs[0], "attempt %d: a", attempt)
		require.NoError(t, errs[1], "attempt %d: b", attempt)
	}
}

// A poll with no time of its own is timed by the database's clock (C2): the
// lease it takes ends a lease after the database's now, whatever the
// platform's own clock says.
func TestPoll_DatabaseClock(t *testing.T) {
	e := newEnv(t)
	i := e.live(t, "db-clock", e.sftp(t))
	req := e.pollRequest("rt-clock-"+unique(), time.Time{}, nil)
	res, err := e.repo.Poll(context.Background(), req)
	require.NoError(t, err)
	assert.Contains(t, given(res), i.ID)
	assert.WithinDuration(t, time.Now(), res.AsOf, time.Minute, "answered as of the database's now")
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = $1
		AND expires_at BETWEEN now() + interval '170 seconds' AND now() + interval '190 seconds'`, i.ID))
	// State and the lease checks read the same clock.
	require.NoError(t, e.repo.PutState(context.Background(), i.ID, req.RuntimeID, "watermark", "1", time.Time{}))
	v, found, err := e.repo.GetState(context.Background(), i.ID, req.RuntimeID, "watermark", time.Time{})
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "1", v)
}

// A suspended or deactivated client's ingestions are listed to no runtime
// (R6).
func TestPoll_InactiveClientIsNotListed(t *testing.T) {
	e := newEnv(t)
	tf := day(23)
	queue := e.live(t, "queue", sqsDef(t, "Queue", "q-status"))
	assert.Contains(t, e.pollAs(t, "rt-st-"+unique(), tf, nil), queue.ID)
	for _, status := range []client.Status{client.StatusSuspended, client.StatusInactive} {
		e.client.SetStatus(status, nil)
		require.NoError(t, withTx(t, e.uow, func(tx *usecasepgx.DbTx) error {
			return client.NewRepository(e.pool).Persist(context.Background(), e.client, tx)
		}))
		assert.Empty(t, e.pollAs(t, "rt-st-"+unique(), tf, nil), string(status))
	}
}

// The permission a runtime declared for its kind is kept (R4f): it still
// applies once no runtime that reports the kind is live.
func TestRequiredPermission_RememberedWhenNoRuntimeIsLive(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	kind := "guarded" + unique()
	const need = "platform:ingestion:ingestion:guarded"
	// Polled an hour ago: not live now.
	past := time.Now().UTC().Add(-time.Hour)
	e.pollAs(t, "rt-guard-"+unique(), past, nil, ingestion.RuntimeKind{Kind: kind, RequiredPermission: need})
	live, err := e.repo.LiveKinds(ctx)
	require.NoError(t, err)
	for _, k := range live {
		require.NotEqual(t, kind, k.Kind, "the runtime is not live")
	}
	perm, err := e.repo.RequiredPermission(ctx, kind)
	require.NoError(t, err)
	assert.Equal(t, need, perm, "the last report still decides")
	none, err := e.repo.RequiredPermission(ctx, "never-reported-"+unique())
	require.NoError(t, err)
	assert.Empty(t, none)
}

// A report is stored whatever bytes the runtime passes through (C4): NUL
// characters are dropped rather than failing the whole report. A failure's
// category is stored (C3, payload when absent), and a time from the future is
// recorded as now (R5).
func TestReport_NULCategoryAndClamp(t *testing.T) {
	e := newEnv(t)
	keep := sqsDef(t, "Keep", "q-nul")
	keep.KeepFailedPayloads = true
	i := e.live(t, "nul", keep)
	rt := "rt-nul-" + unique()
	future := e.t0.Add(48 * time.Hour)
	e.report(t, ingestion.Report{
		RuntimeID: rt, Now: e.t0,
		Routes:  []ingestion.RouteReport{{IngestionID: i.ID, Version: 1, Status: "FAILING", Error: new("bad\x00byte")}},
		Metrics: []ingestion.WindowReport{{IngestionID: i.ID, From: future, To: future.Add(time.Minute), Completed: 1}},
		Failures: []ingestion.FailureReport{
			{IngestionID: i.ID, Version: 1, At: future, Kind: "REJECTED", Message: "nul\x00here", Excerpt: new("a\x00b")},
			{IngestionID: i.ID, Version: 1, At: e.t0, Kind: "NOT_DISPATCHED", Category: ingestion.FailureRejected, Message: "target refused"},
		},
	})
	page, err := e.repo.ListFailures(context.Background(), i.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, page.Items, 2)
	byKind := map[string]ingestion.Failure{}
	for _, f := range page.Items {
		byKind[f.Kind] = f
	}
	payload := byKind["REJECTED"]
	assert.Equal(t, "nulhere", payload.Message)
	assert.Equal(t, "ab", *payload.Excerpt)
	assert.Equal(t, ingestion.FailurePayload, payload.Category, "absent is payload")
	assert.True(t, payload.OccurredAt.Equal(e.t0), "a future time is recorded as now: %s", payload.OccurredAt)
	assert.Equal(t, ingestion.FailureRejected, byKind["NOT_DISPATCHED"].Category)
	assert.True(t, byKind["NOT_DISPATCHED"].Notice())
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_route_states WHERE ingestion_id = $1 AND error = 'badbyte'`, i.ID))
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_exchange_metrics WHERE ingestion_id = $1 AND window_start = $2 AND created_at = $2`, i.ID, e.t0),
		"a window from the future starts now")

	notices, err := e.repo.NoticesOf(context.Background(), []string{i.ID})
	require.NoError(t, err)
	assert.Equal(t, ingestion.Notices{Count: 1, LatestMessage: "target refused", LatestAt: e.t0}, normalise(notices[i.ID]))
}

// normalise drops the location of a time read back from Postgres.
func normalise(n ingestion.Notices) ingestion.Notices {
	n.LatestAt = n.LatestAt.UTC()
	return n
}

// Failures filter by category and by being undismissed; a dismissal is
// stored once.
func TestFailures_FilterAndDismiss(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	i := e.live(t, "dismiss", sqsDef(t, "Dismiss", "q-dismiss"))
	rt := "rt-dis-" + unique()
	e.report(t, ingestion.Report{RuntimeID: rt, Now: e.t0, Failures: []ingestion.FailureReport{
		{IngestionID: i.ID, Version: 1, At: e.t0, Kind: "SOURCE", Message: "timeout"},
		{IngestionID: i.ID, Version: 1, At: e.t0, Kind: "NOT_DISPATCHED", Category: ingestion.FailureRejected, Message: "refused"},
	}})
	rejected, err := e.repo.ListFailuresFiltered(ctx, i.ID, ingestion.FailureFilter{Category: ingestion.FailureRejected, Undismissed: true})
	require.NoError(t, err)
	require.Len(t, rejected.Items, 1)
	f := rejected.Items[0]

	f.Dismiss("prn_dismisser0001", e.t0)
	require.NoError(t, withTx(t, e.uow, func(tx *usecasepgx.DbTx) error { return e.repo.Failures().Persist(ctx, &f, tx) }))
	again := withTx(t, e.uow, func(tx *usecasepgx.DbTx) error { return e.repo.Failures().Persist(ctx, &f, tx) })
	assert.Equal(t, "INGESTION_FAILURE_DISMISSED", conflictCode(t, again), "dismissed once")

	open, err := e.repo.ListFailuresFiltered(ctx, i.ID, ingestion.FailureFilter{Undismissed: true})
	require.NoError(t, err)
	require.Len(t, open.Items, 1)
	assert.Equal(t, "SOURCE", open.Items[0].Kind)
	got, err := e.repo.FindFailure(ctx, i.ID, f.ID)
	require.NoError(t, err)
	require.NotNil(t, got.DismissedAt)
	assert.Equal(t, "prn_dismisser0001", *got.DismissedBy)
	notices, err := e.repo.NoticesOf(ctx, []string{i.ID})
	require.NoError(t, err)
	assert.Empty(t, notices, "a dismissed notice no longer shows")
	missing, err := e.repo.FindFailure(ctx, i.ID, tsid.Generate(tsid.IngestionFailure))
	require.NoError(t, err)
	assert.Nil(t, missing)
}
