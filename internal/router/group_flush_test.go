package router

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

func TestGroupFlushRegistrySuppressesThenExpires(t *testing.T) {
	r := NewGroupFlushRegistry()

	assert.False(t, r.Suppressed("g1"), "unflushed group must not be suppressed")
	require.True(t, r.Flush("g1", 50*time.Millisecond))
	assert.True(t, r.Suppressed("g1"))

	// The window lapses on its own — no resume protocol — so the next
	// message probes the target.
	time.Sleep(70 * time.Millisecond)
	assert.False(t, r.Suppressed("g1"), "suppression must lapse without an explicit clear")

	active, _, _ := r.Stats()
	assert.Equal(t, 0, active, "expired entry must be evicted")
}

func TestGroupFlushRegistryTTLBounds(t *testing.T) {
	r := NewGroupFlushRegistry()

	// Non-positive → default window.
	require.True(t, r.Flush("g_default", 0))
	until, ok := r.SuppressedUntil("g_default")
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(DefaultFlushTTL), until, time.Second)

	// Over the cap → clamped. A target cannot silence a group indefinitely;
	// it must keep re-flushing on each probe.
	require.True(t, r.Flush("g_huge", 10*time.Hour))
	until, ok = r.SuppressedUntil("g_huge")
	require.True(t, ok)
	assert.WithinDuration(t, time.Now().Add(MaxFlushTTL), until, time.Second)
}

func TestGroupFlushRegistryReflushNeverShortensWindow(t *testing.T) {
	r := NewGroupFlushRegistry()
	require.True(t, r.Flush("g1", time.Minute))
	before, ok := r.SuppressedUntil("g1")
	require.True(t, ok)

	// A shorter re-flush (e.g. a probe racing an in-flight sibling) must not
	// pull the expiry in.
	assert.False(t, r.Flush("g1", time.Second))
	after, ok := r.SuppressedUntil("g1")
	require.True(t, ok)
	assert.Equal(t, before, after)

	// A longer one extends it.
	require.True(t, r.Flush("g1", 2*time.Minute))
	extended, _ := r.SuppressedUntil("g1")
	assert.True(t, extended.After(before))
}

func TestGroupFlushRegistryUngroupedIsNoOp(t *testing.T) {
	r := NewGroupFlushRegistry()
	// An ungrouped message has no siblings to suppress; flushing "" must
	// never create a bucket that swallows every other ungrouped message.
	assert.False(t, r.Flush("", time.Minute))
	assert.False(t, r.Suppressed(""))
	active, flushes, _ := r.Stats()
	assert.Equal(t, 0, active)
	assert.Equal(t, uint64(0), flushes)
}

func TestGroupFlushRegistryClearAndStats(t *testing.T) {
	r := NewGroupFlushRegistry()
	require.True(t, r.Flush("g1", time.Minute))
	require.True(t, r.Flush("g2", time.Minute))
	assert.True(t, r.Suppressed("g1"))
	assert.True(t, r.Suppressed("g2"))

	r.Clear("g1")
	assert.False(t, r.Suppressed("g1"))

	active, flushes, suppressed := r.Stats()
	assert.Equal(t, 1, active)
	assert.Equal(t, uint64(2), flushes)
	assert.Equal(t, uint64(2), suppressed, "two Suppressed hits before the clear")
}

// flushMediator succeeds for everything, and asks for a group flush on the
// message whose ID == flushOn.
type flushMediator struct {
	flushOn      string
	delaySeconds int

	mu   sync.Mutex
	seen []string
}

func (m *flushMediator) Mediate(_ context.Context, msg *common.Message) common.MediationOutcome {
	m.mu.Lock()
	m.seen = append(m.seen, msg.ID)
	flush := msg.ID == m.flushOn
	m.mu.Unlock()

	out := common.MediationOutcome{Result: common.MediationSuccess}
	if flush {
		out.FlushGroup = true
		out.DelaySeconds = m.delaySeconds
	}
	return out
}

func (m *flushMediator) delivered() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.seen...)
}

func flushTestMessage(id, group string) common.QueuedMessage {
	g := group
	return common.QueuedMessage{
		Message: common.Message{
			ID:              id,
			MediationType:   common.MediationTypeHTTP,
			MediationTarget: "http://example.invalid",
			MessageGroupID:  &g,
			DispatchMode:    common.DispatchBlockOnError,
		},
		ReceiptHandle: id,
	}
}

// TestPoolFlushGroupAcksSiblingsWithoutDelivering is the end-to-end contract:
// once the target flushes a group, the remaining messages are ACKed off the
// queue without any delivery — spending no rate-limit token and no
// concurrency slot — instead of being fired one by one.
func TestPoolFlushGroupAcksSiblingsWithoutDelivering(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 5, done: make(chan struct{})}
	med := &flushMediator{flushOn: "m1"}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	msgs := []common.QueuedMessage{
		flushTestMessage("m1", "g"), flushTestMessage("m2", "g"),
		flushTestMessage("m3", "g"), flushTestMessage("m4", "g"),
		flushTestMessage("m5", "g"),
	}
	submitBatch(context.Background(), pool, msgs)

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for terminal actions")
	}

	assert.Equal(t, []string{"m1"}, med.delivered(),
		"only the flushing message may reach the target")

	cons.mu.Lock()
	acked := append([]string(nil), cons.acked...)
	nacked := len(cons.nacked)
	cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"m1", "m2", "m3", "m4", "m5"}, acked,
		"every message in a flushed group is ACKed")
	assert.Zero(t, nacked, "flushing must not NACK anything back to the broker")
}

// A flushed group only silences ITS OWN messages — an unrelated group keeps
// delivering normally.
func TestPoolFlushGroupIsScopedToTheGroup(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &flushMediator{flushOn: "a1"}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		flushTestMessage("a1", "ga"),
		flushTestMessage("a2", "ga"), // suppressed
		flushTestMessage("b1", "gb"), // unaffected
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for terminal actions")
	}
	assert.ElementsMatch(t, []string{"a1", "b1"}, med.delivered(),
		"the other group must keep flowing")
}

// After the window lapses the next message is a probe: it reaches the target
// again, so a recovered group resumes with no resume call from the target.
func TestPoolFlushGroupProbesAfterWindow(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 2, done: make(chan struct{})}
	med := &flushMediator{flushOn: "never"}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	// Flush directly so the window can be sub-second (delaySeconds is whole
	// seconds on the wire).
	require.True(t, pool.flushes.Flush("g", 80*time.Millisecond))

	submitBatch(context.Background(), pool, []common.QueuedMessage{flushTestMessage("s1", "g")})
	time.Sleep(150 * time.Millisecond)
	submitBatch(context.Background(), pool, []common.QueuedMessage{flushTestMessage("s2", "g")})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for terminal actions")
	}
	assert.Equal(t, []string{"s2"}, med.delivered(),
		"s1 suppressed inside the window; s2 probes after it lapses")
}
