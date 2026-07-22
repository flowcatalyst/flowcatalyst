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

// state packs the last-issued (millisecond, sequence) pair into one atomic
// word: bits 63..12 the millisecond timestamp, bits 11..0 the 12-bit
// intra-millisecond sequence. Advancing it via CAS means every issued
// (ms, seq) pair is unique within the process, which makes single-process
// collisions structurally impossible — the 10-bit random field only has to
// defend against OTHER processes minting in the same millisecond.
//
// The previous implementation used a free-running 12-bit counter, so a
// process minting >4096 ids inside one millisecond wrapped the counter and
// collided whenever the 10-bit random also matched (p≈1/1024 per wrapped
// pair — observed as a rare TestUniqueness flake on fast hardware).
var state atomic.Uint64

// nextMsSeq issues the next unique (millisecond, sequence) pair.
//
//   - fresh millisecond → sequence restarts at a RANDOM offset (decorrelates
//     sequences across processes that restart or tick over together);
//   - same millisecond → sequence increments;
//   - sequence exhausted (4096 in one ms) → borrow the NEXT millisecond and
//     keep going, i.e. run slightly ahead of the wall clock rather than
//     reuse a sequence value. Sustained >4.096M ids/sec would drift the
//     timestamp ahead; bursts just smear into the following millisecond(s).
//
// The state only moves forward, so a wall-clock step backwards (NTP) cannot
// cause reuse either — ids keep issuing from the high-water millisecond.
func nextMsSeq() (ms, seq uint64) {
	for {
		now := uint64(time.Now().UnixMilli())
		old := state.Load()
		lastMs := old >> 12
		lastSeq := old & 0xFFF
		switch {
		case now > lastMs:
			ms, seq = now, uint64(randomU12())
		case lastSeq < 0xFFF:
			ms, seq = lastMs, lastSeq+1
		default:
			ms, seq = lastMs+1, uint64(randomU12())
		}
		if state.CompareAndSwap(old, ms<<12|seq) {
			return ms, seq
		}
	}
}

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
	ms, seq := nextMsSeq()
	r := uint64(randomU10()) & 0x3FF

	tsid := ((ms & 0x3FFFFFFFFFF) << 22) | (r << 12) | seq
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

// randomU12 returns a 12-bit random value — the per-millisecond sequence
// starting offset. Same fallback posture as randomU10.
func randomU12() uint16 {
	var b [2]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return uint16(time.Now().UnixNano()) & 0xFFF
	}
	return binary.BigEndian.Uint16(b[:]) & 0xFFF
}
