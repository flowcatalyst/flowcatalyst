package oauthtoken

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func itoa(i int) string { return strconv.Itoa(i) }

func TestTokenManagerCachesAndRefreshesBeforeExpiry(t *testing.T) {
	var hits int
	var lastForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		_ = r.ParseForm()
		lastForm = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		// Short TTL so the refresh-before-expiry path is exercised by the clock.
		_, _ = w.Write([]byte(`{"access_token":"tok-` + itoa(hits) + `","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	tm := New(srv.URL, "cid", "csecret", srv.Client())
	clock := time.Unix(1_700_000_000, 0)
	tm.now = func() time.Time { return clock }

	// First call fetches.
	got, err := tm.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if got != "tok-1" || hits != 1 {
		t.Fatalf("first call: got=%q hits=%d, want tok-1/1", got, hits)
	}
	// The grant must be a form-encoded client_credentials request.
	if lastForm.Get("grant_type") != "client_credentials" || lastForm.Get("client_id") != "cid" || lastForm.Get("client_secret") != "csecret" {
		t.Fatalf("unexpected token request form: %v", lastForm)
	}

	// Second call within validity reuses the cache.
	got, _ = tm.Token(context.Background())
	if got != "tok-1" || hits != 1 {
		t.Fatalf("cached call: got=%q hits=%d, want tok-1/1 (no refetch)", got, hits)
	}

	// Advance to within the 60s refresh buffer of expiry → must refetch.
	clock = clock.Add(3600*time.Second - 30*time.Second)
	got, _ = tm.Token(context.Background())
	if got != "tok-2" || hits != 2 {
		t.Fatalf("refresh call: got=%q hits=%d, want tok-2/2", got, hits)
	}
}

func TestTokenManagerErrorsOnNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()

	tm := New(srv.URL, "cid", "bad", srv.Client())
	if _, err := tm.Token(context.Background()); err == nil {
		t.Fatal("expected error on 401 token response, got nil")
	}
}

// Invalidate exists for the caller that has just been told its token is not
// accepted: the next call must mint rather than serve the cached token it was
// about to be rejected for again.
func TestInvalidateForcesAFreshMint(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-` + itoa(hits) + `","token_type":"Bearer","expires_in":3600}`))
	}))
	defer srv.Close()

	tm := New(srv.URL, "cid", "csecret", srv.Client())
	first, err := tm.Token(context.Background())
	if err != nil {
		t.Fatalf("Token: %v", err)
	}
	if again, _ := tm.Token(context.Background()); again != first || hits != 1 {
		t.Fatalf("a valid cached token must be reused: got=%q hits=%d", again, hits)
	}

	tm.Invalidate()
	next, _ := tm.Token(context.Background())
	if next == first || hits != 2 {
		t.Fatalf("after Invalidate the next call must mint: got=%q hits=%d", next, hits)
	}
}
