package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
)

// mailSendTimeout bounds one detached delivery. The request has long since
// been answered, so the only thing left to protect is the goroutine.
const mailSendTimeout = 30 * time.Second

// goFunc starts fn on its own goroutine. It is State.Go's default; tests
// replace it to run the send inline or to wait for it deterministically.
func (s *State) goFunc(fn func()) {
	if s.Go != nil {
		s.Go(fn)
		return
	}
	go fn()
}

// sendInBackground delivers mail off the request path. A request that waits
// for SMTP answers measurably slower for an address that exists than for one
// that does not, which is an account oracle. The context is detached from the
// request (it is cancelled the moment the response is written) and bounded by
// mailSendTimeout. A failure is logged with the principal id only: never the
// token or the link.
func (s *State) sendInBackground(ctx context.Context, principalID string, send func(ctx context.Context) error) {
	detached := context.WithoutCancel(ctx)
	s.goFunc(func() {
		sctx, cancel := context.WithTimeout(detached, mailSendTimeout)
		defer cancel()
		if err := send(sctx); err != nil {
			slog.Warn("failed to send password reset/setup email", "principal", principalID, "err", err)
		}
	})
}

// rejectIP enforces the per-IP password-reset ceiling. It writes the 429 and
// returns true when the caller is over the limit.
func (s *State) rejectIP(w http.ResponseWriter, r *http.Request) bool {
	ip := ratelimit.ClientIP(r)
	if ip == "" {
		return false
	}
	if rej := ratelimit.Enforce(r.Context(), s.RateLimit, ratelimit.BucketPasswordResetIP, ip, s.RateLimitPolicies.PasswordResetIP); rej != nil {
		ratelimit.WriteTooManyRequests(w, rej.RetryAfterSecs, "rate limit exceeded for this IP")
		return true
	}
	return false
}

// emailOverLimit counts one request against the per-email ceiling and reports
// whether it is over. The caller must then behave exactly as it does for an
// unknown address: no token deleted, none minted, no mail, same response.
func (s *State) emailOverLimit(ctx context.Context, email string) bool {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return false
	}
	return ratelimit.Enforce(ctx, s.RateLimit, ratelimit.BucketPasswordResetEmail, email, s.RateLimitPolicies.PasswordResetEmail) != nil
}

// factorBudgetSpent reports whether the principal has used up its failed
// factor proofs for the window. Read-only: it records nothing. Fails open when
// the store cannot count (nil, Noop, backend error).
func (s *State) factorBudgetSpent(ctx context.Context, principalID string) bool {
	cnt, ok := s.RateLimit.(ratelimit.Counter)
	if !ok || s.RateLimit == nil {
		return false
	}
	pol := s.RateLimitPolicies.PasswordResetFactor
	n, _, err := cnt.Count(ctx, ratelimit.BucketPasswordResetFactor, principalID, pol.Window)
	if err != nil {
		slog.Warn("password-reset factor budget check failed; failing open", "err", err)
		return false
	}
	return n >= pol.Limit
}

// noteFactorFailure records one failed factor proof against the principal.
func (s *State) noteFactorFailure(ctx context.Context, principalID string) {
	_ = ratelimit.Enforce(ctx, s.RateLimit, ratelimit.BucketPasswordResetFactor, principalID, s.RateLimitPolicies.PasswordResetFactor)
}
