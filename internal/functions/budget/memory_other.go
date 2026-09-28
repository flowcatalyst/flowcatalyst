//go:build !linux && !darwin

package budget

func newLinearMemory(owner *InstanceMemory, limit uint64) linearMemory {
	return &heapMemory{owner: owner, limit: limit}
}
