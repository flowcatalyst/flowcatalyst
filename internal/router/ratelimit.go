package router

import (
	"context"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter is a per-pool token bucket. The Rust version uses the
// `governor` crate; in Go we use golang.org/x/time/rate's Limiter,
// wrapped so we can hot-swap the underlying limiter on config reload.
type RateLimiter struct {
	limiter atomic.Pointer[rate.Limiter]
	rpm     atomic.Uint32 // 0 means unlimited
}

// NewRateLimiter constructs a limiter at the requested rate. perMinute=0
// disables limiting (a permissive limiter is installed).
func NewRateLimiter(perMinute uint32) *RateLimiter {
	rl := &RateLimiter{}
	rl.replaceUnsafe(perMinute)
	return rl
}

// SetRate atomically replaces the underlying limiter. Burst is set to
// max(perMinute, 1) so the limiter can absorb a small jitter without
// blocking on the very first message.
func (rl *RateLimiter) SetRate(perMinute uint32) {
	rl.replaceUnsafe(perMinute)
}

// Rate returns the configured rate in messages-per-minute. Zero means
// unlimited.
func (rl *RateLimiter) Rate() uint32 { return rl.rpm.Load() }

// IsLimited reports whether the next call to Wait would block (i.e. the
// bucket currently has no token). Non-consuming check used by the
// metrics collector — does not modify limiter state.
func (rl *RateLimiter) IsLimited() bool {
	if rl.rpm.Load() == 0 {
		return false
	}
	return rl.limiter.Load().Tokens() < 1.0
}

func (rl *RateLimiter) replaceUnsafe(perMinute uint32) {
	rl.rpm.Store(perMinute)
	if perMinute == 0 {
		// Effectively unlimited: 1 second of bucket at math.MaxFloat64-ish.
		rl.limiter.Store(rate.NewLimiter(rate.Inf, 1))
		return
	}
	r := rate.Limit(float64(perMinute) / 60.0)
	burst := int(perMinute)
	if burst < 1 {
		burst = 1
	}
	rl.limiter.Store(rate.NewLimiter(r, burst))
}

// Wait blocks until a token is available or ctx is canceled.
func (rl *RateLimiter) Wait(ctx context.Context) error {
	return rl.limiter.Load().Wait(ctx)
}

// Allow returns true if a token is available right now (non-blocking).
func (rl *RateLimiter) Allow() bool { return rl.limiter.Load().Allow() }

// Reserve reserves a token and returns the wait duration. Useful when
// you want to back-pressure rather than block.
func (rl *RateLimiter) Reserve() (waitFor time.Duration) {
	r := rl.limiter.Load().Reserve()
	if !r.OK() {
		return time.Hour // far-future fallback
	}
	return r.Delay()
}
