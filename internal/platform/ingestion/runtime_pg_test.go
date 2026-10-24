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

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/auth"
)

// The runtime side of the repository: polls, leases, state, reports and test
// requests. Every poll is confined to the test's own client, so tests share
// the database without taking each other's leases.

// Every test environment polls on a day of its own (env.e.t0): the runtime table is
// global and fair share counts the runtimes seen within a lease window, so runtimes of
// other tests must not fall into this test's window.
var tBase = time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC)

const lease = 3 * time.Minute

func (e *env) sftp(t *testing.T) ingestion.Definition {
	t.Helper()
	return parse(t, definition(sourceCases[3].source, sourceCases[3].split))
}

func (e *env) custom(t *testing.T, kind string) ingestion.Definition {
	t.Helper()
	return parse(t, definition(`{"kind":"custom","type":"`+kind+`","settings":{}}`, `{"kind":"none"}`))
}

// live saves a live ingestion with the definition and returns it.
func (e *env) live(t *testing.T, code string, def ingestion.Definition) *ingestion.Ingestion {
	t.Helper()
	i := e.newIngestion(code, def, true)
	e.mustSave(t, i)
	return i
}

func (e *env) poll(t *testing.T, runtime string, now time.Time, kinds ...ingestion.RuntimeKind) *ingestion.PollResult {
	t.Helper()
	res, err := e.repo.Poll(context.Background(), e.pollRequest(runtime, now, kinds))
	require.NoError(t, err)
	return res
}

func (e *env) pollRequest(runtime string, now time.Time, kinds []ingestion.RuntimeKind) ingestion.PollRequest {
	own := []string{string(e.client.ID)}
	return ingestion.PollRequest{
		RuntimeID: runtime, ContractVersion: ingestion.ContractVersion, Kinds: kinds,
		Clients: &own, Now: now, Lease: lease,
	}
}

func given(res *ingestion.PollResult) []string {
	out := make([]string, 0, len(res.Ingestions))
	for _, i := range res.Ingestions {
		out = append(out, i.ID)
	}
	return out
}

func (e *env) report(t *testing.T, rep ingestion.Report) ingestion.ReportSummary {
	t.Helper()
	if rep.Caller == nil {
		rep.Caller = &auth.AuthContext{Scope: auth.ScopeAnchor, AllApplications: true}
	}
	sum, err := e.repo.Record(context.Background(), rep)
	require.NoError(t, err)
	return sum
}

func (e *env) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	require.NoError(t, e.pool.QueryRow(context.Background(), query, args...).Scan(&n))
	return n
}

// A shared source (queue, endpoint) goes to every runtime; a paused ingestion
// and one with no live version go to none.
func TestPoll_SharedGoesToEveryRuntime(t *testing.T) {
	e := newEnv(t)
	sqs := e.live(t, "shared", sqsDef(t, "Shared", "q-shared"))
	paused := e.live(t, "paused", sqsDef(t, "Paused", "q-paused"))
	paused = e.mustFind(t, paused.ID)
	paused.Pause()
	e.mustSave(t, paused)
	e.mustSave(t, e.newIngestion("draft", sqsDef(t, "Draft", "q-draft"), false))

	a, b := "rt-a-"+unique(), "rt-b-"+unique()
	assert.Equal(t, []string{sqs.ID}, given(e.poll(t, a, e.t0)))
	assert.Equal(t, []string{sqs.ID}, given(e.poll(t, b, e.t0)))

	got := e.poll(t, a, e.t0).Ingestions[0]
	assert.Equal(t, e.app.Code, got.ApplicationCode)
	assert.Equal(t, e.client.Identifier, got.ClientIdentifier)
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = $1`, sqs.ID), "a shared source has no lease")
}

// Runtimes polling together for the same polled sources: each source goes to
// exactly one of them.
func TestPoll_LeaseRaceExactlyOneWins(t *testing.T) {
	e := newEnv(t)
	var ids []string
	for n := range 10 {
		ids = append(ids, e.live(t, "race-"+string(rune('a'+n)), e.sftp(t)).ID)
	}
	runtimes := []string{"rt-1-" + unique(), "rt-2-" + unique(), "rt-3-" + unique(), "rt-4-" + unique()}

	start := make(chan struct{})
	results := make([]*ingestion.PollResult, len(runtimes))
	var wg sync.WaitGroup
	for n, rt := range runtimes {
		wg.Go(func() {
			<-start
			res, err := e.repo.Poll(context.Background(), e.pollRequest(rt, e.t0, nil))
			assert.NoError(t, err)
			results[n] = res
		})
	}
	close(start)
	wg.Wait()

	holders := map[string]int{}
	for _, res := range results {
		require.NotNil(t, res)
		for _, id := range given(res) {
			holders[id]++
		}
	}
	for _, id := range ids {
		assert.Equal(t, 1, holders[id], "ingestion %s must be held by exactly one runtime", id)
	}
	assert.Equal(t, len(ids), e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = ANY($1)`, ids))
}

// The holder renews by polling; another runtime gets the source only once the
// lease has run out.
func TestLease_RenewalAndExpiryHandover(t *testing.T) {
	e := newEnv(t)
	i := e.live(t, "handover", e.sftp(t))
	a, b := "rt-a-"+unique(), "rt-b-"+unique()

	assert.Equal(t, []string{i.ID}, given(e.poll(t, a, e.t0)), "a takes the lease")
	assert.Empty(t, given(e.poll(t, b, e.t0.Add(time.Minute))), "b waits while a holds it")
	assert.Equal(t, []string{i.ID}, given(e.poll(t, a, e.t0.Add(2*time.Minute))), "a renews (lease now ends at +5m)")
	assert.Empty(t, given(e.poll(t, b, e.t0.Add(4*time.Minute))), "without the renewal it would have gone to b by now")
	assert.Equal(t, []string{i.ID}, given(e.poll(t, b, e.t0.Add(6*time.Minute))), "a stopped renewing: b takes it after +5m")
	assert.Empty(t, given(e.poll(t, a, e.t0.Add(7*time.Minute))), "a does not get it back while b holds it")
}

// A runtime that stops getting an ingestion marks its lease released; nobody
// else takes it before it expires.
func TestLease_ReleasedWhenNoLongerRunnable(t *testing.T) {
	e := newEnv(t)
	i := e.live(t, "released", e.sftp(t))
	a, b := "rt-a-"+unique(), "rt-b-"+unique()
	require.Equal(t, []string{i.ID}, given(e.poll(t, a, e.t0)))

	i = e.mustFind(t, i.ID)
	i.Pause()
	e.mustSave(t, i)
	assert.Empty(t, given(e.poll(t, a, e.t0.Add(time.Minute))))
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = $1 AND runtime_id = $2 AND released`, i.ID, a))

	i = e.mustFind(t, i.ID)
	i.Resume()
	e.mustSave(t, i)
	assert.Empty(t, given(e.poll(t, b, e.t0.Add(2*time.Minute))), "a released lease is not handed over before it expires")
	assert.Equal(t, []string{i.ID}, given(e.poll(t, b, e.t0.Add(4*time.Minute))))
}

// A runtime never gets an ingestion whose kind it does not carry, and the
// kinds it names decide what it carries.
func TestPoll_KindCapabilityFiltering(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	kind := "mix" + unique()
	custom := e.live(t, "custom", e.custom(t, kind))
	sftp := e.live(t, "plain", e.sftp(t))
	sqs := e.live(t, "queue", sqsDef(t, "Queue", "q-kind"))

	plain, ext, limited := "rt-plain-"+unique(), "rt-ext-"+unique(), "rt-limited-"+unique()
	assert.ElementsMatch(t, []string{sftp.ID, sqs.ID}, given(e.poll(t, plain, e.t0)), "no extension: the built-ins, not the custom kind")
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = $1`, custom.ID), "a lease is never taken for a kind nobody carries")

	carries, err := e.repo.AnyLiveRuntimeCarries(ctx)
	require.NoError(t, err)
	assert.False(t, carries(kind), "no runtime carries this kind yet")

	// The extension runtime takes the custom kind and the shared queue; sftp is still held by plain.
	assert.ElementsMatch(t, []string{custom.ID, sqs.ID}, given(e.poll(t, ext, e.t0, ingestion.RuntimeKind{Kind: kind})))
	carries, err = e.repo.AnyLiveRuntimeCarries(ctx)
	require.NoError(t, err)
	assert.True(t, carries(kind))

	// A runtime that names built-in kinds carries exactly those.
	assert.Equal(t, []string{sqs.ID}, given(e.poll(t, limited, e.t0, ingestion.RuntimeKind{Kind: "sqs"})))
}

// Polled-source state belongs to the lease holder: another runtime cannot
// write or read it, nor can a holder whose lease has run out; the value stays
// for the next holder.
func TestState_Fencing(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	i := e.live(t, "state", e.sftp(t))
	a, b := "rt-a-"+unique(), "rt-b-"+unique()
	require.Equal(t, []string{i.ID}, given(e.poll(t, a, e.t0)))

	require.NoError(t, e.repo.PutState(ctx, i.ID, a, "watermark", "2030-01-01T00:00:00Z", e.t0.Add(time.Minute)))
	require.NoError(t, e.repo.PutState(ctx, i.ID, a, "watermark", "2030-01-02T00:00:00Z", e.t0.Add(time.Minute)), "a write replaces")
	v, found, err := e.repo.GetState(ctx, i.ID, a, "watermark", e.t0.Add(time.Minute))
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "2030-01-02T00:00:00Z", v)
	_, found, err = e.repo.GetState(ctx, i.ID, a, "other", e.t0.Add(time.Minute))
	require.NoError(t, err)
	assert.False(t, found)

	assert.Equal(t, "LEASE_NOT_HELD", conflictCode(t, e.repo.PutState(ctx, i.ID, b, "watermark", "stolen", e.t0.Add(time.Minute))))
	_, _, err = e.repo.GetState(ctx, i.ID, b, "watermark", e.t0.Add(time.Minute))
	assert.Equal(t, "LEASE_NOT_HELD", conflictCode(t, err))
	assert.Equal(t, "LEASE_NOT_HELD", conflictCode(t, e.repo.PutState(ctx, i.ID, a, "watermark", "late", e.t0.Add(4*time.Minute))), "an expired lease holds nothing")

	require.Equal(t, []string{i.ID}, given(e.poll(t, b, e.t0.Add(4*time.Minute))))
	v, found, err = e.repo.GetState(ctx, i.ID, b, "watermark", e.t0.Add(4*time.Minute))
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, "2030-01-02T00:00:00Z", v, "the value survives the ingestion moving to another runtime")
}

// A window re-sent replaces the counts: the same window twice is one row with
// the second counts; another runtime's window is its own row.
func TestReport_WindowIsIdempotent(t *testing.T) {
	e := newEnv(t)
	i := e.live(t, "metrics", sqsDef(t, "Metrics", "q-metrics"))
	a, b := "rt-a-"+unique(), "rt-b-"+unique()
	from := e.t0.Add(-time.Minute)
	win := func(completed int64) []ingestion.WindowReport {
		return []ingestion.WindowReport{{IngestionID: i.ID, From: from, To: e.t0, Completed: completed, Failed: 1, Records: completed * 2, Dispatch: completed * 2}}
	}

	e.report(t, ingestion.Report{RuntimeID: a, Metrics: win(10), Now: e.t0})
	e.report(t, ingestion.Report{RuntimeID: a, Metrics: win(10), Now: e.t0.Add(time.Second)})
	e.report(t, ingestion.Report{RuntimeID: a, Metrics: win(12), Now: e.t0.Add(2 * time.Second)})
	e.report(t, ingestion.Report{RuntimeID: b, Metrics: win(5), Now: e.t0})

	var rows, completed, records, secs int
	require.NoError(t, e.pool.QueryRow(context.Background(),
		`SELECT count(*), sum(completed), sum(records), max(window_secs) FROM ing_exchange_metrics WHERE ingestion_id = $1 AND runtime_id = $2`,
		i.ID, a).Scan(&rows, &completed, &records, &secs))
	assert.Equal(t, 1, rows)
	assert.Equal(t, 12, completed, "the later send replaced the counts, nothing was added")
	assert.Equal(t, 24, records)
	assert.Equal(t, 60, secs)
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_exchange_metrics WHERE ingestion_id = $1 AND runtime_id = $2`, i.ID, b))
}

// A report about a polled source the runtime does not hold is ignored; shared
// sources have no lease and are recorded.
func TestReport_LeaseRule(t *testing.T) {
	e := newEnv(t)
	polled := e.live(t, "polled", e.sftp(t))
	shared := e.live(t, "shared", sqsDef(t, "Shared", "q-report"))
	holder, other := "rt-h-"+unique(), "rt-o-"+unique()
	require.Equal(t, []string{polled.ID}, given(e.poll(t, holder, e.t0, ingestion.RuntimeKind{Kind: "sftp"})))

	about := func(rt string) ingestion.Report {
		return ingestion.Report{
			RuntimeID: rt, Now: e.t0.Add(time.Second),
			Routes: []ingestion.RouteReport{
				{IngestionID: polled.ID, Version: 1, Status: "STARTED"},
				{IngestionID: shared.ID, Version: 1, Status: "FAILING", Error: new("login refused")},
				{IngestionID: "ing_missing000", Version: 1, Status: "STARTED"},
			},
			Failures: []ingestion.FailureReport{{IngestionID: polled.ID, Version: 1, At: e.t0, Kind: "SOURCE", Message: "boom"}},
		}
	}
	sum := e.report(t, about(other))
	assert.Equal(t, ingestion.ReportSummary{Accepted: 1, Ignored: 3}, sum, "only the shared route is recorded")
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM ing_route_states WHERE ingestion_id = $1`, polled.ID))
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM ing_failures WHERE ingestion_id = $1`, polled.ID))
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_route_states WHERE ingestion_id = $1 AND runtime_id = $2 AND status = 'FAILING'`, shared.ID, other))

	sum = e.report(t, about(holder))
	assert.Equal(t, ingestion.ReportSummary{Accepted: 3, Ignored: 1}, sum)
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_route_states WHERE ingestion_id = $1 AND runtime_id = $2`, polled.ID, holder))
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_failures WHERE ingestion_id = $1 AND runtime_id = $2`, polled.ID, holder))
}

// A failure carries the start of a payload only when the ingestion keeps
// failed payloads.
func TestReport_FailureExcerptRule(t *testing.T) {
	e := newEnv(t)
	plainDef := sqsDef(t, "Plain", "q-plain")
	keepDef := sqsDef(t, "Keep", "q-keep")
	keepDef.KeepFailedPayloads = true
	plain := e.live(t, "plain", plainDef)
	keep := e.live(t, "keep", keepDef)

	fail := func(id string) ingestion.FailureReport {
		return ingestion.FailureReport{
			IngestionID: id, Version: 1, At: e.t0, Kind: "REJECTED", Message: "not json",
			AfterRecords: new(int64(3)), Excerpt: new("id,name\n1,Ann"),
		}
	}
	e.report(t, ingestion.Report{RuntimeID: "rt-" + unique(), Now: e.t0, Failures: []ingestion.FailureReport{fail(plain.ID), fail(keep.ID)}})

	page, err := e.repo.ListFailures(context.Background(), plain.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	assert.Nil(t, page.Items[0].Excerpt, "no payload kept without the client's agreement")
	assert.Equal(t, int64(3), page.Items[0].AfterRecords)
	assert.Equal(t, "REJECTED", page.Items[0].Kind)

	page, err = e.repo.ListFailures(context.Background(), keep.ID, "", 10)
	require.NoError(t, err)
	require.Len(t, page.Items, 1)
	require.NotNil(t, page.Items[0].Excerpt)
	assert.Equal(t, "id,name\n1,Ann", *page.Items[0].Excerpt)
}

// A test request is taken once, by a runtime that carries its kind, and
// answered by that runtime; a request nobody answers in time reads as EXPIRED.
func TestTestRequest_Lifecycle(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	kind := "wiq" + unique()
	def := e.custom(t, kind)
	queued, err := e.repo.QueueTest(ctx, nil, e.app.ID, e.client.ID, def, nil, e.t0)
	require.NoError(t, err)
	assert.Equal(t, kind, queued.Kind)
	assert.Equal(t, ingestion.TestPending, queued.EffectiveStatus(e.t0))

	assert.Empty(t, e.poll(t, "rt-none-"+unique(), e.t0.Add(time.Second)).Tests, "a runtime that does not carry the kind is not asked")
	ext, other := "rt-ext-"+unique(), "rt-other-"+unique()
	res := e.poll(t, ext, e.t0.Add(time.Second), ingestion.RuntimeKind{Kind: kind})
	require.Len(t, res.Tests, 1)
	assert.Equal(t, queued.ID, res.Tests[0].ID)
	assert.Equal(t, e.app.Code, res.Tests[0].ApplicationCode)
	assert.Equal(t, e.client.Identifier, res.Tests[0].ClientIdentifier)
	assert.Empty(t, e.poll(t, other, e.t0.Add(2*time.Second), ingestion.RuntimeKind{Kind: kind}).Tests, "taken once")
	assert.Equal(t, ingestion.TestRunning, e.mustFindTest(t, queued.ID).EffectiveStatus(e.t0.Add(time.Minute)))

	answer := ingestion.TestAnswer{TestID: queued.ID, Result: ingestion.TestResult{
		Outcome: "RESULTS", Steps: []string{"logged in", "asked once"}, Records: new(int64(7)), Excerpt: new("{\"secret\":1}"),
	}}
	e.report(t, ingestion.Report{RuntimeID: other, Now: e.t0.Add(10 * time.Second), Tests: []ingestion.TestAnswer{answer}})
	assert.Equal(t, ingestion.TestRunning, e.mustFindTest(t, queued.ID).Status, "only the runtime that took it answers it")

	e.report(t, ingestion.Report{RuntimeID: ext, Now: e.t0.Add(20 * time.Second), Tests: []ingestion.TestAnswer{answer}})
	done := e.mustFindTest(t, queued.ID)
	assert.Equal(t, ingestion.TestDone, done.EffectiveStatus(e.t0.Add(time.Hour)), "an answered request does not expire")
	require.NotNil(t, done.Result)
	assert.Equal(t, "RESULTS", done.Result.Outcome)
	assert.Equal(t, []string{"logged in", "asked once"}, done.Result.Steps)
	assert.Equal(t, int64(7), *done.Result.Records)
	assert.Nil(t, done.Result.Excerpt, "the definition does not keep payloads, so the excerpt is dropped")
	assert.NotNil(t, done.CompletedAt)

	// Nobody answers in time: EXPIRED, never claimed after, never answered after.
	late, err := e.repo.QueueTest(ctx, nil, e.app.ID, e.client.ID, def, nil, e.t0)
	require.NoError(t, err)
	pastExpiry := e.t0.Add(ingestion.TestRequestTTL)
	assert.Equal(t, ingestion.TestExpired, late.EffectiveStatus(pastExpiry))
	assert.Empty(t, e.poll(t, ext, pastExpiry, ingestion.RuntimeKind{Kind: kind}).Tests)
	e.report(t, ingestion.Report{RuntimeID: ext, Now: pastExpiry, Tests: []ingestion.TestAnswer{{TestID: late.ID, Result: answer.Result}}})
	assert.Equal(t, ingestion.TestExpired, e.mustFindTest(t, late.ID).EffectiveStatus(pastExpiry))
	assert.Nil(t, e.mustFindTest(t, late.ID).Result)
}

// A runtime takes a request that fell to it while it was running, then
// loses it if it does not answer: a request taken but unanswered expires too.
func TestTestRequest_ExpiresWhenTakenButUnanswered(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	kind := "wiq" + unique()
	queued, err := e.repo.QueueTest(ctx, nil, e.app.ID, e.client.ID, e.custom(t, kind), nil, e.t0)
	require.NoError(t, err)
	rt := "rt-" + unique()
	require.Len(t, e.poll(t, rt, e.t0.Add(time.Second), ingestion.RuntimeKind{Kind: kind}).Tests, 1)
	pastExpiry := e.t0.Add(ingestion.TestRequestTTL + time.Second)
	e.report(t, ingestion.Report{RuntimeID: rt, Now: pastExpiry, Tests: []ingestion.TestAnswer{{
		TestID: queued.ID, Result: ingestion.TestResult{Outcome: "FAILED", Steps: []string{"too late"}},
	}}})
	got := e.mustFindTest(t, queued.ID)
	assert.Equal(t, ingestion.TestExpired, got.EffectiveStatus(pastExpiry))
	assert.Nil(t, got.Result)
}

func (e *env) mustFindTest(t *testing.T, id string) *ingestion.TestRequest {
	t.Helper()
	got, err := e.repo.FindTest(context.Background(), id)
	require.NoError(t, err)
	require.NotNil(t, got, "test request %s not found", id)
	return got
}

// Pruning deletes what is past retention and keeps what is not.
func TestPrune(t *testing.T) {
	ctx := context.Background()
	e := newEnv(t)
	now := time.Now().UTC()
	old, fresh := now.Add(-40*24*time.Hour), now.Add(-time.Hour)
	rt := "rt-prune-" + unique()
	ing := e.live(t, "prune", e.sftp(t))

	exec := func(q string, args ...any) {
		t.Helper()
		_, err := e.pool.Exec(ctx, q, args...)
		require.NoError(t, err)
	}
	for _, at := range []time.Time{old, fresh} {
		exec(`INSERT INTO ing_exchange_metrics (ingestion_id, runtime_id, window_start, window_secs) VALUES ($1, $2, $3, 60)`, ing.ID, rt, at)
		exec(`INSERT INTO ing_failures (id, ingestion_id, runtime_id, kind, message, occurred_at, created_at) VALUES ($1, $2, $3, 'SOURCE', 'x', $4, $4)`,
			"ingf"+unique()[:12], ing.ID, rt, at)
	}
	for _, at := range []time.Time{now.Add(-48 * time.Hour), now.Add(-time.Hour)} {
		exec(`INSERT INTO ing_test_requests (id, application_id, client_id, kind, definition, status, created_at, expires_at)
			VALUES ($1, $2, $3, 'sftp', '{}', 'PENDING', $4, $4)`, "ingt"+unique()[:12], e.app.ID, e.client.ID, at)
	}
	for k, at := range []time.Time{now.Add(-8 * 24 * time.Hour), now.Add(-24 * time.Hour)} {
		exec(`INSERT INTO ing_job_keys (ingestion_id, external_id, job_id, created_at) VALUES ($1, $2, $3, $4)`, ing.ID, fmt.Sprintf("k%d", k), "job"+unique()[:8], at)
	}
	// An expired lease on its own ingestion, and a live one; runtimes seen long ago and lately.
	live := e.live(t, "prune-live", e.sftp(t))
	exec(`INSERT INTO ing_leases (ingestion_id, runtime_id, acquired_at, expires_at) VALUES ($1, $2, $3, $4)`, ing.ID, rt, old, now.Add(-time.Minute))
	exec(`INSERT INTO ing_leases (ingestion_id, runtime_id, acquired_at, expires_at) VALUES ($1, $2, $3, $4)`, live.ID, rt, now, now.Add(time.Minute))
	exec(`INSERT INTO ing_runtimes (runtime_id, contract_version, first_seen_at, last_seen_at) VALUES ($1, 2, $2, $2)`, rt+"-old", now.Add(-8*24*time.Hour))
	exec(`INSERT INTO ing_runtimes (runtime_id, contract_version, first_seen_at, last_seen_at) VALUES ($1, 2, $2, $2)`, rt+"-new", now.Add(-24*time.Hour))

	pruned, err := e.repo.Prune(ctx, now)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, pruned.Metrics, int64(1))
	assert.GreaterOrEqual(t, pruned.Failures, int64(1))
	assert.GreaterOrEqual(t, pruned.TestRequests, int64(1))
	assert.GreaterOrEqual(t, pruned.JobKeys, int64(1))
	assert.GreaterOrEqual(t, pruned.Leases, int64(1))
	assert.GreaterOrEqual(t, pruned.Runtimes, int64(1))

	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_exchange_metrics WHERE runtime_id = $1`, rt), "the recent window stays")
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_failures WHERE runtime_id = $1`, rt))
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_test_requests WHERE client_id = $1`, e.client.ID))
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_job_keys WHERE ingestion_id = $1`, ing.ID), "the key from a day ago stays")
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = $1`, ing.ID), "the expired lease goes")
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_leases WHERE ingestion_id = $1`, live.ID), "the live lease stays")
	assert.Equal(t, 0, e.count(t, `SELECT count(*) FROM ing_runtimes WHERE runtime_id = $1`, rt+"-old"))
	assert.Equal(t, 1, e.count(t, `SELECT count(*) FROM ing_runtimes WHERE runtime_id = $1`, rt+"-new"))
}
