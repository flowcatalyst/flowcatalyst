package payloadstore

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Codec is how an offloaded payload's bytes are compressed in its segment.
// A closed set: every switch over it names each member.
type Codec string

const (
	// CodecZstd: each payload compressed on its own as one zstd frame.
	CodecZstd Codec = "zstd"
	// CodecNone: the payload's bytes as sent.
	CodecNone Codec = "none"
)

// ParseCodec reads a codec name, refusing anything that is not a member
// (case and surrounding space included).
func ParseCodec(s string) (Codec, error) {
	switch c := Codec(s); c {
	case CodecZstd, CodecNone:
		return c, nil
	}
	return "", fmt.Errorf("unknown payload codec %q: zstd or none", s)
}

// refVersion is the first field of every reference this package writes.
const refVersion = "v1"

// maxKeyLen bounds the key of a reference. A key this package writes is
// about 45 characters.
const maxKeyLen = 512

// Ref is where an offloaded payload is (docs/payload-offload-design.md,
// section 5): Length bytes at Offset in the object Key of the payload store,
// compressed with Codec, whose CRC-32C (Castagnoli) is CRC32C. Its text form,
// stored in msg_payloads.ref, is
//
//	v1:<key>:<offset>:<length>:<codec>:<crc32c>
//
// with offset and length in decimal and the checksum as eight lower-case hex
// digits. The bucket and prefix of the store are not in it: they are the
// deployment's configuration. A key holds no colon.
type Ref struct {
	Key    string
	Offset int64
	Length int64
	Codec  Codec
	CRC32C uint32
}

// String is the reference's stored form.
func (r Ref) String() string {
	return fmt.Sprintf("%s:%s:%d:%d:%s:%08x", refVersion, r.Key, r.Offset, r.Length, r.Codec, r.CRC32C)
}

var errMalformedRef = errors.New("payloadstore: malformed payload reference")

// ParseRef reads the stored form of a reference, refusing anything that is
// not exactly what String writes: another version, a key that is not a
// relative object key, an offset or length that is not a canonical decimal
// (no sign, no leading zero), an empty range, an unknown codec, or a checksum
// that is not eight lower-case hex digits.
func ParseRef(s string) (Ref, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return Ref{}, fmt.Errorf("%w: %d fields, want 6", errMalformedRef, len(parts))
	}
	if parts[0] != refVersion {
		return Ref{}, fmt.Errorf("%w: version %q, want %s", errMalformedRef, parts[0], refVersion)
	}
	if err := checkKey(parts[1]); err != nil {
		return Ref{}, fmt.Errorf("%w: %w", errMalformedRef, err)
	}
	offset, ok := parseDecimal(parts[2])
	if !ok {
		return Ref{}, fmt.Errorf("%w: offset %q", errMalformedRef, parts[2])
	}
	length, ok := parseDecimal(parts[3])
	if !ok || length == 0 {
		return Ref{}, fmt.Errorf("%w: length %q", errMalformedRef, parts[3])
	}
	if offset > (1<<63-1)-length {
		return Ref{}, fmt.Errorf("%w: range overflows", errMalformedRef)
	}
	codec, err := ParseCodec(parts[4])
	if err != nil {
		return Ref{}, fmt.Errorf("%w: %w", errMalformedRef, err)
	}
	crc, ok := parseCRC(parts[5])
	if !ok {
		return Ref{}, fmt.Errorf("%w: checksum %q", errMalformedRef, parts[5])
	}
	return Ref{Key: parts[1], Offset: offset, Length: length, Codec: codec, CRC32C: crc}, nil
}

// parseDecimal reads a non-negative int64 in canonical decimal: digits only,
// no leading zero except "0" itself.
func parseDecimal(s string) (int64, bool) {
	if s == "" || len(s) > 19 || (len(s) > 1 && s[0] == '0') {
		return 0, false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// parseCRC reads eight lower-case hex digits.
func parseCRC(s string) (uint32, bool) {
	if len(s) != 8 {
		return 0, false
	}
	for i := range len(s) {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 0, false
	}
	return uint32(n), true
}

// checkKey refuses a key that is not a relative object key made of non-empty
// segments of [A-Za-z0-9._-] separated by "/", none of them "." or "..". The
// same rule guards the file store from a key that would leave its directory.
func checkKey(key string) error {
	if key == "" || len(key) > maxKeyLen {
		return fmt.Errorf("key length %d out of range 1 to %d", len(key), maxKeyLen)
	}
	for seg := range strings.SplitSeq(key, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("key %q has an empty, . or .. segment", key)
		}
		for i := range len(seg) {
			if !keyChar(seg[i]) {
				return fmt.Errorf("key %q holds a character outside [A-Za-z0-9._-]", key)
			}
		}
	}
	return nil
}

func keyChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
		c == '_' || c == '-' || c == '.'
}

// clientSegment is the client's part of a segment key: the client id when it
// is a plain id of [A-Za-z0-9_-] (at most 64 characters), "_" for no client.
// client_id on the messaging tables comes from unchecked SDK payloads, so an
// id that could not be a key segment is also stored under "_".
func clientSegment(clientID string) string {
	if clientID == "" || len(clientID) > 64 {
		return "_"
	}
	for i := range len(clientID) {
		c := clientID[i]
		if c == '.' || !keyChar(c) {
			return "_"
		}
	}
	return clientID
}
