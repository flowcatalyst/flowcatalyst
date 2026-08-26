package router

import (
	"container/list"
	"context"
	"sync"
	"sync/atomic"
)

// semaphore is a counting semaphore whose limit can change while it is in use.
// It is what bounds a pool's concurrent deliveries.
//
// A buffered channel is the usual Go spelling, but its capacity is fixed at
// make() time, so a hot-reloadable concurrency cap meant replacing the channel
// — and then every worker had to remember which channel it acquired from, or
// its release would credit a slot back to a channel it never took one from.
// Here the cap is just a number: acquire and release agree on nothing but this
// one object, so there is nothing to snapshot and nothing to get wrong.
//
// Waiters are served FIFO. A new limit applies to admission immediately, but
// never interrupts work already in flight: after a shrink, held may sit above
// limit until enough workers finish, and no acquire is granted meanwhile.
type semaphore struct {
	mu      sync.Mutex
	held    uint32
	waiters list.List // of chan struct{}, oldest first

	// limit is written only under mu (so notifyWaiters sees a stable value)
	// but read without it, since the capacity gauge and the pre-dispatch
	// backpressure check read it far more often than a reconfigure writes it.
	limit atomic.Uint32
}

func newSemaphore(limit uint32) *semaphore {
	s := &semaphore{}
	s.limit.Store(limit)
	return s
}

// acquire takes a slot, blocking until one is free or ctx is done. A nil error
// always pairs with exactly one release; an error means no slot was taken.
func (s *semaphore) acquire(ctx context.Context) error {
	s.mu.Lock()
	if s.held < s.limit.Load() && s.waiters.Len() == 0 {
		s.held++
		s.mu.Unlock()
		return nil
	}
	// Queue behind the waiters already there. Whoever closes ready has
	// already counted the slot as held on our behalf.
	ready := make(chan struct{})
	elem := s.waiters.PushBack(ready)
	s.mu.Unlock()

	select {
	case <-ready:
		return nil
	case <-ctx.Done():
		s.mu.Lock()
		defer s.mu.Unlock()
		select {
		case <-ready:
			// Granted concurrently with the cancellation. Pretend we didn't
			// notice it: handing the slot straight back would mean unwinding a
			// grant that already skipped every other waiter, and the caller
			// releases it a moment later anyway.
			return nil
		default:
			s.waiters.Remove(elem)
			return ctx.Err()
		}
	}
}

// release returns a slot taken by a successful acquire.
func (s *semaphore) release() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.held == 0 {
		panic("router: semaphore released more times than acquired")
	}
	s.held--
	s.notifyWaiters()
}

// setLimit changes the cap. Growing admits waiting acquirers straight away;
// shrinking only stops new ones (see the type comment).
func (s *semaphore) setLimit(n uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.limit.Store(n)
	s.notifyWaiters()
}

// capacity is the current cap.
func (s *semaphore) capacity() uint32 { return s.limit.Load() }

// notifyWaiters hands free slots to the oldest waiters. Called under mu after
// anything that can free one, which is what maintains the invariant the acquire
// fast path relies on: a non-empty waiter queue means there are no free slots.
func (s *semaphore) notifyWaiters() {
	for s.held < s.limit.Load() {
		next := s.waiters.Front()
		if next == nil {
			return
		}
		s.waiters.Remove(next)
		s.held++
		close(next.Value.(chan struct{}))
	}
}
