package login

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
)

// L2: POST /auth/login sits behind the per-IP sign-in governor. An address over
// its limit gets 429 + Retry-After before any credential work; another address
// is unaffected; routes that are not sign-in (check-domain GET shape aside) do
// not share the bucket.
func TestLoginRouteIsPerIPLimited(t *testing.T) {
	gov := ratelimit.NewGovernor(ratelimit.GovernorConfig{PerMinute: 1, Burst: 2})
	e := New(Config{SignInLimit: ratelimit.GovernorMiddleware(gov, "Too many sign-in attempts")})
	r := chi.NewRouter()
	e.RegisterPublicRoutes(r)

	post := func(ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", strings.NewReader(`{}`))
		req.Header.Set("X-Forwarded-For", ip)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	for i := range 2 {
		if rec := post("203.0.113.10"); rec.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d within burst = %d, want 401 (limiter must not interfere)", i+1, rec.Code)
		}
	}
	rec := post("203.0.113.10")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("third attempt from one address = %d, want 429", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("429 should carry Retry-After")
	}
	if rec := post("203.0.113.11"); rec.Code != http.StatusUnauthorized {
		t.Fatalf("a different address = %d, want 401 (own bucket)", rec.Code)
	}
}

func TestPasswordLoginGovernorDefaults(t *testing.T) {
	t.Setenv("FC_LOGIN_RATE_PER_MIN", "")
	t.Setenv("FC_LOGIN_BURST", "")
	if c := ratelimit.PasswordLoginGovernorFromEnv(); c.PerMinute != 120 || c.Burst != 60 {
		t.Fatalf("defaults = %+v, want 120/min burst 60", c)
	}
	t.Setenv("FC_LOGIN_RATE_PER_MIN", "7")
	t.Setenv("FC_LOGIN_BURST", "3")
	if c := ratelimit.PasswordLoginGovernorFromEnv(); c.PerMinute != 7 || c.Burst != 3 {
		t.Fatalf("overrides = %+v, want 7/3", c)
	}
}
