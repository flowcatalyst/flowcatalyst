package ratelimit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The buckets that guard the public reset, check-domain and token-management
// routes carry explicit defaults, and every window is covered by the prune.
func TestPoliciesForEnforcedPublicRoutes(t *testing.T) {
	p := PoliciesFromEnv()
	for name, tc := range map[string]struct {
		got   Policy
		limit uint32
		win   time.Duration
	}{
		"password_reset_ip":     {p.PasswordResetIP, 20, time.Hour},
		"password_reset_email":  {p.PasswordResetEmail, 5, time.Hour},
		"password_reset_factor": {p.PasswordResetFactor, 10, time.Hour},
		"check_domain_ip":       {p.CheckDomainIP, 600, time.Minute},
		"oauth_introspect_ip":   {p.OAuthIntrospectIP, 0, time.Minute},
		"oauth_revoke_ip":       {p.OAuthRevokeIP, 0, time.Minute},
	} {
		if tc.got.Limit != tc.limit || tc.got.Window != tc.win {
			t.Errorf("%s = %+v, want {%v,%d}", name, tc.got, tc.win, tc.limit)
		}
	}
}

func TestPasswordResetFactorOverride(t *testing.T) {
	t.Setenv("FC_RL_PASSWORD_RESET_FACTOR_PER_HOUR", "3")
	if got := PoliciesFromEnv().PasswordResetFactor.Limit; got != 3 {
		t.Errorf("override limit = %d, want 3", got)
	}
}

// A zero policy is "off" for opt-in buckets, but Enforce alone would read it
// as "always over": only OptInIPLimitMiddleware may turn 0 into pass-through.
func TestOptInIPLimitMiddlewareZeroIsOff(t *testing.T) {
	calls := 0
	store := countingStore{calls: &calls, decision: Decision{Allowed: false, RetryAfterSecs: 9}}
	ok := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	req := func() *http.Request {
		r := httptest.NewRequest(http.MethodPost, "/x", nil)
		r.Header.Set("X-Forwarded-For", "192.0.2.1")
		return r
	}

	rec := httptest.NewRecorder()
	OptInIPLimitMiddleware(store, BucketOAuthRevokeIP, Policy{Window: time.Minute, Limit: 0})(ok).ServeHTTP(rec, req())
	if rec.Code != http.StatusNoContent || calls != 0 {
		t.Errorf("limit 0: code=%d store calls=%d, want 204 and 0", rec.Code, calls)
	}
	rec = httptest.NewRecorder()
	OptInIPLimitMiddleware(store, BucketOAuthRevokeIP, Policy{Window: time.Minute, Limit: 5})(ok).ServeHTTP(rec, req())
	if rec.Code != http.StatusTooManyRequests || calls != 1 {
		t.Errorf("limit 5: code=%d store calls=%d, want 429 and 1", rec.Code, calls)
	}
}

type countingStore struct {
	calls    *int
	decision Decision
}

func (c countingStore) CheckAndRecord(context.Context, Bucket, string, Policy) (Decision, error) {
	*c.calls++
	return c.decision, nil
}
func (countingStore) Prune(context.Context, time.Duration) (int64, error) { return 0, nil }
