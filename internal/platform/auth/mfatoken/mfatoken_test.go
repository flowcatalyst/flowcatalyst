package mfatoken

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/auth/sessiontoken"
)

func testKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	return k
}

func TestMintParseRoundTrip(t *testing.T) {
	iss := NewIssuer(testKey(t), "https://fc.example")
	tok, err := iss.Mint("prn_123", PurposePending, 5*time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	claims, err := iss.Parse(tok, PurposePending)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.Subject != "prn_123" || claims.Purpose != PurposePending {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestWrongPurposeRejected(t *testing.T) {
	iss := NewIssuer(testKey(t), "iss")
	tok, _ := iss.Mint("prn_1", PurposePending, time.Minute)
	if _, err := iss.Parse(tok, PurposeEnroll); err == nil {
		t.Fatal("a pending token must NOT parse as an enroll token")
	}
}

func TestExpiredRejected(t *testing.T) {
	iss := NewIssuer(testKey(t), "iss")
	tok, _ := iss.Mint("prn_1", PurposePending, -time.Minute) // already expired
	if _, err := iss.Parse(tok, PurposePending); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

// splitToken returns the three dot-separated segments of a compact JWT.
func splitToken(t *testing.T, tok string) (header, payload, sig string) {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}
	return parts[0], parts[1], parts[2]
}

// TestTamperedRejected tamper cases each change the signed content or the
// signature bytes for certain, so none can pass by chance. (Replacing the last
// characters of the token with a constant is not such a case: the last
// base64url character of a 32-byte signature carries only 4 significant bits,
// so about 1 token in 1024 already ends in characters that decode to the same
// bytes as "xx".)
func TestTamperedRejected(t *testing.T) {
	iss := NewIssuer(testKey(t), "iss")
	tok, err := iss.Mint("prn_1", PurposePending, time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	header, payload, sig := splitToken(t, tok)

	sigBytes, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	flipped := func(i int, bit byte) string {
		b := bytes.Clone(sigBytes)
		b[i] ^= bit
		return base64.RawURLEncoding.EncodeToString(b)
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	otherSubject := base64.RawURLEncoding.EncodeToString(
		bytes.Replace(payloadBytes, []byte("prn_1"), []byte("prn_2"), 1))
	// A payload character replaced by a different one: the first character
	// carries six significant bits, so the decoded bytes certainly differ.
	first := payload[0]
	swapped := byte('A')
	if first == swapped {
		swapped = 'B'
	}
	changedChar := string(swapped) + payload[1:]

	cases := map[string]string{
		"first signature bit flipped":     header + "." + payload + "." + flipped(0, 0x80),
		"last signature bit flipped":      header + "." + payload + "." + flipped(len(sigBytes)-1, 0x01),
		"payload subject changed":         header + "." + otherSubject + "." + sig,
		"payload character changed":       header + "." + changedChar + "." + sig,
		"signature truncated by one byte": header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(sigBytes[:len(sigBytes)-1]),
		"token truncated by one char":     tok[:len(tok)-1],
		"signature removed":               header + "." + payload + ".",
		"signature segment removed":       header + "." + payload,
	}
	if _, err := iss.Parse(tok, PurposePending); err != nil {
		t.Fatalf("control: the untouched token must parse: %v", err)
	}
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			if bad == tok {
				t.Fatal("test bug: the tampered token equals the issued one")
			}
			if _, err := iss.Parse(bad, PurposePending); err == nil {
				t.Fatal("tampered token must be rejected")
			}
		})
	}
}

// TestNonCanonicalSignatureEncodingRejected pins, on purpose, that a token
// whose signature is the issued signature in a non-canonical base64url form is
// refused. A 32-byte HS256 signature is 43 characters and the last one carries
// four significant bits, so three other characters decode to the same bytes.
// A lenient decoder accepts them, giving several strings that all verify;
// strict decoding (jwt.WithStrictDecoding) accepts only the issued string.
func TestNonCanonicalSignatureEncodingRejected(t *testing.T) {
	iss := NewIssuer(testKey(t), "iss")
	tok, err := iss.Mint("prn_1", PurposePending, time.Minute)
	if err != nil {
		t.Fatalf("mint: %v", err)
	}
	header, payload, sig := splitToken(t, tok)

	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
	last := strings.IndexByte(alphabet, sig[len(sig)-1])
	if last < 0 || last&0x03 != 0 {
		t.Fatalf("issued signature must end in a canonical character (low two bits zero), got %q", sig[len(sig)-1])
	}
	want, err := base64.RawURLEncoding.DecodeString(sig)
	if err != nil {
		t.Fatalf("decode signature: %v", err)
	}
	for _, low := range []int{1, 2, 3} {
		variant := sig[:len(sig)-1] + string(alphabet[last|low])
		t.Run(string(alphabet[last|low]), func(t *testing.T) {
			// Same bytes under the lenient decoder: the variant is only a
			// different spelling, which is what makes the case meaningful.
			got, err := base64.RawURLEncoding.DecodeString(variant)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("test bug: %q does not decode to the issued signature (err %v)", variant, err)
			}
			if _, err := base64.RawURLEncoding.Strict().DecodeString(variant); err == nil {
				t.Fatal("test bug: strict decoding accepts the variant")
			}
			if _, err := iss.Parse(header+"."+payload+"."+variant, PurposePending); err == nil {
				t.Fatal("a non-canonical spelling of the signature must be rejected")
			}
		})
	}
	if _, err := iss.Parse(tok, PurposePending); err != nil {
		t.Fatalf("the issued token must still parse: %v", err)
	}
}

func TestDifferentKeyRejected(t *testing.T) {
	a := NewIssuer(testKey(t), "iss")
	b := NewIssuer(testKey(t), "iss") // different RSA key → different HMAC secret
	tok, _ := a.Mint("prn_1", PurposePending, time.Minute)
	if _, err := b.Parse(tok, PurposePending); err == nil {
		t.Fatal("token from a different key must be rejected")
	}
}

// TestRejectedBySessionValidator is the load-bearing security property: a
// pending/enroll token is HS256 and must be rejected by the RS256-only session
// validator, so it can never be replayed as an fc_session cookie.
func TestRejectedBySessionValidator(t *testing.T) {
	key := testKey(t)
	iss := NewIssuer(key, "iss")
	tok, _ := iss.Mint("prn_1", PurposePending, time.Minute)
	if _, err := sessiontoken.Validate(tok, &key.PublicKey, sessiontoken.Expect{}); err == nil {
		t.Fatal("mfa pending token must NOT validate as a session token")
	}
}
