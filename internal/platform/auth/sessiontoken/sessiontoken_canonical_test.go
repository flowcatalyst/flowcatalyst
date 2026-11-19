package sessiontoken_test

import (
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/compactjws/compactjwstest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
)

// sessionKinds is a session token and the session/bearer validator, for the
// shared JWS checks, once per RSA key size: the spare bits of an RS256
// signature's last character depend on it (2048-bit: 4; 3072-bit: none;
// 4096-bit: 2). The platform generates 2048-bit keys; an operator may supply
// another size.
func sessionKinds(t *testing.T) map[string]compactjwstest.Kind {
	t.Helper()
	out := map[string]compactjwstest.Kind{}
	for _, bits := range []int{2048, 3072, 4096} {
		key, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			t.Fatalf("generate key: %v", err)
		}
		tok, err := sessiontoken.Mint(sessiontoken.Claims{Subject: "prn_alice", Scope: "ANCHOR"}, key, "iss", time.Hour)
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		out[fmt.Sprintf("RS256 %d", bits)] = compactjwstest.Kind{
			Token: tok,
			Verify: func(token string) error {
				_, err := sessiontoken.Validate(token, &key.PublicKey, sessiontoken.Expect{Issuer: "iss"})
				return err
			},
			Claim: "prn_alice", OtherClaim: "prn_mallory",
		}
	}
	return out
}

func TestValidate_RefusesTamperedTokens(t *testing.T) {
	for name, k := range sessionKinds(t) {
		t.Run(name, func(t *testing.T) { compactjwstest.Tampered(t, k) })
	}
}

func TestValidate_AcceptsOnlyTheIssuedSpelling(t *testing.T) {
	for name, k := range sessionKinds(t) {
		t.Run(name, func(t *testing.T) { compactjwstest.NonCanonical(t, k) })
	}
}
