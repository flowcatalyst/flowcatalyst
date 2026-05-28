package oauthapi

import (
	"crypto/sha256"
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyPKCE_S256(t *testing.T) {
	verifier := strings.Repeat("a", 43) // 43 unreserved chars
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	s256 := "S256"
	if err := verifyPKCE(challenge, &s256, verifier); err != nil {
		t.Fatalf("valid S256 verifier rejected: %v (%s)", err.Code, derefOr(err.Description, ""))
	}
	// nil method defaults to S256.
	if err := verifyPKCE(challenge, nil, verifier); err != nil {
		t.Fatalf("valid verifier rejected with default method: %v", err.Code)
	}
}

func TestVerifyPKCE_Plain(t *testing.T) {
	verifier := strings.Repeat("b", 50)
	plain := "plain"
	if err := verifyPKCE(verifier, &plain, verifier); err != nil {
		t.Fatalf("valid plain verifier rejected: %v", err.Code)
	}
}

func TestVerifyPKCE_Rejections(t *testing.T) {
	s256 := "S256"
	verifier := strings.Repeat("a", 43)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	cases := []struct {
		name      string
		challenge string
		method    *string
		verifier  string
	}{
		{"missing verifier", challenge, &s256, ""},
		{"too short", challenge, &s256, strings.Repeat("a", 42)},
		{"too long", challenge, &s256, strings.Repeat("a", 129)},
		{"invalid chars", challenge, &s256, strings.Repeat("a", 42) + "/"},
		{"mismatch", challenge, &s256, strings.Repeat("c", 43)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := verifyPKCE(tc.challenge, tc.method, tc.verifier); err == nil {
				t.Fatalf("expected rejection for %s", tc.name)
			} else if err.Code != "invalid_grant" {
				t.Fatalf("expected invalid_grant, got %s", err.Code)
			}
		})
	}
}

func TestBasicAuthCreds(t *testing.T) {
	r := httptest.NewRequest("POST", "/oauth/token", nil)
	r.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("clt_1:s3cr3t")))
	id, secret, ok := basicAuthCreds(r)
	if !ok || id != "clt_1" || secret != "s3cr3t" {
		t.Fatalf("got (%q,%q,%v), want (clt_1,s3cr3t,true)", id, secret, ok)
	}

	// Empty secret is allowed (public-client probing) — ok with empty secret.
	r2 := httptest.NewRequest("POST", "/oauth/token", nil)
	r2.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("clt_1:")))
	id, secret, ok = basicAuthCreds(r2)
	if !ok || id != "clt_1" || secret != "" {
		t.Fatalf("empty-secret case: got (%q,%q,%v)", id, secret, ok)
	}

	// No Authorization header → not ok.
	r3 := httptest.NewRequest("POST", "/oauth/token", nil)
	if _, _, ok := basicAuthCreds(r3); ok {
		t.Fatal("expected ok=false with no Authorization header")
	}

	// Bearer (not Basic) → not ok.
	r4 := httptest.NewRequest("POST", "/oauth/token", nil)
	r4.Header.Set("Authorization", "Bearer abc")
	if _, _, ok := basicAuthCreds(r4); ok {
		t.Fatal("expected ok=false for Bearer scheme")
	}

	// No colon → not ok.
	r5 := httptest.NewRequest("POST", "/oauth/token", nil)
	r5.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("nocolon")))
	if _, _, ok := basicAuthCreds(r5); ok {
		t.Fatal("expected ok=false when decoded creds lack a colon")
	}
}

func TestScopeHas(t *testing.T) {
	if !scopeHas("openid profile offline_access", "offline_access") {
		t.Error("expected offline_access present")
	}
	if scopeHas("openid profile", "offline_access") {
		t.Error("did not expect offline_access")
	}
	if !scopeHas("openid", "openid") {
		t.Error("expected openid present")
	}
	if scopeHas("", "openid") {
		t.Error("empty scope should contain nothing")
	}
}
