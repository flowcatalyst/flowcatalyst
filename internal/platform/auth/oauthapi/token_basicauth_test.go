package oauthapi

import (
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// postToken drives POST /oauth/token with a form body and an optional Basic
// Authorization header, returning the status and decoded OAuth error body.
func postToken(t *testing.T, s *State, body string, basic ...string) (int, map[string]string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/oauth/token", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if len(basic) == 2 {
		req.Header.Set("Authorization", "Basic "+
			base64.StdEncoding.EncodeToString([]byte(basic[0]+":"+basic[1])))
	}
	rec := httptest.NewRecorder()
	s.Token(rec, req)

	var out map[string]string
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatalf("unmarshal %q: %v", rec.Body.String(), err)
		}
	}
	return rec.Code, out
}

// TestClientCredentialsAcceptsBasicAuth is the regression guard for
// client_secret_basic on the client_credentials grant. That grant authenticates
// inside its own handler off req.ClientID/ClientSecret, so before the fix a
// Basic-only caller — the method the discovery document advertises — was
// rejected with 400 "Missing client_id" without the header ever being read.
//
// The stub finder returns no client, so a request that gets past credential
// resolution lands on "Invalid client credentials" (401). Reaching that error
// at all is the proof: it means the Basic header was decoded into the request.
func TestClientCredentialsAcceptsBasicAuth(t *testing.T) {
	s := testState(t)
	s.OAuthClients = fakeClientFinder{}

	status, body := postToken(t, s, "grant_type=client_credentials", "oac_abc", "s3cr3t")

	if body["error_description"] == "Missing client_id" {
		t.Fatalf("Basic credentials were ignored: %d %v", status, body)
	}
	if status != 401 || body["error"] != "invalid_client" {
		t.Fatalf("got %d %v, want 401 invalid_client", status, body)
	}
}

// TestClientCredentialsBasicMatchingBodyClientID: RFC 6749 §2.3.1 permits a
// body client_id alongside Basic as long as it names the same client.
func TestClientCredentialsBasicMatchingBodyClientID(t *testing.T) {
	s := testState(t)
	s.OAuthClients = fakeClientFinder{}

	status, body := postToken(t, s,
		"grant_type=client_credentials&client_id=oac_abc", "oac_abc", "s3cr3t")

	if status != 401 || body["error"] != "invalid_client" {
		t.Fatalf("got %d %v, want 401 invalid_client (past credential resolution)", status, body)
	}
}

// TestTokenRejectsConflictingClientIdentities: RFC 6749 §3.2.1 — a request must
// not present two different client identities. A body client_id naming a
// different client than the Basic header is refused rather than silently
// overridden by whichever source wins.
func TestTokenRejectsConflictingClientIdentities(t *testing.T) {
	s := testState(t)
	s.OAuthClients = fakeClientFinder{}

	for _, grant := range []string{"client_credentials", "authorization_code", "refresh_token"} {
		t.Run(grant, func(t *testing.T) {
			status, body := postToken(t, s,
				"grant_type="+grant+"&client_id=oac_other", "oac_abc", "s3cr3t")

			if status != 400 || body["error"] != "invalid_request" {
				t.Fatalf("got %d %v, want 400 invalid_request", status, body)
			}
			if body["error_description"] != "client_id does not match the authenticated client" {
				t.Errorf("description = %q", body["error_description"])
			}
		})
	}
}

// TestClientCredentialsStillRequiresCredentials: with neither Basic nor body
// credentials the grant must still refuse — the fix must not turn an
// unauthenticated request into an authenticated one.
func TestClientCredentialsStillRequiresCredentials(t *testing.T) {
	s := testState(t)
	s.OAuthClients = fakeClientFinder{}

	status, body := postToken(t, s, "grant_type=client_credentials")

	if status != 400 || body["error_description"] != "Missing client_id" {
		t.Fatalf("got %d %v, want 400 Missing client_id", status, body)
	}
}
