package sqs

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type visEntry struct {
	Id                string
	ReceiptHandle     string
	VisibilityTimeout int32
}

// fakeVisSQS answers ChangeMessageVisibilityBatch over the SQS JSON protocol.
// handler receives the entries and returns the indexes to report as Failed, or a
// non-zero HTTP status to fail the whole call.
type fakeVisSQS struct {
	srv      *httptest.Server
	calls    atomic.Int64
	maxBatch atomic.Int64
	handler  func(entries []visEntry) (failIdx map[int]bool, httpStatus int)

	mu   sync.Mutex
	seen map[string]int32 // receipt -> VisibilityTimeout sent
}

func newFakeVisSQS(t *testing.T, h func([]visEntry) (map[int]bool, int)) *fakeVisSQS {
	f := &fakeVisSQS{handler: h, seen: map[string]int32{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.Header.Get("X-Amz-Target"), "ChangeMessageVisibilityBatch") {
			http.Error(w, "unexpected target", 400)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var in struct{ Entries []visEntry }
		_ = json.Unmarshal(body, &in)
		f.calls.Add(1)
		n := int64(len(in.Entries))
		for {
			cur := f.maxBatch.Load()
			if n <= cur || f.maxBatch.CompareAndSwap(cur, n) {
				break
			}
		}
		f.mu.Lock()
		for _, e := range in.Entries {
			f.seen[e.ReceiptHandle] = e.VisibilityTimeout
		}
		f.mu.Unlock()
		fail, status := f.handler(in.Entries)
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		if status != 0 {
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"__type":"AWS.SimpleQueueService.NonExistentQueue","message":"boom"}`))
			return
		}
		type ok struct{ Id string }
		type bad struct {
			Id          string
			Code        string
			SenderFault bool
			Message     string
		}
		resp := struct {
			Successful []ok
			Failed     []bad
		}{Successful: []ok{}, Failed: []bad{}}
		for i, e := range in.Entries {
			if fail[i] {
				resp.Failed = append(resp.Failed, bad{Id: e.Id, Code: "ReceiptHandleIsInvalid", SenderFault: true, Message: "nope"})
			} else {
				resp.Successful = append(resp.Successful, ok{Id: e.Id})
			}
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeVisSQS) queue() *Queue {
	q := newGuardQueue()
	q.receiptPolledAt = map[string]time.Time{}
	q.queueName = "q"
	q.queueURL = f.srv.URL + "/000000000000/q"
	q.client = sqs.NewFromConfig(aws.Config{
		Region:           "us-east-1",
		Credentials:      aws.AnonymousCredentials{},
		BaseEndpoint:     aws.String(f.srv.URL),
		RetryMaxAttempts: 1,
	})
	return q
}

// captureLogs routes slog to a buffer for the test's duration.
type logBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuf) Write(p []byte) (int, error) { l.mu.Lock(); defer l.mu.Unlock(); return l.b.Write(p) }

func (l *logBuf) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.b.String() }

func captureLogs(t *testing.T) *logBuf {
	lb := &logBuf{}
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(lb, &slog.HandlerOptions{Level: slog.LevelWarn})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return lb
}

func waitDrainersGone(t *testing.T, q *Queue) {
	done := make(chan struct{})
	go func() { q.vis.drainers.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("drainers did not exit")
	}
}

func TestManyDefersCoalesceWithTheirOwnVisibilityTimeouts(t *testing.T) {
	f := newFakeVisSQS(t, func([]visEntry) (map[int]bool, int) {
		time.Sleep(20 * time.Millisecond)
		return nil, 0
	})
	q := f.queue()
	defer q.Stop()

	const n = 300
	for i := range n {
		require.NoError(t, q.Defer(context.Background(), "r"+strconv.Itoa(i), new(uint32(i))))
	}
	require.Eventually(t, func() bool { return q.deferred.Load() == n }, 10*time.Second, 10*time.Millisecond)

	assert.LessOrEqual(t, f.maxBatch.Load(), int64(10), "SQS hard limit")
	assert.Greater(t, f.maxBatch.Load(), int64(1), "queued changes must share a batch")
	assert.Less(t, f.calls.Load(), int64(n), "changes must coalesce into fewer calls")
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Len(t, f.seen, n)
	for i := range n {
		assert.EqualValues(t, i, f.seen["r"+strconv.Itoa(i)], "each entry carries its own VisibilityTimeout")
	}
}

func TestNackWithDelayCountsAsNackedNotDeferred(t *testing.T) {
	f := newFakeVisSQS(t, func([]visEntry) (map[int]bool, int) { return nil, 0 })
	q := f.queue()
	defer q.Stop()
	require.NoError(t, q.Nack(context.Background(), "r1", new(uint32(7))))
	require.Eventually(t, func() bool { return q.nacked.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	assert.Zero(t, q.deferred.Load())
}

func TestDeferReturnsBeforeTheBrokerAnswers(t *testing.T) {
	release := make(chan struct{})
	f := newFakeVisSQS(t, func([]visEntry) (map[int]bool, int) {
		<-release
		return nil, 0
	})
	q := f.queue()
	defer q.Stop()
	defer close(release)

	start := time.Now()
	require.NoError(t, q.Defer(context.Background(), "r1", new(uint32(5))))
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	assert.Zero(t, q.deferred.Load(), "not counted until the broker answers")
	release <- struct{}{}
	require.Eventually(t, func() bool { return q.deferred.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
}

func TestWholeCallFailureLogsAndStillCounts(t *testing.T) {
	lb := captureLogs(t)
	f := newFakeVisSQS(t, func([]visEntry) (map[int]bool, int) { return nil, http.StatusBadRequest })
	q := f.queue()
	defer q.Stop()

	require.NoError(t, q.Defer(context.Background(), "r1", new(uint32(5))), "the caller cannot receive a broker error")
	require.Eventually(t, func() bool { return q.deferred.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	assert.Contains(t, lb.String(), "ChangeMessageVisibilityBatch failed")
}

func TestPerEntryFailureLogsAndStillCounts(t *testing.T) {
	lb := captureLogs(t)
	f := newFakeVisSQS(t, func(es []visEntry) (map[int]bool, int) {
		fail := map[int]bool{}
		for i, e := range es {
			if e.ReceiptHandle == "bad" {
				fail[i] = true
			}
		}
		return fail, 0
	})
	q := f.queue()
	defer q.Stop()

	for _, r := range []string{"a", "bad", "c"} {
		require.NoError(t, q.Defer(context.Background(), r, new(uint32(5))))
	}
	require.Eventually(t, func() bool { return q.deferred.Load() == 3 }, 5*time.Second, 5*time.Millisecond)
	assert.Contains(t, lb.String(), "entry failed")
	assert.Contains(t, lb.String(), "ReceiptHandleIsInvalid")
}

func TestFullQueueBlocksDeferUntilDrainedAndHonoursCtx(t *testing.T) {
	release := make(chan struct{})
	f := newFakeVisSQS(t, func([]visEntry) (map[int]bool, int) {
		<-release
		return nil, 0
	})
	q := f.queue()
	var once sync.Once
	open := func() { once.Do(func() { close(release) }) }
	// Let the drainers flush and exit so none outlives the test.
	defer func() { open(); q.Stop(); waitDrainersGone(t, q) }()

	// Wedge the drainers on the first batches, then top the channel up to capacity.
	for i := range visibilityDrainers {
		require.NoError(t, q.Defer(context.Background(), "w"+strconv.Itoa(i), new(uint32(1))))
		require.Eventually(t, func() bool { return f.calls.Load() == int64(i+1) }, 5*time.Second, time.Millisecond)
	}
	for i := range visibilityQueueDepth {
		require.NoError(t, q.Defer(context.Background(), "r"+strconv.Itoa(i), new(uint32(1))))
	}

	// ctx cancel while full.
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := q.Defer(ctx, "blocked", new(uint32(1)))
	require.ErrorIs(t, err, context.DeadlineExceeded)
	assert.GreaterOrEqual(t, time.Since(start), 100*time.Millisecond, "a full queue must block the caller")

	// An uncancelled enqueue unblocks once the broker drains.
	done := make(chan error, 1)
	go func() { done <- q.Defer(context.Background(), "blocked2", new(uint32(1))) }()
	select {
	case <-done:
		t.Fatal("Defer returned while the queue was full")
	case <-time.After(150 * time.Millisecond):
	}
	open()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Defer stayed blocked after the queue drained")
	}
}

func TestStopDoesNotHangABlockedEnqueueAndFlushesTheQueue(t *testing.T) {
	release := make(chan struct{})
	f := newFakeVisSQS(t, func([]visEntry) (map[int]bool, int) {
		<-release
		return nil, 0
	})
	q := f.queue()
	var once sync.Once
	open := func() { once.Do(func() { close(release) }) }
	defer open()

	for i := range visibilityDrainers {
		require.NoError(t, q.Defer(context.Background(), "w"+strconv.Itoa(i), new(uint32(1))))
		require.Eventually(t, func() bool { return f.calls.Load() == int64(i+1) }, 5*time.Second, time.Millisecond)
	}
	for i := range visibilityQueueDepth {
		require.NoError(t, q.Defer(context.Background(), "r"+strconv.Itoa(i), new(uint32(1))))
	}
	done := make(chan error, 1)
	go func() { done <- q.Defer(context.Background(), "blocked", new(uint32(1))) }()
	time.Sleep(100 * time.Millisecond)

	q.Stop()
	open() // the broker recovers; stop must not strand the blocked enqueue
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("an enqueue hung after Stop")
	}
	// Everything queued is flushed, then the drainers go.
	total := visibilityDrainers + visibilityQueueDepth + 1
	require.Eventually(t, func() bool { return q.deferred.Load() == uint64(total) }, 10*time.Second, 10*time.Millisecond)
	waitDrainersGone(t, q)

	// A nack after Stop still reaches the broker (the router nacks buffered
	// messages around consumer Stop).
	require.NoError(t, q.Nack(context.Background(), "late", new(uint32(3))))
	require.Eventually(t, func() bool { return q.nacked.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	waitDrainersGone(t, q)
}

func TestDrainersExitWhenIdleAndRestartOnNextDefer(t *testing.T) {
	old := visibilityIdleExit
	visibilityIdleExit = 100 * time.Millisecond
	defer func() { visibilityIdleExit = old }()

	f := newFakeVisSQS(t, func([]visEntry) (map[int]bool, int) { return nil, 0 })
	q := f.queue()
	defer q.Stop()

	require.NoError(t, q.Defer(context.Background(), "r1", new(uint32(1))))
	require.Eventually(t, func() bool { return q.deferred.Load() == 1 }, 5*time.Second, 5*time.Millisecond)
	waitDrainersGone(t, q) // exited on their own, with no Stop
	q.vis.mu.Lock()
	assert.Zero(t, q.vis.live)
	q.vis.mu.Unlock()

	require.NoError(t, q.Defer(context.Background(), "r2", new(uint32(1))))
	require.Eventually(t, func() bool { return q.deferred.Load() == 2 }, 5*time.Second, 5*time.Millisecond)
	waitDrainersGone(t, q)
}

func TestStalledVisibilityBatchIsBoundedByTheCallTimeout(t *testing.T) {
	stalled := make(chan struct{})
	f := newFakeVisSQS(t, func([]visEntry) (map[int]bool, int) {
		<-stalled
		return nil, 0
	})
	defer close(stalled)
	old := apiCallTimeout
	apiCallTimeout = 200 * time.Millisecond
	q := f.queue()
	defer func() { apiCallTimeout = old }()

	var c atomic.Uint64
	start := time.Now()
	q.sendVisibilityBatch([]visibilityItem{{receipt: "r1", seconds: 1, counter: &c}})
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.EqualValues(t, 1, c.Load())
}
