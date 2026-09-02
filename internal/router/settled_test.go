package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// fakeSettledReporter is a SettledReporter test double: every ReportSettled
// call is pushed onto a buffered channel so a test can wait on it with a
// timeout, plus a `never` helper for the "must not have been called" side.
// err, if set, is returned from every call (but the call is still recorded)
// — used to pin that a reporter failure never reaches the broker ACK path.
type fakeSettledReporter struct {
	calls chan SettledReport
	err   error
}

func newFakeSettledReporter() *fakeSettledReporter {
	return &fakeSettledReporter{calls: make(chan SettledReport, 16)}
}

func (f *fakeSettledReporter) ReportSettled(_ context.Context, report SettledReport) error {
	f.calls <- report
	return f.err
}

// awaitCall waits up to 2s for exactly one ReportSettled call and returns it.
func (f *fakeSettledReporter) awaitCall(t *testing.T) SettledReport {
	t.Helper()
	select {
	case r := <-f.calls:
		return r
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for ReportSettled")
		return SettledReport{}
	}
}

// assertNeverCalled waits a short grace period (long enough for the
// fire-and-forget goroutine to have run, since it does no real I/O here)
// and fails if a call arrived in that window.
func (f *fakeSettledReporter) assertNeverCalled(t *testing.T) {
	t.Helper()
	select {
	case r := <-f.calls:
		t.Fatalf("ReportSettled must not have been called, got %+v", r)
	case <-time.After(200 * time.Millisecond):
	}
}

// mkOrderedTok builds a BLOCK_ON_ERROR (or explicit mode) ordered message
// carrying an AuthToken, so it qualifies as a platform dispatch job for the
// settled hook (see dispatchJobFromMessage). Token defaults to "tok-"+id
// when non-empty is wanted; pass "" to build a message with NO AuthToken
// (simulating a non-dispatch-job message).
func mkOrderedTok(id string, group *string, mode common.DispatchMode, token string) common.QueuedMessage {
	m := common.QueuedMessage{
		Message: common.Message{
			ID:              id,
			MediationType:   common.MediationTypeHTTP,
			MediationTarget: "http://example.invalid",
			MessageGroupID:  group,
			DispatchMode:    mode,
		},
		ReceiptHandle: id,
	}
	if token != "" {
		m.Message.AuthToken = &token
	}
	return m
}

// configErrorAlways is a Mediator outcome factory: MediationErrorConfig is
// the disposition class DispositionOf maps to BrokerAck + (GroupBlock under
// BLOCK_ON_ERROR, GroupContinue otherwise) — a permanent, terminal
// ACK-classified failure, as opposed to a retryable/releasable one. Exactly
// the outcome that drives ackBuffered.
var configErrorOutcome = &common.MediationOutcome{Result: common.MediationErrorConfig, StatusCode: 400}

// TestAckBufferedReportsSettledOnBlockOnErrorHeadFailure pins the core
// wiring: a BLOCK_ON_ERROR head that fails terminally reports exactly its
// untried buffered siblings (not the head itself) to the SettledReporter,
// with the ackBuffered reason, after the broker ACKs have already happened.
func TestAckBufferedReportsSettledOnBlockOnErrorHeadFailure(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: configErrorOutcome}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	reporter := newFakeSettledReporter()
	pool.SetSettledReporter(reporter)

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		mkOrderedTok("m1", &group, common.DispatchBlockOnError, "tok-m1"),
		mkOrderedTok("m2", &group, common.DispatchBlockOnError, "tok-m2"),
		mkOrderedTok("m3", &group, common.DispatchBlockOnError, "tok-m3"),
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for 3 ACKs")
	}

	// All three ACKed (m1 terminally, m2/m3 as untried buffered siblings);
	// nothing NACKed.
	cons.mu.Lock()
	acked := append([]string(nil), cons.acked...)
	nacked := append([]string(nil), cons.nacked...)
	cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, acked)
	assert.Empty(t, nacked)

	report := reporter.awaitCall(t)
	assert.Equal(t, "test", report.PoolCode)
	assert.Equal(t, group, report.Group)
	assert.Equal(t, "head failed under BLOCK_ON_ERROR", report.Reason)
	assert.Equal(t, []SettledJob{
		{ID: "m2", Token: "tok-m2"},
		{ID: "m3", Token: "tok-m3"},
	}, report.Jobs, "must carry exactly the ACKed siblings, not the head, in FIFO order")

	reporter.assertNeverCalled(t) // exactly one call
}

// TestAckBufferedReportSkipsMessagesWithNoAuthToken pins requirement 4: a
// buffered sibling with no AuthToken is not a platform dispatch job (no
// signing material the hook could verify), so it must be silently excluded
// from the report rather than sent as an unverifiable id. Here m3 has none;
// only m2 should be reported.
func TestAckBufferedReportSkipsMessagesWithNoAuthToken(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: configErrorOutcome}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	reporter := newFakeSettledReporter()
	pool.SetSettledReporter(reporter)

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		mkOrderedTok("m1", &group, common.DispatchBlockOnError, "tok-m1"),
		mkOrderedTok("m2", &group, common.DispatchBlockOnError, "tok-m2"),
		mkOrderedTok("m3", &group, common.DispatchBlockOnError, ""), // no AuthToken
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for 3 ACKs")
	}

	report := reporter.awaitCall(t)
	assert.Equal(t, []SettledJob{{ID: "m2", Token: "tok-m2"}}, report.Jobs,
		"m3 has no AuthToken and must be excluded, not sent with an empty token")
}

// TestAckBufferedReportSkippedWhenNoBufferedJobQualifies proves that when
// EVERY buffered sibling lacks AuthToken, reportSettled makes no call at
// all (not even an empty one) — there is nothing for the hook to act on.
func TestAckBufferedReportSkippedWhenNoBufferedJobQualifies(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 2, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: configErrorOutcome}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	reporter := newFakeSettledReporter()
	pool.SetSettledReporter(reporter)

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		mkOrderedTok("m1", &group, common.DispatchBlockOnError, "tok-m1"),
		mkOrderedTok("m2", &group, common.DispatchBlockOnError, ""), // no AuthToken
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for 2 ACKs")
	}

	reporter.assertNeverCalled(t)
}

// TestAckBufferedReportNotFiredOnOrdinarySuccess proves the hook fires only
// on the BLOCK_ON_ERROR head-failure ACK path, not on an ordinary
// successful drain of a group with no failures.
func TestAckBufferedReportNotFiredOnOrdinarySuccess(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{} // nothing fails
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	reporter := newFakeSettledReporter()
	pool.SetSettledReporter(reporter)

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		mkOrderedTok("m1", &group, common.DispatchBlockOnError, "tok-m1"),
		mkOrderedTok("m2", &group, common.DispatchBlockOnError, "tok-m2"),
		mkOrderedTok("m3", &group, common.DispatchBlockOnError, "tok-m3"),
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for 3 ACKs")
	}

	reporter.assertNeverCalled(t)
}

// TestAckBufferedReportNotFiredUnderNextOnError proves the hook is specific
// to BLOCK_ON_ERROR's group-block path: under NEXT_ON_ERROR the same
// terminal head failure is a GroupContinue (the drainer just moves on to
// the next message), never reaching ackBuffered at all, so no report fires
// even though the head still fails the same way.
func TestAckBufferedReportNotFiredUnderNextOnError(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: configErrorOutcome}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	reporter := newFakeSettledReporter()
	pool.SetSettledReporter(reporter)

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		mkOrderedTok("m1", &group, common.DispatchNextOnError, "tok-m1"),
		mkOrderedTok("m2", &group, common.DispatchNextOnError, "tok-m2"),
		mkOrderedTok("m3", &group, common.DispatchNextOnError, "tok-m3"),
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for 3 ACKs")
	}

	cons.mu.Lock()
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, acked,
		"NEXT_ON_ERROR delivers m2/m3 normally past the failed head")

	reporter.assertNeverCalled(t)
}

// TestAckBufferedNilReporterIsANoOp proves the default (unwired) state is
// safe: no reporter set, BLOCK_ON_ERROR head fails terminally, ACKs proceed
// exactly as they did before this feature existed, and nothing panics.
func TestAckBufferedNilReporterIsANoOp(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: configErrorOutcome}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	// Deliberately no SetSettledReporter call — nil is the zero value.

	require.NotPanics(t, func() {
		submitBatch(context.Background(), pool, []common.QueuedMessage{
			mkOrderedTok("m1", &group, common.DispatchBlockOnError, "tok-m1"),
			mkOrderedTok("m2", &group, common.DispatchBlockOnError, "tok-m2"),
			mkOrderedTok("m3", &group, common.DispatchBlockOnError, "tok-m3"),
		})
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for 3 ACKs")
	}

	cons.mu.Lock()
	acked := append([]string(nil), cons.acked...)
	nacked := append([]string(nil), cons.nacked...)
	cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, acked)
	assert.Empty(t, nacked)
}

// TestAckBufferedReportErrorDoesNotAffectAcks proves a reporter failure is
// swallowed (logged, not surfaced) and never affects the broker ACKs, which
// have already happened by the time the report is even attempted.
func TestAckBufferedReportErrorDoesNotAffectAcks(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: configErrorOutcome}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	reporter := newFakeSettledReporter()
	reporter.err = errors.New("platform unreachable")
	pool.SetSettledReporter(reporter)

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		mkOrderedTok("m1", &group, common.DispatchBlockOnError, "tok-m1"),
		mkOrderedTok("m2", &group, common.DispatchBlockOnError, "tok-m2"),
		mkOrderedTok("m3", &group, common.DispatchBlockOnError, "tok-m3"),
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for 3 ACKs")
	}

	cons.mu.Lock()
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, acked,
		"ACKs must have happened regardless of the reporter's outcome")

	// The call still happened (and returned the configured error) — proves
	// the error path was actually exercised, not just skipped.
	report := reporter.awaitCall(t)
	assert.Equal(t, []SettledJob{{ID: "m2", Token: "tok-m2"}, {ID: "m3", Token: "tok-m3"}}, report.Jobs)
}

// --- HTTPSettledReporter --------------------------------------------------

// settledCapture is what the httptest handler below records per request.
type settledCapture struct {
	method      string
	path        string
	contentType string
	body        settledWireRequest
}

// TestHTTPSettledReporter_RequestShape asserts the wire request against
// settled.go's contract: POST /api/dispatch/settled, JSON content type,
// body {"reason": ..., "jobs": [{"id":..., "token":...}]} — no separate
// auth header, since settled.go verifies each job's token individually
// rather than gating the whole request on one credential.
func TestHTTPSettledReporter_RequestShape(t *testing.T) {
	var mu sync.Mutex
	var got []settledCapture
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body settledWireRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		got = append(got, settledCapture{
			method:      r.Method,
			path:        r.URL.Path,
			contentType: r.Header.Get("Content-Type"),
			body:        body,
		})
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"settled": len(body.Jobs)})
	}))
	defer ts.Close()

	reporter := NewHTTPSettledReporter(SettledReporterConfig{Endpoint: ts.URL})
	err := reporter.ReportSettled(context.Background(), SettledReport{
		Reason: "head failed under BLOCK_ON_ERROR",
		Jobs:   []SettledJob{{ID: "m2", Token: "tok-m2"}, {ID: "m3", Token: "tok-m3"}},
	})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1)
	c := got[0]
	assert.Equal(t, http.MethodPost, c.method)
	assert.Equal(t, "/api/dispatch/settled", c.path)
	assert.Equal(t, "application/json", c.contentType)
	assert.Equal(t, "head failed under BLOCK_ON_ERROR", c.body.Reason)
	assert.Equal(t, []settledWireJob{{ID: "m2", Token: "tok-m2"}, {ID: "m3", Token: "tok-m3"}}, c.body.Jobs)
}

// TestHTTPSettledReporter_Chunking pins the sizing-defect fix: the endpoint
// caps a request body at 1 MiB with a 10,000-item ceiling that in practice
// blows the body cap first, so the reporter must split at settledChunkSize
// (1,000) rather than sending one giant batch. 2,500 ids must arrive as 3
// requests: 1000 + 1000 + 500.
func TestHTTPSettledReporter_Chunking(t *testing.T) {
	var mu sync.Mutex
	var chunkSizes []int
	var allIDs []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body settledWireRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		mu.Lock()
		chunkSizes = append(chunkSizes, len(body.Jobs))
		for _, j := range body.Jobs {
			allIDs = append(allIDs, j.ID)
		}
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"settled": len(body.Jobs)})
	}))
	defer ts.Close()

	const total = 2500
	jobs := make([]SettledJob, total)
	for i := range jobs {
		id := "job-" + strconv.Itoa(i)
		jobs[i] = SettledJob{ID: id, Token: "tok-" + id}
	}

	reporter := NewHTTPSettledReporter(SettledReporterConfig{Endpoint: ts.URL})
	err := reporter.ReportSettled(context.Background(), SettledReport{Reason: "big group", Jobs: jobs})
	require.NoError(t, err)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, chunkSizes, 3, "2500 ids at chunk size 1000 must be 3 requests")
	assert.Equal(t, []int{1000, 1000, 500}, chunkSizes)
	assert.Len(t, allIDs, total, "every id must have been sent exactly once across the chunks")
}

// TestHTTPSettledReporter_NonOKIsReported proves a non-200 response from the
// hook surfaces as an error from ReportSettled (which pool.go's
// reportSettled goroutine then only logs — this test pins the reporter's
// own contract, not the fire-and-forget wrapper around it).
func TestHTTPSettledReporter_NonOKIsReported(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"settled":0}`))
	}))
	defer ts.Close()

	reporter := NewHTTPSettledReporter(SettledReporterConfig{Endpoint: ts.URL})
	err := reporter.ReportSettled(context.Background(), SettledReport{
		Reason: "r",
		Jobs:   []SettledJob{{ID: "m1", Token: "bad"}},
	})
	require.Error(t, err)
}

// TestHTTPSettledReporter_EmptyJobsIsNoOp proves an empty report never makes
// a request at all.
func TestHTTPSettledReporter_EmptyJobsIsNoOp(t *testing.T) {
	called := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	reporter := NewHTTPSettledReporter(SettledReporterConfig{Endpoint: ts.URL})
	err := reporter.ReportSettled(context.Background(), SettledReport{Reason: "r"})
	require.NoError(t, err)
	assert.False(t, called)
}
