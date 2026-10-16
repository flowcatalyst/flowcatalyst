package runner

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/functions/control"
)

// LoadRetry bounds how long a version's artifact fetch is retried, inside the
// runner, before the version is reported FAILED. Only TRANSIENT failures are
// retried (see isTransientLoadError); the platform latches the first FAILED
// report for a version, so a blip must never be reported as one.
type LoadRetry struct {
	// Initial is the first delay; each retry doubles it up to Max.
	Initial time.Duration
	// Max caps a single delay.
	Max time.Duration
	// Budget bounds the total time spent on one load, retries included. When
	// the next delay would overrun it the version is reported FAILED.
	Budget time.Duration
}

// Defaults for LoadRetry (FC_FUNCTIONS_LOAD_RETRY_*).
const (
	DefaultLoadRetryInitial = time.Second
	DefaultLoadRetryMax     = 30 * time.Second
	DefaultLoadRetryBudget  = 5 * time.Minute
)

func (p LoadRetry) withDefaults() LoadRetry {
	if p.Initial <= 0 {
		p.Initial = DefaultLoadRetryInitial
	}
	if p.Max <= 0 {
		p.Max = DefaultLoadRetryMax
	}
	if p.Max < p.Initial {
		p.Max = p.Initial
	}
	if p.Budget <= 0 {
		p.Budget = DefaultLoadRetryBudget
	}
	return p
}

// delay is the wait before retry number attempt (1 = after the first failure):
// Initial·2^(attempt-1) capped at Max, with "equal jitter" (uniform in
// [d/2, d]) so runners that failed together do not retry in lockstep.
func (p LoadRetry) delay(attempt int) time.Duration {
	d := p.Max
	if shift := attempt - 1; shift < 40 && p.Initial<<shift > 0 && p.Initial<<shift < p.Max {
		d = p.Initial << shift
	}
	half := d / 2
	if half <= 0 {
		return d
	}
	return half + rand.N(half+1) // #nosec G404 -- jitter, not security
}

// fetchError marks an error from fetching a module's bytes (as opposed to
// validating or compiling them): the only phase whose failures can be
// transient.
type fetchError struct{ err error }

func (e *fetchError) Error() string { return "ARTIFACT: " + e.err.Error() }
func (e *fetchError) Unwrap() error { return e.err }

// permanentError is a fetch-phase failure that retrying cannot fix: the bytes
// arrived but are not the module the platform named.
type permanentError struct{ error }

func (e permanentError) Unwrap() error { return e.error }

// isTransientLoadError classifies a load failure.
//
// Transient (retried, never reported FAILED until the budget runs out):
// control-plane 5xx, 429, 408/425, 401/403 (token expiry, a presigned URL that
// lapsed), and every transport-level failure fetching the artifact: timeouts,
// connection resets/refusals, DNS, truncated bodies, S3 errors surfaced as the
// redirect target's status.
//
// Permanent (reported FAILED at once): the artifact is missing (404/410) or
// otherwise rejected (other 4xx), the bytes do not match their digest, the
// artifact is oversized or the digest malformed, and everything after the
// fetch: module validation, ABI/describe failures, compile and instantiate
// errors.
func isTransientLoadError(err error) bool {
	fe, ok := errors.AsType[*fetchError](err)
	if !ok {
		return false // validation / ABI / compile: the same bytes fail the same way
	}
	if _, perm := errors.AsType[permanentError](fe.err); perm {
		return false
	}
	if se, ok := errors.AsType[*control.ErrStatus](fe.err); ok {
		return transientStatus(se.Status)
	}
	// Transport failures and anything unrecognised: never poison a version on
	// an error we cannot prove permanent.
	return true
}

func transientStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
		http.StatusUnauthorized, http.StatusForbidden:
		return true
	}
	return code >= 500
}

// retryDetail is the in-progress text a version reports while it is retrying.
func retryDetail(attempt int, err error, next time.Duration) string {
	return fmt.Sprintf("attempt %d failed, retrying in %s: %s", attempt, next.Round(time.Millisecond), reasonOf(err))
}

// exhaustedReason is the FAILED reason once the budget is spent (or the
// failure is permanent after retries): the last error plus the attempt count.
func exhaustedReason(err error, attempts int, elapsed time.Duration) string {
	noun := "attempts"
	if attempts == 1 {
		noun = "attempt"
	}
	return fmt.Sprintf("%s (gave up after %d %s over %s)", reasonOf(err), attempts, noun, elapsed.Round(time.Second))
}

// sleepCtx waits d or until ctx ends, reporting whether the wait completed.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}
