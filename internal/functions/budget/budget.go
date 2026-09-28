// Package budget is the function runner's memory accounting
// (docs/function-runner-plan.md §7). The runner knows its limit, keeps a
// reserve for itself, and admits guest memory against the remainder: every
// page a guest commits is charged, and a grow the budget cannot cover fails
// inside the guest instead of taking the process down.
package budget

import (
	"fmt"
	"sync/atomic"
)

// Budget is a hard cap on the bytes charged against it. Safe for concurrent use.
type Budget struct {
	limit   int64 // total memory the runner may use
	reserve int64 // kept back for the runner itself
	budget  int64 // limit − reserve: what guests may commit
	used    atomic.Int64
	refused atomic.Int64 // charges refused, for metrics
}

// New builds a budget of limit bytes of which reserve are kept back.
func New(limit, reserve int64) (*Budget, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("budget: limit must be positive, got %d", limit)
	}
	if reserve < 0 || reserve >= limit {
		return nil, fmt.Errorf("budget: reserve %d must be in [0, limit %d)", reserve, limit)
	}
	return &Budget{limit: limit, reserve: reserve, budget: limit - reserve}, nil
}

// DefaultReserve is max(128 MiB, 10% of limit), capped below limit.
func DefaultReserve(limit int64) int64 {
	r := max(int64(128<<20), limit/10)
	if r >= limit {
		r = limit / 2
	}
	return r
}

// TryCharge charges n bytes if the budget can cover them.
func (b *Budget) TryCharge(n int64) bool {
	if n <= 0 {
		return true
	}
	for {
		cur := b.used.Load()
		if cur+n > b.budget {
			b.refused.Add(1)
			return false
		}
		if b.used.CompareAndSwap(cur, cur+n) {
			return true
		}
	}
}

// Release credits n bytes back.
func (b *Budget) Release(n int64) {
	if n > 0 {
		b.used.Add(-n)
	}
}

// Stats is a point-in-time view, for heartbeats and metrics.
type Stats struct {
	LimitBytes   int64 `json:"limitBytes"`
	ReserveBytes int64 `json:"reserveBytes"`
	BudgetBytes  int64 `json:"budgetBytes"`
	UsedBytes    int64 `json:"linearBytes"`
	Refused      int64 `json:"refused"`
}

// Stats reports the budget's current state.
func (b *Budget) Stats() Stats {
	return Stats{
		LimitBytes:   b.limit,
		ReserveBytes: b.reserve,
		BudgetBytes:  b.budget,
		UsedBytes:    b.used.Load(),
		Refused:      b.refused.Load(),
	}
}

// Limit is the total the runner may use.
func (b *Budget) Limit() int64 { return b.limit }

// Available is what a charge could take right now.
func (b *Budget) Available() int64 { return b.budget - b.used.Load() }
