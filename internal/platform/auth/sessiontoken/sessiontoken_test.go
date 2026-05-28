package sessiontoken_test

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
)

func mustKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return k
}

func TestMintAndValidate_RoundTrip(t *testing.T) {
	key := mustKey(t)

	in := sessiontoken.Claims{
		Subject:      "prn_abc",
		Scope:        "ANCHOR",
		Email:        "admin@example.com",
		Clients:      []string{"clt_1", "clt_2"},
		Roles:        []string{"platform:super-admin"},
		Applications: []string{"app_platform"},
		Permissions:  []string{"platform:*:*:*", "*"},
	}

	tok, err := sessiontoken.Mint(in, key, "http://localhost", time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	if tok == "" {
		t.Fatalf("mint returned empty token")
	}

	out, err := sessiontoken.Validate(tok, &key.PublicKey)
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if out.Subject != in.Subject {
		t.Errorf("Subject=%q want %q", out.Subject, in.Subject)
	}
	if out.Scope != in.Scope {
		t.Errorf("Scope=%q want %q", out.Scope, in.Scope)
	}
	if out.Email != in.Email {
		t.Errorf("Email=%q want %q", out.Email, in.Email)
	}
	if got, want := len(out.Clients), len(in.Clients); got != want {
		t.Errorf("Clients len=%d want %d", got, want)
	}
	if got, want := len(out.Permissions), len(in.Permissions); got != want {
		t.Errorf("Permissions len=%d want %d", got, want)
	}
}

func TestValidate_RejectsBadSignature(t *testing.T) {
	k1 := mustKey(t)
	k2 := mustKey(t)

	tok, err := sessiontoken.Mint(sessiontoken.Claims{
		Subject: "prn_abc",
		Scope:   "ANCHOR",
	}, k1, "iss", time.Hour)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	if _, err := sessiontoken.Validate(tok, &k2.PublicKey); err == nil {
		t.Fatalf("validate should fail with mismatched key")
	}
}

func TestValidate_RejectsExpired(t *testing.T) {
	key := mustKey(t)

	tok, err := sessiontoken.Mint(sessiontoken.Claims{
		Subject: "prn_abc",
		Scope:   "ANCHOR",
	}, key, "iss", -time.Minute) // already expired
	if err != nil {
		t.Fatalf("mint: %v", err)
	}

	_, err = sessiontoken.Validate(tok, &key.PublicKey)
	if err == nil {
		t.Fatalf("validate should reject expired token")
	}
	if !errors.Is(err, jwt.ErrTokenExpired) {
		t.Errorf("expected ErrTokenExpired, got %v", err)
	}
}

func TestMint_RejectsEmptySubject(t *testing.T) {
	key := mustKey(t)
	_, err := sessiontoken.Mint(sessiontoken.Claims{Scope: "ANCHOR"}, key, "iss", time.Hour)
	if err == nil {
		t.Fatalf("mint should reject empty subject")
	}
}

func TestMint_RejectsNilKey(t *testing.T) {
	_, err := sessiontoken.Mint(sessiontoken.Claims{Subject: "prn_abc"}, nil, "iss", time.Hour)
	if err == nil {
		t.Fatalf("mint should reject nil key")
	}
}
