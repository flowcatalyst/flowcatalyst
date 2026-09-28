package fn

import (
	"encoding/binary"
	"errors"
)

// Frame layout, used in both directions and for host calls (Guest ABI v1,
// plan §5.2/§5.3):
//
//	u32 little-endian  metaLen
//	metaLen bytes      meta   (UTF-8 JSON)
//	remaining bytes    body   (raw)
var (
	errFrameTooShort    = errors.New("fn: frame shorter than the 4-byte length prefix")
	errFrameMetaOverrun = errors.New("fn: frame metaLen exceeds the frame length")
)

// encodeFrame lays out meta and body as a single ABI v1 frame.
func encodeFrame(meta, body []byte) []byte {
	buf := make([]byte, 4+len(meta)+len(body))
	binary.LittleEndian.PutUint32(buf[0:4], uint32(len(meta)))
	copy(buf[4:], meta)
	copy(buf[4+len(meta):], body)
	return buf
}

// decodeFrame splits an ABI v1 frame into its meta and body slices. The
// returned slices alias data; callers that retain them beyond the frame's
// validity window must copy.
func decodeFrame(data []byte) (meta, body []byte, err error) {
	if len(data) < 4 {
		return nil, nil, errFrameTooShort
	}
	metaLen := binary.LittleEndian.Uint32(data[0:4])
	if uint64(metaLen) > uint64(len(data)-4) {
		return nil, nil, errFrameMetaOverrun
	}
	meta = data[4 : 4+metaLen]
	body = data[4+metaLen:]
	return meta, body, nil
}
