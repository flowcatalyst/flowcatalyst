//go:build wasip1

// Spike guest, Go (GOOS=wasip1 c-shared), raw ABI v1 (no SDK), for the shapes
// the benchmark guest does not have. Request meta is {"w":N,"n":M}:
//
//	w=10  unconditional loop, never returns (a compiled runaway)
//	w=11  allocate and touch 1 MiB blocks until memory runs out
//	w=12  n host calls (config.get "BENCH"), then answer "ok" (per-host-call cost)
//	w=13  return "ok" (the baseline for w=12 and for "the next call works")
package main

import (
	"encoding/binary"
	"encoding/json"
	"strconv"
	"unsafe"
)

//go:wasmimport fc call
func fcCall(op int32, ptr int32, ln int32) int64

//go:wasmimport fc take
func fcTake(dst int32)

var allocBuf, outBuf []byte

//go:wasmexport fc_abi_v1
func fcAbiV1() {}

//go:wasmexport fc_alloc
func fcAlloc(n int32) int32 {
	allocBuf = make([]byte, n)
	if n == 0 {
		return 0
	}
	return int32(uintptr(unsafe.Pointer(&allocBuf[0])))
}

//go:wasmexport fc_describe
func fcDescribe() int64 { return pack([]byte(`{"abi":1}`)) }

func pack(b []byte) int64 {
	outBuf = b
	if len(b) == 0 {
		return 0
	}
	return int64(uintptr(unsafe.Pointer(&b[0])))<<32 | int64(uint32(len(b)))
}

func frame(meta, body []byte) []byte {
	f := make([]byte, 4+len(meta)+len(body))
	binary.LittleEndian.PutUint32(f, uint32(len(meta)))
	copy(f[4:], meta)
	copy(f[4+len(meta):], body)
	return f
}

var sink uint64
var keep [][]byte

//go:wasmexport fc_handle
func fcHandle(ptr int32, ln int32) int64 {
	in := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), ln)
	ml := int(binary.LittleEndian.Uint32(in))
	var m struct{ W, N int }
	_ = json.Unmarshal(in[4:4+ml], &m)
	resp := []byte("ok")
	switch m.W {
	case 10:
		x := uint64(1)
		for {
			x = x*6364136223846793005 + 1442695040888963407
			sink = x
		}
	case 11:
		for {
			b := make([]byte, 1<<20)
			for i := 0; i < len(b); i += 4096 {
				b[i] = 1
			}
			keep = append(keep, b)
		}
	case 12:
		meta := frame([]byte(`{"key":"BENCH"}`), nil)
		sum := 0
		for i := 0; i < m.N; i++ {
			r := fcCall(2, int32(uintptr(unsafe.Pointer(&meta[0]))), int32(len(meta)))
			n := int(uint32(r))
			res := make([]byte, n)
			fcTake(int32(uintptr(unsafe.Pointer(&res[0]))))
			sum += n
		}
		resp = []byte(strconv.Itoa(sum))
	}
	return pack(frame([]byte(`{"status":200}`), resp))
}

func main() {}
