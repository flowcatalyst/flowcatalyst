package oauthapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth"
)

// TestMatchRedirectURIParityCases keeps the original /oauth/authorize
// redirect_uri cases, now exercising the unified MatchRedirectURI.
func TestMatchRedirectURIParityCases(t *testing.T) {
	cases := []struct {
		uri        string
		registered []string
		want       bool
	}{
		{"https://app.example.com/cb", []string{"https://app.example.com/cb"}, true}, // exact
		{"https://app.example.com/cb", []string{"https://other/cb"}, false},          // no match
		{"https://x.example.com/cb", []string{"https://*.example.com/cb"}, true},     // wildcard one segment
		{"https://x.y.example.com/cb", []string{"https://*.example.com/cb"}, false},  // dotted segment rejected
		{"https://.example.com/cb", []string{"https://*.example.com/cb"}, false},     // empty segment rejected
		{"https://app.example.com/foo", []string{"https://app.example.com/*"}, true}, // trailing wildcard
		{"https://evil.com/cb", []string{"https://app.example.com/cb", "https://*.x.com/cb"}, false},
	}
	for _, c := range cases {
		if got := MatchRedirectURI(c.uri, c.registered); got != c.want {
			t.Errorf("MatchRedirectURI(%q, %v) = %v, want %v", c.uri, c.registered, got, c.want)
		}
	}
}

func TestPctEncode(t *testing.T) {
	cases := map[string]string{
		"abc-_.~":          "abc-_.~",
		"a b":              "a%20b",
		"https://x/cb?a=1": "https%3A%2F%2Fx%2Fcb%3Fa%3D1",
		"openid profile":   "openid%20profile",
	}
	for in, want := range cases {
		if got := pctEncode(in); got != want {
			t.Errorf("pctEncode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestInvalidScopes(t *testing.T) {
	if got := invalidScopes("openid profile email offline_access", nil); len(got) != 0 {
		t.Errorf("standard scopes should be valid, got %v", got)
	}
	if got := invalidScopes("openid custom", []string{"custom"}); len(got) != 0 {
		t.Errorf("client scope should be valid, got %v", got)
	}
	got := invalidScopes("openid bogus", nil)
	if len(got) != 1 || got[0] != "bogus" {
		t.Errorf("want [bogus], got %v", got)
	}
}

func TestMaxAgeExceeded(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name     string
		maxAge   string
		issuedAt time.Time
		want     bool
	}{
		{"absent max_age", "", now.Add(-time.Hour), false},
		{"zero issuedAt", "60", time.Time{}, false},
		{"within window", "3600", now.Add(-10 * time.Minute), false},
		{"exceeded", "60", now.Add(-10 * time.Minute), true},
		{"max_age=0 always re-auth", "0", now.Add(-time.Second), true},
		{"invalid max_age ignored", "abc", now.Add(-time.Hour), false},
		{"negative max_age ignored", "-5", now.Add(-time.Hour), false},
	}
	for _, c := range cases {
		if got := maxAgeExceeded(c.maxAge, c.issuedAt); got != c.want {
			t.Errorf("%s: maxAgeExceeded(%q, %v) = %v, want %v", c.name, c.maxAge, c.issuedAt, got, c.want)
		}
	}
}

// fakeClientFinder injects a client (or the "unknown client" nil) into the
// authorize handler without a database.
type fakeClientFinder struct {
	client *auth.OAuthClient
	err    error
}

func (f fakeClientFinder) FindByClientID(context.Context, string) (*auth.OAuthClient, error) {
	return f.client, f.err
}

// activeClient is a minimal active client with one registered redirect_uri.
func activeClient(redirectURIs ...string) *auth.OAuthClient {
	return &auth.OAuthClient{Active: true, RedirectURIs: redirectURIs}
}

// Once the client + redirect_uri are validated, an unsupported response_type
// is reported by bouncing back to the (now-vetted) redirect_uri.
func TestAuthorizeUnsupportedResponseType(t *testing.T) {
	s := &State{OAuthClients: fakeClientFinder{client: activeClient("https://app/cb")}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/authorize?response_type=token&client_id=c&redirect_uri=https://app/cb&state=xyz", nil)
	s.Authorize(rec, req)

	if rec.Code != 307 {
		t.Fatalf("status = %d, want 307", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://app/cb?") {
		t.Fatalf("Location = %q", loc)
	}
	if !strings.Contains(loc, "error=unsupported_response_type") || !strings.Contains(loc, "state=xyz") {
		t.Errorf("Location missing error/state: %q", loc)
	}
}

// A ?provider= authorize request with no session chains into the OIDC bridge
// (provider-direct login) carrying the full oauth_* param set, so the
// downstream app's code can be issued after the IdP callback. It must NOT
// touch the pending-auth stash (State.PendingAuth is nil here — a DB write
// would panic) and must NOT bounce to the SPA login page.
func TestAuthorizeProviderChainsIntoBridge(t *testing.T) {
	s := &State{OAuthClients: fakeClientFinder{client: activeClient("https://app/cb")}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/authorize?response_type=code&client_id=c&redirect_uri=https://app/cb&state=xyz"+
		"&provider=idp_123&scope=openid&code_challenge=abc&code_challenge_method=S256&nonce=n1", nil)
	s.Authorize(rec, req)

	if rec.Code != 307 {
		t.Fatalf("status = %d, want 307", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "/auth/oidc/login?provider_id=idp_123") {
		t.Fatalf("Location = %q, want bridge provider-direct URL", loc)
	}
	for _, want := range []string{
		"oauth_client_id=c",
		"oauth_redirect_uri=https%3A%2F%2Fapp%2Fcb",
		"oauth_state=xyz",
		"oauth_scope=openid",
		"oauth_code_challenge=abc",
		"oauth_code_challenge_method=S256",
		"oauth_nonce=n1",
	} {
		if !strings.Contains(loc, want) {
			t.Errorf("Location missing %q: %q", want, loc)
		}
	}
}

// H1 regression: an UNKNOWN client must never trigger a redirect to the
// caller-supplied redirect_uri — that would be an open redirect (RFC 6749
// §4.1.2.1). The error must be a direct 4xx, not a 3xx bounce to evil.com.
func TestAuthorizeUnknownClientDoesNotOpenRedirect(t *testing.T) {
	s := &State{OAuthClients: fakeClientFinder{client: nil}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/authorize?response_type=token&client_id=c&redirect_uri=https://evil.com&state=xyz", nil)
	s.Authorize(rec, req)

	if rec.Code >= 300 && rec.Code < 400 {
		t.Fatalf("open redirect: unknown client produced a %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// H1 regression: a KNOWN client with an UNREGISTERED redirect_uri must also be
// a direct 4xx — we must not bounce to an unvetted URI even for a valid client.
func TestAuthorizeUnregisteredRedirectURIDoesNotOpenRedirect(t *testing.T) {
	s := &State{OAuthClients: fakeClientFinder{client: activeClient("https://app/cb")}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/authorize?response_type=code&client_id=c&redirect_uri=https://evil.com&state=xyz", nil)
	s.Authorize(rec, req)

	if rec.Code >= 300 && rec.Code < 400 {
		t.Fatalf("open redirect: unregistered redirect_uri produced a %d to %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAuthorizeMissingState(t *testing.T) {
	s := &State{}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/oauth/authorize?response_type=code&redirect_uri=https://app/cb", nil)
	s.Authorize(rec, req)

	if rec.Code != 400 {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["error"] != "invalid_request" {
		t.Errorf("error = %v, want invalid_request", body["error"])
	}
}
