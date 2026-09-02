package loginbackoff

import (
	"context"
	"testing"
	"time"
)

func defaultPolicy() Policy {
	return Policy{
		FreeAttempts:     3,
		BaseDelaySecs:    2,
		MaxDelaySecs:     300,
		GlobalWindowSecs: 3600,
		GlobalCeiling:    100,
		GlobalLockSecs:   900,
	}
}

func TestComputeDelaySecs(t *testing.T) {
	p := defaultPolicy()
	cases := map[uint32]uint32{
		0: 0, 1: 0, 2: 0, 3: 0, // free
		4: 2, 5: 4, 6: 8, 7: 16, 8: 32, 9: 64, 10: 128, 11: 256,
		12: 300, 20: 300, 100: 300, // capped
	}
	for in, want := range cases {
		if got := p.ComputeDelaySecs(in); got != want {
			t.Errorf("ComputeDelaySecs(%d) = %d, want %d", in, got, want)
		}
	}
	if got := p.ComputeDelaySecs(^uint32(0)); got != 300 {
		t.Errorf("ComputeDelaySecs(max) = %d, want 300 (no overflow)", got)
	}
}

// fakeRepo implements statsRepo for Check tests.
type fakeRepo struct {
	lastSuccess  *time.Time
	pairCount    int
	pairLastFail *time.Time
	// globalTrippedAt, when set, is returned verbatim by
	// GlobalCeilingTrippedAt regardless of the (identifier, since, ceiling)
	// it's called with — the tests below control it directly to place the
	// "trip" at a specific point in the past.
	globalTrippedAt *time.Time
}

func (f *fakeRepo) LastSuccessAt(context.Context, string) (*time.Time, error) {
	return f.lastSuccess, nil
}

func (f *fakeRepo) FailureStatsByIdentifierIPSince(context.Context, string, string, time.Time) (int, *time.Time, error) {
	return f.pairCount, f.pairLastFail, nil
}

func (f *fakeRepo) GlobalCeilingTrippedAt(context.Context, string, time.Time, int64) (*time.Time, error) {
	return f.globalTrippedAt, nil
}

func TestCheckAllowsCleanIdentifier(t *testing.T) {
	d, err := Check(context.Background(), &fakeRepo{}, defaultPolicy(), "a@b.com", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed {
		t.Errorf("clean identifier should be allowed, got %+v", d)
	}
}

func TestCheckPairBackoffRejects(t *testing.T) {
	// 4 failures (curve start = 2s required) with the last failure just now
	// → elapsed ~0 < 2 → reject.
	now := time.Now().UTC()
	d, err := Check(context.Background(), &fakeRepo{pairCount: 4, pairLastFail: &now}, defaultPolicy(), "a@b.com", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != ReasonPairBackoff {
		t.Errorf("want pair_backoff reject, got %+v", d)
	}
	if d.RetryAfterSecs == 0 || d.RetryAfterSecs > 2 {
		t.Errorf("retry_after = %d, want (0,2]", d.RetryAfterSecs)
	}
}

func TestCheckPairBackoffElapsedAllows(t *testing.T) {
	// 4 failures but the last was 10s ago (> 2s required) → allowed.
	old := time.Now().UTC().Add(-10 * time.Second)
	d, err := Check(context.Background(), &fakeRepo{pairCount: 4, pairLastFail: &old}, defaultPolicy(), "a@b.com", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed {
		t.Errorf("elapsed past required delay should allow, got %+v", d)
	}
}

func TestCheckGlobalCeilingRejects(t *testing.T) {
	// defaultPolicy: GlobalWindowSecs=3600, GlobalLockSecs=900. Tripped just
	// now → countEnds (now+3600) outlasts lockEnds (now+900), so Retry-After
	// is bounded by the window, not the (shorter) advertised lock.
	now := time.Now().UTC()
	d, err := Check(context.Background(), &fakeRepo{globalTrippedAt: &now}, defaultPolicy(), "a@b.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != ReasonGlobalCeiling {
		t.Errorf("want global_ceiling reject, got %+v", d)
	}
	if d.RetryAfterSecs < 3599 || d.RetryAfterSecs > 3600 {
		t.Errorf("retry_after = %d, want ~3600 (bounded by GlobalWindowSecs)", d.RetryAfterSecs)
	}
}

func TestCheckNoIPSkipsPairBackoff(t *testing.T) {
	// Even with a high pair count, an empty IP skips the per-pair gate;
	// only the global ceiling applies (here: never tripped → allowed).
	now := time.Now().UTC()
	d, err := Check(context.Background(), &fakeRepo{pairCount: 50, pairLastFail: &now}, defaultPolicy(), "a@b.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed {
		t.Errorf("empty IP should skip pair backoff, got %+v", d)
	}
}

// TestCheckGlobalLockOutlastsWindow pins the A-19 fix: GlobalLockSecs is a
// real lock, not just the Retry-After number. With a policy whose lock is
// longer than its counting window (the inverse of defaultPolicy), waiting
// past the window but still inside the lock must stay denied — under the
// old behaviour (re-querying a live count against a window-bounded cutoff)
// this would have silently allowed the attempt back in once the window
// alone had elapsed.
func TestCheckGlobalLockOutlastsWindow(t *testing.T) {
	p := Policy{GlobalWindowSecs: 60, GlobalCeiling: 5, GlobalLockSecs: 300}
	trippedAt := time.Now().UTC().Add(-70 * time.Second) // past the 60s window, inside the 300s lock
	d, err := Check(context.Background(), &fakeRepo{globalTrippedAt: &trippedAt}, p, "a@b.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed || d.Reason != ReasonGlobalCeiling {
		t.Errorf("want still-locked reject past the window but inside the lock, got %+v", d)
	}
	// lockEnds = trippedAt + 300s ≈ now + 230s.
	if d.RetryAfterSecs < 225 || d.RetryAfterSecs > 231 {
		t.Errorf("retry_after = %d, want ~230 (bounded by the outlasting lock)", d.RetryAfterSecs)
	}
}

// TestCheckGlobalLockAndWindowBothElapsed: once both the window and the
// lock have elapsed, the identifier is allowed again.
func TestCheckGlobalLockAndWindowBothElapsed(t *testing.T) {
	p := Policy{GlobalWindowSecs: 60, GlobalCeiling: 5, GlobalLockSecs: 300}
	trippedAt := time.Now().UTC().Add(-310 * time.Second) // past both bounds
	d, err := Check(context.Background(), &fakeRepo{globalTrippedAt: &trippedAt}, p, "a@b.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed {
		t.Errorf("want allowed once both the window and the lock have elapsed, got %+v", d)
	}
}

// capturingRepo wraps fakeRepo and records the `since` bound Check actually
// passes to FailureStatsByIdentifierIPSince (Window 1's per-pair lookup),
// so tests can pin exactly what cutoff a given LastSuccessAt result
// produces — and that it is always a concrete bound, never the zero value
// (which a real SQL query would read as unbounded).
type capturingRepo struct {
	fakeRepo
	gotSince time.Time
}

func (c *capturingRepo) FailureStatsByIdentifierIPSince(ctx context.Context, identifier, ip string, since time.Time) (int, *time.Time, error) {
	c.gotSince = since
	return c.fakeRepo.FailureStatsByIdentifierIPSince(ctx, identifier, ip, since)
}

// TestCheckNilLastSuccessAppliesStandardWindowInFull pins the 2026-09-03
// owner ruling. A nil LastSuccessAt can mean two different things —
// genuinely never succeeded, or a true success that lies beyond
// loginattempt.LastSuccessAt's bounded lookback (lastSuccessLookback) — but
// Check cannot tell them apart, and per the ruling must not try to: both
// are treated as NEVER-SUCCEEDED, with the standard 30-day failure-counting
// window applied in full. This test drives Check with two fakeRepos that
// are indistinguishable at this boundary (both report lastSuccess = nil,
// exactly as loginattempt.LastSuccessAt does for both cases — see
// TestLastSuccessAt_OutsideLookbackReadsAsNone and
// TestLastSuccessAt_NeverSucceededReturnsNil in loginattempt_pg_test.go)
// and asserts they produce byte-identical decisions and identical, bounded
// cutoffs — i.e. dormancy never weakens the lockout, and there is no
// second, unbounded lookup lurking behind either case.
func TestCheckNilLastSuccessAppliesStandardWindowInFull(t *testing.T) {
	p := defaultPolicy()
	now := time.Now().UTC()

	neverSucceeded := &capturingRepo{fakeRepo: fakeRepo{pairCount: 20, pairLastFail: &now}}
	dormantBeyondBound := &capturingRepo{fakeRepo: fakeRepo{lastSuccess: nil, pairCount: 20, pairLastFail: &now}}

	dNever, err := Check(context.Background(), neverSucceeded, p, "a@b.com", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	dDormant, err := Check(context.Background(), dormantBeyondBound, p, "a@b.com", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if dNever != dDormant {
		t.Errorf("never-succeeded and dormant-beyond-bound must yield identical decisions, got %+v vs %+v", dNever, dDormant)
	}

	wantCutoff := now.AddDate(0, 0, -30)
	for name, got := range map[string]time.Time{
		"never-succeeded":      neverSucceeded.gotSince,
		"dormant-beyond-bound": dormantBeyondBound.gotSince,
	} {
		if got.IsZero() {
			t.Errorf("%s: FailureStatsByIdentifierIPSince got a zero-value since — unbounded, not the standard window", name)
		}
		if diff := got.Sub(wantCutoff); diff < -time.Second || diff > time.Second {
			t.Errorf("%s: since = %v, want ~%v (the standard 30-day window, applied in full)", name, got, wantCutoff)
		}
	}
}

// TestCheckRecentSuccessNarrowsWindow pins the other half of the ruling: it
// governs only the nil case. A genuine, in-bound success must still narrow
// Window 1's cutoff to the success timestamp itself — not the 30-day
// fallback — exactly as designed before this ruling.
func TestCheckRecentSuccessNarrowsWindow(t *testing.T) {
	p := defaultPolicy()
	now := time.Now().UTC()
	// 5 days ago: inside the 30-day fallback window too, so a bug that
	// ignored lastSuccess and always used the fallback would go unnoticed
	// unless the cutoff is checked verbatim, which is exactly what this
	// test does.
	recentSuccess := now.Add(-5 * 24 * time.Hour)
	repo := &capturingRepo{fakeRepo: fakeRepo{lastSuccess: &recentSuccess, pairCount: 1, pairLastFail: &now}}

	if _, err := Check(context.Background(), repo, p, "a@b.com", "1.2.3.4"); err != nil {
		t.Fatal(err)
	}
	if diff := repo.gotSince.Sub(recentSuccess); diff < -time.Millisecond || diff > time.Millisecond {
		t.Errorf("since = %v, want the success timestamp %v verbatim (narrowed, not the 30-day fallback)", repo.gotSince, recentSuccess)
	}
}
