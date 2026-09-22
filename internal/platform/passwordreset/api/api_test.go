package api

import (
	"regexp"
	"testing"
)

func TestHashTokenIsLowercaseHexSHA256(t *testing.T) {
	h := hashToken("test-token-value")
	if len(h) != 64 {
		t.Fatalf("SHA-256 hex must be 64 chars; got %d", len(h))
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(h) {
		t.Fatalf("hash must be lowercase hex; got %q", h)
	}
	// Known SHA-256 of the empty string.
	if got := hashToken(""); got != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("empty-string hash mismatch; got %q", got)
	}
	// Deterministic + collision-distinct.
	if a, b := hashToken("same"), hashToken("same"); a != b {
		t.Fatal("hash must be deterministic")
	}
	if hashToken("a") == hashToken("b") {
		t.Fatal("different inputs must hash differently")
	}
}

func TestGenerateRawToken(t *testing.T) {
	tok, err := generateRawToken()
	if err != nil {
		t.Fatalf("generateRawToken: %v", err)
	}
	// 32 bytes → URL-safe base64 no-pad → 43 chars.
	if len(tok) != 43 {
		t.Fatalf("expected 43 chars for 32 bytes base64 no-pad; got %d (%q)", len(tok), tok)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(tok) {
		t.Fatalf("token must be URL-safe base64; got %q", tok)
	}
	tok2, _ := generateRawToken()
	if tok == tok2 {
		t.Fatal("tokens must be unique")
	}
	// A generated token hashes to a valid stored hash.
	if h := hashToken(tok); len(h) != 64 {
		t.Fatalf("generated token must hash to 64-char hex; got %d", len(h))
	}
}

// A self-service reset may resume exactly one thing: the OAuth authorize
// round-trip the user was in when they clicked "Forgot password". Anything
// else — another path on this origin, an absolute URL, a protocol-relative
// one — is dropped, because a reset is requested by whoever knows an email
// address (2026-09-22).
func TestResetReturnURLAcceptsOnlyAnAuthorizeRoundTrip(t *testing.T) {
	ok := "/oauth/authorize?response_type=code&client_id=app&redirect_uri=https%3A%2F%2Fapp.example%2Fcb&state=s1"
	got := resetReturnURL(&ok)
	if got == nil || *got != ok {
		t.Fatalf("authorize round-trip must be kept, got %v", got)
	}
	for _, bad := range []string{
		"", "/", "/dashboard", "/oauth/authorize", "/oauth/token?x=1",
		"//evil.example/oauth/authorize?x=1", "https://evil.example/oauth/authorize?x=1",
		"/\\evil.example", " /oauth/authorize?x=1 ",
	} {
		b := bad
		if got := resetReturnURL(&b); got != nil && bad != " /oauth/authorize?x=1 " {
			t.Errorf("%q must be dropped, got %q", bad, *got)
		}
	}
	trimmed := " /oauth/authorize?x=1 "
	if got := resetReturnURL(&trimmed); got == nil || *got != "/oauth/authorize?x=1" {
		t.Errorf("surrounding whitespace is trimmed, got %v", got)
	}
	if resetReturnURL(nil) != nil {
		t.Error("nil stays nil")
	}
}
