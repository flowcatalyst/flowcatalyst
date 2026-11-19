package compactjws_test

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/go-sdk/internal/compactjws"
	"github.com/flowcatalyst/go-sdk/internal/compactjws/compactjwstest"
)

func TestDecodeSegmentAcceptsTheEncodersOutputForEveryLength(t *testing.T) {
	// Lengths 0..9 cover every remainder mod 3 of the byte length, so every
	// count of spare bits (0, 2, 4) at the end of the string.
	for n := range 10 {
		b := make([]byte, n)
		for i := range b {
			b[i] = byte(0xff - i)
		}
		s := base64.RawURLEncoding.EncodeToString(b)
		got, err := compactjws.DecodeSegment(s)
		require.NoError(t, err, "length %d: %q", n, s)
		assert.Equal(t, b, got)
	}
}

func TestDecodeSegmentRefusesOtherSpellings(t *testing.T) {
	sig := base64.RawURLEncoding.EncodeToString([]byte{0xfb, 0xff, 0xbf, 0xfe}) // "-_-_-g": 4 spare bits
	for name, bad := range map[string]string{
		"padded":            sig + "==",
		"standard alphabet": strings.NewReplacer("-", "+", "_", "/").Replace(sig),
		"line feed":         sig[:2] + "\n" + sig[2:],
		"carriage return":   sig[:2] + "\r" + sig[2:],
		"trailing CRLF":     sig + "\r\n",
		"space":             sig[:2] + " " + sig[2:],
	} {
		t.Run(name, func(t *testing.T) {
			_, err := compactjws.DecodeSegment(bad)
			assert.ErrorIs(t, err, compactjws.ErrNonCanonical)
		})
	}
	spellings := compactjwstest.NonCanonicalSpellings(sig)
	require.Len(t, spellings, 15)
	for _, v := range spellings {
		_, err := compactjws.DecodeSegment(v)
		assert.ErrorIs(t, err, compactjws.ErrNonCanonical, v)
	}
}

func TestCheckWantsThreeCanonicalSegments(t *testing.T) {
	assert.NoError(t, compactjws.Check("eyJhbGciOiJIUzI1NiJ9.e30.AAAAAA"))
	for _, bad := range []string{"", "a.b", "a.b.c.d", "eyJhbGciOiJIUzI1NiJ9.e30.AAAAAB", "eyJhbGciOiJIUzI1NiJ9.e30=.AAAAAA", "eyJhbGciOiJIUzI1NiJ9.e30.AAAAAA\n"} {
		err := compactjws.Check(bad)
		assert.True(t, errors.Is(err, compactjws.ErrNonCanonical), "%q: %v", bad, err)
	}
}

func TestSpareBitsBySignatureLength(t *testing.T) {
	for _, c := range []struct {
		name  string
		bytes int
		spare int
	}{
		{"HS256", 32, 2},
		{"ES256 / EdDSA (Ed25519)", 64, 4},
		{"RS256, 2048-bit key", 256, 4},
		{"RS256, 3072-bit key", 384, 0},
		{"RS256, 4096-bit key", 512, 2},
	} {
		n := base64.RawURLEncoding.EncodedLen(c.bytes)
		assert.Equal(t, c.spare, compactjwstest.SpareBits(n), "%s: %d bytes, %d chars", c.name, c.bytes, n)
	}
}

// FuzzDecodeSegmentIsOneToOne: whatever DecodeSegment accepts is exactly what
// the standard encoder writes for the bytes it returns, so no two accepted
// strings decode to the same bytes.
func FuzzDecodeSegmentIsOneToOne(f *testing.F) {
	for _, s := range []string{"", "A", "AA", "AB", "AAA", "AAB", "AAAA", "-_-_-g", "-_-_-h", "QQ==", "Q\nQ"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		b, err := compactjws.DecodeSegment(s)
		if err != nil {
			return
		}
		if again := base64.RawURLEncoding.EncodeToString(b); again != s {
			t.Fatalf("%q accepted but the encoder writes %q for its bytes", s, again)
		}
	})
}

// FuzzEncoderOutputIsAccepted: every string the standard encoder writes, for
// any bytes, is accepted and decodes back to those bytes. The platform's
// tokens are written by that encoder, so every token it issued is canonical
// and verifies with this SDK.
func FuzzEncoderOutputIsAccepted(f *testing.F) {
	for _, b := range [][]byte{nil, {0}, {0xff}, {0xff, 0xff}, {0xfb, 0xff, 0xbf}} {
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		s := base64.RawURLEncoding.EncodeToString(b)
		got, err := compactjws.DecodeSegment(s)
		if err != nil || string(got) != string(b) {
			t.Fatalf("encoder output %q refused or changed: %v", s, err)
		}
	})
}
