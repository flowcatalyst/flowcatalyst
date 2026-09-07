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
// every method besides Metadata/Data/Term is a no-op stub, since forward's
// happy path never calls them (Ack/Nack/Term are exercised elsewhere,
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

// --- a scriptable fake messagesIterator ---

// scriptedIterator.Next returns whatever the next entry in results says to:
// a message, or an error. Once exhausted it blocks until stopCh closes,
// simulating a genuinely idle-but-healthy subscription.
type scriptedIterator struct {
	results []nextResult
	i       int
	stopCh  chan struct{}
}

type nextResult struct {
	msg jetstream.Msg
	err error
}

func (it *scriptedIterator) Next(...jetstream.NextOpt) (jetstream.Msg, error) {
	if it.i < len(it.results) {
		r := it.results[it.i]
		it.i++
		return r.msg, r.err
	}
	<-it.stopCh
	return nil, jetstream.ErrMsgIteratorClosed
}

func (it *scriptedIterator) Stop() {}

// alwaysErrIterator.Next always returns the same error immediately —
// "the iterator is dead and nothing will ever come out of it again".
type alwaysErrIterator struct{ err error }

func (it alwaysErrIterator) Next(...jetstream.NextOpt) (jetstream.Msg, error) { return nil, it.err }
func (it alwaysErrIterator) Stop()                                            {}

// newBareQueue builds a Queue with just enough wired up to exercise
// forward/Poll/Stop directly, without a live broker connection. Tests own
// msgsCtx and resubscribe.
func newBareQueue(maxMessages int) *Queue {
	q := &Queue{
		cfg:         Config{MaxMessagesPerPoll: maxMessages},
		msgCh:       make(chan common.QueuedMessage, maxMessages),
		stopCh:      make(chan struct{}),
		forwardDone: make(chan struct{}),
		pending:     make(map[string]jetstream.Msg),
	}
	q.running.Store(true)
	q.healthy.Store(true)
	return q
}

// TestPollErrorsWhenSubscriptionUnhealthy pins G14: once forward() has
// seen an iterator error and resubscribing keeps failing, Poll must
// surface an error at the caller's deadline — never (nil, nil) — so the
// router's poll-error handling (and, eventually, the stall watchdog) can
// see the consumer is not actually making progress. This is the direct
// regression test for the bug found on the bench rig: a dead subscription
// silently looking like an idle-but-healthy one.
//
// Mutant: remove the `if q.healthy.Load()` guard in Poll's deadline branch
// (always return (nil, nil)) and this test fails — Poll never errors.
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
// must never let an iterator error end the subscription permanently. When
// Next errors ONCE and the next resubscribe attempt succeeds, forward
// must pick back up and keep delivering — proven by an actual message
// flowing through Poll afterward, not just the healthy flag flipping back.
//
// Mutant: change forward's error branch back to a bare `return` (the
// pre-fix behaviour) and this test fails — Poll never gets the message
// because nothing is pulling for it any more.
func TestForwardResubscribesAndRecovers(t *testing.T) {
	q := newBareQueue(10)
	q.msgsCtx = alwaysErrIterator{err: errors.New("transient: no heartbeat received")}

	recovered := &scriptedIterator{
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
