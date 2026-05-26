package outbox

import "sync"

// GroupDistributor enforces FIFO ordering within a message group. Items
// without a message_group are dispatched in parallel (no ordering).
// Mirrors fc-outbox/src/group_distributor.rs.
type GroupDistributor struct {
	mu     sync.Mutex
	groups map[string]*groupQueue
}

type groupQueue struct {
	pending []func()
	running bool
}

// NewGroupDistributor builds a fresh distributor.
func NewGroupDistributor() *GroupDistributor {
	return &GroupDistributor{groups: make(map[string]*groupQueue)}
}

// Submit dispatches the supplied work function, respecting FIFO order
// within message_group when set.
func (d *GroupDistributor) Submit(item Item, work func()) {
	if item.MessageGroup == nil || *item.MessageGroup == "" {
		go work()
		return
	}
	d.mu.Lock()
	g, ok := d.groups[*item.MessageGroup]
	if !ok {
		g = &groupQueue{}
		d.groups[*item.MessageGroup] = g
	}
	g.pending = append(g.pending, work)
	shouldStart := !g.running
	if shouldStart {
		g.running = true
	}
	d.mu.Unlock()

	if shouldStart {
		go d.drain(*item.MessageGroup)
	}
}

func (d *GroupDistributor) drain(group string) {
	for {
		d.mu.Lock()
		g := d.groups[group]
		if g == nil || len(g.pending) == 0 {
			if g != nil {
				g.running = false
			}
			d.mu.Unlock()
			return
		}
		work := g.pending[0]
		g.pending = g.pending[1:]
		d.mu.Unlock()

		work()
	}
}
