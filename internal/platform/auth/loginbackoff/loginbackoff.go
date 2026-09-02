// Package loginbackoff provides layered
// brute-force protection for the password login endpoint.
//
// Two checks run before credentials are evaluated:
//
//  1. Per-(identifier, IP) exponential backoff — the first few failures
//     are free, then each additional failure doubles the required delay up
//     to a cap. Slows targeted brute force from one source without locking
//     out the legitimate user coming from a different IP.
//  2. Per-identifier global ceiling — caps total failures across all IPs in
//     a sliding window, catching distributed attacks. A high threshold so
//     it never trips on normal usage.
//
// Federated principals must be screened out before calling Check (the
// email-domain gate redirects them to their IdP before any credential
// check).
package loginbackoff

import (
	"context"
	"math"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/envutil"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/loginattempt"
)

// Policy holds the backoff/ceiling knobs (all env-overridable).
type Policy struct {
	FreeAttempts     uint32 // failures allowed with no delay
	BaseDelaySecs    uint32 // delay applied at FreeAttempts+1
	MaxDelaySecs     uint32 // cap on the per-pair backoff delay
	GlobalWindowSecs int64  // window for the global ceiling
	GlobalCeiling    int64  // failures (any IP) in-window that trigger a lock
	GlobalLockSecs   int64  // lock duration when the ceiling trips
}

// PolicyFromEnv builds a Policy from FC_LOGIN_* env vars, falling back to
// the defaults below.
func PolicyFromEnv() Policy {
	return Policy{
		FreeAttempts:     uint32(envutil.Int("FC_LOGIN_BACKOFF_FREE_ATTEMPTS", 3)),
		BaseDelaySecs:    uint32(envutil.Int("FC_LOGIN_BACKOFF_BASE_SECS", 2)),
		MaxDelaySecs:     uint32(envutil.Int("FC_LOGIN_BACKOFF_MAX_SECS", 300)),
		GlobalWindowSecs: int64(envutil.Int("FC_LOGIN_GLOBAL_WINDOW_SECS", 3600)),
		GlobalCeiling:    int64(envutil.Int("FC_LOGIN_GLOBAL_CEILING", 100)),
		GlobalLockSecs:   int64(envutil.Int("FC_LOGIN_GLOBAL_LOCK_SECS", 900)),
	}
}

// ComputeDelaySecs returns the required delay given the failure count since
// the last success from the same (identifier, IP) pair: 0 below
// FreeAttempts, then base*2^(n-free-1), capped at MaxDelaySecs.
func (p Policy) ComputeDelaySecs(failureCount uint32) uint32 {
	if failureCount <= p.FreeAttempts {
		return 0
	}
	exponent := failureCount - p.FreeAttempts - 1
	if exponent > 31 {
		exponent = 31
	}
	scaled := uint64(p.BaseDelaySecs) << exponent
	if scaled > uint64(p.MaxDelaySecs) {
		return p.MaxDelaySecs
	}
	return uint32(scaled)
}

// Reason identifies which gate rejected an attempt.
type Reason string

const (
	ReasonPairBackoff   Reason = "pair_backoff"
	ReasonGlobalCeiling Reason = "global_ceiling"
)

// Decision is the outcome of a Check. Allowed=false carries the seconds the
// caller should wait (surfaced as a 429 + Retry-After).
type Decision struct {
	Allowed        bool
	RetryAfterSecs uint32
	Reason         Reason
}

// statsRepo is the subset of loginattempt.Repository the backoff needs.
type statsRepo interface {
	LastSuccessAt(ctx context.Context, identifier string) (*time.Time, error)
	FailureStatsByIdentifierIPSince(ctx context.Context, identifier, ip string, since time.Time) (int, *time.Time, error)
	// GlobalCeilingTrippedAt returns the timestamp of the ceiling-th most
	// recent FAILURE for identifier at or after since — the failure whose
	// arrival pushed the in-window count to ceiling — or nil when fewer
	// than ceiling failures exist in that range (never tripped). Both the
	// lock expiry and the point the count would naturally fall back below
	// ceiling are anchored to this single timestamp.
	GlobalCeilingTrippedAt(ctx context.Context, identifier string, since time.Time, ceiling int64) (*time.Time, error)
}

var _ statsRepo = (*loginattempt.Repository)(nil)

// Check runs the per-pair backoff + global ceiling. ip is best-effort —
// pass "" when unknown (local dev) and only the global ceiling applies.
//
// The identifier is lower-cased before querying: all current callers key on
// an email, attempts are recorded lower-cased, and a raw `identifier = $1`
// compare on the typed casing would let an attacker dodge the per-email
// ceiling by rotating case.
func Check(ctx context.Context, repo statsRepo, policy Policy, identifier, ip string) (Decision, error) {
	identifier = strings.ToLower(strings.TrimSpace(identifier))
	now := time.Now().UTC()

	// Window 1: failures since the last success bound the per-pair count.
	lastSuccess, err := repo.LastSuccessAt(ctx, identifier)
	if err != nil {
		return Decision{}, err
	}
	// A nil lastSuccess collapses two cases that are indistinguishable at
	// this boundary: the identifier has genuinely never succeeded, or its
	// true last success lies beyond loginattempt.LastSuccessAt's bounded
	// lookback (see lastSuccessLookback in loginattempt.go). Owner ruling
	// (2026-09-03): both are treated as NEVER-SUCCEEDED — the standard
	// 30-day window below applies in full, never weakened by dormancy.
	// Deliberately no second, unbounded query here to chase a stale
	// success: that fallback was proposed and retracted, because a
	// never-succeeded identifier — including every enumeration probe —
	// would then trigger a full-partition scan on every attempt.
	lastSuccessCutoff := now.AddDate(0, 0, -30)
	if lastSuccess != nil {
		lastSuccessCutoff = *lastSuccess
	}

	if ip != "" {
		count, lastFailure, err := repo.FailureStatsByIdentifierIPSince(ctx, identifier, ip, lastSuccessCutoff)
		if err != nil {
			return Decision{}, err
		}
		if count < 0 {
			count = 0
		}
		required := policy.ComputeDelaySecs(uint32(count))
		if required > 0 {
			last := now
			if lastFailure != nil {
				last = *lastFailure
			}
			elapsed := int64(now.Sub(last).Seconds())
			if elapsed < 0 {
				elapsed = 0
			}
			if uint32(elapsed) < required {
				return Decision{Allowed: false, RetryAfterSecs: required - uint32(elapsed), Reason: ReasonPairBackoff}, nil
			}
		}
	}

	// Window 2: per-identifier global ceiling. The lock is real: once
	// tripped, deny until BOTH the advertised lock has elapsed AND the
	// in-window count would naturally have fallen back below the ceiling —
	// not just whichever comes first. Re-querying a live count alone (the
	// previous approach) let attempts through the instant enough failures
	// aged out of GlobalWindowSecs, even when that happened sooner than the
	// GlobalLockSecs already advertised to the caller as the Retry-After.
	//
	// Both bounds are anchored to one timestamp: the ceiling-th most recent
	// failure, i.e. the failure whose arrival pushed the in-window count up
	// to GlobalCeiling.
	//   lockEnds  = trippedAt + GlobalLockSecs   (the advertised lock)
	//   countEnds = trippedAt + GlobalWindowSecs (when the count next drops
	//               below ceiling on its own — same failure, since it is
	//               the oldest of the current in-window ceiling-sized set)
	// Search back far enough to catch any trip whose derived expiry could
	// still be in the future; a trip older than that has certainly expired
	// either way, so it's safe to miss.
	searchWindowSecs := policy.GlobalWindowSecs
	if policy.GlobalLockSecs > searchWindowSecs {
		searchWindowSecs = policy.GlobalLockSecs
	}
	searchSince := now.Add(-time.Duration(searchWindowSecs) * time.Second)
	if lastSuccessCutoff.After(searchSince) {
		searchSince = lastSuccessCutoff
	}
	trippedAt, err := repo.GlobalCeilingTrippedAt(ctx, identifier, searchSince, policy.GlobalCeiling)
	if err != nil {
		return Decision{}, err
	}
	if trippedAt != nil {
		lockEnds := trippedAt.Add(time.Duration(policy.GlobalLockSecs) * time.Second)
		countEnds := trippedAt.Add(time.Duration(policy.GlobalWindowSecs) * time.Second)
		end := lockEnds
		if countEnds.After(end) {
			end = countEnds
		}
		if now.Before(end) {
			retrySecs := int64(math.Ceil(end.Sub(now).Seconds()))
			if retrySecs < 1 {
				retrySecs = 1
			}
			if retrySecs > math.MaxUint32 {
				retrySecs = math.MaxUint32
			}
			return Decision{Allowed: false, RetryAfterSecs: uint32(retrySecs), Reason: ReasonGlobalCeiling}, nil
		}
	}

	return Decision{Allowed: true}, nil
}
