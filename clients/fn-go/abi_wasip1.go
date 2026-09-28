//go:build wasip1

package fn

import (
	"encoding/json"
	"unsafe"
)

// This file is the entire ABI surface (plan §5): the two `fc` host imports
// and the four guest exports. Everything else in the package is portable Go
// exercised through the host interface (host.go) and dispatch.go, so it can
// be unit-tested on the host OS via abi_other.go's fake host.

//go:wasmimport fc call
func fcImportCall(op int32, ptr int32, ln int32) int64

//go:wasmimport fc take
func fcImportTake(dst int32)

// wasipHost implements host over the real `fc` module imports.
type wasipHost struct{}

func init() {
	currentHost = wasipHost{}
}

func (wasipHost) call(op opCode, meta, body []byte) (respMeta, respBody []byte, hostErr *Error, err error) {
	frame := encodeFrame(meta, body)
	var ptr int32
	if len(frame) > 0 {
		ptr = int32(uintptr(unsafe.Pointer(&frame[0])))
	}
	packed := fcImportCall(int32(op), ptr, int32(len(frame)))
	code := int32(uint64(packed) >> 32)
	resultLen := uint32(uint64(packed))

	if resultLen == 0 {
		if code != 0 {
			return nil, nil, &Error{Code: "UNKNOWN", Message: "host call failed with no result frame"}, nil
		}
		return nil, nil, nil, nil
	}

	result := make([]byte, resultLen)
	fcImportTake(int32(uintptr(unsafe.Pointer(&result[0]))))

	rm, rb, derr := decodeFrame(result)
	if derr != nil {
		return nil, nil, nil, derr
	}
	if code != 0 {
		var e Error
		if uerr := json.Unmarshal(rm, &e); uerr != nil {
			return nil, nil, &Error{Code: "UNKNOWN", Message: string(rm)}, nil
		}
		return nil, nil, &e, nil
	}
	return rm, rb, nil, nil
}

// guestOutBuf holds the most recent response/describe frame handed back to
// the host. Per plan §5.1 it is "valid until the next call into the guest",
// so a single package-level buffer per kind is correct: fc_alloc's buffer is
// independent of fc_handle/fc_describe's output buffer.
var (
	guestAllocBuf []byte
	guestOutBuf   []byte
)

//go:wasmexport fc_abi_v1
func fcAbiV1Export() {}

//go:wasmexport fc_alloc
func fcAllocExport(size int32) int32 {
	guestAllocBuf = make([]byte, size)
	if size == 0 {
		return 0
	}
	return int32(uintptr(unsafe.Pointer(&guestAllocBuf[0])))
}

//go:wasmexport fc_handle
func fcHandleExport(ptr int32, ln int32) int64 {
	var req []byte
	if ln > 0 {
		req = unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), ln)
	}
	guestOutBuf = handleRequestFrame(req)
	return packPtrLen(guestOutBuf)
}

//go:wasmexport fc_describe
func fcDescribeExport() int64 {
	guestOutBuf = describeJSON()
	return packPtrLen(guestOutBuf)
}

func packPtrLen(b []byte) int64 {
	if len(b) == 0 {
		return 0
	}
	return int64(uintptr(unsafe.Pointer(&b[0])))<<32 | int64(uint32(len(b)))
}
