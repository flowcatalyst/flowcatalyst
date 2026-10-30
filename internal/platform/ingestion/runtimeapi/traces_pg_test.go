//go:build integration

package runtimeapi_test

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/ingestion"
)

const (
	permTraceManage = "platform:ingestion:trace:manage"
	permTraceBodies = "platform:ingestion:trace:view-bodies"
	permTraceView   = "platform:ingestion:trace:view"
)

// asOperator lets the test's one principal also switch tracing on and read it,
// beside the runtime's own permission.
func (e *env) asOperator() {
	e.who = e.principal(permRuntime, permView, permTraceManage, permTraceBodies, permTraceView)
}

// enableTrace switches tracing on for an ingestion and returns its window.
func (e *env) enableTrace(id string, body map[string]any) response {
	e.t.Helper()
	e.asOperator()
	r := e.do(http.MethodPut, "/api/ingestions/"+id+"/trace", body)
	require.Equal(e.t, http.StatusOK, r.code, r.raw)
	return r
}

// traceOf is the poll answer's trace entry for an ingestion, nil when none.
func traceOf(r response, id string) map[string]any {
	for _, t := range r.list("traces") {
		if t["ingestionId"] == id {
			return t
		}
	}
	return nil
}

// A poll answer lists the open windows of the ingestions it lists to that
// runtime and nothing else: a polled ingestion only to its lease holder, an
// ended window not at all, and no window at all as no `traces` field.
func TestPoll_TracesListOpenWindowsOfTheListedIngestions(t *testing.T) {
	e := newEnv(t)
	shared := e.save("shared", sqsSource, true)
	polled := e.save("polled", sftpSource, true)
	untraced := e.save("untraced", sqsSource, true)
	holder, other := "rt-h-"+unique(), "rt-o-"+unique()

	r := e.poll(holder, ingestion.ContractVersion)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	require.Len(t, r.list("ingestions"), 3)
	assert.NotContains(t, r.json, "traces", "nothing is traced: the field is absent, and an older runtime is none the wiser")

	w1 := e.enableTrace(shared.ID, map[string]any{"windowSecs": 300, "sampleEverySecs": 10})
	w2 := e.enableTrace(polled.ID, map[string]any{})

	r = e.poll(holder, ingestion.ContractVersion)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	got := traceOf(r, shared.ID)
	require.NotNil(t, got, r.raw)
	assert.Equal(t, w1.str("windowId"), got["windowId"])
	assert.EqualValues(t, 300, got["windowSecs"])
	assert.EqualValues(t, 10, got["sampleEverySecs"])
	assert.Equal(t, w1.str("hardStop"), got["hardStop"], "the platform's hard stop, for the runtime to compare on every use")
	polledTrace := traceOf(r, polled.ID)
	require.NotNil(t, polledTrace, "the lease holder is told of its polled ingestion's window")
	assert.Equal(t, w2.str("windowId"), polledTrace["windowId"])
	assert.Nil(t, traceOf(r, untraced.ID))

	// Another runtime is given the shared ingestion and so its window, not the
	// polled one it does not hold: that window is not for it.
	r = e.poll(other, ingestion.ContractVersion)
	require.Equal(t, http.StatusOK, r.code, r.raw)
	assert.NotNil(t, traceOf(r, shared.ID))
	assert.Nil(t, traceOf(r, polled.ID), "a polled ingestion's window goes only to its lease holder")

	// Switched on again: a new window id, so runtimes start afresh.
	again := e.enableTrace(shared.ID, map[string]any{})
	assert.NotEqual(t, w1.str("windowId"), again.str("windowId"))
	assert.Equal(t, again.str("windowId"), traceOf(e.poll(holder, ingestion.ContractVersion), shared.ID)["windowId"])

	// Switched off: an ended window is no longer listed.
	e.asOperator()
	require.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, "/api/ingestions/"+shared.ID+"/trace", nil).code)
	e.now = time.Now().UTC().Add(time.Second)
	assert.Nil(t, traceOf(e.poll(holder, ingestion.ContractVersion), shared.ID), "switched off")
	assert.NotNil(t, traceOf(e.poll(holder, ingestion.ContractVersion), polled.ID))

	// A window that ran out is not listed either (its hard stop is past).
	e.now = time.Now().UTC().Add(time.Hour)
	assert.Nil(t, traceOf(e.poll(holder, ingestion.ContractVersion), polled.ID), "past its hard stop")
}

func (e *env) traces(runtime string, records ...map[string]any) response {
	e.t.Helper()
	e.who = e.principal(permRuntime)
	if records == nil {
		records = []map[string]any{}
	}
	return e.do(http.MethodPost, base+"/traces", map[string]any{"runtimeId": runtime, "records": records})
}

func traceRec(ing, window string, at time.Time, kind string, extra map[string]any) map[string]any {
	rec := map[string]any{
		"id": unique() + unique(), "ingestionId": ing, "version": 1, "windowId": window, "captureId": "c-" + unique(),
		"at": at.UnixMilli(), "kind": kind, "level": "info", "message": kind + " message",
		"data": map[string]any{"outcome": "ok"},
	}
	maps.Copy(rec, extra)
	return rec
}

func (e *env) storedTrace(id string) []ingestion.TraceLogRecord {
	e.t.Helper()
	recs, err := e.repo.QueryTraceLog(context.Background(), ingestion.TraceLogFilter{IngestionID: id, Limit: 1000})
	require.NoError(e.t, err)
	return recs
}

// Records are accepted only inside the ingestion's own open window: the right
// window id, dated no later than the hard stop plus 10 seconds, of an ingestion
// that was switched on; a record dated a minute ahead is clamped to arrival and
// one more than an hour behind is dropped.
func TestTraces_AcceptanceRules(t *testing.T) {
	e := newEnv(t)
	i := e.save("traced", sqsSource, true)
	untraced := e.save("untraced", sqsSource, true)
	win := e.enableTrace(i.ID, map[string]any{"windowSecs": 60})
	window := win.str("windowId")
	hardStop := mustParse(t, win.str("hardStop"))
	now := time.Now().UTC().Truncate(time.Millisecond)
	e.now = now

	good := traceRec(i.ID, window, now.Add(-time.Second), "poll", nil)
	r := e.traces("rt-1",
		good,
		traceRec(i.ID, "itw_other", now, "poll", nil),                        // not the window
		traceRec(untraced.ID, window, now, "poll", nil),                      // never switched on
		traceRec("ing_unknown0000", window, now, "poll", nil),                // not an ingestion
		traceRec(i.ID, window, now, "mystery", nil),                          // not a kind
		traceRec(i.ID, window, now, "poll", map[string]any{"captureId": ""}), // no capture
		traceRec(i.ID, window, now.Add(-2*time.Hour), "poll", nil),           // more than an hour behind
		traceRec(i.ID, window, now, "poll", map[string]any{"at": 0}),         // no time
	)
	require.Equal(t, http.StatusAccepted, r.code, r.raw)
	assert.EqualValues(t, 1, r.json["accepted"])
	assert.EqualValues(t, 7, r.json["dropped"])
	require.Len(t, e.storedTrace(i.ID), 1)

	// A future `at` more than a minute ahead is clamped to arrival; within the
	// minute it stays.
	r = e.traces("rt-1",
		traceRec(i.ID, window, now.Add(10*time.Minute), "http", nil),
		traceRec(i.ID, window, now.Add(30*time.Second), "exchange", nil),
	)
	assert.EqualValues(t, 2, r.json["accepted"], r.raw)
	byKind := map[string]ingestion.TraceLogRecord{}
	for _, rec := range e.storedTrace(i.ID) {
		byKind[rec.Kind] = rec
	}
	assert.Equal(t, now.UnixMilli(), byKind["http"].At.UnixMilli(), "clamped to arrival")
	assert.Equal(t, now.Add(30*time.Second).UnixMilli(), byKind["exchange"].At.UnixMilli())
	assert.WithinDuration(t, time.Now(), byKind["http"].CreatedAt, 10*time.Second, "the platform sets created_at")

	// After the hard stop (+10 s) nothing is accepted: the runtime's clock is
	// moved past it so `at` is not clamped back.
	e.now = hardStop.Add(30 * time.Second)
	r = e.traces("rt-1",
		traceRec(i.ID, window, hardStop.Add(5*time.Second), "poll", nil),  // inside the 10 s of slack
		traceRec(i.ID, window, hardStop.Add(20*time.Second), "poll", nil), // past it
	)
	assert.EqualValues(t, 1, r.json["accepted"], r.raw)
	assert.EqualValues(t, 1, r.json["dropped"])

	// A switched-off window refuses records dated after the switch-off (+10 s).
	e.asOperator()
	require.Equal(t, http.StatusNoContent, e.do(http.MethodDelete, "/api/ingestions/"+i.ID+"/trace", nil).code)
	off := time.Now().UTC()
	e.now = off.Add(time.Minute)
	r = e.traces("rt-1",
		traceRec(i.ID, window, off.Add(-time.Second), "error", nil),
		traceRec(i.ID, window, off.Add(40*time.Second), "error", nil),
	)
	assert.EqualValues(t, 1, r.json["accepted"], "captured just before the switch-off, still queued: kept")
	assert.EqualValues(t, 1, r.json["dropped"])
}

func mustParse(t *testing.T, s string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, s)
	require.NoError(t, err)
	return v
}

// The limits: a record's id makes a retried batch store once, a message is cut
// to 1000 characters, data over 80 KiB becomes a stub, 500 records at most,
// and an ingestion takes 50 records a second.
func TestTraces_LimitsAndIdempotence(t *testing.T) {
	e := newEnv(t)
	i := e.save("limits", sqsSource, true)
	window := e.enableTrace(i.ID, map[string]any{}).str("windowId")
	now := time.Now().UTC()
	e.now = now

	a := traceRec(i.ID, window, now, "http", map[string]any{
		"message": strings.Repeat("é", 1500), "data": map[string]any{"blob": strings.Repeat("x", 100<<10)},
	})
	b := traceRec(i.ID, window, now, "poll", nil)
	r := e.traces("rt-1", a, b)
	require.Equal(t, http.StatusAccepted, r.code, r.raw)
	assert.EqualValues(t, 2, r.json["accepted"])
	// The same batch again (a retry) stores nothing new.
	r = e.traces("rt-1", a, b)
	require.Equal(t, http.StatusAccepted, r.code, r.raw)
	recs := e.storedTrace(i.ID)
	require.Len(t, recs, 2, "a record id the platform already holds is skipped")
	for _, rec := range recs {
		assert.Equal(t, "itr_"+map[string]string{"http": a["id"].(string), "poll": b["id"].(string)}[rec.Kind], rec.ID)
		assert.Equal(t, "rt-1", rec.RuntimeID)
	}
	for _, rec := range recs {
		if rec.Kind != "http" {
			continue
		}
		assert.Len(t, []rune(rec.Message), 1000, "cut to 1000 characters")
		var stub map[string]any
		require.NoError(t, json.Unmarshal(rec.Data, &stub))
		assert.Equal(t, true, stub["truncated"])
		assert.Greater(t, stub["bytes"], float64(80<<10))
	}

	// 500 records a batch at most.
	var many []map[string]any
	for range 501 {
		many = append(many, traceRec(i.ID, window, now, "poll", nil))
	}
	r = e.traces("rt-1", many...)
	assert.Equal(t, http.StatusBadRequest, r.code, r.raw)

	// 50 records a second per ingestion: the burst is a second's worth, the
	// rest of a batch counts as dropped.
	other := e.save("limits-2", sqsSource, true)
	otherWindow := e.enableTrace(other.ID, map[string]any{}).str("windowId")
	var burst []map[string]any
	for range 120 {
		burst = append(burst, traceRec(other.ID, otherWindow, now, "poll", nil))
	}
	r = e.traces("rt-1", burst...)
	require.Equal(t, http.StatusAccepted, r.code, r.raw)
	assert.EqualValues(t, 50, r.json["accepted"])
	assert.EqualValues(t, 70, r.json["dropped"])
	// The cap is per ingestion: another one is not held back by it.
	r = e.traces("rt-1", traceRec(i.ID, otherWindow, now, "poll", nil), traceRec(i.ID, window, now, "poll", nil))
	assert.EqualValues(t, 1, r.json["accepted"], r.raw)
	assert.Equal(t, http.StatusAccepted, e.traces("rt-1").code, "an empty batch")
}

// The traces route is the runtime's: another principal is refused.
func TestTraces_RuntimeOnly(t *testing.T) {
	e := newEnv(t)
	i := e.save("runtime-only", sqsSource, true)
	window := e.enableTrace(i.ID, map[string]any{}).str("windowId")
	e.who = e.principal(permView, permTraceView)
	r := e.do(http.MethodPost, base+"/traces", map[string]any{"runtimeId": "rt-1", "records": []map[string]any{traceRec(i.ID, window, time.Now(), "poll", nil)}})
	assert.Equal(t, http.StatusForbidden, r.code, r.raw)
	assert.Empty(t, e.storedTrace(i.ID))
	// A newer runtime's extra fields on a batch or a record are ignored.
	e.who = e.principal(permRuntime)
	rec := traceRec(i.ID, window, time.Now(), "poll", map[string]any{"futureField": 1})
	r = e.do(http.MethodPost, base+"/traces", map[string]any{"runtimeId": "rt-1", "extra": true, "records": []map[string]any{rec}})
	assert.Equal(t, http.StatusAccepted, r.code, r.raw)
	assert.Len(t, e.storedTrace(i.ID), 1)
}

// A report stores polls and lastPollAt on the window beside the counts, a
// window sent again replaces them, and the activity read sums polls per period
// and reads the newest last poll.
func TestReport_PollsAndLastPollAt(t *testing.T) {
	e := newEnv(t)
	i := e.save("polling", sqsSource, true)
	rt := "rt-" + unique()
	t0 := e.now.Truncate(time.Second)
	window := func(from time.Time, polls int, last time.Time) map[string]any {
		return map[string]any{
			"ingestionId": i.ID, "from": from.Format(time.RFC3339), "to": from.Add(time.Minute).Format(time.RFC3339),
			"completed": 0, "failed": 0, "records": 0, "dispatchJobs": 0, "polls": polls, "lastPollAt": last.Format(time.RFC3339),
		}
	}
	report := func(windows ...map[string]any) {
		e.who = e.principal(permRuntime)
		r := e.do(http.MethodPost, base+"/report", map[string]any{"runtimeId": rt, "metrics": windows})
		require.Equal(t, http.StatusNoContent, r.code, r.raw)
	}
	// A window with polls and nothing else is reported and stored.
	report(window(t0.Add(-3*time.Minute), 4, t0.Add(-150*time.Second)), window(t0.Add(-2*time.Minute), 6, t0.Add(-70*time.Second)))
	assert.Equal(t, 2, e.count(`SELECT count(*) FROM ing_exchange_metrics WHERE ingestion_id = $1 AND runtime_id = $2 AND completed = 0 AND polls > 0 AND last_poll_at IS NOT NULL`, i.ID, rt))
	// Sent again, the window replaces (never adds to) its polls.
	report(window(t0.Add(-3*time.Minute), 5, t0.Add(-140*time.Second)))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_exchange_metrics WHERE ingestion_id = $1 AND runtime_id = $2 AND polls = 5`, i.ID, rt))
	// A window that omits them (an older runtime) reads zero polls, no last poll.
	e.who = e.principal(permRuntime)
	require.Equal(t, http.StatusNoContent, e.do(http.MethodPost, base+"/report", map[string]any{"runtimeId": rt, "metrics": []map[string]any{{
		"ingestionId": i.ID, "from": t0.Add(-time.Minute).Format(time.RFC3339), "to": t0.Format(time.RFC3339),
		"completed": 1, "failed": 0, "records": 1, "dispatchJobs": 1,
	}}}).code)
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_exchange_metrics WHERE ingestion_id = $1 AND runtime_id = $2 AND polls = 0 AND last_poll_at IS NULL`, i.ID, rt))
	// A last poll from a runtime clock a day ahead is recorded as now.
	report(window(t0.Add(-5*time.Minute), 1, t0.Add(24*time.Hour)))
	assert.Equal(t, 1, e.count(`SELECT count(*) FROM ing_exchange_metrics WHERE ingestion_id = $1 AND last_poll_at = $2`, i.ID, e.now))

	a, err := e.repo.ActivityOf(context.Background(), i.ID, t0)
	require.NoError(t, err)
	assert.EqualValues(t, 5+6+1, a.Day.Polls, "summed over the windows")
	assert.EqualValues(t, 5+6+1, a.Week.Polls)
	require.NotNil(t, a.LastPoll)
	assert.True(t, a.LastPoll.Equal(e.now), "the newest last poll reported: %v", a.LastPoll)
}
