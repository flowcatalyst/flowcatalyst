//go:build integration

// tracelog_pg_test.go covers the trace slice of the control plane
// (docs/function-trace-design.md sections 4 and 5): the ingest endpoint's
// acceptance rule and limits, and the live trace windows in the desired
// document (so a toggle changes the ETag and wakes the long-poll).
package control

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	fncontrol "github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/function/operations"
	"github.com/flowcatalyst/flowcatalyst-go/internal/testpg"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecase"
	"github.com/flowcatalyst/flowcatalyst/pkg/fcsdk/usecaseop"
)

// traceFn seeds an application and a function in its own pool.
func traceFn(t *testing.T, s *State, name ...string) (fn *function.Function, poolName string) {
	t.Helper()
	suffix := strings.Join(name, "")
	pool := testpg.Pool(t)
	poolName = "pool-" + t.Name()
	appID := "app_" + shortID(t) + suffix
	appCode := "traceapp" + shortID(t) + suffix
	seedApplication(t, pool, appID, appCode)
	fn = seedFunction(t, pool, s.Repo, appID, appCode, "fn", func(f *function.Function) { f.Pool = &poolName })
	return fn, poolName
}

// setWindow writes a context row directly.
func setWindow(t *testing.T, s *State, fnID, context_ string, until *time.Time, sample int32) {
	t.Helper()
	c := function.TraceContext{FunctionID: fnID, Context: context_, EnabledUntil: until, SampleRate: sample, UpdatedAt: time.Now().UTC()}
	withRawTx(t, testpg.Pool(t), func(tx pgx.Tx) error {
		return s.Repo.UpsertTraceContextTx(context.Background(), &c, tx)
	})
}

func inFuture(d time.Duration) *time.Time { t := time.Now().Add(d); return &t }

func rec(fn *function.Function, context_, kind string, at time.Time) fncontrol.TraceRecord {
	return fncontrol.TraceRecord{
		At: at.UnixMilli(), Address: fn.Address, Version: 3, InvocationID: "inv-1", CorrelationID: "corr-1",
		Reference: "TRIP-1", Context: context_, Kind: kind, Level: "info", Message: "hello",
		Data: json.RawMessage(`{"a":1}`),
	}
}

func ship(t *testing.T, s *State, records ...fncontrol.TraceRecord) (fncontrol.TraceAck, error) {
	t.Helper()
	out, err := s.traceLog(anchorCtx(), &traceLogInput{Body: fncontrol.TraceBatch{RunnerID: "runner-1", Pool: "p", Records: records}})
	if err != nil {
		return fncontrol.TraceAck{}, err
	}
	return out.Body, nil
}

func stored(t *testing.T, s *State, fnID string) []function.TraceLogRecord {
	t.Helper()
	recs, err := s.Repo.QueryTraceLog(context.Background(), function.TraceLogFilter{FunctionID: fnID, Limit: 1000})
	require.NoError(t, err)
	return recs
}

func TestTraceLog_AcceptsLiveContextAndStoresTheRecord(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, _ := traceFn(t, s)
	setWindow(t, s, fn.ID, "orders", inFuture(time.Minute), 1)

	ack, err := ship(t, s, rec(fn, "orders", "trace", time.Now()))
	require.NoError(t, err)
	assert.Equal(t, fncontrol.TraceAck{Accepted: 1, Dropped: 0}, ack)

	got := stored(t, s, fn.ID)
	require.Len(t, got, 1)
	r := got[0]
	assert.True(t, strings.HasPrefix(r.ID, "ftl_"), "id %q", r.ID)
	assert.Equal(t, fn.ID, r.FunctionID)
	assert.EqualValues(t, 3, r.Version)
	assert.Equal(t, "inv-1", r.InvocationID)
	require.NotNil(t, r.CorrelationID)
	assert.Equal(t, "corr-1", *r.CorrelationID)
	require.NotNil(t, r.Reference)
	assert.Equal(t, "TRIP-1", *r.Reference)
	assert.Equal(t, "orders", r.Context)
	assert.Equal(t, "trace", r.Kind)
	assert.JSONEq(t, `{"a":1}`, string(r.Data))
	require.NotNil(t, r.RunnerID)
	assert.Equal(t, "runner-1", *r.RunnerID)
	assert.False(t, r.CreatedAt.IsZero())
}

func TestTraceLog_AcceptanceRule(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, _ := traceFn(t, s)
	until := time.Now().Add(-time.Minute) // closed a minute ago
	setWindow(t, s, fn.ID, "expired", &until, 1)
	setWindow(t, s, fn.ID, "off", nil, 1)
	setWindow(t, s, fn.ID, "live", inFuture(time.Minute), 1)

	// A record the runner stamped just before the window closed is accepted
	// (the 10 s slack covers a late flush); one stamped after it is not.
	justBefore := until.Add(-2 * time.Second)
	withinSlack := until.Add(9 * time.Second)
	afterSlack := until.Add(11 * time.Second)
	unknownAddr := rec(fn, "live", "trace", time.Now())
	unknownAddr.Address = "no.such-function"

	ack, err := ship(t, s,
		rec(fn, "expired", "trace", justBefore),   // accepted
		rec(fn, "expired", "trace", withinSlack),  // accepted (slack)
		rec(fn, "expired", "trace", afterSlack),   // dropped
		rec(fn, "off", "trace", time.Now()),       // dropped: switched off
		rec(fn, "never", "trace", time.Now()),     // dropped: no row
		unknownAddr,                               // dropped: unknown address
		rec(fn, "live", "bogus-kind", time.Now()), // dropped: kind
		rec(fn, "live", "http", time.Now()),       // accepted
	)
	require.NoError(t, err)
	assert.Equal(t, fncontrol.TraceAck{Accepted: 3, Dropped: 5}, ack)
	assert.Len(t, stored(t, s, fn.ID), 3)
}

// The runner's `at` is bounded by the arrival time: a clock running ahead is
// clamped to now (so it can neither sit in the future nor slip past a closed
// window), one far behind is dropped.
func TestTraceLog_AtIsBoundedByArrival(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, _ := traceFn(t, s)
	setWindow(t, s, fn.ID, "live", inFuture(time.Minute), 1)
	until := time.Now().Add(-time.Minute)
	setWindow(t, s, fn.ID, "closed", &until, 1)

	ahead := rec(fn, "live", "trace", time.Now().Add(48*time.Hour))
	ahead.InvocationID = "ahead"
	slightlyAhead := rec(fn, "live", "trace", time.Now().Add(20*time.Second))
	slightlyAhead.InvocationID = "slightly"
	before := time.Now()
	ack, err := ship(t, s,
		ahead,         // accepted, at clamped to arrival
		slightlyAhead, // accepted as stamped (within the skew)
		rec(fn, "live", "trace", time.Now().Add(-2*time.Hour)),    // dropped: far behind
		rec(fn, "closed", "trace", time.Now().Add(365*24*time.Hour)), // dropped: clamped to now, after the window
	)
	require.NoError(t, err)
	assert.Equal(t, fncontrol.TraceAck{Accepted: 2, Dropped: 2}, ack)
	for _, r := range stored(t, s, fn.ID) {
		switch r.InvocationID {
		case "ahead":
			assert.WithinDuration(t, before, r.At, 5*time.Second, "clamped to arrival")
		case "slightly":
			assert.True(t, r.At.After(before.Add(10*time.Second)), "kept as stamped")
		}
	}
}

// A runner-supplied record id makes a retried batch idempotent; a record
// without one (an older runner) still gets a platform id.
func TestTraceLog_RunnerRecordIDDeduplicates(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, _ := traceFn(t, s)
	setWindow(t, s, fn.ID, "orders", inFuture(time.Minute), 1)
	a := rec(fn, "orders", "trace", time.Now())
	a.ID = "0b8f9f5e-7f0e-4c1e-9d55-6a9f0d0d1a01"
	b := rec(fn, "orders", "trace", time.Now())
	b.ID = "0b8f9f5e-7f0e-4c1e-9d55-6a9f0d0d1a02"
	old := rec(fn, "orders", "trace", time.Now()) // no id
	bad := rec(fn, "orders", "trace", time.Now())
	bad.ID = "x y" // not a usable id: a platform id instead

	_, err := ship(t, s, a, b, old, bad)
	require.NoError(t, err)
	_, err = ship(t, s, a, b) // the runner retries after a lost answer
	require.NoError(t, err)
	got := stored(t, s, fn.ID)
	assert.Len(t, got, 4, "the retried records are stored once")
	ids := map[string]bool{}
	for _, r := range got {
		ids[r.ID] = true
	}
	assert.True(t, ids["ftl_r"+a.ID])
	assert.True(t, ids["ftl_r"+b.ID])
}

// Switching off ends the window at the switch-off time, so records the
// runner captured just before and ships a little later are still accepted.
func TestTraceLog_AcceptsQueuedRecordsAfterSwitchOff(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, _ := traceFn(t, s)
	setWindow(t, s, fn.ID, "orders", inFuture(time.Hour), 1)
	captured := time.Now()
	withRawTx(t, testpg.Pool(t), func(tx pgx.Tx) error {
		return s.Repo.DisableTraceContextTx(context.Background(), fn.ID, "orders", time.Now().UTC(), tx)
	})
	ack, err := ship(t, s,
		rec(fn, "orders", "trace", captured),                     // accepted
		rec(fn, "orders", "trace", time.Now().Add(30*time.Second)), // dropped: after the grace
	)
	require.NoError(t, err)
	assert.Equal(t, fncontrol.TraceAck{Accepted: 1, Dropped: 1}, ack)
}

func TestTraceLog_Truncation(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, _ := traceFn(t, s)
	setWindow(t, s, fn.ID, "orders", inFuture(time.Minute), 1)
	setWindow(t, s, fn.ID, "http", inFuture(time.Minute), 1)

	bigTrace := rec(fn, "orders", "trace", time.Now())
	bigTrace.InvocationID = "big-trace"
	bigTrace.Message = strings.Repeat("é", 5000) // characters, not bytes
	bigTrace.Reference = strings.Repeat("r", 300)
	bigTrace.Data = json.RawMessage(`{"x":"` + strings.Repeat("a", 33000) + `"}`) // over 32768

	midHTTP := rec(fn, "http", "http", time.Now())
	midHTTP.InvocationID = "mid-http"
	midHTTP.Data = json.RawMessage(`{"x":"` + strings.Repeat("a", 40000) + `"}`) // over 32768, under 65536: kept for http

	bigHTTP := rec(fn, "http", "http", time.Now())
	bigHTTP.InvocationID = "big-http"
	bigHTTP.Data = json.RawMessage(`{"x":"` + strings.Repeat("a", 70000) + `"}`)

	badLevel := rec(fn, "orders", "trace", time.Now())
	badLevel.InvocationID = "bad-level"
	badLevel.Level = "shouting"
	badLevel.Data = json.RawMessage(`not json`)

	nul := rec(fn, "orders", "trace", time.Now())
	nul.InvocationID = "nul"
	nul.Message = "a\x00b"
	nul.Data = json.RawMessage(`{"k":"x\u0000y"}`)

	ack, err := ship(t, s, bigTrace, midHTTP, bigHTTP, badLevel, nul)
	require.NoError(t, err)
	assert.Equal(t, 5, ack.Accepted)

	by := map[string]function.TraceLogRecord{}
	for _, r := range stored(t, s, fn.ID) {
		by[r.InvocationID] = r
	}
	require.Len(t, by, 5)

	assert.Equal(t, 4096, len([]rune(by["big-trace"].Message)), "message cut to 4096 characters")
	assert.Equal(t, 200, len(*by["big-trace"].Reference), "reference cut to 200")
	assert.JSONEq(t, `{"truncated":true,"bytes":`+itoaSeq(len(bigTrace.Data))+`}`, string(by["big-trace"].Data))

	assert.Greater(t, len(by["mid-http"].Data), 40000, "an http record may carry up to 65536 bytes")
	assert.JSONEq(t, `{"truncated":true,"bytes":`+itoaSeq(len(bigHTTP.Data))+`}`, string(by["big-http"].Data))

	assert.Equal(t, "info", by["bad-level"].Level)
	assert.Nil(t, by["bad-level"].Data, "data that is not JSON is dropped, the record kept")

	assert.Equal(t, "ab", by["nul"].Message, "NULs are removed")
	assert.NotNil(t, by["nul"].Data, "a NUL escape in data does not fail the insert")
}

func TestTraceLog_BatchTooLarge(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, _ := traceFn(t, s)
	setWindow(t, s, fn.ID, "orders", inFuture(time.Minute), 1)

	records := make([]fncontrol.TraceRecord, fncontrol.MaxTraceBatchRecords+1)
	for i := range records {
		records[i] = rec(fn, "orders", "trace", time.Now())
	}
	_, err := ship(t, s, records...)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "TRACE_BATCH_TOO_LARGE")
	assert.Empty(t, stored(t, s, fn.ID), "a refused batch stores nothing")

	// Exactly the cap is fine at the batch level (the rate cap is separate).
	ack, err := ship(t, s, records[:fncontrol.MaxTraceBatchRecords]...)
	require.NoError(t, err)
	assert.Equal(t, fncontrol.MaxTraceBatchRecords, ack.Accepted+ack.Dropped)
}

func TestTraceLog_PerFunctionRateCap(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, _ := traceFn(t, s)
	other, _ := traceFn(t, s, "x")
	setWindow(t, s, fn.ID, "orders", inFuture(time.Minute), 1)
	setWindow(t, s, other.ID, "orders", inFuture(time.Minute), 1)

	records := make([]fncontrol.TraceRecord, 0, 500)
	for range 500 {
		records = append(records, rec(fn, "orders", "trace", time.Now()))
	}
	// A different function in the same batch has its own bucket.
	records = append(records[:499], rec(other, "orders", "trace", time.Now()))

	ack, err := ship(t, s, records...)
	require.NoError(t, err)
	assert.Equal(t, fncontrol.MaxTraceRecordsPerSec+1, ack.Accepted, "200 for the busy function plus the other function's one")
	assert.Equal(t, 499-fncontrol.MaxTraceRecordsPerSec, ack.Dropped)
	assert.Len(t, stored(t, s, fn.ID), fncontrol.MaxTraceRecordsPerSec)
	assert.Len(t, stored(t, s, other.ID), 1)

	// The bucket is empty right after, so a second batch is mostly dropped
	// (a few tokens refill in the milliseconds between the two calls).
	more := make([]fncontrol.TraceRecord, 100)
	for i := range more {
		more[i] = rec(fn, "orders", "trace", time.Now())
	}
	ack, err = ship(t, s, more...)
	require.NoError(t, err)
	assert.Less(t, ack.Accepted, 40)
	assert.Equal(t, 100, ack.Accepted+ack.Dropped)
}

func TestTraceLog_RequiresRunnerControl(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	body := &traceLogInput{Body: fncontrol.TraceBatch{RunnerID: "r", Pool: "p"}}

	_, err := s.traceLog(context.Background(), body)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "UNAUTHENTICATED")
	_, err = s.traceLog(anchorCtx("platform:function:function:view"), body)
	testpg.RequireUsecaseError(t, err, usecase.KindAuthorization, "PERMISSION_REQUIRED")

	body.Body.RunnerID = ""
	_, err = s.traceLog(anchorCtx(), body)
	testpg.RequireUsecaseError(t, err, usecase.KindValidation, "RUNNER_ID_REQUIRED")

	out, err := s.traceLog(anchorCtx(), &traceLogInput{Body: fncontrol.TraceBatch{RunnerID: "r", Pool: "p"}})
	require.NoError(t, err)
	assert.Equal(t, fncontrol.TraceAck{}, out.Body, "an empty batch is acknowledged")
}

// ── Desired state ───────────────────────────────────────────────────────

func TestDesired_TraceWindowsLiveOnlyAndETagChanges(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	fn, poolName := traceFn(t, s)
	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	first, err := s.desired(anchorCtx(), &desiredInput{Pool: poolName})
	require.NoError(t, err)
	require.Len(t, first.Body.Functions, 1)
	assert.Empty(t, first.Body.Functions[0].Trace, "no windows: the key is absent")

	until := time.Now().Add(10 * time.Minute)
	setWindow(t, s, fn.ID, "http", &until, 5)
	past := time.Now().Add(-time.Minute)
	setWindow(t, s, fn.ID, "expired", &past, 1)
	setWindow(t, s, fn.ID, "off", nil, 1)

	second, err := s.desired(anchorCtx(), &desiredInput{Pool: poolName})
	require.NoError(t, err)
	assert.NotEqual(t, first.ETag, second.ETag, "a live window changes the ETag")
	require.Len(t, second.Body.Functions[0].Trace, 1, "only the live window; the expired and the switched-off ones drop out")
	assert.Equal(t, fncontrol.TraceState{Context: "http", EnabledUntilMs: until.UnixMilli(), SampleRate: 5}, second.Body.Functions[0].Trace[0])

	// A re-render with nothing changed keeps the ETag (so a held poll stays held).
	again, err := s.desired(anchorCtx(), &desiredInput{Pool: poolName, IfNoneMatch: second.ETag})
	require.NoError(t, err)
	assert.Equal(t, 304, again.Status)

	// Closing the window changes it back.
	setWindow(t, s, fn.ID, "http", nil, 5)
	third, err := s.desired(anchorCtx(), &desiredInput{Pool: poolName})
	require.NoError(t, err)
	assert.Equal(t, first.ETag, third.ETag)
}

func TestDesired_EnableTraceWakesLongPoll(t *testing.T) {
	t.Parallel()
	pool := testpg.Pool(t)
	s := newTestState(t)
	s.Listener = NewListener(pool, nil)
	s.ReRenderInterval = 10 * time.Second
	listenCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go s.Listener.Run(listenCtx)
	waitForListener(t, pool)

	fn, poolName := traceFn(t, s)
	live := seedVersion(t, s.Repo, fn.ID, 1, function.VersionReady, "wasm", describeJSON(t))
	seedAlias(t, s.Repo, fn.ID, "live", live.ID)

	first, err := s.desired(anchorCtx(), &desiredInput{Pool: poolName})
	require.NoError(t, err)

	done := make(chan *desiredOutput, 1)
	go func() {
		out, err := s.desired(anchorCtx(), &desiredInput{Pool: poolName, Wait: 30, IfNoneMatch: first.ETag})
		require.NoError(t, err)
		done <- out
	}()
	time.Sleep(100 * time.Millisecond)

	uow := testpg.NewUoW(t)
	_, err = usecaseop.RunTx(testpg.AnchorCtx(), uow, operations.EnableTraceContext(s.Repo, function.DefaultTraceLimits()),
		operations.EnableTraceCommand{FunctionID: fn.ID, Context: "http", Minutes: 5, SampleRate: 1}, testpg.TestEC())
	require.NoError(t, err)

	select {
	case out := <-done:
		assert.Equal(t, 200, out.Status)
		assert.NotEqual(t, first.ETag, out.ETag)
		require.Len(t, out.Body.Functions[0].Trace, 1)
		assert.Equal(t, "http", out.Body.Functions[0].Trace[0].Context)
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("desired() did not wake within ~1s of an enable")
	}
}

// TestE2E_TraceLog_ClientAgainstRealServer ships a batch with the runner's own
// client over real HTTP behind the real Authenticator: 202 and a TraceAck. A
// body over the batch cap is refused before it is read.
func TestE2E_TraceLog_ClientAgainstRealServer(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ti := newTestTokenIssuer(t)
	srv := newTestServer(t, s, ti)
	token := ti.mint(t, "sa_fn_runner_trace", []string{"platform:function:runner:control"})
	client := fncontrol.NewClient(srv.URL, func(context.Context) (string, error) { return token, nil }, nil)

	fn, poolName := traceFn(t, s)
	setWindow(t, s, fn.ID, "orders", inFuture(time.Minute), 1)

	ack, err := client.TraceLog(context.Background(), fncontrol.TraceBatch{
		RunnerID: "runner-e2e", Pool: poolName,
		Records: []fncontrol.TraceRecord{
			rec(fn, "orders", "trace", time.Now()),
			rec(fn, "nope", "trace", time.Now()),
		},
	})
	require.NoError(t, err)
	assert.Equal(t, fncontrol.TraceAck{Accepted: 1, Dropped: 1}, *ack)
	assert.Len(t, stored(t, s, fn.ID), 1)

	huge := rec(fn, "orders", "trace", time.Now())
	huge.Message = strings.Repeat("x", fncontrol.MaxTraceBatchBytes+10)
	_, err = client.TraceLog(context.Background(), fncontrol.TraceBatch{RunnerID: "runner-e2e", Pool: poolName, Records: []fncontrol.TraceRecord{huge}})
	require.Error(t, err, "a body over 1 MiB is refused")
	assert.Len(t, stored(t, s, fn.ID), 1)
}

// TestE2E_TraceLog_AcceptsFieldsAnOlderPlatformDoesNotKnow: a newer runner may
// add fields to the batch and to a record. The whole batch must still be taken
// (the runner drops a batch the platform refuses), the extra fields ignored.
func TestE2E_TraceLog_AcceptsFieldsAnOlderPlatformDoesNotKnow(t *testing.T) {
	t.Parallel()
	s := newTestState(t)
	ti := newTestTokenIssuer(t)
	srv := newTestServer(t, s, ti)
	token := ti.mint(t, "sa_fn_runner_trace_lenient", []string{"platform:function:runner:control"})
	fn, poolName := traceFn(t, s)
	setWindow(t, s, fn.ID, "orders", inFuture(time.Minute), 1)

	r := rec(fn, "orders", "trace", time.Now())
	raw, err := json.Marshal(map[string]any{
		"runnerId": "runner-new", "pool": poolName, "futureBatchField": map[string]any{"a": 1},
		"records": []any{
			func() map[string]any {
				var m map[string]any
				b, _ := json.Marshal(r)
				_ = json.Unmarshal(b, &m)
				m["futureRecordField"] = "x"
				m["anotherOne"] = []int{1, 2}
				return m
			}(),
		},
	})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, srv.URL+fncontrol.PathTraceLog, bytes.NewReader(raw))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	require.Equal(t, http.StatusAccepted, resp.StatusCode, string(body))
	assert.Len(t, stored(t, s, fn.ID), 1)
}
