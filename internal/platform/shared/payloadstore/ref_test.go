package payloadstore

import (
	"bytes"
	"strings"
	"testing"

	"github.com/klauspost/compress/zstd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRef_RoundTrip(t *testing.T) {
	r := Ref{Key: "v1/clt_0ABCDEFGHJKMN/2026-11-20/0ABCDEFGHJKMN", Offset: 12345, Length: 678, Codec: CodecZstd, CRC32C: 0x0a1b2c3d}
	s := r.String()
	assert.Equal(t, "v1:v1/clt_0ABCDEFGHJKMN/2026-11-20/0ABCDEFGHJKMN:12345:678:zstd:0a1b2c3d", s)
	got, err := ParseRef(s)
	require.NoError(t, err)
	assert.Equal(t, r, got)

	zero := Ref{Key: "v1/_/2026-01-01/X", Offset: 0, Length: 1, Codec: CodecNone, CRC32C: 0}
	got, err = ParseRef(zero.String())
	require.NoError(t, err)
	assert.Equal(t, zero, got)
	assert.Equal(t, "v1:v1/_/2026-01-01/X:0:1:none:00000000", zero.String())
}

func TestParseRef_RejectsMalformed(t *testing.T) {
	cases := map[string]string{
		"empty":               "",
		"too few fields":      "v1:k:0:1:zstd",
		"too many fields":     "v1:k:0:1:zstd:00000000:x",
		"colon in key":        "v1:a:b:0:1:zstd:00000000",
		"other version":       "v2:k:0:1:zstd:00000000",
		"upper version":       "V1:k:0:1:zstd:00000000",
		"empty key":           "v1::0:1:zstd:00000000",
		"absolute key":        "v1:/k:0:1:zstd:00000000",
		"trailing slash":      "v1:k/:0:1:zstd:00000000",
		"double slash":        "v1:a//b:0:1:zstd:00000000",
		"dot dot":             "v1:a/../b:0:1:zstd:00000000",
		"dot":                 "v1:a/./b:0:1:zstd:00000000",
		"space in key":        "v1:a b:0:1:zstd:00000000",
		"backslash":           `v1:a\b:0:1:zstd:00000000`,
		"key too long":        "v1:" + strings.Repeat("a", maxKeyLen+1) + ":0:1:zstd:00000000",
		"negative offset":     "v1:k:-1:1:zstd:00000000",
		"plus offset":         "v1:k:+1:1:zstd:00000000",
		"leading zero offset": "v1:k:01:1:zstd:00000000",
		"empty offset":        "v1:k::1:zstd:00000000",
		"hex offset":          "v1:k:0x1:1:zstd:00000000",
		"offset overflow":     "v1:k:9223372036854775808:1:zstd:00000000",
		"zero length":         "v1:k:0:0:zstd:00000000",
		"leading zero length": "v1:k:0:010:zstd:00000000",
		"range overflow":      "v1:k:9223372036854775807:1:zstd:00000000",
		"unknown codec":       "v1:k:0:1:gzip:00000000",
		"upper codec":         "v1:k:0:1:ZSTD:00000000",
		"short crc":           "v1:k:0:1:zstd:0000000",
		"long crc":            "v1:k:0:1:zstd:000000000",
		"upper crc":           "v1:k:0:1:zstd:0000000A",
		"non hex crc":         "v1:k:0:1:zstd:0000000g",
		"signed crc":          "v1:k:0:1:zstd:+0000000",
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := ParseRef(s)
			require.Error(t, err)
			assert.ErrorIs(t, err, errMalformedRef)
		})
	}
}

func TestParseCodec(t *testing.T) {
	for _, c := range []Codec{CodecZstd, CodecNone} {
		got, err := ParseCodec(string(c))
		require.NoError(t, err)
		assert.Equal(t, c, got)
	}
	for _, s := range []string{"", "ZSTD", " zstd", "gzip", "None"} {
		_, err := ParseCodec(s)
		assert.Error(t, err, s)
	}
}

func TestClientSegment(t *testing.T) {
	assert.Equal(t, "clt_0ABCDEFGHJKMN", clientSegment("clt_0ABCDEFGHJKMN"))
	assert.Equal(t, "a-b_C9", clientSegment("a-b_C9"))
	for _, unsafe := range []string{"", "..", ".", "a/b", "a:b", "a b", "a.b", strings.Repeat("x", 65), "é"} {
		assert.Equal(t, "_", clientSegment(unsafe), unsafe)
	}
}

func TestCodec_RoundTrip(t *testing.T) {
	payload := bytes.Repeat([]byte(`{"order":"SO-1001","lines":[{"sku":"A","qty":3}]}`), 200)
	for _, c := range []Codec{CodecZstd, CodecNone} {
		t.Run(string(c), func(t *testing.T) {
			prefix := []byte("earlier payloads")
			stored, err := encodeAppend(c, prefix, payload)
			require.NoError(t, err)
			require.Equal(t, prefix, stored[:len(prefix)], "appends after what is there")
			got, err := decode(c, stored[len(prefix):], len(payload))
			require.NoError(t, err)
			assert.Equal(t, payload, got)
			if c == CodecZstd {
				assert.Less(t, len(stored)-len(prefix), len(payload), "compresses")
			}
		})
	}
}

// Decompression is bounded by the size the row records: a frame that decodes
// to more, or less, is refused, including a frame that does not state its
// size (a streamed one), which a corrupt or forged object could be.
func TestCodec_DecodeIsBoundedBySize(t *testing.T) {
	payload := bytes.Repeat([]byte("x"), 10_000)
	stored, err := encodeAppend(CodecZstd, nil, payload)
	require.NoError(t, err)

	_, err = decode(CodecZstd, stored, len(payload)-1)
	assert.Error(t, err, "one byte less than the frame holds")
	_, err = decode(CodecZstd, stored, len(payload)+1)
	assert.ErrorIs(t, err, errSizeMismatch, "one byte more than the frame holds")

	// A streamed frame carries no content size, so only the cap limit stops
	// it: 16 MiB of zeros must not be decoded into a 1 000-byte payload.
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	require.NoError(t, err)
	_, err = w.Write(make([]byte, 16<<20))
	require.NoError(t, err)
	require.NoError(t, w.Close())
	_, err = decode(CodecZstd, buf.Bytes(), 1000)
	require.Error(t, err)

	// Not a zstd frame at all.
	_, err = decode(CodecZstd, []byte("not zstd"), 8)
	assert.Error(t, err)

	// none: the length must be the size.
	_, err = decode(CodecNone, []byte("abc"), 4)
	assert.ErrorIs(t, err, errSizeMismatch)
}
