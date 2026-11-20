package payloadstore

import (
	"errors"
	"fmt"
	"sync"

	"github.com/klauspost/compress/zstd"
)

// The zstd encoder and decoder are made once and shared: they are not free to
// make, and EncodeAll and DecodeAll are safe for concurrent use. The decoder
// limits DecodeAll to the capacity of the buffer it is given
// (WithDecodeAllCapLimit), so a payload is never decoded past the size its row
// records, whatever the object holds.
var (
	zstdEncoder = sync.OnceValues(func() (*zstd.Encoder, error) {
		return zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))
	})
	zstdDecoder = sync.OnceValues(func() (*zstd.Decoder, error) {
		return zstd.NewReader(nil,
			zstd.WithDecodeAllCapLimit(true),
			zstd.WithDecoderMaxMemory(maxSize),
		)
	})
)

// encodeAppend appends payload, compressed with codec, to dst.
func encodeAppend(codec Codec, dst, payload []byte) ([]byte, error) {
	switch codec {
	case CodecZstd:
		enc, err := zstdEncoder()
		if err != nil {
			return dst, fmt.Errorf("payloadstore: zstd encoder: %w", err)
		}
		return enc.EncodeAll(payload, dst), nil
	case CodecNone:
		return append(dst, payload...), nil
	}
	return dst, fmt.Errorf("payloadstore: unknown codec %q", codec)
}

var errSizeMismatch = errors.New("decoded size does not match the stored size")

// decode decompresses stored, which codec produced, into a payload of exactly
// size bytes. It never produces more than size bytes: a frame that would
// decode to more, or less, is an error.
func decode(codec Codec, stored []byte, size int) ([]byte, error) {
	switch codec {
	case CodecZstd:
		dec, err := zstdDecoder()
		if err != nil {
			return nil, fmt.Errorf("payloadstore: zstd decoder: %w", err)
		}
		out, err := dec.DecodeAll(stored, make([]byte, 0, size))
		if err != nil {
			return nil, fmt.Errorf("zstd: %w", err)
		}
		if len(out) != size {
			return nil, fmt.Errorf("%w: %d bytes, want %d", errSizeMismatch, len(out), size)
		}
		return out, nil
	case CodecNone:
		if len(stored) != size {
			return nil, fmt.Errorf("%w: %d bytes, want %d", errSizeMismatch, len(stored), size)
		}
		return stored, nil
	}
	return nil, fmt.Errorf("payloadstore: unknown codec %q", codec)
}
