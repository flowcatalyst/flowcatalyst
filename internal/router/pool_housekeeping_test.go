package router

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// ackedIDs is a copy of what cons has acked so far.
func ackedIDs(cons *cascadeConsumer) []string {
	cons.mu.Lock()
	defer cons.mu.Unlock()
	return append([]string(nil), cons.acked...)
}

// requireGroupGoneAndReusable checks the state a settled group must leave:
// no entry in groupQs and nothing counted as queued, and then that the same
// group id, submitted again, is delivered as a group never seen would be.
// nextID must be a message the test's mediator delivers successfully.
func requireGroupGoneAndReusable(t *testing.T, pool *Pool, cons *cascadeConsumer, group, nextID string) {
	t.Helper()
	require.Eventually(t, func() bool { return pool.MessageGroupCount() == 0 }, time.Second, 2*time.Millisecond,
		"a settled group must not keep an entry in groupQs (groups: %+v)", pool.GroupSnapshot())
	assert.Empty(t, pool.GroupSnapshot())
	assert.Zero(t, pool.QueueSize())

	submitBatch(context.Background(), pool, []common.QueuedMessage{releaseMsg(nextID, group, common.DispatchNextOnError)})
	require.Eventually(t, func() bool { return slices.Contains(ackedIDs(cons), nextID) }, 3*time.Second, 2*time.Millisecond,
		"a later message of the same group must be delivered")
	assert.Eventually(t, func() bool { return pool.MessageGroupCount() == 0 && pool.QueueSize() == 0 },
		time.Second, 2*time.Millisecond, "and its group must be gone again once it has drained")
}

// A group whose head finds the target unreachable is released whole, and its
// entry goes with it. The drainer returns straight after the release, so an
// entry left behind (empty, working cleared) lived until the same group id was
// submitted again — for group ids that never recur, for the life of the
// process, counted by MessageGroupCount and listed by GroupSnapshot.
func TestReleasedGroupLeavesNoEntry(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	out := common.ErrorConnection("refused")
	pool := newCascadePool(&cascadeMediator{failID: "m1", failWith: &out}, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		releaseMsg("m1", "g", common.DispatchNextOnError),
		releaseMsg("m2", "g", common.DispatchNextOnError),
		releaseMsg("m3", "g", common.DispatchNextOnError),
	})
	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the group was not released")
	}
	cons.mu.Lock()
	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, cons.nacked)
	cons.mu.Unlock()

	requireGroupGoneAndReusable(t, pool, cons, "g", "m4")
}

// A BLOCK_ON_ERROR head that fails terminally takes its buffered siblings
// with it (acked away), and the group's entry goes too.
func TestBlockedGroupLeavesNoEntry(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	pool := newCascadePool(&cascadeMediator{failID: "m1", failWith: rejected(400)}, func(string) queue.Consumer { return cons })

	submitBatch(context.Background(), pool, []common.QueuedMessage{
		releaseMsg("m1", "g", common.DispatchBlockOnError),
		releaseMsg("m2", "g", common.DispatchBlockOnError),
		releaseMsg("m3", "g", common.DispatchBlockOnError),
	})
	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the group was not acked away")
	}
	assert.ElementsMatch(t, []string{"m1", "m2", "m3"}, ackedIDs(cons))

	requireGroupGoneAndReusable(t, pool, cons, "g", "m4")
}

// sizeProbeMediator records the pool's queueSize as each message enters
// delivery, holds the message named hold until release is closed, and
// reports every message it has finished on seen.
type sizeProbeMediator struct {
	pool    *Pool
	hold    string
	held    chan struct{}
	release chan struct{}
	seen    chan string

	mu    sync.Mutex
	sizes map[string]uint32
}

func (m *sizeProbeMediator) Mediate(ctx context.Context, msg *common.Message) common.MediationOutcome {
	m.mu.Lock()
	m.sizes[msg.ID] = m.pool.QueueSize()
	m.mu.Unlock()
	if msg.ID == m.hold {
		close(m.held)
		select {
		case <-m.release:
		case <-ctx.Done():
		}
	}
	m.seen <- msg.ID
	return common.Success(200)
}

func (m *sizeProbeMediator) sizeAt(id string) uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sizes[id]
}

// queueSize counts a message into a group buffer under the same hold of p.mu
// that puts it there. Counted after the unlock, a drainer could pop the
// message and count it down first, and the Uint32 wrapped to about 4 billion
// until the increment landed: the pool read as over capacity (submit deferred
// messages it had room for), capacityChanged flipped full, and capacityFreed
// fired again on the way back for a pool that had never been full.
//
// The window between the unlock and the increment has nothing in it a test
// can stop on for a single message. For a run it had: the run's increments
// were made one at a time, and the first one's capacityChanged can run
// capacityFreed before the second. The test makes that call happen (full
// set, standing for a pool that was full when its concurrency was raised), and
// in it lets the group's drainer, which is busy with an earlier message, go on
// to pop the run. Before the fix the drainer counted both members down
// against one increment and delivered the second with queueSize at 2^32-1.
func TestQueueSizeNeverDropsBelowWhatIsBuffered(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &sizeProbeMediator{
		hold: "h", held: make(chan struct{}), release: make(chan struct{}),
		seen: make(chan string, 8), sizes: map[string]uint32{},
	}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })
	med.pool = pool
	ctx := context.Background()

	// A drainer for g is busy delivering h.
	submitBatch(ctx, pool, []common.QueuedMessage{releaseMsg("h", "g", common.DispatchNextOnError)})
	select {
	case <-med.held:
	case <-time.After(3 * time.Second):
		t.Fatal("h was not delivered")
	}

	var freed atomic.Int32
	var once sync.Once
	pool.SetCapacityFreed(func() {
		freed.Add(1)
		once.Do(func() {
			close(med.release)
			// The drainer finishes h and goes on to the run's members.
			deadline := time.After(3 * time.Second)
			for {
				select {
				case id := <-med.seen:
					if id == "b" {
						return
					}
				case <-deadline:
					t.Error("the drainer did not deliver the run while capacityFreed ran")
					return
				}
			}
		})
	})
	pool.full.Store(true)

	pool.submitRun(ctx, "g", []common.QueuedMessage{
		releaseMsg("a", "g", common.DispatchNextOnError),
		releaseMsg("b", "g", common.DispatchNextOnError),
	})
	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the group was not delivered")
	}

	assert.Equal(t, uint32(1), med.sizeAt("a"), "with a delivering, b is still buffered and must be counted")
	assert.Equal(t, uint32(0), med.sizeAt("b"), "queueSize wrapped below zero")
	assert.Equal(t, int32(1), freed.Load(), "capacityFreed fires once, for the crossing the test set up, and not for a wrap")
	assert.Zero(t, pool.QueueSize())
}

// groupGateMediator holds every delivery until release is closed, reports each
// message entering delivery on entered, and keeps the most deliveries of one
// group it has held at once.
type groupGateMediator struct {
	release chan struct{}
	entered chan string

	mu        sync.Mutex
	inFlight  map[string]int
	maxFlight map[string]int
}

func newGroupGateMediator() *groupGateMediator {
	return &groupGateMediator{
		release: make(chan struct{}), entered: make(chan string, 16),
		inFlight: map[string]int{}, maxFlight: map[string]int{},
	}
}

func (m *groupGateMediator) Mediate(ctx context.Context, msg *common.Message) common.MediationOutcome {
	group := msg.GroupID()
	m.mu.Lock()
	m.inFlight[group]++
	m.maxFlight[group] = max(m.maxFlight[group], m.inFlight[group])
	m.mu.Unlock()
	m.entered <- msg.ID
	select {
	case <-m.release:
	case <-ctx.Done():
	}
	m.mu.Lock()
	m.inFlight[group]--
	m.mu.Unlock()
	return common.Success(200)
}

func (m *groupGateMediator) groupInFlight(group string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.inFlight[group]
}

func (m *groupGateMediator) groupMaxFlight(group string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.maxFlight[group]
}

// hookedNackConsumer is a cascadeConsumer that runs onFirstNack, once, at the
// start of the first Nack it is sent.
type hookedNackConsumer struct {
	cascadeConsumer
	once        sync.Once
	onFirstNack func(rh string)
}

func (c *hookedNackConsumer) Nack(ctx context.Context, rh string, delay *uint32) error {
	c.once.Do(func() { c.onFirstNack(rh) })
	return c.cascadeConsumer.Nack(ctx, rh, delay)
}

// ReleaseParkedGroups chose its groups under p.mu and took them after letting
// it go. A drainer that claimed a chosen group in between (a broker
// redelivery's resume, Manager.route's tryDrainGroup kick) popped the parked
// head and began delivering it; the take then released the rest of the
// buffer behind it and cleared its working flag while it still ran. The
// released sibling, redelivered at once by a broker that does not hold a
// delayed return (NATS), then started a second drainer for the group, and two
// members of one ordered group were in delivery at the same time.
//
// The gap is reached deterministically through the sweep's own first nack:
// with two parked groups, the second is resumed while the first is being
// nacked.
func TestReleaseParkedGroupsLeavesAGroupADrainerClaimedInTheGap(t *testing.T) {
	med := newGroupGateMediator()
	cons := &hookedNackConsumer{wantTotal: 99, done: make(chan struct{})}
	pool := NewPool(common.PoolConfig{Code: "test", Concurrency: 4}, med, nil,
		func(string) queue.Consumer { return cons })
	ctx := context.Background()

	pool.mu.Lock()
	for _, g := range []string{"a", "b"} {
		pool.groupQs[g] = &groupQueue{
			msgs: []common.QueuedMessage{
				releaseMsg(g+"1", g, common.DispatchNextOnError),
				releaseMsg(g+"2", g, common.DispatchNextOnError),
			},
			parkedAt: time.Now().Add(-time.Hour),
		}
	}
	pool.mu.Unlock()
	pool.queueSize.Store(4)

	var resumed string // the group resumed in the gap
	var resumedHead string
	cons.onFirstNack = func(rh string) {
		resumed = "b"
		if rh[:1] == "b" {
			resumed = "a"
		}
		pool.tryDrainGroup(ctx, resumed) // the redelivery resume
		pool.mu.Lock()
		gq := pool.groupQs[resumed]
		claimed := gq != nil && gq.working
		pool.mu.Unlock()
		if !claimed {
			return // the sweep had already taken the group: nothing to resume
		}
		select {
		case resumedHead = <-med.entered:
		case <-time.After(3 * time.Second):
			t.Error("the resumed group's drainer did not start delivering")
		}
	}

	released := pool.ReleaseParkedGroups(ctx, time.Minute)

	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	cons.mu.Unlock()
	assert.Empty(t, resumedHead, "a group the sweep chose must not also be delivered")
	assert.Equal(t, 4, released)
	assert.ElementsMatch(t, []string{"a1", "a2", "b1", "b2"}, nacked)
	assert.Zero(t, pool.MessageGroupCount())
	assert.Zero(t, pool.QueueSize())

	// The broker hands the second member of the resumed group straight back
	// (NATS does not hold a nack's delay), and it is routed as a new message.
	second := resumed + "2"
	submitBatch(ctx, pool, []common.QueuedMessage{releaseMsg(second, resumed, common.DispatchNextOnError)})
	select {
	case id := <-med.entered:
		assert.Equal(t, second, id)
	case <-time.After(3 * time.Second):
		t.Fatal("the redelivered member was not delivered")
	}
	assert.Equal(t, 1, med.groupInFlight(resumed),
		"two members of one ordered group are in delivery at once: the group has two drainers")
	close(med.release)
	assert.Eventually(t, func() bool {
		return pool.QueueSize() == 0 && pool.ActiveWorkers() == 0 && pool.MessageGroupCount() == 0
	},
		3*time.Second, 2*time.Millisecond)
	assert.Equal(t, 1, med.groupMaxFlight(resumed))
}
