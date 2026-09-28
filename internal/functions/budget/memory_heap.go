package budget

// heapMemory is the portable backing store: a Go slice, grown by copying.
// It charges exactly as mmapMemory does; what it lacks is prompt return of
// freed memory to the OS (that waits for the GC).
type heapMemory struct {
	owner *InstanceMemory
	buf   []byte
	limit uint64 // 0 = no limit beyond the owner's cap
}

func (m *heapMemory) Reallocate(size uint64) []byte {
	if m.limit > 0 && size > m.limit {
		m.owner.Refused.Store(true)
		return nil
	}
	if size > uint64(len(m.buf)) {
		if !m.owner.charge(size) {
			return nil
		}
		grown := make([]byte, size)
		copy(grown, m.buf)
		m.buf = grown
	}
	return m.buf[:size:size]
}

func (m *heapMemory) Free() {
	m.buf = nil
	m.owner.free()
}
