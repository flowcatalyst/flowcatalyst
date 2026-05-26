package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHostLimiter_CapHoldsUnderLoad fires N concurrent requests through
// a limiter sized for cap=K and asserts the server never sees more than
// K concurrent in-flight. This is the load-bearing correctness check
// for the per-host bound.
func TestHostLimiter_CapHoldsUnderLoad(t *testing.T) {
	const cap = 4
	const total = 40

	var current, peak int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		c := atomic.AddInt32(&current, 1)
		for {
			p := atomic.LoadInt32(&peak)
			if c <= p || atomic.CompareAndSwapInt32(&peak, p, c) {
				break
			}
		}
		// Hold the request long enough that concurrent fanout has a
		// chance to pile up if the limiter weren't enforcing.
		time.Sleep(10 * time.Millisecond)
		atomic.AddInt32(&current, -1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: newHostLimiter(http.DefaultTransport, cap)}

	var wg sync.WaitGroup
	wg.Add(total)
	for i := 0; i < total; i++ {
		go func() {
			defer wg.Done()
			req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
			resp, err := client.Do(req)
			if err != nil {
				t.Errorf("req: %v", err)
				return
			}
			resp.Body.Close()
		}()
	}
	wg.Wait()

	got := atomic.LoadInt32(&peak)
	if got > cap {
		t.Fatalf("peak concurrent=%d exceeded cap=%d", got, cap)
	}
	if got == 0 {
		t.Fatal("peak concurrent=0 — server never saw a request, test is broken")
	}
}

// TestHostLimiter_DifferentHostsDontShareQuota fires requests to two
// distinct hosts simultaneously. The limiter is sized cap=1 per host;
// without per-host isolation the two requests would serialise and
// total runtime would be ~2*sleep. With proper isolation, they run
// in parallel and total is ~1*sleep.
func TestHostLimiter_DifferentHostsDontShareQuota(t *testing.T) {
	const sleep = 80 * time.Millisecond

	handler := func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(sleep)
		w.WriteHeader(http.StatusOK)
	}
	srvA := httptest.NewServer(http.HandlerFunc(handler))
	defer srvA.Close()
	srvB := httptest.NewServer(http.HandlerFunc(handler))
	defer srvB.Close()

	client := &http.Client{Transport: newHostLimiter(http.DefaultTransport, 1)}

	start := time.Now()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodGet, srvA.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("A: %v", err)
			return
		}
		resp.Body.Close()
	}()
	go func() {
		defer wg.Done()
		req, _ := http.NewRequest(http.MethodGet, srvB.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Errorf("B: %v", err)
			return
		}
		resp.Body.Close()
	}()
	wg.Wait()
	elapsed := time.Since(start)

	// Allow generous slack but fail if we serialised.
	if elapsed > sleep+sleep/2 {
		t.Fatalf("hosts serialised: elapsed=%v expected <%v (1.5x sleep)", elapsed, sleep+sleep/2)
	}
}

// TestHostLimiter_ContextCancelReleasesQueued verifies a cancellation
// while waiting in the host queue returns promptly without dispatching
// to the inner transport. Without this guarantee, a client.Timeout
// firing while many requests were queued behind a saturated host could
// leak through.
func TestHostLimiter_ContextCancelReleasesQueued(t *testing.T) {
	var dispatched int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&dispatched, 1)
		time.Sleep(200 * time.Millisecond) // hold the only slot
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{Transport: newHostLimiter(http.DefaultTransport, 1)}

	// First request grabs the slot.
	go func() {
		req, _ := http.NewRequest(http.MethodGet, srv.URL, nil)
		resp, _ := client.Do(req)
		if resp != nil {
			resp.Body.Close()
		}
	}()
	time.Sleep(20 * time.Millisecond) // let it acquire

	// Second request blocks on the host semaphore; cancel its context.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)

	start := time.Now()
	_, err := client.Do(req)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected cancellation error, got nil")
	}
	// Should return at the 30ms cancel boundary, not wait 200ms for slot.
	if elapsed > 100*time.Millisecond {
		t.Fatalf("cancel didn't release promptly: elapsed=%v", elapsed)
	}
	// Only the first request should have reached the server.
	if got := atomic.LoadInt32(&dispatched); got != 1 {
		t.Fatalf("dispatched=%d expected 1 (the cancelled request should NOT reach the server)", got)
	}
}

// TestHostLimiter_ZeroCapDisables verifies maxPerHost<=0 returns the
// inner transport unchanged (no overhead, no behavior change).
func TestHostLimiter_ZeroCapDisables(t *testing.T) {
	inner := http.DefaultTransport
	if got := newHostLimiter(inner, 0); got != inner {
		t.Fatalf("expected inner transport, got wrapped: %T", got)
	}
	if got := newHostLimiter(inner, -1); got != inner {
		t.Fatalf("expected inner transport for negative cap, got: %T", got)
	}
}
