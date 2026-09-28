// Package abi is the function runner's guest calling convention, version 1
// (docs/function-runner-plan.md §5): the frame layout both directions use,
// the request/response metadata, the host-call op codes and error codes, and
// the describe document a module declares its needs with.
//
// It is shared by the runner (which speaks it to guests) and the platform
// (which reads a module's describe document at publish), so it has no
// dependency on the wasm engine.
package abi

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

// A frame is `u32 little-endian metaLen | meta (UTF-8 JSON) | body (the rest)`.
// Bodies travel raw, never base64.
const frameHeaderLen = 4

// ErrMalformedFrame is returned when bytes do not form a frame.
var ErrMalformedFrame = errors.New("abi: malformed frame")

// EncodeFrame lays out meta and body as one frame. meta must already be JSON.
func EncodeFrame(meta, body []byte) []byte {
	out := make([]byte, frameHeaderLen+len(meta)+len(body))
	binary.LittleEndian.PutUint32(out, uint32(len(meta)))
	copy(out[frameHeaderLen:], meta)
	copy(out[frameHeaderLen+len(meta):], body)
	return out
}

// MarshalFrame JSON-encodes meta and lays it out with body as one frame.
func MarshalFrame(meta any, body []byte) ([]byte, error) {
	m, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("abi: encode frame meta: %w", err)
	}
	return EncodeFrame(m, body), nil
}

// DecodeFrame splits a frame into its meta and body. Both alias b.
func DecodeFrame(b []byte) (meta, body []byte, err error) {
	if len(b) < frameHeaderLen {
		return nil, nil, fmt.Errorf("%w: %d bytes, shorter than the header", ErrMalformedFrame, len(b))
	}
	n := binary.LittleEndian.Uint32(b)
	if uint64(n) > uint64(len(b)-frameHeaderLen) {
		return nil, nil, fmt.Errorf("%w: meta length %d exceeds the %d bytes after the header", ErrMalformedFrame, n, len(b)-frameHeaderLen)
	}
	end := frameHeaderLen + int(n)
	return b[frameHeaderLen:end], b[end:], nil
}

// UnmarshalFrame decodes a frame and JSON-decodes its meta into v.
func UnmarshalFrame(b []byte, v any) (body []byte, err error) {
	meta, body, err := DecodeFrame(b)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(meta, v); err != nil {
		return nil, fmt.Errorf("%w: meta is not the expected JSON: %w", ErrMalformedFrame, err)
	}
	return body, nil
}

// Pack joins a guest pointer and length into the i64 that fc_handle,
// fc_describe and fc.call return: `hi<<32 | lo`.
func Pack(hi, lo uint32) uint64 { return uint64(hi)<<32 | uint64(lo) }

// Unpack is the inverse of Pack.
func Unpack(v uint64) (hi, lo uint32) { return uint32(v >> 32), uint32(v) }
