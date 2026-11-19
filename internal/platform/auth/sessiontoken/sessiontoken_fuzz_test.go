package sessiontoken_test

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/compactjws/compactjwstest"
	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
)

// FuzzValidate feeds arbitrary strings to the session/access-token verifier.
// It must never panic, and nothing but a token signed by our key may validate:
// the only inputs allowed to succeed are the seeded valid token itself. That
// holds only because Validate accepts the issued spelling alone (compactjws):
// the seeds include the valid token with each other last character of its
// signature, padded, and with a line break inside the signature, each of which
// a lenient decoder reads as the same signature bytes.
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
	head, sig := valid[:strings.LastIndexByte(valid, '.')+1], valid[strings.LastIndexByte(valid, '.')+1:]
	for _, v := range compactjwstest.NonCanonicalSpellings(sig) {
		f.Add(head + v)
	}
	f.Add(valid + "==")
	f.Add(head + sig[:10] + "\n" + sig[10:])
	f.Add(valid + "\r\n")
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
