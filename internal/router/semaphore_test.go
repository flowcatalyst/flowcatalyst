package router

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// held reads the count of taken slots. Test-only; production code never needs
// it (the whole point is that acquire/release keep it right).
func (s *semaphore) heldCount() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.held
}

func TestSemaphoreBoundsConcurrency(t *testing.T) {
	s := newSemaphore(2)
	ctx := context.Background()
	require.NoError(t, s.acquire(ctx))
	require.NoError(t, s.acquire(ctx))

	// A third acquire must block until one of the two is released.
	acquired := make(chan struct{})
	go func() {
		_ = s.acquire(ctx)
		close(acquired)
	}()
	select {
	case <-acquired:
		t.Fatal("acquire past the cap must block")
	case <-time.After(50 * time.Millisecond):
	}

	s.release()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("release must hand the slot to the waiter")
	}
	assert.Equal(t, uint32(2), s.heldCount())
}

// A cancelled acquire must take no slot — the caller returns without ever
// calling release, so a slot leaked here would shrink the pool permanently.
func TestSemaphoreCancelledAcquireTakesNoSlot(t *testing.T) {
	s := newSemaphore(1)
	require.NoError(t, s.acquire(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	errc := make(chan error, 1)
	go func() { errc <- s.acquire(ctx) }()
	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-errc:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("cancelled acquire must return")
	}
	assert.Equal(t, uint32(1), s.heldCount(), "the abandoned waiter must not hold a slot")

	s.release()
	assert.Equal(t, uint32(0), s.heldCount())
	require.NoError(t, s.acquire(context.Background()), "the freed slot is still usable")
}

// Waiters are served oldest-first: an ordered drainer parked on the acquire
// must not be starved by a later arrival.
func TestSemaphoreServesWaitersFIFO(t *testing.T) {
	s := newSemaphore(1)
	ctx := context.Background()
	require.NoError(t, s.acquire(ctx))

	var mu sync.Mutex
	var order []int
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			require.NoError(t, s.acquire(ctx))
			mu.Lock()
			order = append(order, i)
			mu.Unlock()
			s.release()
		}(i)
		// Stagger so the queue order is deterministic.
		time.Sleep(20 * time.Millisecond)
	}
	s.release()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []int{0, 1, 2}, order)
}

// Growing the cap admits waiters straight away — this is the online resize the
// swappable channel existed to provide, without the swap.
func TestSemaphoreGrowAdmitsWaiters(t *testing.T) {
	s := newSemaphore(1)
	require.NoError(t, s.acquire(context.Background()))

	acquired := make(chan struct{})
	go func() {
		_ = s.acquire(context.Background())
		close(acquired)
	}()
	time.Sleep(20 * time.Millisecond)

	s.setLimit(2)
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("growing the cap must admit a waiting acquirer")
	}
	assert.Equal(t, uint32(2), s.capacity())
}

// Shrinking applies to admission only: work already running is never
// interrupted, so held rides above the new cap until those workers finish and
// no new acquire is granted meanwhile.
func TestSemaphoreShrinkOnlyStopsNewWork(t *testing.T) {
	s := newSemaphore(3)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		require.NoError(t, s.acquire(ctx))
	}

	s.setLimit(1)
	assert.Equal(t, uint32(3), s.heldCount(), "in-flight work must not be interrupted")

	var granted atomic.Bool
	go func() {
		_ = s.acquire(ctx)
		granted.Store(true)
	}()

	s.release() // 2 held, still over the new cap
	s.release() // 1 held, at the cap
	time.Sleep(50 * time.Millisecond)
	assert.False(t, granted.Load(), "no acquire may be granted while held is at the new cap")

	s.release() // 0 held — now there is room
	require.Eventually(t, granted.Load, time.Second, 5*time.Millisecond,
		"the waiter must be admitted once the pool has converged on the new cap")
}

// The marquee property the channel swap could not offer: resizing under load
// never loses or invents a slot. Every acquire is matched by exactly one
// release, so held must land back on zero however the cap moved meanwhile.
func TestSemaphoreResizeUnderLoadConservesSlots(t *testing.T) {
	s := newSemaphore(4)
	ctx := context.Background()

	done := make(chan struct{})
	var resizers sync.WaitGroup
	resizers.Add(1)
	go func() {
		defer resizers.Done()
		for caps, i := []uint32{1, 8, 2, 16, 3}, 0; ; i++ {
			select {
			case <-done:
				return
			default:
			}
			s.setLimit(caps[i%len(caps)])
			time.Sleep(time.Millisecond)
		}
	}()

	var workers sync.WaitGroup
	for i := 0; i < 200; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			require.NoError(t, s.acquire(ctx))
			time.Sleep(time.Millisecond)
			s.release()
		}()
	}
	workers.Wait()
	close(done)
	resizers.Wait()

	assert.Equal(t, uint32(0), s.heldCount(), "every slot must come back")
}
