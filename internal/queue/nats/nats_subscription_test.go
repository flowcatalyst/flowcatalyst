package nats

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// startTestQueue spins up a fresh embedded (in-process) NATS server with
// JetStream enabled, provisions a Queue against it, and registers cleanup.
// Each test gets its own server + stream so tests never share state.
func startTestQueue(t *testing.T, maxMessages int) *Queue {
	t.Helper()

	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      -1, // random free port
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("nats-server: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats-server: did not become ready for connections")
	}
	t.Cleanup(srv.Shutdown)

	uri := fmt.Sprintf("nats://%s?stream=TEST&consumer=c&subject=test.>&max-messages=%d&ack-wait-secs=5",
		srv.Addr().String(), maxMessages)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	q, err := newQueue(ctx, common.QueueConfig{URI: uri})
	if err != nil {
		t.Fatalf("newQueue: %v", err)
	}
	t.Cleanup(q.Stop)
	return q
}

// publishN publishes n test messages (ids m-0..m-(n-1), in order) to q.
func publishN(t *testing.T, q *Queue, n int) {
	t.Helper()
	ctx := context.Background()
	for i := range n {
		_, err := q.Publish(ctx, common.Message{
			ID:              fmt.Sprintf("m-%d", i),
			MediationType:   common.MediationTypeHTTP,
			MediationTarget: "http://example.invalid/webhook",
		})
		if err != nil {
			t.Fatalf("publish %d: %v", i, err)
		}
	}
}

// waitForBuffered blocks until at least n messages are sitting in q.msgCh
// (pulled by forward() off the continuous subscription) or the timeout
// elapses. Used to get messages pre-buffered BEFORE the timed portion of a
// test starts, so what's being timed is genuinely "already buffered", not
// "arrived over the network during the timed window".
func waitForBuffered(t *testing.T, q *Queue, n int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(q.msgCh) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d buffered messages (have %d)", n, len(q.msgCh))
}

type pollResult struct {
	msgs    []common.QueuedMessage
	err     error
	elapsed time.Duration
}

// (a) Buffered messages are returned in order, up to max, without waiting.
//
// This is the assertion that pins "continuous subscription, not a
// per-poll broker round-trip": Poll must be a channel read against
// messages forward() already pulled, not a fresh Fetch. Break it by
// reverting Poll to issue a jetstream.Fetch per call (the old behaviour)
// and this test starts failing the <100ms bound (a Fetch is a real
// network round-trip, and — with nothing new published — would have to
// wait out FetchMaxWait before returning what it has).
func TestPollReturnsBufferedMessagesInOrderWithoutWaiting(t *testing.T) {
	q := startTestQueue(t, 10)
	publishN(t, q, 5)
	waitForBuffered(t, q, 5, 5*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	msgs, err := q.Poll(ctx, 10)
	elapsed := time.Since(start)

	require.NoError(t, err)
	require.Len(t, msgs, 5)
	assert.Less(t, elapsed, 100*time.Millisecond,
		"already-buffered messages must be handed back immediately, not after a broker round-trip")
	for i, m := range msgs {
		assert.Equal(t, fmt.Sprintf("m-%d", i), m.Message.ID,
			"messages must come back in publish order")
	}
}

// (b) Poll on an empty stream blocks until a publish, then returns
// promptly. Pins "block untimed on the first message" — a poller with a
// fixed poll-timeout would either busy-loop or wait out that timeout
// before noticing the publish; this asserts it notices close to
// immediately instead.
func TestPollBlocksUntilPublishThenReturnsPromptly(t *testing.T) {
	q := startTestQueue(t, 10)

	resultCh := make(chan pollResult, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		start := time.Now()
		msgs, err := q.Poll(ctx, 10)
		resultCh <- pollResult{msgs: msgs, err: err, elapsed: time.Since(start)}
	}()

	// Confirm Poll is genuinely parked, not returning empty-handed.
	select {
	case r := <-resultCh:
		t.Fatalf("Poll returned before anything was published: %+v", r)
	case <-time.After(300 * time.Millisecond):
	}

	publishStart := time.Now()
	publishN(t, q, 1)

	select {
	case r := <-resultCh:
		require.NoError(t, r.err)
		require.Len(t, r.msgs, 1)
		assert.Equal(t, "m-0", r.msgs[0].Message.ID)
		assert.Less(t, time.Since(publishStart), 2*time.Second,
			"Poll must return promptly once a message exists, not after some fixed poll interval")
	case <-time.After(5 * time.Second):
		t.Fatal("Poll never returned after the publish")
	}
}

// (c) Stop while a Poll is blocked returns within 500ms with ErrStopped.
//
// Pins "Stop() closes the subscription and unblocks a parked Poll". Break
// it by making Stop not close stopCh (or not call msgsCtx.Stop) and Poll
// hangs until its own context deadline instead of reacting to Stop.
func TestStopUnblocksParkedPollWithin500ms(t *testing.T) {
	q := startTestQueue(t, 10)

	resultCh := make(chan pollResult, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		start := time.Now()
		msgs, err := q.Poll(ctx, 10)
		resultCh <- pollResult{msgs: msgs, err: err, elapsed: time.Since(start)}
	}()

	// Let Poll actually reach its parked select before we Stop.
	time.Sleep(150 * time.Millisecond)

	stopStart := time.Now()
	q.Stop()

	select {
	case r := <-resultCh:
		assert.ErrorIs(t, r.err, queue.ErrStopped)
		assert.Empty(t, r.msgs)
		assert.Less(t, time.Since(stopStart), 500*time.Millisecond,
			"a parked Poll must unblock within 500ms of Stop")
	case <-time.After(1 * time.Second):
		t.Fatal("Poll never unblocked after Stop")
	}
}

// (d) With the channel full, the subscription stops requesting more:
// num_ack_pending stays bounded to a couple of batches, never anywhere
// near the full backlog, and most of the backlog stays un-pulled
// (num_pending > 0) on the broker.
//
// Mutant: make msgCh unbounded (e.g. `make(chan common.QueuedMessage)`
// with no capacity limit, or a capacity far larger than max-messages) and
// this test fails — nothing then stops forward() from draining the
// entire backlog into the channel, so num_ack_pending climbs toward the
// full publish count instead of staying near 2×max-messages.
func TestFullChannelStopsRequestingMoreBatches(t *testing.T) {
	const max = 5
	const total = 50 // 10x max-messages
	q := startTestQueue(t, max)
	publishN(t, q, total)

	// Give forward() ample time to pull as much as it's ever going to
	// without anything draining msgCh via Poll.
	time.Sleep(1500 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := q.consumer.Info(ctx)
	require.NoError(t, err)

	t.Logf("num_ack_pending=%d num_pending=%d (max-messages=%d, published=%d)",
		info.NumAckPending, info.NumPending, max, total)

	assert.LessOrEqual(t, int(info.NumAckPending), 3*max,
		"num_ack_pending must stay bounded to roughly one or two batches, not grow toward the full backlog")
	assert.Greater(t, info.NumPending, uint64(0),
		"most of the backlog must still be sitting un-pulled on the broker")
}

// Poll never returns (nil, nil): manager.go's empty-batch 1s pause
// (runConsumer, len(msgs)==0 branch) is unreachable for this backend
// because Poll always returns at least one message or a non-nil error.
// This is the behaviour the package doc promises; pin it directly rather
// than only inferring it from the blocking tests above.
func TestPollNeverReturnsEmptyWithoutError(t *testing.T) {
	q := startTestQueue(t, 10)

	// Cancelled context: must return an error, never (nil, nil) or ([], nil).
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	msgs, err := q.Poll(ctx, 10)
	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Empty(t, msgs)

	// Stopped consumer: must return ErrStopped, never (nil, nil).
	q.Stop()
	msgs, err = q.Poll(context.Background(), 10)
	require.Error(t, err)
	assert.ErrorIs(t, err, queue.ErrStopped)
	assert.Empty(t, msgs)
}
