package router

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// TestTrackerShardIsCacheLinePadded pins trackerShardPad's arithmetic: if a
// field is added to trackerShard (or sync.Mutex changes size) the pad must be
// recomputed, or neighbouring shards' locks share a cache line again.
func TestTrackerShardIsCacheLinePadded(t *testing.T) {
	assert.Equal(t, uintptr(128), unsafe.Sizeof(trackerShard{}))
	assert.Zero(t, trackerShards&(trackerShards-1), "shard count must be a power of two (index is a mask)")
}

// assertShardInvariant checks the sharding invariant documented on
// InFlightTracker: every byMessage entry sits in its message id's shard, and
// every byBroker alias sits in the same shard as — and points at the very
// entry held by — its message's byMessage slot.
func assertShardInvariant(t *testing.T, tr *InFlightTracker) {
	t.Helper()
	for i := range tr.shards {
		s := &tr.shards[i]
		s.mu.Lock()
		for id, im := range s.byMessage {
			assert.True(t, s == tr.shardFor(id), "byMessage[%s] is in the wrong shard", id)
			assert.Equal(t, id, im.MessageID)
		}
		for key, im := range s.byBroker {
			assert.True(t, s == tr.shardFor(im.MessageID), "byBroker[%v] is in the wrong shard", key)
			assert.True(t, im == s.byMessage[im.MessageID], "byBroker[%v] is not its message's live entry", key)
		}
		s.mu.Unlock()
	}
}

func trackerEntries(tr *InFlightTracker) (byMessage, byBroker int) {
	for i := range tr.shards {
		s := &tr.shards[i]
		s.mu.Lock()
		byMessage += len(s.byMessage)
		byBroker += len(s.byBroker)
		s.mu.Unlock()
	}
	return byMessage, byBroker
}

// TestInFlightTrackerConcurrentRegisterRequeueRemove drives every hot-path
// operation from many goroutines at once (run it under -race). For each
// message id, several copies race to Register: redeliveries of one broker
// message and an external requeue under a second broker id. Whatever order
// they land in, exactly one copy must own the pipeline, every other copy of
// the owner's broker message must be a redelivery, and every copy under the
// other broker id must be an external requeue — the same verdicts the
// single-lock tracker gave, because both broker ids of one message id are
// classified in that id's one shard. Readers (Count, Snapshot, CurrentReceipt,
// CountForQueue, Reap with bounds nothing reaches) run throughout, and after
// every message is Removed the tracker must be completely empty, aliases
// included.
func TestInFlightTrackerConcurrentRegisterRequeueRemove(t *testing.T) {
	const (
		messages     = 2000
		redeliveries = 4 // copies under the first broker id
		requeues     = 2 // copies under the second broker id
	)
	tr := NewInFlightTracker()

	type verdicts struct {
		owners             atomic.Int32
		ownerBroker        atomic.Value // string
		redelivery, extReq [2]atomic.Int32
	}
	results := make([]verdicts, messages)

	stop := make(chan struct{})
	var readers sync.WaitGroup
	for r := range 4 {
		readers.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				_ = tr.Count()
				_ = tr.DeferredCount()
				_ = tr.Snapshot()
				_ = tr.CountForQueue("q-" + strconv.Itoa(r))
				_, _ = tr.CurrentReceipt("m-"+strconv.Itoa(r), "")
				_ = tr.Reap(time.Hour, 24*time.Hour)
			}
		})
	}

	var writers sync.WaitGroup
	for i := range messages {
		msg := common.Message{ID: "m-" + strconv.Itoa(i)}
		queue := "q-" + strconv.Itoa(i%4)
		brokers := [2]string{fmt.Sprintf("b1-%d", i), fmt.Sprintf("b2-%d", i)}
		copies := map[int]int{0: redeliveries, 1: requeues}
		for which, n := range copies {
			for c := range n {
				writers.Go(func() {
					im := common.NewInFlightMessage(&msg, brokers[which], queue, "", fmt.Sprintf("rh-%d-%d", which, c))
					v := &results[i]
					switch tr.Register(im) {
					case RegisterNew:
						v.owners.Add(1)
						v.ownerBroker.Store(brokers[which])
						// The pool's process-time backstop finds the entry.
						assert.True(t, tr.EnsureTracked(im))
					case RegisterRedelivery:
						v.redelivery[which].Add(1)
					case RegisterExternalRequeue:
						v.extReq[which].Add(1)
					}
				})
			}
		}
	}
	writers.Wait()

	assertShardInvariant(t, tr)
	require.Equal(t, messages, tr.Count())

	for i := range results {
		v := &results[i]
		require.Equal(t, int32(1), v.owners.Load(), "message %d: exactly one copy owns the pipeline", i)
		owner := 0
		if v.ownerBroker.Load().(string) == fmt.Sprintf("b2-%d", i) {
			owner = 1
		}
		other := 1 - owner
		want := map[int]int{0: redeliveries, 1: requeues}
		assert.Equal(t, int32(want[owner]-1), v.redelivery[owner].Load(), "message %d: owner-broker copies are redeliveries", i)
		assert.Zero(t, v.extReq[owner].Load(), "message %d", i)
		assert.Equal(t, int32(want[other]), v.extReq[other].Load(), "message %d: other-broker copies are external requeues", i)
		assert.Zero(t, v.redelivery[other].Load(), "message %d", i)
	}

	for i := range messages {
		writers.Go(func() { tr.Remove("m-"+strconv.Itoa(i), "") })
	}
	writers.Wait()
	close(stop)
	readers.Wait()

	assert.Zero(t, tr.Count())
	assert.Empty(t, tr.Snapshot())
	nm, nb := trackerEntries(tr)
	assert.Zero(t, nm, "no byMessage entry may survive Remove")
	assert.Zero(t, nb, "no byBroker alias may survive Remove (it would dedup-drop a later copy)")
}

// TestInFlightTrackerWholeTrackerViewsSpanShards: the counting and snapshot
// views must see entries in every shard, not just one.
func TestInFlightTrackerWholeTrackerViewsSpanShards(t *testing.T) {
	tr := NewInFlightTracker()
	const n = 4 * trackerShards
	used := map[*trackerShard]bool{}
	for i := range n {
		msg := common.Message{ID: "m-" + strconv.Itoa(i)}
		im := common.NewInFlightMessage(&msg, "b-"+strconv.Itoa(i), "q", "", "rh")
		if i%2 == 0 {
			im.StartedAt = time.Now().Add(-time.Hour)
		}
		require.Equal(t, RegisterNew, tr.Register(im))
		used[tr.shardFor(msg.ID)] = true
	}
	require.Greater(t, len(used), trackerShards/2, "ids should spread over most shards")
	tr.MarkDeferred("m-1", time.Now().Add(time.Minute))

	assert.Equal(t, n-1, tr.Count())
	assert.Equal(t, 1, tr.DeferredCount())
	assert.Len(t, tr.Snapshot(), n)
	assert.Equal(t, n, tr.CountForQueue("q"))
	assert.Equal(t, n/2, tr.CountForQueueBefore("q", time.Now().Add(-time.Minute)))

	// Reap's idle bound catches the back-dated half (their LastSeenAt equals
	// StartedAt) across every shard; the deferred one is exempt anyway.
	for i := 0; i < n; i += 2 {
		im := tr.shardFor("m-" + strconv.Itoa(i)).byMessage["m-"+strconv.Itoa(i)]
		im.LastSeenAt = im.StartedAt
	}
	assert.Equal(t, n/2, tr.Reap(30*time.Minute, 24*time.Hour))
	assert.Equal(t, n/2-1, tr.Count())
	assertShardInvariant(t, tr)
}

// BenchmarkInFlightTrackerParallel is the per-message hot path (Register at
// route, EnsureTracked at dispatch, CurrentReceipt + Remove at ack) from
// GOMAXPROCS goroutines at once.
func BenchmarkInFlightTrackerParallel(b *testing.B) {
	tr := NewInFlightTracker()
	var seq atomic.Uint64
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			id := strconv.FormatUint(seq.Add(1), 36)
			msg := common.Message{ID: id}
			im := common.NewInFlightMessage(&msg, id, "q", "", "rh")
			tr.Register(im)
			tr.EnsureTrackedMessage(&msg, id, "q", "", "rh")
			tr.CurrentReceipt(id, id)
			tr.Remove(id, id)
		}
	})
}
