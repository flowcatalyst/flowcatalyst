package fieldauth

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/compactjws/compactjwstest"
)

var proofNow = time.Unix(1_800_000_000, 0)

// mintProof signs a DPoP proof as the field libraries do (ES256, raw r||s,
// the public JWK in the header, the standard base64url encoder).
func mintProof(t testing.TB, key *ecdsa.PrivateKey, jti string) string {
	t.Helper()
	b64 := base64.RawURLEncoding.EncodeToString
	pt, err := key.PublicKey.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	hdr, _ := json.Marshal(map[string]any{
		"typ": "dpop+jwt", "alg": "ES256",
		"jwk": map[string]string{"kty": "EC", "crv": "P-256", "x": b64(pt[1:33]), "y": b64(pt[33:65])},
	})
	claims, _ := json.Marshal(map[string]any{"htm": "POST", "htu": "https://fc.example.test/field/auth/login", "iat": proofNow.Unix(), "jti": jti})
	input := b64(hdr) + "." + b64(claims)
	sum := sha256.Sum256([]byte(input))
	r, s, err := ecdsa.Sign(rand.Reader, key, sum[:])
	if err != nil {
		t.Fatal(err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return input + "." + b64(sig)
}

func proofKind(t *testing.T) compactjwstest.Kind {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return compactjwstest.Kind{
		Token: mintProof(t, key, "jti-alice"),
		Verify: func(token string) error {
			_, err := ParseProof(token, proofNow)
			return err
		},
		Claim: "jti-alice", OtherClaim: "jti-mallory",
	}
}

func TestParseProof_RefusesTamperedProofs(t *testing.T) {
	compactjwstest.Tampered(t, proofKind(t))
}

// TestParseProof_AcceptsOnlyTheIssuedSpelling: a 64-byte ES256 signature is 86
// characters whose last one has four spare bits, so fifteen other characters
// decode to the same bytes.
func TestParseProof_AcceptsOnlyTheIssuedSpelling(t *testing.T) {
	compactjwstest.NonCanonical(t, proofKind(t))
}

// TestParseECJWK_RefusesNonCanonicalCoordinates: x and y are 32 bytes, 43
// characters with two spare bits; only the canonical spelling is a key. (The
// thumbprint was always computed from the re-encoded coordinates, so this
// changes which keys are accepted, never a key's jkt.)
func TestParseECJWK_RefusesNonCanonicalCoordinates(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pt, _ := key.PublicKey.Bytes()
	x, y := base64.RawURLEncoding.EncodeToString(pt[1:33]), base64.RawURLEncoding.EncodeToString(pt[33:65])
	jwk := func(x, y string) []byte {
		b, _ := json.Marshal(map[string]string{"kty": "EC", "crv": "P-256", "x": x, "y": y})
		return b
	}
	if _, _, _, err := ParseECJWK(jwk(x, y)); err != nil {
		t.Fatalf("canonical key refused: %v", err)
	}
	bad := map[string][]byte{"x padded": jwk(x+"=", y), "y with a line feed": jwk(x, y[:5]+"\n"+y[5:])}
	for _, v := range compactjwstest.NonCanonicalSpellings(x) {
		bad["x ends in "+v[len(v)-1:]] = jwk(v, y)
	}
	for _, v := range compactjwstest.NonCanonicalSpellings(y) {
		bad["y ends in "+v[len(v)-1:]] = jwk(x, v)
	}
	if len(bad) != 2+3+3 {
		t.Fatalf("test bug: %d cases", len(bad))
	}
	for name, raw := range bad {
		t.Run(name, func(t *testing.T) {
			if _, _, _, err := ParseECJWK(raw); err == nil {
				t.Fatal("must be refused")
			}
		})
	}
}

// FuzzParseProof: a proof carries its own key, so any key's proof verifies;
// but no input other than the issued proof may verify for the seeded key's
// signature. Mutations cannot re-sign, so the only accepted input is the seed.
func FuzzParseProof(f *testing.F) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		f.Fatal(err)
	}
	valid := mintProof(f, key, "jti-fuzz")
	dot := strings.LastIndexByte(valid, '.')
	head, sig := valid[:dot+1], valid[dot+1:]
	f.Add(valid)
	for _, v := range compactjwstest.NonCanonicalSpellings(sig) {
		f.Add(head + v)
	}
	f.Add(valid + "==")
	f.Add(head + sig[:10] + "\n" + sig[10:])
	f.Add("a.b.c")
	f.Add("")
	f.Fuzz(func(t *testing.T, s string) {
		if _, err := ParseProof(s, proofNow); err == nil && s != valid {
			t.Fatalf("a proof other than the issued one verified: %q", s)
		}
	})
}
