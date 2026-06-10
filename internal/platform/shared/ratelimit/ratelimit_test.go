package ratelimit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPoliciesFromEnvDefaults(t *testing.T) {
	p := PoliciesFromEnv()
	if p.OAuthTokenClient.Limit != 300 || p.OAuthTokenClient.Window != time.Minute {
		t.Errorf("oauth_token_client = %+v, want {1m,300}", p.OAuthTokenClient)
	}
	if p.OAuthTokenIP.Limit != 600 {
		t.Errorf("oauth_token_ip limit = %d, want 600", p.OAuthTokenIP.Limit)
	}
	if p.PasswordResetEmail.Limit != 5 || p.PasswordResetEmail.Window != time.Hour {
		t.Errorf("password_reset_email = %+v, want {1h,5}", p.PasswordResetEmail)
	}
	if got := p.MaxWindow(); got != time.Hour {
		t.Errorf("MaxWindow = %v, want 1h", got)
	}
}

func TestPoliciesFromEnvOverride(t *testing.T) {
	t.Setenv("FC_RL_OAUTH_TOKEN_CLIENT_PER_MIN", "7")
	if got := PoliciesFromEnv().OAuthTokenClient.Limit; got != 7 {
		t.Errorf("override limit = %d, want 7", got)
	}
}

func TestNoopStoreAllows(t *testing.T) {
	d, err := NoopStore{}.CheckAndRecord(context.Background(), BucketOAuthTokenIP, "k", Policy{time.Minute, 1})
	if err != nil || !d.Allowed {
		t.Errorf("noop should allow, got %+v err=%v", d, err)
	}
}

// fakeStore returns a fixed decision/error.
type fakeStore struct {
	decision Decision
	err      error
}

func (f fakeStore) CheckAndRecord(context.Context, Bucket, string, Policy) (Decision, error) {
	return f.decision, f.err
}
func (f fakeStore) Prune(context.Context, time.Duration) (int64, error) { return 0, nil }

func TestEnforce(t *testing.T) {
	pol := Policy{time.Minute, 10}
	// Allowed → nil.
	if rej := Enforce(context.Background(), fakeStore{decision: Decision{Allowed: true}}, BucketOAuthTokenClient, "c", pol); rej != nil {
		t.Errorf("allowed should return nil, got %+v", rej)
	}
	// Rejected → *Rejection with retry-after.
	if rej := Enforce(context.Background(), fakeStore{decision: Decision{Allowed: false, RetryAfterSecs: 42}}, BucketOAuthTokenClient, "c", pol); rej == nil || rej.RetryAfterSecs != 42 {
		t.Errorf("rejected should carry retry-after 42, got %+v", rej)
	}
	// Backend error → fail open (nil).
	if rej := Enforce(context.Background(), fakeStore{err: errors.New("boom")}, BucketOAuthTokenClient, "c", pol); rej != nil {
		t.Errorf("backend error should fail open, got %+v", rej)
	}
	// Nil store → nil.
	if rej := Enforce(context.Background(), nil, BucketOAuthTokenClient, "c", pol); rej != nil {
		t.Errorf("nil store should return nil, got %+v", rej)
	}
}

func TestRedactURL(t *testing.T) {
	cases := map[string]string{
		"redis://user:pass@host:6379": "redis://***@host:6379",
		"redis://:secret@host:6379/0": "redis://***@host:6379/0",
		"redis://host:6379":           "redis://host:6379",
		"host:6379":                   "host:6379",
	}
	for in, want := range cases {
		if got := redactURL(in); got != want {
			t.Errorf("redactURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestClientIP(t *testing.T) {
	// Rightmost XFF hop: the leftmost entries are client-supplied; only the
	// last was appended by our own proxy. A spoofed prefix must not let a
	// caller choose its rate-limit identity.
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 203.0.113.7")
	if got := ClientIP(r); got != "203.0.113.7" {
		t.Errorf("XFF rightmost hop = %q, want 203.0.113.7", got)
	}
	// Single-entry header (one trusted proxy in front).
	r1 := httptest.NewRequest("GET", "/", nil)
	r1.Header.Set("X-Forwarded-For", "203.0.113.7")
	if got := ClientIP(r1); got != "203.0.113.7" {
		t.Errorf("single XFF hop = %q, want 203.0.113.7", got)
	}
	r2 := httptest.NewRequest("GET", "/", nil)
	r2.RemoteAddr = "198.51.100.4:54321"
	if got := ClientIP(r2); got != "198.51.100.4" {
		t.Errorf("RemoteAddr host = %q, want 198.51.100.4", got)
	}
	// Degenerate trailing comma falls back to RemoteAddr.
	r3 := httptest.NewRequest("GET", "/", nil)
	r3.RemoteAddr = "198.51.100.4:54321"
	r3.Header.Set("X-Forwarded-For", "203.0.113.7,")
	if got := ClientIP(r3); got != "198.51.100.4" {
		t.Errorf("trailing-comma XFF = %q, want RemoteAddr fallback 198.51.100.4", got)
	}
}

func TestWriteTooManyRequests(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteTooManyRequests(rec, 30, "rate limit exceeded")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", rec.Code)
	}
	if ra := rec.Header().Get("Retry-After"); ra != "30" {
		t.Errorf("Retry-After = %q, want 30", ra)
	}
	if body := rec.Body.String(); body != `{"error":"TOO_MANY_REQUESTS","message":"rate limit exceeded"}` {
		t.Errorf("body = %s", body)
	}
}

func TestIPLimitMiddleware(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(200) })

	// Rejecting store → 429, next not called.
	mw := IPLimitMiddleware(fakeStore{decision: Decision{Allowed: false, RetryAfterSecs: 5}}, BucketOAuthTokenIP, Policy{time.Minute, 1})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/oauth/token", nil)
	req.RemoteAddr = "1.2.3.4:5"
	mw(next).ServeHTTP(rec, req)
	if rec.Code != 429 || called {
		t.Errorf("rejecting store should 429 and not call next (code=%d called=%v)", rec.Code, called)
	}

	// No resolvable IP → pass through.
	called = false
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/oauth/token", nil)
	req2.RemoteAddr = ""
	mw(next).ServeHTTP(rec2, req2)
	if !called {
		t.Error("no IP should pass through to next")
	}
}
