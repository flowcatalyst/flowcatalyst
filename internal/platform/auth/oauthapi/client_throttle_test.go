package oauthapi

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/shared/ratelimit"
)

// recordingStore counts the per-client_id charges that reach the distributed
// store and optionally rejects them.
type recordingStore struct {
	calls atomic.Int64
	deny  bool
}

func (s *recordingStore) CheckAndRecord(context.Context, ratelimit.Bucket, string, ratelimit.Policy) (ratelimit.Decision, error) {
	s.calls.Add(1)
	return ratelimit.Decision{Allowed: !s.deny, RetryAfterSecs: 5}, nil
}

func (*recordingStore) Prune(context.Context, time.Duration) (int64, error) { return 0, nil }

// namedFinder knows exactly one client_id.
type namedFinder struct{ id string }

func (f namedFinder) FindByClientID(_ context.Context, id string) (*auth.OAuthClient, error) {
	if id == f.id {
		return &auth.OAuthClient{ClientID: id, Active: true, RedirectURIs: []string{"https://app/cb"}}, nil
	}
	return nil, nil
}

// L1: the per-client buckets are charged only for a client_id that names an
// existing client. An unknown id touches neither the in-memory governor nor the
// distributed store and gets the same error as before; a real client's own
// traffic still reaches its limit.
func TestTokenClientThrottleChargedOnlyForExistingClient(t *testing.T) {
	gov := ratelimit.NewGovernor(ratelimit.GovernorConfig{PerMinute: 1, Burst: 1})
	store := &recordingStore{}
	s := testState(t)
	s.OAuthClients = namedFinder{id: "oac_l1_real"}
	s.ClientGovernor = gov
	s.RateLimit = store

	for range 5 {
		status, body := postToken(t, s, "grant_type=authorization_code&client_id=oac_l1_ghost&client_secret=x")
		if status != 401 || body["error"] != "invalid_client" {
			t.Fatalf("unknown client: got %d %v, want 401 invalid_client", status, body)
		}
	}
	if n := store.calls.Load(); n != 0 {
		t.Fatalf("unknown client_id charged the distributed bucket %d times", n)
	}
	if ok, _ := gov.Check("oac_l1_ghost"); !ok {
		t.Fatal("unknown client_id consumed its governor bucket")
	}

	// A real client is still throttled: burst 1, so the second request is a 429.
	first, _ := postToken(t, s, "grant_type=authorization_code&client_id=oac_l1_real&client_secret=x")
	if first == 429 {
		t.Fatal("first request of a real client was throttled")
	}
	if second, _ := postToken(t, s, "grant_type=authorization_code&client_id=oac_l1_real&client_secret=x"); second != 429 {
		t.Fatalf("real client second request = %d, want 429", second)
	}
	if store.calls.Load() != 1 {
		t.Fatalf("store charges = %d, want 1 (the first real request)", store.calls.Load())
	}
}

func TestAuthorizeClientThrottleChargedOnlyForExistingClient(t *testing.T) {
	store := &recordingStore{deny: true}
	s := &State{OAuthClients: namedFinder{id: "oac_l1a_real"}, RateLimit: store}

	get := func(clientID string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		s.Authorize(rec, httptest.NewRequest("GET",
			"/oauth/authorize?response_type=code&client_id="+clientID+"&redirect_uri=https://app/cb&state=xyz", nil))
		return rec
	}

	for range 3 {
		rec := get("oac_l1a_ghost")
		if rec.Code != 400 || !strings.Contains(rec.Body.String(), "Unknown client") {
			t.Fatalf("unknown client: got %d %s, want 400 Unknown client", rec.Code, rec.Body.String())
		}
	}
	if n := store.calls.Load(); n != 0 {
		t.Fatalf("unknown client_id charged the distributed bucket %d times", n)
	}
	if rec := get("oac_l1a_real"); rec.Code != 429 {
		t.Fatalf("real client over its limit = %d, want 429", rec.Code)
	}
}
