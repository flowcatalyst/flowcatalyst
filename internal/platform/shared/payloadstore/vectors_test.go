package payloadstore

import (
	"encoding/hex"
	"encoding/json"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// The golden vectors of testdata/payload_vectors.json are segments this
// package wrote and the references into them. They are the storage contract
// between implementations (docs/java-handoff-2026-11-20-payload-table.md): a
// port proves it can read what this one writes by resolving every reference
// of the file to its payload, and this test proves the file still matches the
// code. A port's own encoder may produce different bytes for the same
// payload; any standard zstd frame of the right decoded length is valid.
//
// Regenerate with UPDATE_PAYLOAD_VECTORS=1 go test -run TestPayloadVectors ./internal/platform/shared/payloadstore/
// and only when the format changes on purpose.

type vectorPayload struct {
	Name    string `json:"name"`
	Ref     string `json:"ref"`
	Size    int    `json:"size"`
	Payload string `json:"payload"`
}

type vectorSegment struct {
	Key      string          `json:"key"`
	Codec    string          `json:"codec"`
	Hex      string          `json:"segmentHex"`
	Payloads []vectorPayload `json:"payloads"`
}

const vectorsFile = "testdata/payload_vectors.json"

func vectorPayloads() []vectorPayload {
	lines := make([]string, 0, 40)
	for i := range 40 {
		lines = append(lines, `{"sku":"SKU-`+strings.Repeat("7", i%5+1)+`","description":"Sparkling water 500 ml","quantity":`+string(rune('1'+i%9))+`,"picked":true}`)
	}
	return []vectorPayload{
		{Name: "odd spacing and key order, as a producer sent it", Payload: `{"b":1,  "a":2, "a":3}`},
		{Name: "non-ASCII text", Payload: `{"name":"Zoë Nyembezi","note":"näive café, 5 €, 日本語"}`},
		{Name: "empty object, the data of an event with none", Payload: `{}`},
		{Name: "a repetitive document that compresses well", Payload: `{"orderId":"ORD-000000042","lines":[` + strings.Join(lines, ",") + `]}`},
		{Name: "not JSON: a job's payload may be any text", Payload: "plain text\nwith two lines\tand a tab"},
	}
}

func buildVectorSegment(t *testing.T, codec Codec) vectorSegment {
	t.Helper()
	key := "v1/clt_0HZXK3M9P2Q7R/2026-11-20/0HZXK3M9P2Q7S"
	if codec == CodecNone {
		key = "v1/_/2026-11-20/0HZXK3M9P2Q7T"
	}
	var seg []byte
	out := vectorSegment{Key: key, Codec: string(codec)}
	for _, p := range vectorPayloads() {
		off := len(seg)
		var err error
		seg, err = encodeAppend(codec, seg, []byte(p.Payload))
		require.NoError(t, err)
		ref := Ref{Key: key, Offset: int64(off), Length: int64(len(seg) - off), Codec: codec, CRC32C: crc32.Checksum(seg[off:], castagnoli)}
		p.Ref, p.Size = ref.String(), len(p.Payload)
		out.Payloads = append(out.Payloads, p)
	}
	out.Hex = hex.EncodeToString(seg)
	return out
}

func TestPayloadVectors(t *testing.T) {
	if os.Getenv("UPDATE_PAYLOAD_VECTORS") == "1" {
		segs := []vectorSegment{buildVectorSegment(t, CodecZstd), buildVectorSegment(t, CodecNone)}
		b, err := json.MarshalIndent(segs, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(vectorsFile), 0o755))
		require.NoError(t, os.WriteFile(vectorsFile, append(b, '\n'), 0o644))
	}
	// PAYLOAD_VECTORS_FILE runs this reader over a file a port wrote with its
	// own encoder (same shape): the proof that this implementation reads what
	// the port writes.
	file := vectorsFile
	if f := os.Getenv("PAYLOAD_VECTORS_FILE"); f != "" {
		file = f
	}
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	var segs []vectorSegment
	require.NoError(t, json.Unmarshal(raw, &segs))
	require.Len(t, segs, 2)
	for _, seg := range segs {
		stored, err := hex.DecodeString(seg.Hex)
		require.NoError(t, err)
		require.Len(t, seg.Payloads, len(vectorPayloads()))
		for _, p := range seg.Payloads {
			t.Run(seg.Codec+"/"+p.Name, func(t *testing.T) {
				ref, err := ParseRef(p.Ref)
				require.NoError(t, err)
				require.Equal(t, p.Ref, ref.String(), "a reference round-trips unchanged")
				require.Equal(t, seg.Key, ref.Key)
				require.Equal(t, seg.Codec, string(ref.Codec))
				require.LessOrEqual(t, ref.Offset+ref.Length, int64(len(stored)))
				part := stored[ref.Offset : ref.Offset+ref.Length]
				require.Equal(t, ref.CRC32C, crc32.Checksum(part, castagnoli), "CRC-32C of the stored bytes")
				got, err := decode(ref.Codec, part, p.Size)
				require.NoError(t, err)
				require.Equal(t, p.Payload, string(got))
				// One byte short or long is refused: the size bounds the decode.
				_, err = decode(ref.Codec, part, p.Size+1)
				require.Error(t, err)
			})
		}
	}
}
