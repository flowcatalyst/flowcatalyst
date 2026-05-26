package router

import (
	"net/http"
	"sync"
)

// hostLimiter wraps an http.RoundTripper with a per-host concurrency
// cap. Each target host gets its own bounded semaphore; requests beyond
// the cap block until an in-flight request to the same host returns.
// Different hosts never share quota.
//
// This sits ABOVE the http.Transport so it bounds in-flight requests
// regardless of how the transport satisfies them (one H2 connection
// multiplexing many streams, an H1 pool of TCP connections, etc.).
// `http.Transport.MaxConnsPerHost` only bounds TCP connections — under
// HTTP/2 multiplexing that's effectively unlimited request concurrency
// per host, which is exactly what we want to cap.
//
// Designed for the AWS-ALB case: ALBs advertise an H2 stream limit
// (~128) via SETTINGS, which `StrictMaxConcurrentStreams` honours, but
// other webhook receivers may not advertise anything. The host
// limiter is the defence-in-depth layer that applies regardless of
// what the peer says.
//
// Blocking respects the request's context — if the caller cancels (or
// the http.Client timeout fires) while waiting in the host queue, the
// RoundTrip returns context.Cause(err) without ever dispatching.
type hostLimiter struct {
	inner   http.RoundTripper
	maxPer  int

	mu   sync.Mutex
	sems map[string]chan struct{}
}

// newHostLimiter wraps inner with a per-host cap. maxPerHost <= 0
// disables the limit (pass-through). The returned RoundTripper is safe
// for concurrent use.
func newHostLimiter(inner http.RoundTripper, maxPerHost int) http.RoundTripper {
	if maxPerHost <= 0 {
		return inner
	}
	return &hostLimiter{
		inner:  inner,
		maxPer: maxPerHost,
		sems:   make(map[string]chan struct{}),
	}
}

func (h *hostLimiter) RoundTrip(req *http.Request) (*http.Response, error) {
	sem := h.semFor(req.URL.Host)
	ctx := req.Context()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-sem }()
	return h.inner.RoundTrip(req)
}

func (h *hostLimiter) semFor(host string) chan struct{} {
	h.mu.Lock()
	defer h.mu.Unlock()
	if sem, ok := h.sems[host]; ok {
		return sem
	}
	sem := make(chan struct{}, h.maxPer)
	h.sems[host] = sem
	return sem
}
