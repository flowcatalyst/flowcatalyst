package nats

import (
	"context"
	"encoding/json"
	"errors"
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
// every method besides Metadata/Data is a no-op stub, since handleMsg's
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

// fakeConsumeContext is a minimal jetstream.ConsumeContext: Stop is the
// only method our code ever calls on one (in Queue.Stop). Returned by a
// fake resubscribe to prove a resubscribe attempt swapped it in.
type fakeConsumeContext struct{ stopped chan struct{} }

func newFakeConsumeContext() *fakeConsumeContext {
	return &fakeConsumeContext{stopped: make(chan struct{})}
}
func (c *fakeConsumeContext) Stop()                   { close(c.stopped) }
func (c *fakeConsumeContext) Drain()                  {}
func (c *fakeConsumeContext) Closed() <-chan struct{} { return c.stopped }

// newBareQueue builds a Queue with just enough wired up to exercise
// handleMsg/handleConsumeErr/Poll/Stop directly, without a live broker
// connection. Tests own consumeCtx and resubscribe, and invoke
// handleMsg/handleConsumeErr themselves the way the library would.
func newBareQueue(maxMessages int) *Queue {
	q := &Queue{
		cfg:        Config{MaxMessagesPerPoll: maxMessages},
		msgCh:      make(chan common.QueuedMessage, maxMessages),
		stopCh:     make(chan struct{}),
		pending:    make(map[string]jetstream.Msg),
		consumeCtx: newFakeConsumeContext(),
	}
	q.running.Store(true)
	q.healthy.Store(true)
	return q
}

// TestHandleMsgDeliversToChannel is a direct, no-broker unit test of the
// Consume callback: a message handed to handleMsg (the way the library's
// delivery goroutine would call it) must come back out of Poll unchanged.
func TestHandleMsgDeliversToChannel(t *testing.T) {
	q := newBareQueue(10)
	t.Cleanup(q.Stop)

	q.handleMsg(newFakeMsg(t, 7, common.Message{ID: "m7", MediationType: common.MediationTypeHTTP, MediationTarget: "http://x"}))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	msgs, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	assert.Equal(t, "m7", msgs[0].Message.ID)
	assert.Equal(t, "7", msgs[0].BrokerMessageID)
}

// TestPollErrorsWhenSubscriptionUnhealthy pins G14/G16: once
// handleConsumeErr has seen a TERMINAL error (the ConsumeContext has
// stopped for good) and resubscribing keeps failing, Poll must surface an
// error at the caller's deadline — never (nil, nil) — so the router's
// poll-error handling (and, eventually, the stall watchdog) can see the
// consumer is not actually making progress.
//
// Mutant: remove the `if q.healthy.Load()` guard in Poll's deadline branch
// (always return (nil, nil)) and this test fails — Poll never errors.
func TestPollErrorsWhenSubscriptionUnhealthy(t *testing.T) {
	q := newBareQueue(10)
	// resubscribe never succeeds — the "can't recover" path.
	q.resubscribe = func() (jetstream.ConsumeContext, error) {
		return nil, errors.New("still dead")
	}
	t.Cleanup(q.Stop)

	q.handleConsumeErr(nil, jetstream.ErrConnectionClosed)

	require.Eventually(t, func() bool { return !q.healthy.Load() }, time.Second, time.Millisecond,
		"handleConsumeErr must mark the queue unhealthy immediately on a terminal error")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	msgs, err := q.Poll(ctx, 10)
	elapsed := time.Since(start)

	require.Error(t, err, "Poll must surface an error once the subscription is unhealthy")
	assert.Contains(t, err.Error(), "connection closed",
		"the error should carry forward WHY the subscription went unhealthy in the first place")
	assert.Empty(t, msgs)
	assert.Less(t, elapsed, 500*time.Millisecond, "must error at roughly one deadline, not hang")
}

// TestTerminalConsumeErrResubscribesAndRecovers pins the other half of
// G14/G16: a terminal error must never end the Queue permanently. When
// resubscribe succeeds, healthy must be restored and the NEW
// ConsumeContext swapped in — proven by Stop() then stopping the new one,
// not the old (already-dead) one.
//
// Mutant: change handleConsumeErr to never call resubscribeUntilHealthy
// (the pre-G16-equivalent "give up" behaviour) and this test fails —
// healthy never recovers.
func TestTerminalConsumeErrResubscribesAndRecovers(t *testing.T) {
	q := newBareQueue(10)
	oldCtx := q.consumeCtx.(*fakeConsumeContext)
	newCtx := newFakeConsumeContext()
	resubCalls := 0
	q.resubscribe = func() (jetstream.ConsumeContext, error) {
		resubCalls++
		return newCtx, nil
	}

	q.handleConsumeErr(nil, jetstream.ErrConnectionClosed)

	require.Eventually(t, func() bool { return q.healthy.Load() }, time.Second, time.Millisecond,
		"healthy must be restored once resubscribe succeeds")
	assert.Equal(t, 1, resubCalls)

	q.Stop()
	select {
	case <-newCtx.stopped:
	default:
		t.Fatal("Stop must stop the CURRENT (post-resubscribe) ConsumeContext")
	}
	select {
	case <-oldCtx.stopped:
		t.Fatal("Stop must not touch the old, already-dead ConsumeContext")
	default:
	}
}

// TestNonTerminalConsumeErrDoesNotResubscribe pins G16's narrower trigger:
// Consume recovers from a missed heartbeat (and similar transient
// conditions) internally, without ending the ConsumeContext — so
// handleConsumeErr must NOT mark the queue unhealthy or resubscribe for
// those, only log. Resubscribing unnecessarily would tear down a
// perfectly healthy subscription.
//
// Mutant: remove the isTerminalConsumeErr check (treat every error as
// terminal) and this test fails — healthy flips false and resubscribe is
// called for a no-heartbeat notice that the library already handled.
func TestNonTerminalConsumeErrDoesNotResubscribe(t *testing.T) {
	q := newBareQueue(10)
	t.Cleanup(q.Stop)
	resubCalls := 0
	q.resubscribe = func() (jetstream.ConsumeContext, error) {
		resubCalls++
		return newFakeConsumeContext(), nil
	}

	q.handleConsumeErr(nil, jetstream.ErrNoHeartbeat)

	// Give a wrongly-triggered resubscribe a chance to run before asserting.
	time.Sleep(50 * time.Millisecond)
	assert.True(t, q.healthy.Load(), "a self-recovering notice must not mark the queue unhealthy")
	assert.Equal(t, 0, resubCalls, "a self-recovering notice must not trigger a resubscribe")
}
