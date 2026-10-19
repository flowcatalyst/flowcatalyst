package sessiontoken_test

import (
	"crypto/rand"
	"crypto/rsa"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
)

// FuzzValidate feeds arbitrary strings to the session/access-token verifier.
// It must never panic, and nothing but a token signed by our key may validate:
// the only inputs allowed to succeed are the seeded valid token itself.
func FuzzValidate(f *testing.F) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		f.Fatal(err)
	}
	valid, err := sessiontoken.Mint(sessiontoken.Claims{Subject: "prn_x", Scope: "ANCHOR"}, key, "iss", time.Hour)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add(valid[:len(valid)-2])
	f.Add("a.b.c")
	f.Add("eyJhbGciOiJub25lIn0.eyJzdWIiOiJwcm5feCJ9.")
	f.Add("eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJwcm5feCJ9.AAAA")
	f.Add("")
	f.Add("....")
	f.Fuzz(func(t *testing.T, tok string) {
		c, err := sessiontoken.Validate(tok, &key.PublicKey, sessiontoken.Expect{Issuer: "iss"})
		if err == nil && tok != valid {
			t.Fatalf("a token that is not ours validated: %q -> %+v", tok, c)
		}
	})
}
