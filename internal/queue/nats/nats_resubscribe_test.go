package nats

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// --- a minimal fake jetstream.Msg, just enough for toQueuedMessage ---

// fakeMsg implements jetstream.Msg with a real stream sequence and body;
// every method besides Metadata/Data is a no-op stub, since forward's
// happy path never calls the rest (Ack/Nack/Term are exercised elsewhere,
// against the real embedded-server harness in nats_subscription_test.go).
type fakeMsg struct {
	streamSeq uint64
	body      []byte
}

func newFakeMsg(t *testing.T, streamSeq uint64, m common.Message) *fakeMsg {
	t.Helper()
	body, err := json.Marshal(m)
	require.NoError(t, err)
	return &fakeMsg{streamSeq: streamSeq, body: body}
}

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{Sequence: jetstream.SequencePair{Stream: m.streamSeq}}, nil
}
func (m *fakeMsg) Data() []byte                     { return m.body }
func (m *fakeMsg) Headers() natsgo.Header           { return nil }
func (m *fakeMsg) Subject() string                  { return "test" }
func (m *fakeMsg) Reply() string                    { return "" }
func (m *fakeMsg) Ack() error                       { return nil }
func (m *fakeMsg) DoubleAck(context.Context) error  { return nil }
func (m *fakeMsg) Nak() error                       { return nil }
func (m *fakeMsg) NakWithDelay(time.Duration) error { return nil }
func (m *fakeMsg) InProgress() error                { return nil }
func (m *fakeMsg) Term() error                      { return nil }
func (m *fakeMsg) TermWithReason(string) error      { return nil }

// --- a scriptable, call-counting fake messagesIterator ---

// nextResult is one scripted answer for countingIterator.Next: either a
// message or an error, never both.
type nextResult struct {
	msg jetstream.Msg
	err error
}

// countingIterator.Next returns whatever the next entry in results says
// to, and counts every call (including ones a test never inspects the
// result of) so a test can assert forward called Next exactly as many
// times as expected — no more, no fewer. Once results is exhausted it
// blocks until stopCh closes, simulating a genuinely idle-but-healthy
// subscription (never erroring on its own just because the script ran
// out).
type countingIterator struct {
	mu      sync.Mutex
	calls   int
	results []nextResult
	stopCh  chan struct{}
}

func (it *countingIterator) Next(...jetstream.NextOpt) (jetstream.Msg, error) {
	it.mu.Lock()
	idx := it.calls
	it.calls++
	it.mu.Unlock()
	if idx < len(it.results) {
		r := it.results[idx]
		return r.msg, r.err
	}
	<-it.stopCh
	return nil, jetstream.ErrMsgIteratorClosed
}

func (it *countingIterator) Stop() {}

func (it *countingIterator) callCount() int {
	it.mu.Lock()
	defer it.mu.Unlock()
	return it.calls
}

// alwaysErrIterator.Next always returns the same error immediately —
// "the iterator is dead and nothing will ever come out of it again".
type alwaysErrIterator struct{ err error }

func (it alwaysErrIterator) Next(...jetstream.NextOpt) (jetstream.Msg, error) { return nil, it.err }
func (it alwaysErrIterator) Stop()                                            {}

// newBareQueue builds a Queue with just enough wired up to exercise
// forward/Poll/Stop directly, without a live broker connection. Tests own
// msgsCtx and resubscribe. room is pre-filled with maxMessages permits,
// matching newQueue's real construction (G17).
func newBareQueue(maxMessages int) *Queue {
	room := make(chan struct{}, maxMessages)
	for range maxMessages {
		room <- struct{}{}
	}
	q := &Queue{
		cfg:         Config{MaxMessagesPerPoll: maxMessages},
		room:        room,
		msgCh:       make(chan common.QueuedMessage, maxMessages),
		stopCh:      make(chan struct{}),
		forwardDone: make(chan struct{}),
		pending:     make(map[string]jetstream.Msg),
	}
	q.running.Store(true)
	q.healthy.Store(true)
	return q
}

// TestForwardNeverCallsNextWhileChannelFull pins G17's core invariant
// directly: forward must not call Next again once msgCh is at capacity —
// it has to wait for Poll to free a room permit first. This is the
// regression test for BOTH earlier defects (calling Next then blocking on
// the send; delivering via a callback that then blocks the library's own
// goroutine) — in this design there is no way to over-fetch because the
// permit is acquired before Next is ever called.
//
// Mutant: acquire the room permit AFTER Next instead of before (or drop
// the acquire entirely) and this test fails — Next gets called far more
// than maxMessages times before anything drains the channel, because
// nothing stops it.
func TestForwardNeverCallsNextWhileChannelFull(t *testing.T) {
	const maxMessages = 3
	q := newBareQueue(maxMessages)
	it := &countingIterator{stopCh: q.stopCh}
	for i := range 20 {
		it.results = append(it.results, nextResult{msg: newFakeMsg(t, uint64(i+1), common.Message{
			ID: "m", MediationType: common.MediationTypeHTTP, MediationTarget: "http://x",
		})})
	}
	q.msgsCtx = it
	go q.forward()
	t.Cleanup(q.Stop)

	require.Eventually(t, func() bool { return len(q.msgCh) == maxMessages }, time.Second, time.Millisecond,
		"forward must fill msgCh to capacity")
	// Settle: give a buggy implementation time to over-fetch before we check.
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, maxMessages, it.callCount(),
		"forward must not call Next beyond msgCh's capacity while nothing drains it — 20 more messages were available to wrongly fetch")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msgs, err := q.Poll(ctx, 1)
	require.NoError(t, err)
	require.Len(t, msgs, 1)

	require.Eventually(t, func() bool { return it.callCount() == maxMessages+1 }, time.Second, time.Millisecond,
		"draining exactly one message must let forward pull exactly one more, not a burst")
}

// TestNonTerminalNextErrDoesNotMarkUnhealthyAndContinues pins the other
// half of G17: nats.go re-issues the pull request internally when a
// message-free Next call reports a missing heartbeat — this is exactly
// the "long park expires idle, next call re-requests" case the
// permit-gating enables (no Next in flight while forward waits for room,
// so nothing to time out until Next is genuinely waiting on the broker
// again). It must not be treated as a subscription failure: healthy must
// stay true throughout and delivery must continue with the very next
// message.
//
// Mutant: remove the isNonTerminalNextErr check (treat ErrNoHeartbeat
// like any other error) and this test fails — healthy flips false and a
// resubscribe is triggered for a condition the library already recovers
// from on its own.
func TestNonTerminalNextErrDoesNotMarkUnhealthyAndContinues(t *testing.T) {
	q := newBareQueue(10)
	it := &countingIterator{
		stopCh: q.stopCh,
		results: []nextResult{
			{err: jetstream.ErrNoHeartbeat},
			{msg: newFakeMsg(t, 1, common.Message{ID: "after-heartbeat-miss", MediationType: common.MediationTypeHTTP, MediationTarget: "http://x"})},
		},
	}
	q.msgsCtx = it
	resubCalls := 0
	q.resubscribe = func() (messagesIterator, error) {
		resubCalls++
		return it, nil
	}
	go q.forward()
	t.Cleanup(q.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msgs, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "after-heartbeat-miss", msgs[0].Message.ID)

	assert.True(t, q.healthy.Load(), "a missing-heartbeat notice must never mark the queue unhealthy")
	assert.Equal(t, 0, resubCalls, "a missing-heartbeat notice must never trigger a resubscribe")
}

// TestPollErrorsWhenSubscriptionUnhealthy pins G14: once forward() has
// seen a genuine (non-heartbeat) iterator error and resubscribing keeps
// failing, Poll must surface an error at the caller's deadline — never
// (nil, nil) — so the router's poll-error handling (and, eventually, the
// stall watchdog) can see the consumer is not actually making progress.
//
// Mutant: remove the `if q.healthy.Load()` guard in Poll's deadline
// branch (always return (nil, nil)) and this test fails — Poll never
// errors.
func TestPollErrorsWhenSubscriptionUnhealthy(t *testing.T) {
	q := newBareQueue(10)
	q.msgsCtx = alwaysErrIterator{err: errors.New("boom: connection dead")}
	// resubscribe never succeeds — the "can't recover" path.
	q.resubscribe = func() (messagesIterator, error) {
		return nil, errors.New("still dead")
	}
	go q.forward()
	t.Cleanup(q.Stop)

	require.Eventually(t, func() bool { return !q.healthy.Load() }, time.Second, time.Millisecond,
		"forward must mark the queue unhealthy as soon as Next errors")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	msgs, err := q.Poll(ctx, 10)
	elapsed := time.Since(start)

	require.Error(t, err, "Poll must surface an error once the subscription is unhealthy")
	assert.Contains(t, err.Error(), "boom: connection dead",
		"the error should carry forward WHY the subscription went unhealthy in the first place")
	assert.Empty(t, msgs)
	assert.Less(t, elapsed, 500*time.Millisecond, "must error at roughly one deadline, not hang")
}

// TestForwardResubscribesAndRecovers pins the other half of G14: forward
// must never let a genuine iterator error end the subscription
// permanently. When Next errors ONCE and the next resubscribe attempt
// succeeds, forward must pick back up and keep delivering — proven by an
// actual message flowing through Poll afterward, not just the healthy
// flag flipping back.
//
// Mutant: change forward's error branch back to a bare `return` and this
// test fails — Poll never gets the message because nothing is pulling
// for it any more.
func TestForwardResubscribesAndRecovers(t *testing.T) {
	q := newBareQueue(10)
	q.msgsCtx = alwaysErrIterator{err: errors.New("connection reset")}

	recovered := &countingIterator{
		stopCh: q.stopCh,
		results: []nextResult{
			{msg: newFakeMsg(t, 42, common.Message{ID: "after-recovery", MediationType: common.MediationTypeHTTP, MediationTarget: "http://x"})},
		},
	}
	resubCalls := 0
	q.resubscribe = func() (messagesIterator, error) {
		resubCalls++
		return recovered, nil
	}
	go q.forward()
	t.Cleanup(q.Stop)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	msgs, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "after-recovery", msgs[0].Message.ID)
	assert.Equal(t, "42", msgs[0].BrokerMessageID)
	assert.True(t, q.healthy.Load(), "healthy must be restored once resubscribe succeeds")
	assert.Equal(t, 1, resubCalls)
}
