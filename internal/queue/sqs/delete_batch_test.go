package sqs

import (
	"context"
	"encoding/json"
	"io"
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

	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// fakeSQS answers DeleteMessageBatch over the SQS JSON protocol. handler gets
// the receipt handles in entry order and returns the set of entry indexes to
// report in Failed (nil = all succeed), or writes its own failure via status != 0.
type fakeSQS struct {
	srv      *httptest.Server
	calls    atomic.Int64
	maxBatch atomic.Int64
	handler  func(receipts []string) (failIdx map[int]bool, httpStatus int)
}

func newFakeSQS(t *testing.T, h func(receipts []string) (map[int]bool, int)) *fakeSQS {
	f := &fakeSQS{handler: h}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.Header.Get("X-Amz-Target"), "DeleteMessageBatch") {
			http.Error(w, "unexpected target", 400)
			return
		}
		body, _ := io.ReadAll(r.Body)
		var in struct {
			Entries []struct{ Id, ReceiptHandle string }
		}
		_ = json.Unmarshal(body, &in)
		f.calls.Add(1)
		n := int64(len(in.Entries))
		for {
			cur := f.maxBatch.Load()
			if n <= cur || f.maxBatch.CompareAndSwap(cur, n) {
				break
			}
		}
		receipts := make([]string, len(in.Entries))
		for i, e := range in.Entries {
			receipts[i] = e.ReceiptHandle
		}
		fail, status := f.handler(receipts)
		if status != 0 {
			w.Header().Set("Content-Type", "application/x-amz-json-1.0")
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
		w.Header().Set("Content-Type", "application/x-amz-json-1.0")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeSQS) queue() *Queue {
	q := newGuardQueue()
	q.queueName = "q"
	q.queueURL = f.srv.URL + "/000000000000/q"
	q.client = sqs.NewFromConfig(aws.Config{
		Region:           "us-east-1",
		Credentials:      aws.AnonymousCredentials{},
		BaseEndpoint:     aws.String(f.srv.URL),
		RetryMaxAttempts: 1,
	})
	q.del.lingerMax = 20 * time.Millisecond // short default so partial-batch tests stay fast
	return q
}

func TestLoneAckDeletesAfterTheShortTestLinger(t *testing.T) {
	f := newFakeSQS(t, func([]string) (map[int]bool, int) { return nil, 0 })
	q := f.queue()
	defer q.Stop()

	start := time.Now()
	require.NoError(t, q.Ack(context.Background(), "r1", "m1"))
	assert.Less(t, time.Since(start), 500*time.Millisecond)
	assert.EqualValues(t, 1, q.acked.Load())
	assert.EqualValues(t, 1, f.calls.Load())
	assert.True(t, q.alreadyDeleted("m1"))
}

func TestConcurrentAcksCoalesceAndNeverExceedTenPerBatch(t *testing.T) {
	// Slow the round trip so acks pile up behind the drainers.
	f := newFakeSQS(t, func([]string) (map[int]bool, int) {
		time.Sleep(20 * time.Millisecond)
		return nil, 0
	})
	q := f.queue()
	defer q.Stop()

	const n = 500
	var wg sync.WaitGroup
	var failures atomic.Int64
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := q.Ack(context.Background(), "r"+strconv.Itoa(i), "m"+strconv.Itoa(i)); err != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Zero(t, failures.Load())
	assert.LessOrEqual(t, f.maxBatch.Load(), int64(10), "SQS hard limit")
	assert.Less(t, f.calls.Load(), int64(n), "acks must coalesce into fewer calls")
	assert.Greater(t, f.maxBatch.Load(), int64(1), "concurrent acks must actually share a batch")
	assert.EqualValues(t, n, q.acked.Load())
}

func TestPerEntryFailureFailsOnlyThatAck(t *testing.T) {
	f := newFakeSQS(t, func(receipts []string) (map[int]bool, int) {
		fail := map[int]bool{}
		for i, r := range receipts {
			if r == "bad" {
				fail[i] = true
			}
		}
		return fail, 0
	})
	q := f.queue()
	defer q.Stop()

	receipts := []string{"a", "bad", "c", "d", "bad2"}
	errs := make([]error, len(receipts))
	var wg sync.WaitGroup
	for i, r := range receipts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = q.Ack(context.Background(), r, "m-"+r)
		}()
	}
	wg.Wait()
	for i, r := range receipts {
		if r == "bad" {
			assert.Error(t, errs[i], "the failed entry's ack must fail")
		} else {
			assert.NoError(t, errs[i], "sibling %s must succeed", r)
		}
	}
	assert.EqualValues(t, 4, q.acked.Load(), "acked counts successful entries only")
}

func TestWholeCallErrorFailsEveryAckInTheBatch(t *testing.T) {
	f := newFakeSQS(t, func([]string) (map[int]bool, int) {
		time.Sleep(30 * time.Millisecond)
		return nil, http.StatusBadRequest
	})
	q := f.queue()
	defer q.Stop()

	var wg sync.WaitGroup
	var okCount atomic.Int64
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if q.Ack(context.Background(), "r"+strconv.Itoa(i), "m"+strconv.Itoa(i)) == nil {
				okCount.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Zero(t, okCount.Load())
	assert.Zero(t, q.acked.Load())
}

func TestStopFailsWaitingAcksWithoutHanging(t *testing.T) {
	release := make(chan struct{})
	f := newFakeSQS(t, func([]string) (map[int]bool, int) {
		<-release // every drainer wedged in a round trip
		return nil, 0
	})
	q := f.queue()
	defer close(release)

	const n = 200 // far more than 4 drainers x 10 can hold
	errs := make(chan error, n)
	for i := range n {
		go func() { errs <- q.Ack(context.Background(), "r"+strconv.Itoa(i), "m"+strconv.Itoa(i)) }()
	}
	time.Sleep(200 * time.Millisecond)
	q.Stop()

	for range n {
		select {
		case err := <-errs:
			assert.Error(t, err)
		case <-time.After(5 * time.Second):
			t.Fatal("an ack hung after Stop")
		}
	}
}

func TestCancelledCallerReturnsButDeleteCompletes(t *testing.T) {
	release := make(chan struct{})
	f := newFakeSQS(t, func([]string) (map[int]bool, int) {
		<-release
		return nil, 0
	})
	q := f.queue()
	defer q.Stop()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- q.Ack(ctx, "r1", "m1") }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("cancelled caller stayed blocked")
	}
	close(release)
	require.Eventually(t, func() bool { return q.acked.Load() == 1 }, 2*time.Second, 10*time.Millisecond,
		"the drainer must still complete the item")
}

func TestStalledBatchCallIsBoundedByTheCallTimeout(t *testing.T) {
	stalled := make(chan struct{})
	f := newFakeSQS(t, func([]string) (map[int]bool, int) {
		<-stalled
		return nil, 0
	})
	defer close(stalled)
	old := apiCallTimeout
	apiCallTimeout = 200 * time.Millisecond
	q := f.queue()
	defer func() { q.Stop(); q.del.drainers.Wait(); apiCallTimeout = old }()

	// Call the batch sender directly so the caller's own timeout cannot mask a
	// missing bound on the batch call itself.
	it := &deleteItem{receipt: "r1", done: make(chan error, 1)}
	start := time.Now()
	q.sendDeleteBatch(context.Background(), []*deleteItem{it})
	require.Error(t, <-it.done)
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestSteadyLoadFillsBatchesNearTen(t *testing.T) {
	f := newFakeSQS(t, func([]string) (map[int]bool, int) { return nil, 0 })
	q := f.queue()
	defer q.Stop()

	q.del.lingerMax = 5 * time.Second // batches must fill, never time out

	const n = 2000
	var wg sync.WaitGroup
	var failures atomic.Int64
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := q.Ack(context.Background(), "r"+strconv.Itoa(i), "m"+strconv.Itoa(i)); err != nil {
				failures.Add(1)
			}
		}()
		// One producer, ~100us apart (busy-wait: sleeps are far coarser).
		for end := time.Now().Add(100 * time.Microsecond); time.Now().Before(end); {
		}
	}
	wg.Wait()

	assert.Zero(t, failures.Load())
	assert.EqualValues(t, n, q.acked.Load())
	avg := float64(n) / float64(f.calls.Load())
	assert.GreaterOrEqual(t, avg, 8.0, "average batch size, calls=%d", f.calls.Load())
	assert.Less(t, f.calls.Load(), int64(n/4))
}

func TestLoneAckCompletesAtAboutTheLingerCap(t *testing.T) {
	f := newFakeSQS(t, func([]string) (map[int]bool, int) { return nil, 0 })
	q := f.queue()
	defer q.Stop()

	q.del.lingerMax = 40 * time.Millisecond

	start := time.Now()
	require.NoError(t, q.Ack(context.Background(), "r1", "m1"))
	el := time.Since(start)
	assert.GreaterOrEqual(t, el, 30*time.Millisecond, "must not be sent instantly")
	assert.Less(t, el, 100*time.Millisecond, "must not be stuck")
	assert.EqualValues(t, 1, f.calls.Load())
}

func TestBurstEngagesHelpersAndHonoursBatchAndHelperCaps(t *testing.T) {
	var inflight, maxInflight atomic.Int64
	f := newFakeSQS(t, func(r []string) (map[int]bool, int) {
		cur := inflight.Add(1)
		for {
			m := maxInflight.Load()
			if cur <= m || maxInflight.CompareAndSwap(m, cur) {
				break
			}
		}
		time.Sleep(20 * time.Millisecond)
		inflight.Add(-1)
		return nil, 0
	})
	q := f.queue()
	defer q.Stop()

	const n = 500
	var wg sync.WaitGroup
	var failures atomic.Int64
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := q.Ack(context.Background(), "r"+strconv.Itoa(i), "m"+strconv.Itoa(i)); err != nil {
				failures.Add(1)
			}
		}()
	}
	wg.Wait()

	assert.Zero(t, failures.Load())
	assert.EqualValues(t, n, q.acked.Load())
	assert.LessOrEqual(t, f.maxBatch.Load(), int64(deleteBatchMax))
	assert.GreaterOrEqual(t, maxInflight.Load(), int64(2), "helpers must engage under a burst")
	assert.LessOrEqual(t, maxInflight.Load(), int64(1+deleteMaxHelpers), "primary + capped helpers")
	require.Eventually(t, func() bool { return q.del.helpers.Load() == 0 }, 2*time.Second, 5*time.Millisecond,
		"helpers must exit when idle")
}

func TestStopWithHelpersRunningLeavesNoDrainers(t *testing.T) {
	release := make(chan struct{})
	f := newFakeSQS(t, func([]string) (map[int]bool, int) {
		<-release
		return nil, 0
	})
	q := f.queue()

	const n = 200
	errs := make(chan error, n)
	for i := range n {
		go func() { errs <- q.Ack(context.Background(), "r"+strconv.Itoa(i), "m"+strconv.Itoa(i)) }()
	}
	require.Eventually(t, func() bool { return q.del.helpers.Load() > 0 }, 2*time.Second, 5*time.Millisecond)
	q.Stop()
	close(release)
	for range n {
		select {
		case <-errs:
		case <-time.After(5 * time.Second):
			t.Fatal("an ack hung after Stop")
		}
	}
	q.del.drainers.Wait()
	assert.Zero(t, q.del.helpers.Load())
}

func TestFullBatchGoesImmediatelyWithHugeCap(t *testing.T) {
	f := newFakeSQS(t, func([]string) (map[int]bool, int) { return nil, 0 })
	q := f.queue()
	defer q.Stop()
	q.del.lingerMax = time.Hour

	start := time.Now()
	var wg sync.WaitGroup
	for i := range deleteBatchMax {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, q.Ack(context.Background(), "r"+strconv.Itoa(i), "m"+strconv.Itoa(i)))
		}()
	}
	wg.Wait()
	assert.Less(t, time.Since(start), 2*time.Second, "a full batch must not wait for the cap")
	assert.EqualValues(t, 1, f.calls.Load())
	assert.EqualValues(t, deleteBatchMax, f.maxBatch.Load())
}

func TestUrgentAckCutsTheBatchAtOnceAndIncludesCollectedAcks(t *testing.T) {
	var mu sync.Mutex
	var sizes []int
	f := newFakeSQS(t, func(r []string) (map[int]bool, int) {
		mu.Lock()
		sizes = append(sizes, len(r))
		mu.Unlock()
		return nil, 0
	})
	q := f.queue()
	defer q.Stop()
	q.del.lingerMax = time.Hour

	var wg sync.WaitGroup
	for i := range 3 { // normal acks, lingering
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.NoError(t, q.Ack(context.Background(), "n"+strconv.Itoa(i), "mn"+strconv.Itoa(i)))
		}()
	}
	time.Sleep(100 * time.Millisecond)
	require.EqualValues(t, 0, f.calls.Load(), "normal acks must still be lingering")

	start := time.Now()
	require.NoError(t, q.Ack(queue.WithUrgentAck(context.Background()), "u", "mu"))
	assert.Less(t, time.Since(start), 2*time.Second, "urgent ack must not wait for the cap")
	wg.Wait()
	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{4}, sizes, "the urgent ack is sent with the acks collected so far")
}

func TestPendingMarkerCompletesOnSuccessFailureAndDroppedEnqueue(t *testing.T) {
	f := newFakeSQS(t, func(r []string) (map[int]bool, int) {
		fail := map[int]bool{}
		for i, x := range r {
			if x == "bad" {
				fail[i] = true
			}
		}
		return fail, 0
	})
	q := f.queue()
	defer q.Stop()

	_ = q.Ack(context.Background(), "good", "m-good")
	_ = q.Ack(context.Background(), "bad", "m-bad")
	q.mu.Lock()
	assert.True(t, q.pendingDelete["m-good"].done)
	assert.True(t, q.pendingDelete["m-bad"].done, "a failed delete is also completed (forgotten after the grace)")
	q.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	q.del.stop()
	_ = q.Ack(ctx, "x", "m-x")
	q.mu.Lock()
	defer q.mu.Unlock()
	assert.True(t, q.pendingDelete["m-x"].done, "an ack that never enqueued must not stay in flight forever")
}
