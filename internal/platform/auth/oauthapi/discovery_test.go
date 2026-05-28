package oauthapi

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"testing"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/authservice"
)

func testState(t *testing.T) *State {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	privPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
	svc, err := authservice.New(authservice.Config{
		Issuer:           "https://fc.example",
		Audience:         "https://fc.example",
		RSAPrivateKeyPEM: privPEM,
	})
	if err != nil {
		t.Fatalf("authservice.New: %v", err)
	}
	if svc.Algorithm() != "RS256" {
		t.Fatalf("want RS256, got %s", svc.Algorithm())
	}
	return &State{Auth: svc, BaseURL: "https://fc.example"}
}

func TestOpenIDConfiguration(t *testing.T) {
	s := testState(t)
	rec := httptest.NewRecorder()
	s.OpenIDConfiguration(rec, httptest.NewRequest("GET", "/.well-known/openid-configuration", nil))

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var doc map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if doc["issuer"] != "https://fc.example" {
		t.Errorf("issuer = %v", doc["issuer"])
	}
	if doc["token_endpoint"] != "https://fc.example/oauth/token" {
		t.Errorf("token_endpoint = %v", doc["token_endpoint"])
	}
	if doc["userinfo_endpoint"] != "https://fc.example/oauth/userinfo" {
		t.Errorf("userinfo_endpoint = %v", doc["userinfo_endpoint"])
	}
	if doc["end_session_endpoint"] != "https://fc.example/auth/oidc/session/end" {
		t.Errorf("end_session_endpoint = %v", doc["end_session_endpoint"])
	}

	assertContains(t, doc, "grant_types_supported", "authorization_code")
	assertContains(t, doc, "grant_types_supported", "refresh_token")
	assertContains(t, doc, "grant_types_supported", "client_credentials")
	assertContains(t, doc, "scopes_supported", "offline_access")
	assertContains(t, doc, "code_challenge_methods_supported", "S256")
	assertContains(t, doc, "code_challenge_methods_supported", "plain")
	assertContains(t, doc, "response_types_supported", "code id_token")
	assertContains(t, doc, "claims_supported", "type")
}

func TestJWKS(t *testing.T) {
	s := testState(t)
	rec := httptest.NewRecorder()
	s.JWKS(rec, httptest.NewRequest("GET", "/.well-known/jwks.json", nil))

	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
	var resp jwksResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Keys) != 1 {
		t.Fatalf("want 1 key, got %d", len(resp.Keys))
	}
	k := resp.Keys[0]
	if k.Kty != "RSA" || k.Use != "sig" || k.Alg != "RS256" {
		t.Errorf("key header = %+v", k)
	}
	if k.Kid == "" || k.N == "" || k.E == "" {
		t.Errorf("key missing kid/n/e: %+v", k)
	}
}

func assertContains(t *testing.T, doc map[string]any, key, want string) {
	t.Helper()
	arr, ok := doc[key].([]any)
	if !ok {
		t.Fatalf("%s is not an array: %T", key, doc[key])
	}
	for _, v := range arr {
		if v == want {
			return
		}
	}
	t.Errorf("%s missing %q (got %v)", key, want, arr)
}
