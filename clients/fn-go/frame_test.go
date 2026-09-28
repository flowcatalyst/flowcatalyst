//go:build !wasip1

package fn

import (
	"bytes"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		meta []byte
		body []byte
	}{
		{"both empty", nil, nil},
		{"meta only", []byte(`{"a":1}`), nil},
		{"body only", []byte(`{}`), []byte("hello world")},
		{"both present", []byte(`{"status":200}`), []byte(`{"ok":true}`)},
		{"binary body", []byte(`{}`), []byte{0x00, 0xff, 0x10, 0x00, 0x00}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			frame := encodeFrame(tc.meta, tc.body)
			gotMeta, gotBody, err := decodeFrame(frame)
			if err != nil {
				t.Fatalf("decodeFrame: %v", err)
			}
			if !bytes.Equal(gotMeta, tc.meta) {
				t.Errorf("meta = %q, want %q", gotMeta, tc.meta)
			}
			if !bytes.Equal(gotBody, tc.body) {
				t.Errorf("body = %q, want %q", gotBody, tc.body)
			}
		})
	}
}

// TestFrameLittleEndian pins the wire layout byte-for-byte: the metaLen
// prefix must be little-endian (plan §5.2), so a metaLen of e.g. 0x0102
// (258) is encoded as bytes {0x02, 0x01, 0x00, 0x00}, not big-endian
// {0x00, 0x00, 0x01, 0x02}. This is the case a big->little mutation flips.
func TestFrameLittleEndian(t *testing.T) {
	meta := bytes.Repeat([]byte("x"), 258) // metaLen = 0x0102
	body := []byte("body")
	frame := encodeFrame(meta, body)

	want := []byte{0x02, 0x01, 0x00, 0x00}
	if !bytes.Equal(frame[0:4], want) {
		t.Fatalf("length prefix = % x, want % x (little-endian 258)", frame[0:4], want)
	}
}

func TestDecodeFrameShort(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3} {
		if _, _, err := decodeFrame(make([]byte, n)); err == nil {
			t.Errorf("decodeFrame(%d zero bytes): want error, got nil", n)
		}
	}
}

func TestDecodeFrameMetaOverrun(t *testing.T) {
	// metaLen claims 100 bytes but the frame only carries 4.
	frame := make([]byte, 8)
	frame[0] = 100
	if _, _, err := decodeFrame(frame); err == nil {
		t.Error("decodeFrame with metaLen > available bytes: want error, got nil")
	}
}

func TestDecodeFrameExactMetaNoBody(t *testing.T) {
	frame := encodeFrame([]byte(`{"a":1}`), nil)
	meta, body, err := decodeFrame(frame)
	if err != nil {
		t.Fatalf("decodeFrame: %v", err)
	}
	if string(meta) != `{"a":1}` {
		t.Errorf("meta = %q", meta)
	}
	if len(body) != 0 {
		t.Errorf("body = %q, want empty", body)
	}
}
