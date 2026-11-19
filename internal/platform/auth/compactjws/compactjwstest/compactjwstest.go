// Package compactjwstest holds the checks every verifier of a compact JWS runs
// in its tests, so each kind of token is tested the same way without copying:
// [Tampered] (changes that certainly alter the signed content or the
// signature bytes) and [NonCanonical] (the issued token spelled differently:
// every other last character of the signature, padding, line breaks, the
// standard alphabet). Both refuse to pass by chance: each case is checked to
// differ from the issued token, and each re-spelling to decode, leniently, to
// exactly the issued bytes.
package compactjwstest

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"
)

// Alphabet is the base64url alphabet, in value order.
const Alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

// Kind is one kind of token and its verifier.
type Kind struct {
	// Token is a token as its issuer minted it; Verify accepts it.
	Token string
	// Verify returns nil when the verifier accepts token, and an error when
	// it refuses it.
	Verify func(token string) error
	// Claim is a string that occurs in the decoded payload (the subject, say)
	// and OtherClaim a different string to put in its place.
	Claim, OtherClaim string
}

// SpareBits returns how many low bits of the last character of an unpadded
// base64url string of n characters carry nothing: 0 when n is a multiple of
// four, 4 when n is 2 mod 4 (one byte in the last group), 2 when n is 3 mod 4
// (two bytes). n of 1 mod 4 is not valid base64 and returns -1.
func SpareBits(n int) int {
	switch n % 4 {
	case 0:
		return 0
	case 2:
		return 4
	case 3:
		return 2
	default:
		return -1
	}
}

// NonCanonicalSpellings returns every string that differs from seg only in its
// last character and that a lenient decoder decodes to the same bytes: the
// last character with each nonzero combination of its spare bits set. seg must
// end in a canonical character (spare bits zero); there are 2^SpareBits - 1.
func NonCanonicalSpellings(seg string) []string {
	if seg == "" {
		return nil
	}
	spare := SpareBits(len(seg))
	if spare <= 0 {
		return nil
	}
	last := strings.IndexByte(Alphabet, seg[len(seg)-1])
	if last < 0 {
		return nil
	}
	mask := 1<<spare - 1
	out := make([]string, 0, mask)
	for low := 1; low <= mask; low++ {
		out = append(out, seg[:len(seg)-1]+string(Alphabet[last&^mask|low]))
	}
	return out
}

func split(t *testing.T, tok string) (header, payload, sig string) {
	t.Helper()
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("token has %d segments, want 3", len(parts))
	}
	return parts[0], parts[1], parts[2]
}

func decode(t *testing.T, what, seg string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.Strict().DecodeString(seg)
	if err != nil {
		t.Fatalf("the issued %s is not canonical base64url: %v", what, err)
	}
	return b
}

func control(t *testing.T, k Kind) {
	t.Helper()
	if err := k.Verify(k.Token); err != nil {
		t.Fatalf("control: the issued token must verify: %v", err)
	}
}

func refuseAll(t *testing.T, k Kind, cases map[string]string) {
	t.Helper()
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			if bad == k.Token {
				t.Fatal("test bug: the changed token equals the issued one")
			}
			if err := k.Verify(bad); err == nil {
				t.Fatalf("must be refused: %q", bad)
			}
		})
	}
}

// Tampered checks that the verifier refuses the token changed in ways that
// each certainly alter the signed content or the signature bytes. (Replacing
// the last characters of a token with a constant is not such a change: the
// last character of a signature has spare bits, so a constant can decode to
// the same bytes by chance.)
func Tampered(t *testing.T, k Kind) {
	t.Helper()
	control(t, k)
	header, payload, sig := split(t, k.Token)
	sigBytes := decode(t, "signature", sig)
	payloadBytes := decode(t, "payload", payload)
	flipped := func(i int, bit byte) string {
		b := bytes.Clone(sigBytes)
		b[i] ^= bit
		return base64.RawURLEncoding.EncodeToString(b)
	}
	if k.Claim == "" || k.Claim == k.OtherClaim || !bytes.Contains(payloadBytes, []byte(k.Claim)) {
		t.Fatalf("test bug: Claim %q must occur in the payload and differ from OtherClaim", k.Claim)
	}
	otherClaim := base64.RawURLEncoding.EncodeToString(
		bytes.Replace(payloadBytes, []byte(k.Claim), []byte(k.OtherClaim), 1))
	// The first character carries six significant bits, so a different first
	// character certainly decodes to different bytes.
	swapped := byte('A')
	if payload[0] == swapped {
		swapped = 'B'
	}
	refuseAll(t, k, map[string]string{
		"first signature bit flipped":     header + "." + payload + "." + flipped(0, 0x80),
		"last signature bit flipped":      header + "." + payload + "." + flipped(len(sigBytes)-1, 0x01),
		"payload claim changed":           header + "." + otherClaim + "." + sig,
		"payload character changed":       header + "." + string(swapped) + payload[1:] + "." + sig,
		"signature truncated by one byte": header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(sigBytes[:len(sigBytes)-1]),
		"token truncated by one char":     k.Token[:len(k.Token)-1],
		"signature removed":               header + "." + payload + ".",
		"signature segment removed":       header + "." + payload,
	})
}

// NonCanonical checks that the verifier accepts the issued spelling only: the
// signature with any other last character that decodes to the same bytes, a
// segment with '=' padding, a line break inside the signature or after the
// token, and the signature in the standard alphabet are all refused, and the
// issued token still verifies afterwards.
func NonCanonical(t *testing.T, k Kind) {
	t.Helper()
	control(t, k)
	header, payload, sig := split(t, k.Token)
	sigBytes := decode(t, "signature", sig)
	decode(t, "header", header)
	decode(t, "payload", payload)

	spare := SpareBits(len(sig))
	spellings := NonCanonicalSpellings(sig)
	if want := 1<<spare - 1; len(spellings) != want {
		t.Fatalf("test bug: %d spellings for a %d-byte signature with %d spare bits, want %d", len(spellings), len(sigBytes), spare, want)
	}
	if spare == 0 {
		t.Logf("a %d-byte signature is %d characters with no spare bits: no other spelling of its last character", len(sigBytes), len(sig))
	}

	cases := map[string]string{}
	for _, v := range spellings {
		// Same bytes under the lenient decoder: the variant is only another
		// spelling, which is what makes the case meaningful.
		got, err := base64.RawURLEncoding.DecodeString(v)
		if err != nil || !bytes.Equal(got, sigBytes) {
			t.Fatalf("test bug: %q does not decode to the issued signature (err %v)", v, err)
		}
		if _, err := base64.RawURLEncoding.Strict().DecodeString(v); err == nil {
			t.Fatalf("test bug: strict decoding accepts %q", v)
		}
		cases["signature ends in "+v[len(v)-1:]] = header + "." + payload + "." + v
	}
	if pad := strings.Repeat("=", (4-len(sig)%4)%4); pad != "" {
		cases["signature padded"] = header + "." + payload + "." + sig + pad
	}
	if pad := strings.Repeat("=", (4-len(payload)%4)%4); pad != "" {
		cases["payload padded"] = header + "." + payload + pad + "." + sig
	}
	mid := len(sig) / 2
	cases["line feed inside the signature"] = header + "." + payload + "." + sig[:mid] + "\n" + sig[mid:]
	cases["carriage return inside the signature"] = header + "." + payload + "." + sig[:mid] + "\r" + sig[mid:]
	cases["CRLF after the token"] = k.Token + "\r\n"
	if std := strings.NewReplacer("-", "+", "_", "/").Replace(sig); std != sig {
		cases["signature in the standard alphabet"] = header + "." + payload + "." + std
	} else {
		t.Logf("the signature has no '-' or '_': no standard-alphabet spelling to try")
	}
	refuseAll(t, k, cases)
	control(t, k)
}

// SameAsReencoded reports whether token equals the standard encoder's output
// for the bytes a lenient decoder reads from each of its segments: true for a
// token spelled as issued, false for any other spelling of the same bytes (or
// for text that is not three base64url segments at all). It is written
// independently of compactjws so a fuzz target can hold a verifier to it.
func SameAsReencoded(token string) bool {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return false
	}
	for i, p := range parts {
		b, err := base64.RawURLEncoding.DecodeString(p)
		if err != nil {
			return false
		}
		parts[i] = base64.RawURLEncoding.EncodeToString(b)
	}
	return strings.Join(parts, ".") == token
}
