package router

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// These tests drive one pool against simBroker, a broker model run on a fake
// clock. A message handed back (Defer or Nack) is held until the clock
// reaches its delay and is then "polled" again: the test submits it to the
// pool as Manager.route would a copy that registers as new. With honours
// false the model is NATS: nothing keeps a group in order, a hand-back's
// delay is all there is, and messages due at the same second come back in
// the worst order (the one handed back last first). With honours true it
// stands for Postgres or SQS at the pool's level; those brokers' own group
// locks are not modelled.

// fakeClock is a settable clock for Pool.now.
type fakeClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *fakeClock) advanceTo(t time.Time) {
	c.mu.Lock()
	if t.After(c.t) {
		c.t = t
	}
	c.mu.Unlock()
}

// simHandback is one Defer or Nack the broker was sent.
type simHandback struct {
	id    string
	kind  string // "defer" or "nack"
	delay uint32
	at    time.Time // the fake clock when it was sent
}

type simParked struct {
	qm        common.QueuedMessage
	visibleAt time.Time
	seq       int
}

type simBroker struct {
	clock   *fakeClock
	honours bool

	mu        sync.Mutex
	known     map[string]common.QueuedMessage // by receipt handle
	total     int                             // messages published so far
	out       []simParked
	seq       int
	acked     []string
	handbacks []simHandback
}

func newSimBroker(clock *fakeClock, honours bool) *simBroker {
	return &simBroker{clock: clock, honours: honours, known: map[string]common.QueuedMessage{}}
}

// publish makes ordered messages of group known to the broker and returns
// them, in order, for the test to submit.
func (b *simBroker) publish(group string, ids ...string) []common.QueuedMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]common.QueuedMessage, 0, len(ids))
	for _, id := range ids {
		m := releaseMsg(id, group, common.DispatchNextOnError)
		m.QueueIdentifier = "sim"
		b.known[m.ReceiptHandle] = m
		b.total++
		out = append(out, m)
	}
	return out
}

func (b *simBroker) handBack(rh, kind string, delay *uint32) {
	var d uint32
	if delay != nil {
		d = *delay
	}
	now := b.clock.Now()
	b.mu.Lock()
	defer b.mu.Unlock()
	m := b.known[rh]
	b.handbacks = append(b.handbacks, simHandback{id: m.Message.ID, kind: kind, delay: d, at: now})
	b.seq++
	b.out = append(b.out, simParked{qm: m, visibleAt: now.Add(time.Duration(d) * time.Second), seq: b.seq})
}

func (b *simBroker) Defer(_ context.Context, rh string, delay *uint32) error {
	b.handBack(rh, "defer", delay)
	return nil
}

func (b *simBroker) Nack(_ context.Context, rh string, delay *uint32) error {
	b.handBack(rh, "nack", delay)
	return nil
}

func (b *simBroker) Ack(_ context.Context, rh string, _ string) error {
	b.mu.Lock()
	b.acked = append(b.acked, b.known[rh].Message.ID)
	b.mu.Unlock()
	return nil
}

func (b *simBroker) Poll(context.Context, uint32) ([]common.QueuedMessage, error) { return nil, nil }
func (b *simBroker) Identifier() string                                           { return "sim" }
func (b *simBroker) HonoursDelayedReturn() bool                                   { return b.honours }
func (b *simBroker) Healthy() bool                                                { return true }
func (b *simBroker) Stop()                                                        {}
func (b *simBroker) Metrics(context.Context) (*queue.Metrics, error)              { return &queue.Metrics{}, nil }
func (b *simBroker) Counters() *queue.Metrics                                     { return nil }

// handbacksOf is every hand-back of id so far, oldest first.
func (b *simBroker) handbacksOf(id string) []simHandback {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []simHandback
	for _, h := range b.handbacks {
		if h.id == id {
			out = append(out, h)
		}
	}
	return out
}

func (b *simBroker) allHandbacks() []simHandback {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]simHandback(nil), b.handbacks...)
}

// settled reports whether every published message is acked or out at the
// broker, so nothing is still on its way through the pool.
func (b *simBroker) settled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.acked)+len(b.out) == b.total
}

// next takes the message the broker redelivers next, advancing the clock to
// when it is visible. Ties go to the one handed back last: the worst order a
// broker without group locks may choose.
func (b *simBroker) next() (common.QueuedMessage, bool) {
	b.mu.Lock()
	if len(b.out) == 0 {
		b.mu.Unlock()
		return common.QueuedMessage{}, false
	}
	sort.SliceStable(b.out, func(i, j int) bool {
		if !b.out[i].visibleAt.Equal(b.out[j].visibleAt) {
			return b.out[i].visibleAt.Before(b.out[j].visibleAt)
		}
		return b.out[i].seq > b.out[j].seq
	})
	p := b.out[0]
	b.out = b.out[1:]
	b.mu.Unlock()
	b.clock.advanceTo(p.visibleAt)
	return p.qm, true
}

// dueBatch takes every message visible by the earliest one's time plus
// window, in the worst order (latest handed back first), advancing the clock.
func (b *simBroker) dueBatch(window time.Duration) []common.QueuedMessage {
	b.mu.Lock()
	if len(b.out) == 0 {
		b.mu.Unlock()
		return nil
	}
	earliest := b.out[0].visibleAt
	for _, p := range b.out {
		if p.visibleAt.Before(earliest) {
			earliest = p.visibleAt
		}
	}
	cutoff := earliest.Add(window)
	var batch []simParked
	var rest []simParked
	for _, p := range b.out {
		if !p.visibleAt.After(cutoff) {
			batch = append(batch, p)
		} else {
			rest = append(rest, p)
		}
	}
	b.out = rest
	b.mu.Unlock()
	sort.Slice(batch, func(i, j int) bool { return batch[i].seq > batch[j].seq })
	b.clock.advanceTo(cutoff)
	out := make([]common.QueuedMessage, len(batch))
	for i, p := range batch {
		out[i] = p.qm
	}
	return out
}

// simMediator records the order of successful deliveries and fails the ids
// in fail, each the given number of times, with its outcome.
type simMediator struct {
	mu        sync.Mutex
	delivered []string
	fail      map[string]*simFailure
}

type simFailure struct {
	times   int
	outcome common.MediationOutcome
}

func (m *simMediator) Mediate(_ context.Context, msg *common.Message) common.MediationOutcome {
	m.mu.Lock()
	defer m.mu.Unlock()
	if f := m.fail[msg.ID]; f != nil && f.times > 0 {
		f.times--
		return f.outcome
	}
	m.delivered = append(m.delivered, msg.ID)
	return common.Success(200)
}

func (m *simMediator) order() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.delivered...)
}

// newSimPool is a concurrency-1 pool whose source is broker, on the broker's
// clock, measured as completing a delivery every 10ms so a capacity deferral
// is booked at the deferralMinDelay floor.
func newSimPool(t *testing.T, broker *simBroker, med Mediator) *Pool {
	t.Helper()
	p := NewPool(common.PoolConfig{Code: "sim", Concurrency: 1}, med, nil,
		func(string) queue.Consumer { return broker })
	p.now = broker.clock.Now
	p.metrics.mu.Lock()
	now := time.Now()
	for i := 500; i > 0; i-- {
		p.metrics.samples = append(p.metrics.samples,
			metricSample{ts: now.Add(-time.Duration(i) * 10 * time.Millisecond), durationMs: 1, success: true})
	}
	p.metrics.mu.Unlock()
	return p
}

// submitWithRoom admits run as one poll batch's members of group into a pool
// that has room for exactly room more messages.
func submitWithRoom(p *Pool, room uint32, group string, run []common.QueuedMessage) {
	pad := p.queueCapacity() - room
	p.queueSize.Add(pad)
	p.submitRun(context.Background(), group, run)
	p.queueSize.Add(^(pad - 1)) // -pad
}

// settle waits until nothing is on its way through the pool.
func settle(t *testing.T, p *Pool, b *simBroker) {
	t.Helper()
	require.Eventually(t, func() bool {
		return b.settled() && p.MessageGroupCount() == 0 && p.ActiveWorkers() == 0
	}, 3*time.Second, time.Millisecond, "the pool did not settle")
}

// runBrokerToEnd lets the broker redeliver everything it holds, one message
// per poll, until nothing is left out.
func runBrokerToEnd(t *testing.T, p *Pool, b *simBroker) {
	t.Helper()
	for range 500 {
		settle(t, p, b)
		m, ok := b.next()
		if !ok {
			return
		}
		p.submitRun(context.Background(), m.Message.GroupID(), []common.QueuedMessage{m})
	}
	t.Fatal("the broker was still redelivering after 500 polls")
}

// handbackGroupCount is how many groups the pool's hand-back gate holds.
func handbackGroupCount(p *Pool) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.handbacks.groups)
}

func secondsAfter(start time.Time, h simHandback) time.Duration {
	return h.at.Add(time.Duration(h.delay) * time.Second).Sub(start)
}

// Sequence 1: a later member must not overtake one the pool deferred. Batch
// [m1, m2] arrives with room for one: m1 is admitted and m2 deferred. The
// pool then has room, and m3, published later, is polled while m2 is still
// out. Admitting m3 delivered it before m2 came back.
func TestHandbackGateLaterMemberWaitsForADeferredOne(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	broker := newSimBroker(clock, false)
	med := &simMediator{}
	p := newSimPool(t, broker, med)

	submitWithRoom(p, 1, "g", broker.publish("g", "m1", "m2"))
	settle(t, p, broker)
	require.Len(t, broker.handbacksOf("m2"), 1, "m2 is deferred for capacity")

	clock.advanceTo(clock.Now().Add(time.Second))
	p.submitRun(context.Background(), "g", broker.publish("g", "m3"))
	runBrokerToEnd(t, p, broker)

	assert.Equal(t, []string{"m1", "m2", "m3"}, med.order())
	assert.Zero(t, handbackGroupCount(p), "the gate must hold nothing once the group has drained")
}

// Sequence 2: a deferred member must not overtake a released head. m2 is
// deferred for about 5s; m1, the head, is then released for an unreachable
// target with 30s. m2 came back first and was delivered before m1.
func TestHandbackGateDeferredMemberWaitsForAReleasedHead(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	broker := newSimBroker(clock, false)
	med := &simMediator{fail: map[string]*simFailure{
		"m1": {times: 1, outcome: common.ErrorConnection("refused")},
	}}
	p := newSimPool(t, broker, med)

	submitWithRoom(p, 1, "g", broker.publish("g", "m1", "m2"))
	settle(t, p, broker)
	require.Len(t, broker.handbacksOf("m1"), 1, "m1 is released")
	require.Len(t, broker.handbacksOf("m2"), 1, "m2 is deferred")

	runBrokerToEnd(t, p, broker)

	assert.Equal(t, []string{"m1", "m2"}, med.order())
	assert.Zero(t, handbackGroupCount(p))
}

// Sequence 3a: deferralMinDelay floored several short reservations of one
// group to the same 5s, so they came back in whatever order the broker
// chose. Each member of a group handed back together must be due at least
// deferralOrderedSpacing after the one before it, and they must come back
// and be delivered in group order.
func TestHandbackGateSpacesAGroupDeferredAtTheFloor(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	broker := newSimBroker(clock, false)
	med := &simMediator{}
	p := newSimPool(t, broker, med)
	start := clock.Now()

	ids := []string{"m1", "m2", "m3", "m4", "m5"}
	submitWithRoom(p, 0, "g", broker.publish("g", ids...))
	settle(t, p, broker)

	var prev time.Duration
	for i, id := range ids {
		hb := broker.handbacksOf(id)
		require.Len(t, hb, 1)
		due := secondsAfter(start, hb[0])
		if i > 0 {
			assert.GreaterOrEqual(t, due-prev, deferralOrderedSpacing,
				"%s is due %s, not a spacing after the member before it (%s)", id, due, prev)
		}
		prev = due
	}

	runBrokerToEnd(t, p, broker)
	assert.Equal(t, ids, med.order())
	assert.Zero(t, handbackGroupCount(p))
}

// Sequence 3b: past the horizon a reservation is random jitter over its last
// quarter, so two members of one group could be given delays in the wrong
// order. A group's members must still be due in group order (equal only at
// the horizon itself), and must be delivered in group order.
func TestHandbackGateKeepsAGroupInOrderPastTheHorizon(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	broker := newSimBroker(clock, false)
	med := &simMediator{}
	p := newSimPool(t, broker, med)
	const horizon = 10 * time.Minute
	p.SetMaxDeferral(horizon)
	start := clock.Now()
	p.queueSize.Store(100)
	for range 1000 { // push the schedule's cursor (a second a slot) far past the horizon
		p.admissionDelay(start, false, true)
	}
	p.queueSize.Store(0)

	ids := []string{"m1", "m2", "m3", "m4", "m5", "m6", "m7", "m8", "m9", "m10"}
	submitWithRoom(p, 0, "g", broker.publish("g", ids...))
	settle(t, p, broker)

	var prev time.Duration
	for i, id := range ids {
		hb := broker.handbacksOf(id)
		require.Len(t, hb, 1)
		due := secondsAfter(start, hb[0])
		assert.LessOrEqual(t, due, horizon, "never past the horizon")
		if i > 0 && due < horizon {
			assert.GreaterOrEqual(t, due-prev, deferralOrderedSpacing,
				"%s is due %s, not a spacing after the member before it (%s)", id, due, prev)
		}
		if i > 0 {
			assert.GreaterOrEqual(t, due, prev, "%s is due before the member ahead of it", id)
		}
		prev = due
	}

	runBrokerToEnd(t, p, broker)
	assert.Equal(t, ids, med.order())
	assert.Zero(t, handbackGroupCount(p))
}

// A released group's members on a broker without group locks come back
// spaced in group order (the head with its own delay, each sibling no earlier
// than the head and a spacing after the one before it), and a batch that
// brings several of them back at once in the wrong order is admitted in
// group order.
func TestHandbackGateReleasedGroupComesBackInOrder(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	broker := newSimBroker(clock, false)
	med := &simMediator{fail: map[string]*simFailure{
		"m1": {times: 1, outcome: common.ErrorConnection("refused")},
	}}
	p := newSimPool(t, broker, med)
	start := clock.Now()

	p.submitRun(context.Background(), "g", broker.publish("g", "m1", "m2", "m3"))
	settle(t, p, broker)

	var dues []time.Duration
	for _, id := range []string{"m1", "m2", "m3"} {
		hb := broker.handbacksOf(id)
		require.Len(t, hb, 1)
		require.Equal(t, "nack", hb[0].kind)
		dues = append(dues, secondsAfter(start, hb[0]))
	}
	assert.Equal(t, 30*time.Second, dues[0], "the head keeps the outcome's own delay")
	assert.GreaterOrEqual(t, dues[1], dues[0]+deferralOrderedSpacing)
	assert.GreaterOrEqual(t, dues[2], dues[1]+deferralOrderedSpacing)

	// All three come back in one poll, last first.
	batch := broker.dueBatch(time.Minute)
	require.Len(t, batch, 3)
	p.submitRun(context.Background(), "g", batch)
	settle(t, p, broker)
	runBrokerToEnd(t, p, broker)

	assert.Equal(t, []string{"m1", "m2", "m3"}, med.order())
	assert.Zero(t, handbackGroupCount(p))
}

// A member handed back that never returns (acked as a duplicate,
// dead-lettered, its process restarted) holds its group back for at most
// handbackReturnGrace past when it was due. After that the group moves on:
// a later member is admitted and delivered, and if the lost member does come
// back after all it is admitted as a new one.
func TestHandbackGateExpiresAMemberThatNeverReturns(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	broker := newSimBroker(clock, false)
	med := &simMediator{}
	p := newSimPool(t, broker, med)

	submitWithRoom(p, 1, "g", broker.publish("g", "m1", "m2"))
	settle(t, p, broker)
	m2 := broker.handbacksOf("m2")
	require.Len(t, m2, 1)
	m2Due := m2[0].at.Add(time.Duration(m2[0].delay) * time.Second)
	// The broker loses m2 from here on: it is never redelivered.
	broker.mu.Lock()
	broker.out = nil
	broker.total--
	broker.mu.Unlock()

	clock.advanceTo(clock.Now().Add(time.Second))
	p.submitRun(context.Background(), "g", broker.publish("g", "m3"))
	settle(t, p, broker)
	require.NotEmpty(t, broker.handbacksOf("m3"), "m3 waits behind m2 while m2 may still return")

	runBrokerToEnd(t, p, broker)
	assert.Equal(t, []string{"m1", "m3"}, med.order())
	hb := broker.handbacksOf("m3")
	last := hb[len(hb)-1]
	lastDue := last.at.Add(time.Duration(last.delay) * time.Second)
	assert.True(t, lastDue.After(m2Due.Add(handbackReturnGrace)),
		"m3 was admitted at %s, before m2 (due %s) was given up on", lastDue, m2Due)
	assert.LessOrEqual(t, lastDue, m2Due.Add(handbackReturnGrace+deferralMinDelay+deferralOrderedSpacing),
		"m3 was held back past the bound on a member that never returned")
	assert.Zero(t, handbackGroupCount(p), "the expired entry is gone")

	// m2 turns up after all: it is admitted as new, not wedged behind
	// anything.
	broker.mu.Lock()
	broker.total++
	broker.mu.Unlock()
	p.submitRun(context.Background(), "g", []common.QueuedMessage{broker.known["m2"]})
	settle(t, p, broker)
	assert.Equal(t, []string{"m1", "m3", "m2"}, med.order())
	assert.Zero(t, handbackGroupCount(p))
}

// On a broker that keeps a group in order itself (Postgres, SQS:
// HonoursDelayedReturn true) the pool behaves exactly as before the gate:
// the later member is admitted at once, a capacity deferral is booked at
// the schedule's own delay, a released head's siblings carry the sibling
// delay unchanged, and the gate records nothing.
func TestHandbackGateIsInertForABrokerThatOrdersGroups(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	broker := newSimBroker(clock, true)
	med := &simMediator{}
	p := newSimPool(t, broker, med)

	submitWithRoom(p, 1, "g", broker.publish("g", "m1", "m2", "m3"))
	settle(t, p, broker)
	for _, id := range []string{"m2", "m3"} {
		hb := broker.handbacksOf(id)
		require.Len(t, hb, 1)
		assert.Equal(t, uint32(deferralMinDelay/time.Second), hb[0].delay,
			"%s keeps the schedule's own delay: no spacing on a broker that orders groups", id)
	}
	assert.Zero(t, handbackGroupCount(p))

	p.submitRun(context.Background(), "g", broker.publish("g", "m4"))
	settle(t, p, broker)
	assert.Equal(t, []string{"m1", "m4"}, med.order(), "m4 is admitted at once, as before")
	assert.Zero(t, handbackGroupCount(p))

	// A release: the siblings carry siblingNackDelay(head) exactly.
	med.mu.Lock()
	med.fail = map[string]*simFailure{"r1": {times: 1, outcome: common.ErrorConnection("refused")}}
	med.mu.Unlock()
	p.submitRun(context.Background(), "h", broker.publish("h", "r1", "r2", "r3"))
	settle(t, p, broker)
	for _, id := range []string{"r1", "r2", "r3"} {
		hb := broker.handbacksOf(id)
		require.Len(t, hb, 1)
		assert.Equal(t, uint32(30), hb[0].delay, "%s", id)
	}
	assert.Zero(t, handbackGroupCount(p))

	runBrokerToEnd(t, p, broker)
	assert.Zero(t, handbackGroupCount(p))
	assert.Len(t, broker.allHandbacks(), 5, "nothing is handed back but the two deferrals and the release")
}

// The gate keeps nothing for a group whose members never come back once they
// are overdue by the grace, even if the group is never seen again: the
// reaper's tick (ReleaseParkedGroups) sweeps it, and so does the next gated
// admission of any other group.
func TestHandbackGateForgetsGroupsNothingComesBackFor(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	broker := newSimBroker(clock, false)
	p := newSimPool(t, broker, &simMediator{})

	for i := range 50 {
		g := "g" + strconv.Itoa(i)
		submitWithRoom(p, 0, g, broker.publish(g, g+"a", g+"b"))
	}
	settle(t, p, broker)
	require.Equal(t, 50, handbackGroupCount(p))

	// Within the grace nothing is forgotten.
	clock.advanceTo(clock.Now().Add(time.Minute))
	p.ReleaseParkedGroups(context.Background(), time.Hour)
	require.Equal(t, 50, handbackGroupCount(p))

	// Past it, the reaper's tick clears them all. (A hundred ordered
	// deferrals on this broker are booked a second apart, so the last is due
	// about 105s out.)
	clock.advanceTo(clock.Now().Add(handbackReturnGrace + 2*time.Minute))
	p.ReleaseParkedGroups(context.Background(), time.Hour)
	assert.Zero(t, handbackGroupCount(p))

	// The same through another group's admission.
	for i := range 10 {
		g := "h" + strconv.Itoa(i)
		submitWithRoom(p, 0, g, broker.publish(g, g+"a"))
	}
	settle(t, p, broker)
	require.Equal(t, 10, handbackGroupCount(p))
	clock.advanceTo(clock.Now().Add(2*handbackReturnGrace + time.Hour))
	p.submitRun(context.Background(), "other", broker.publish("other", "o1"))
	settle(t, p, broker)
	assert.Zero(t, handbackGroupCount(p))
}

// A parked group of a gated broker released by the reaper's sweep is listed
// with the gate and comes back spaced and in order.
func TestHandbackGateParkedGroupReleaseIsListed(t *testing.T) {
	clock := &fakeClock{t: time.Now()}
	broker := newSimBroker(clock, false)
	med := &simMediator{}
	p := newSimPool(t, broker, med)
	start := clock.Now()

	msgs := broker.publish("g", "m1", "m2", "m3")
	p.mu.Lock()
	gq := p.groupLocked("g")
	gq.msgs = msgs
	gq.gated = true
	gq.parkedAt = time.Now().Add(-time.Hour)
	p.countBufferedLocked(len(msgs))
	p.mu.Unlock()

	require.Equal(t, 3, p.ReleaseParkedGroups(context.Background(), time.Minute))
	settle(t, p, broker)
	require.Equal(t, 1, handbackGroupCount(p))
	var prev time.Duration
	for i, id := range []string{"m1", "m2", "m3"} {
		hb := broker.handbacksOf(id)
		require.Len(t, hb, 1)
		due := secondsAfter(start, hb[0])
		if i == 0 {
			assert.Equal(t, time.Duration(siblingNackDelaySeconds)*time.Second, due)
		} else {
			assert.GreaterOrEqual(t, due, prev+deferralOrderedSpacing)
		}
		prev = due
	}

	batch := broker.dueBatch(time.Minute)
	require.Len(t, batch, 3)
	p.submitRun(context.Background(), "g", batch)
	settle(t, p, broker)
	assert.Equal(t, []string{"m1", "m2", "m3"}, med.order())
	assert.Zero(t, handbackGroupCount(p))
}
