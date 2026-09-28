package budget

import (
	"sync"
	"testing"
)

const page = 64 << 10 // a wasm page

func TestTryChargeRespectsBudget(t *testing.T) {
	b, err := New(10*page, 2*page)
	if err != nil {
		t.Fatal(err)
	}
	if !b.TryCharge(8 * page) {
		t.Fatal("8 pages should fit a budget of 8")
	}
	if b.TryCharge(1) {
		t.Fatal("a full budget accepted one more byte")
	}
	b.Release(page)
	if !b.TryCharge(page) {
		t.Fatal("a released page could not be charged again")
	}
	if s := b.Stats(); s.UsedBytes != 8*page || s.Refused != 1 {
		t.Fatalf("stats = %+v", s)
	}
}

func TestTryChargeConcurrent(t *testing.T) {
	b, _ := New(1000*page, 0)
	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for range 2000 {
		wg.Go(func() {
			if b.TryCharge(page) {
				mu.Lock()
				won++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if won != 1000 || b.Stats().UsedBytes != 1000*page {
		t.Fatalf("won %d, used %d; want exactly the budget", won, b.Stats().UsedBytes)
	}
}

func TestAdmitGrowFree(t *testing.T) {
	b, _ := New(10*page, 0)
	m := b.Admit(2*page, 6*page)
	if m == nil {
		t.Fatal("admit refused")
	}
	lm := m.Allocate(2*page, 1<<32).(interface {
		Reallocate(uint64) []byte
		Free()
	})
	buf := lm.Reallocate(2 * page)
	if len(buf) != 2*page {
		t.Fatalf("initial buffer %d bytes", len(buf))
	}
	if used := b.Stats().UsedBytes; used != 2*page {
		t.Fatalf("after initial allocation used = %d, want the admitted minimum only", used)
	}
	buf[0], buf[len(buf)-1] = 1, 2 // committed memory is writable
	grown := lm.Reallocate(4 * page)
	if len(grown) != 4*page || grown[0] != 1 || grown[2*page-1] != 2 {
		t.Fatal("grow lost the existing contents")
	}
	grown[4*page-1] = 3
	if used := b.Stats().UsedBytes; used != 4*page {
		t.Fatalf("after grow used = %d", used)
	}
	if lm.Reallocate(7*page) != nil || !m.Refused.Load() {
		t.Fatal("a grow past the instance cap was not refused")
	}
	lm.Free()
	if used := b.Stats().UsedBytes; used != 0 {
		t.Fatalf("after free used = %d, want 0", used)
	}
}

func TestGrowRefusedWhenBudgetExhausted(t *testing.T) {
	b, _ := New(5*page, 0)
	m := b.Admit(page, 64*page)
	lm := m.Allocate(page, 64*page)
	lm.Reallocate(page)
	if !b.TryCharge(3 * page) { // a neighbour takes most of what's left
		t.Fatal("neighbour charge failed")
	}
	if lm.Reallocate(3*page) != nil {
		t.Fatal("grow beyond the shared budget succeeded")
	}
	if !m.Refused.Load() {
		t.Fatal("refusal not recorded")
	}
	if lm.Reallocate(2*page) == nil {
		t.Fatal("a grow the budget can still cover was refused")
	}
	lm.Free()
	b.Release(3 * page)
	if b.Stats().UsedBytes != 0 {
		t.Fatalf("leak: used = %d", b.Stats().UsedBytes)
	}
}

func TestAdmitRefusedAndCancel(t *testing.T) {
	b, _ := New(4*page, 0)
	if b.Admit(5*page, 8*page) != nil {
		t.Fatal("admitted a minimum larger than the budget")
	}
	if b.Admit(2*page, page) != nil {
		t.Fatal("admitted a minimum above the instance cap")
	}
	m := b.Admit(3*page, 8*page)
	m.Cancel()
	if b.Stats().UsedBytes != 0 {
		t.Fatal("cancel did not return the admitted minimum")
	}
}

func TestDefaultReserve(t *testing.T) {
	if r := DefaultReserve(256 << 20); r != 128<<20 {
		t.Fatalf("256 MiB → %d", r)
	}
	if r := DefaultReserve(4 << 30); r != (4<<30)/10 {
		t.Fatalf("4 GiB → %d", r)
	}
	if r := DefaultReserve(100 << 20); r >= 100<<20 {
		t.Fatalf("reserve %d not below a 100 MiB limit", r)
	}
}

func TestDetectLimit(t *testing.T) {
	if v, ok := DetectLimit(); !ok || v <= 0 {
		t.Fatalf("DetectLimit = %d, %v", v, ok)
	}
}
