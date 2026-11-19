// Package compactjws holds a JWS in compact serialisation
// (header.payload.signature, RFC 7515 §7.1) to its one canonical spelling.
// It is the SDK's copy of the platform's rule (docs/wire-contract.md,
// Authentication, "Token spelling"); the SDK module cannot import the
// platform's packages, and the two are held to the same vectors
// (api/field-fixtures/noncanonical.json).
//
// A base64url segment whose length is not a multiple of four ends in a
// character with spare low bits (two when the length is 3 mod 4, four when it
// is 2 mod 4). A lenient decoder ignores them, so several strings decode to
// the same bytes; the signature segment is decoded before it is checked, so a
// re-spelled signature verifies. The JWT library the SDK uses (jwx) decodes
// with no check on the spare bits, accepts '=' padding and the standard
// alphabet on its key-set path, and Go's decoder skips CR and LF even in
// strict mode. Without this package one token has many spellings that verify.
//
// The canonical spelling is RFC 7515 §2's base64url: the URL-safe alphabet of
// RFC 4648 §5, no '=' padding, no line breaks or other characters, and (RFC
// 4648 §3.5) the spare bits zero. A standard encoder emits exactly that, so a
// token the platform issued is always canonical.
package compactjws

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// ErrNonCanonical is returned for a token or segment that is not spelled as
// canonical base64url.
var ErrNonCanonical = errors.New("compactjws: not canonical base64url")

// strict is base64url without padding that refuses nonzero spare bits. It
// still skips CR and LF, which [DecodeSegment] refuses before decoding.
var strict = base64.RawURLEncoding.Strict()

// DecodeSegment decodes one base64url segment, accepting only its canonical
// spelling: characters of the URL-safe alphabet only (no '=', no '+' or '/',
// no whitespace or line breaks) and the spare bits of the last character zero.
// For any input it accepts, encoding the result with
// base64.RawURLEncoding gives the input back.
func DecodeSegment(seg string) ([]byte, error) {
	for i := 0; i < len(seg); i++ {
		if !isURLAlphabet(seg[i]) {
			return nil, fmt.Errorf("%w: character %q at %d", ErrNonCanonical, seg[i], i)
		}
	}
	b, err := strict.DecodeString(seg)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNonCanonical, err)
	}
	return b, nil
}

// Check reports whether token is a JWS in compact serialisation (three
// dot-separated segments) with every segment spelled canonically. It checks
// the spelling only: the caller still verifies the signature and the claims.
func Check(token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return fmt.Errorf("%w: %d segments, want 3", ErrNonCanonical, len(parts))
	}
	for i, seg := range parts {
		if _, err := DecodeSegment(seg); err != nil {
			return fmt.Errorf("segment %d: %w", i, err)
		}
	}
	return nil
}

func isURLAlphabet(c byte) bool {
	return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' || c == '_'
}
