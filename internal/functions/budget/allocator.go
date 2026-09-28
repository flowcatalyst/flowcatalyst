package budget

import (
	"sync/atomic"

	"github.com/tetratelabs/wazero/experimental"
)

// InstanceMemory is the linear-memory allocator for ONE guest instance:
// wazero calls Allocate once when the instance is created. It caps the
// instance at capBytes and charges every committed byte to the budget.
//
// The instance's minimum memory is paid for up front (see Admit), because
// wazero requires the first Reallocate — the module's declared minimum — to
// succeed. Every grow after that is charged on demand, and one the budget or
// the cap cannot cover returns nil: the guest's memory.grow answers -1 and
// only that guest's call fails.
type InstanceMemory struct {
	b        *Budget
	capBytes uint64
	// total is every byte charged to the budget for this instance: the
	// admitted minimum plus each grow beyond it. All of it goes back on free.
	total int64
	// Refused is set when a grow was refused; the runner uses it to tell an
	// out-of-memory trap from any other.
	Refused atomic.Bool
	lm      linearMemory
}

// Admit charges a new instance's minimum memory to the budget before the
// instance is created. It returns nil when the budget cannot cover it; the
// caller answers 503 rather than instantiating.
func (b *Budget) Admit(minBytes, capBytes uint64) *InstanceMemory {
	if minBytes > capBytes {
		return nil
	}
	if !b.TryCharge(int64(minBytes)) {
		return nil
	}
	return &InstanceMemory{b: b, capBytes: capBytes, total: int64(minBytes)}
}

// Cancel returns everything charged when instantiation failed. Freeing is
// idempotent, so it is safe whether or not wazero got as far as allocating
// (or already freed on its own error path).
func (m *InstanceMemory) Cancel() {
	if m.lm != nil {
		m.lm.Free()
		return
	}
	m.free()
}

// Charged is the bytes this instance currently has charged to the budget.
func (m *InstanceMemory) Charged() int64 { return m.total }

// Allocate implements experimental.MemoryAllocator.
func (m *InstanceMemory) Allocate(capHint, maxBytes uint64) experimental.LinearMemory {
	m.lm = newLinearMemory(m, min(maxBytes, m.capBytes))
	return m.lm
}

// linearMemory is the platform-specific backing store.
type linearMemory interface {
	experimental.LinearMemory
}

// charge makes sure size bytes are paid for, charging only what the instance
// has not already paid (the admitted minimum covers the first allocation).
// It reports whether the grow may proceed.
func (m *InstanceMemory) charge(size uint64) bool {
	if size > m.capBytes {
		m.Refused.Store(true)
		return false
	}
	need := int64(size) - m.total
	if need <= 0 {
		return true
	}
	if !m.b.TryCharge(need) {
		m.Refused.Store(true)
		return false
	}
	m.total += need
	return true
}

// free credits everything this instance charged.
func (m *InstanceMemory) free() {
	m.b.Release(m.total)
	m.total = 0
}
