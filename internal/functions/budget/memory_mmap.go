//go:build linux || darwin

package budget

import (
	"golang.org/x/sys/unix"
)

// mmapMemory reserves the instance's whole cap as inaccessible address space
// and makes pages readable and writable as the guest grows. The memory lives
// outside the Go heap, never moves, is zeroed by the kernel, and is returned
// to the OS the moment the instance closes rather than when the GC runs.
type mmapMemory struct {
	owner     *InstanceMemory
	buf       []byte // the whole reservation
	committed uint64
}

func newLinearMemory(owner *InstanceMemory, limit uint64) linearMemory {
	if limit == 0 {
		return &heapMemory{owner: owner}
	}
	buf, err := unix.Mmap(-1, 0, int(limit), unix.PROT_NONE, unix.MAP_PRIVATE|unix.MAP_ANON)
	if err != nil {
		// Out of address space is not a reason to fail the call outright;
		// the heap-backed store charges identically.
		return &heapMemory{owner: owner, limit: limit}
	}
	return &mmapMemory{owner: owner, buf: buf}
}

func (m *mmapMemory) Reallocate(size uint64) []byte {
	if size > uint64(len(m.buf)) {
		m.owner.Refused.Store(true)
		return nil
	}
	if size > m.committed {
		if !m.owner.charge(size) {
			return nil
		}
		// Wasm pages are 64 KiB, a multiple of every OS page size, so both
		// ends of the range are page-aligned. On failure the charge stays
		// with the instance and is released when it closes.
		if err := unix.Mprotect(m.buf[m.committed:size], unix.PROT_READ|unix.PROT_WRITE); err != nil {
			m.owner.Refused.Store(true)
			return nil
		}
		m.committed = size
	}
	return m.buf[:size:size]
}

func (m *mmapMemory) Free() {
	if m.buf == nil {
		return
	}
	_ = unix.Munmap(m.buf)
	m.buf = nil
	m.committed = 0
	m.owner.free()
}
