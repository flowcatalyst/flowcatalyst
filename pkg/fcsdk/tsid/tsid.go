// Package tsid generates Time-Sorted IDs as Crockford Base32 strings.
//
// Compatible with the Rust fc-common::tsid generator and the Java
// TsidGenerator. A raw TSID is 13 characters; a typed TSID is
// {prefix}_{13-char-raw} (e.g. "clt_0HZXEQ5Y8JY5Z").
//
// Layout of the 64-bit value (matches Rust):
//
//	bits 63..22  timestamp (42 bits, millis since epoch)
//	bits 21..12  random  (10 bits)
//	bits 11..0   counter (12 bits)
//
// This package is the SDK-exported primitive set: generation, encoding,
// decoding. FlowCatalyst's typed-entity catalog lives in the platform's
// internal/tsid package and forwards to these primitives, so SDK
// consumers (and operations within the platform) share a single
// implementation.
package tsid

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"strings"
	"sync/atomic"
	"time"
)

// alphabet is the Crockford Base32 alphabet (excludes I, L, O, U).
const alphabet = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// counter is a monotonic 12-bit counter.
var counter atomic.Uint32

// Generate returns a typed TSID with the entity's 3-char prefix:
// "{prefix}_{13-char-raw}", e.g. "clt_0HZXEQ5Y8JY5Z".
func Generate(e EntityType) string {
	return e.Prefix() + "_" + GenerateRaw()
}

// GenerateWithPrefix returns a typed TSID with a custom prefix.
// Use this for application-specific entity types not in EntityType.
func GenerateWithPrefix(prefix string) string {
	return prefix + "_" + GenerateRaw()
}

// GenerateUntyped returns a raw 13-character TSID with no prefix. Used
// for execution IDs, trace IDs, event IDs, and other non-entity contexts.
func GenerateUntyped() string {
	return GenerateRaw()
}

// GenerateRaw produces the 13-character Crockford Base32 TSID.
func GenerateRaw() string {
	now := uint64(time.Now().UnixMilli())
	cnt := uint64(counter.Add(1)) & 0xFFF
	r := uint64(randomU10()) & 0x3FF

	tsid := ((now & 0x3FFFFFFFFFF) << 22) | (r << 12) | cnt
	return encodeCrockford(tsid)
}

// ToLong converts a TSID string (typed or raw) to its numeric form.
// Returns ok=false if the input is not a valid 13-char Crockford Base32.
func ToLong(s string) (int64, bool) {
	raw := s
	if len(s) > 14 && strings.Contains(s, "_") {
		parts := strings.SplitN(s, "_", 2)
		if len(parts) != 2 {
			return 0, false
		}
		raw = parts[1]
	}
	v, ok := decodeCrockford(raw)
	return int64(v), ok
}

// FromLong converts a numeric TSID to its raw string form (no prefix).
func FromLong(v int64) string { return encodeCrockford(uint64(v)) }

func encodeCrockford(v uint64) string {
	var b [13]byte
	for i := 12; i >= 0; i-- {
		b[i] = alphabet[v&0x1F]
		v >>= 5
	}
	return string(b[:])
}

// DecodeCrockford parses a 13-char Crockford Base32 string back to its
// 64-bit value. Returns 0, false on malformed input.
func DecodeCrockford(s string) (uint64, bool) {
	return decodeCrockford(s)
}

func decodeCrockford(s string) (uint64, bool) {
	if len(s) != 13 {
		return 0, false
	}
	var v uint64
	for i := range len(s) {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 32 // toupper
		}
		var d uint64
		switch {
		case c >= '0' && c <= '9':
			d = uint64(c - '0')
		case c >= 'A' && c <= 'H':
			d = uint64(c-'A') + 10
		case c >= 'J' && c <= 'K':
			d = uint64(c-'J') + 18
		case c >= 'M' && c <= 'N':
			d = uint64(c-'M') + 20
		case c >= 'P' && c <= 'T':
			d = uint64(c-'P') + 22
		case c >= 'V' && c <= 'Z':
			d = uint64(c-'V') + 27
		default:
			return 0, false
		}
		v = (v << 5) | d
	}
	return v, true
}

// randomU10 returns a 10-bit random value. Falls back to a
// time-derived value if crypto/rand fails.
func randomU10() uint16 {
	var b [2]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return uint16(time.Now().UnixNano()) & 0x3FF
	}
	return binary.BigEndian.Uint16(b[:]) & 0x3FF
}
