//go:build wasip1 || tinygo

// Benchmark function, Go (GOOS=wasip1 c-shared, or TinyGo), raw ABI v1 (no SDK).
// Same workloads and byte-identical outputs as js/workloads.ts. Request meta is `{"w":N}`.
package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"strconv"
	"strings"
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

//go:wasmexport fc_handle
func fcHandle(ptr int32, ln int32) int64 {
	in := unsafe.Slice((*byte)(unsafe.Pointer(uintptr(ptr))), ln)
	ml := int(binary.LittleEndian.Uint32(in))
	meta, body := in[4:4+ml], in[4+ml:]
	var resp []byte
	switch meta[5] - '0' {
	case 1:
		resp = append([]byte(nil), body...)
	case 2:
		resp = w2(body)
	case 3:
		resp = w3(body)
	case 4:
		resp = w4()
	case 5:
		resp = w5()
	}
	return pack(frame([]byte(`{"status":200}`), resp))
}

func enc(v any) []byte {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	_ = e.Encode(v)
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

type inLine struct {
	Sku       string `json:"sku"`
	Qty       int64  `json:"qty"`
	UnitCents int64  `json:"unitCents"`
}
type order struct {
	OrderID  string `json:"orderId"`
	Customer struct {
		Code string `json:"code"`
	} `json:"customer"`
	Currency string   `json:"currency"`
	Lines    []inLine `json:"lines"`
}
type outLine struct {
	Sku         string `json:"sku"`
	AmountCents int64  `json:"amountCents"`
}
type outOrder struct {
	ID         string    `json:"id"`
	Customer   string    `json:"customer"`
	Currency   string    `json:"currency"`
	LineCount  int       `json:"lineCount"`
	TotalCents int64     `json:"totalCents"`
	Lines      []outLine `json:"lines"`
}

func w2(body []byte) []byte {
	var o order
	if err := json.Unmarshal(body, &o); err != nil {
		panic(err)
	}
	var total int64
	lines := make([]outLine, 0, len(o.Lines))
	for _, l := range o.Lines {
		a := l.Qty * l.UnitCents
		total += a
		lines = append(lines, outLine{l.Sku, a})
	}
	return enc(outOrder{o.OrderID, o.Customer.Code, o.Currency, len(lines), total, lines})
}

type xLoc struct {
	Code string `xml:"Code" json:"code"`
	Name string `xml:"Name" json:"name"`
}
type xLine struct {
	Number string `xml:"number,attr"`
	Sku    string `xml:"Sku"`
	Desc   string `xml:"Description"`
	Qty    struct {
		Uom string `xml:"uom,attr"`
		Val string `xml:",chardata"`
	} `xml:"Quantity"`
}
type xTrip struct {
	TripNumber string `xml:"TripNumber"`
	Status     string `xml:"Status"`
	Vehicle    struct {
		Registration string `xml:"Registration"`
		FleetNumber  string `xml:"FleetNumber"`
	} `xml:"Vehicle"`
	Driver       xLoc    `xml:"Driver"`
	Origin       xLoc    `xml:"Origin"`
	Destination  xLoc    `xml:"Destination"`
	PlannedStart string  `xml:"PlannedStart"`
	PlannedEnd   string  `xml:"PlannedEnd"`
	ActualStart  string  `xml:"ActualStart"`
	Lines        []xLine `xml:"Lines>Line"`
}
type oVeh struct {
	Registration string `json:"registration"`
	FleetNumber  string `json:"fleetNumber"`
}
type oLine struct {
	LineNumber  string `json:"lineNumber"`
	Sku         string `json:"sku"`
	Description string `json:"description"`
	Quantity    int64  `json:"quantity"`
	Uom         string `json:"uom"`
}
type oTrip struct {
	TripID       string  `json:"tripId"`
	Status       string  `json:"status"`
	Vehicle      oVeh    `json:"vehicle"`
	Driver       xLoc    `json:"driver"`
	Origin       xLoc    `json:"origin"`
	Destination  xLoc    `json:"destination"`
	PlannedStart string  `json:"plannedStart"`
	PlannedEnd   string  `json:"plannedEnd"`
	ActualStart  string  `json:"actualStart"`
	Lines        []oLine `json:"lines"`
}

func w3(body []byte) []byte {
	var t xTrip
	if err := xml.Unmarshal(body, &t); err != nil {
		panic(err)
	}
	lines := make([]oLine, 0, len(t.Lines))
	for _, l := range t.Lines {
		q, _ := strconv.ParseInt(strings.TrimSpace(l.Qty.Val), 10, 64)
		lines = append(lines, oLine{l.Number, strings.TrimSpace(l.Sku), strings.TrimSpace(l.Desc), q, l.Qty.Uom})
	}
	tr := func(s string) string { return strings.TrimSpace(s) }
	loc := func(l xLoc) xLoc { return xLoc{tr(l.Code), tr(l.Name)} }
	return enc(oTrip{tr(t.TripNumber), tr(t.Status), oVeh{tr(t.Vehicle.Registration), tr(t.Vehicle.FleetNumber)},
		loc(t.Driver), loc(t.Origin), loc(t.Destination), tr(t.PlannedStart), tr(t.PlannedEnd), tr(t.ActualStart), lines})
}

func w4() []byte {
	x := uint32(2463534242)
	var acc uint32
	for i := 0; i < 5_000_000; i++ {
		x ^= x << 13
		x ^= x >> 17
		x ^= x << 5
		acc += x
	}
	return []byte(strconv.FormatUint(uint64(acc), 10))
}

func hostCall(op int32, meta []byte) (int32, int) {
	f := frame(meta, nil)
	r := fcCall(op, int32(uintptr(unsafe.Pointer(&f[0]))), int32(len(f)))
	code, n := int32(uint64(r)>>32), int(uint32(r))
	if n == 0 {
		return code, 0
	}
	res := make([]byte, n)
	fcTake(int32(uintptr(unsafe.Pointer(&res[0]))))
	ml := int(binary.LittleEndian.Uint32(res))
	_ = string(res[4 : 4+ml])
	return code, n - 4 - ml
}

func w5() []byte {
	sum := 0
	for i := 0; i < 10; i++ {
		c, b := hostCall(1, []byte(`{"level":"info","msg":"bench"}`))
		sum += int(c) + b
		c, b = hostCall(2, []byte(`{"key":"BENCH"}`))
		sum += int(c) + b
	}
	return []byte(strconv.Itoa(sum))
}

func main() {}
