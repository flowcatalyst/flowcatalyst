package passwordhash

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

var cheapParams = Params{Memory: 1024, Iterations: 1, Parallelism: 1, KeyLength: 32, SaltLength: 16}

// withSlots installs a derivation-slot channel of size k plus a concurrency
// probe for the duration of the test.
func withSlots(t *testing.T, k int) (maxSeen *atomic.Int64) {
	t.Helper()
	oldSlots, oldHook := kdfSlots, kdfEntered
	kdfSlots = make(chan struct{}, k)
	var cur, peak atomic.Int64
	kdfEntered = func() func() {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(15 * time.Millisecond) // hold the slot so overlap is observable
		return func() { cur.Add(-1) }
	}
	t.Cleanup(func() { kdfSlots, kdfEntered = oldSlots, oldHook })
	return &peak
}

// L2: no more than the slot count of Argon2 derivations ever run at once, and
// every caller still gets the right answer.
func TestVerifyConcurrencyIsBounded(t *testing.T) {
	enc, err := HashWithParams("pw-slots", cheapParams)
	if err != nil {
		t.Fatal(err)
	}
	const k = 2
	peak := withSlots(t, k)

	var wg sync.WaitGroup
	errs := make(chan error, 16)
	for range 16 {
		wg.Go(func() { errs <- VerifyContext(context.Background(), "pw-slots", enc) })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("verify: %v", err)
		}
	}
	if got := peak.Load(); got > k || got < 1 {
		t.Fatalf("peak concurrent derivations = %d, want 1..%d", got, k)
	}
}

// L2: a caller waiting for a slot gives up with its context, promptly, and a
// pre-cancelled context never runs the derivation.
func TestVerifyContextCancelledWhileWaiting(t *testing.T) {
	enc, err := HashWithParams("pw-wait", cheapParams)
	if err != nil {
		t.Fatal(err)
	}
	_ = dummyHash() // computed lazily on first use; do it before the slot is taken
	peak := withSlots(t, 1)
	kdfSlots <- struct{}{} // the only slot is taken
	defer func() { <-kdfSlots }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err = VerifyContext(ctx, "pw-wait", enc)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("waited %v after the context ended", d)
	}

	cctx, ccancel := context.WithCancel(context.Background())
	ccancel()
	if err := EqualizeTimingContext(cctx, "x"); !errors.Is(err, context.Canceled) {
		t.Fatalf("EqualizeTimingContext on a cancelled ctx = %v, want context.Canceled", err)
	}
	if peak.Load() != 0 {
		t.Fatalf("a derivation ran despite the slot being taken / ctx cancelled")
	}
}

func TestSlotCountDefault(t *testing.T) {
	if c := cap(kdfSlots); c < 4 {
		t.Fatalf("slot capacity = %d, want >= 4", c)
	}
}
