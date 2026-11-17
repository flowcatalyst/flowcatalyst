package router

import (
	"context"
	"slices"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// The hand-back gate: keeping an ordered group in order across a hand-back
// on a broker that does not do it itself.
//
// Postgres and SQS FIFO keep a group in order while one of its members is
// out at the broker on a hand-back: Postgres claims nothing of a group behind
// a member nacked with a delay, and SQS FIFO hands out nothing of a group
// while a member is in flight. NATS does neither (HonoursDelayedReturn
// false): every message is redelivered on its own timer, and a later member
// of the group is handed out as soon as it is published. So on NATS a member
// the pool deferred for capacity, or released with a delay, could be
// overtaken by a later member admitted while it was out, a deferred member
// could come back before the head that was released after it, and members
// handed back together came back in whatever order their timers fired.
//
// The gate remembers, per ordered group, the members this pool has handed
// back to such a broker and when each is due, in group order, and admits a
// member of the group only when no member ahead of it is still out. A member
// that may not be admitted yet is handed back again, due after the member in
// front of it. Members handed back together are given delays in group order,
// at least deferralOrderedSpacing apart.
//
// It applies only to an ordered message whose source consumer answers
// HonoursDelayedReturn false. For every other message nothing is recorded or
// consulted, and admission, deferral and release are exactly as they were.

// handbackReturnGrace is how long past its due time a handed-back member is
// waited for. Past it the member is presumed not to be coming back (acked as
// a duplicate, dead-lettered, lost with a broker or a router restart), its
// entry is dropped, and its group moves on without it. Should it come back
// after all it is admitted as a new member: behind whatever of its group is
// still out, or at once if nothing is, which is after the members that went
// ahead of it.
//
// A minute covers a broker's redelivery being late by the time it takes the
// consumer to poll it, which is what lateness on NATS amounts to: the
// consumer keeps polling while any of its pools has room, and a consumer
// parked for capacity wakes when its earliest deferral is due (the deferral
// ledger), which a gate hand-back is booked on like any deferral.
const handbackReturnGrace = time.Minute

// handbackGate is one pool's record of the ordered-group members it has
// handed back to a broker that does not keep a group in order itself. Every
// field is guarded by Pool.mu: whether a member may be admitted is decided in
// the same hold of the lock that buffers it, and a release is recorded in the
// same hold that takes the group's buffer (takeGroupForRelease), so an
// admission can never fall between a release's take and its record.
//
// Invariant, for each group in groups:
//   - the list holds the group's members that this pool handed back to such a
//     broker (a capacity deferral, a gate hand-back, a release) and that have
//     not come back, in group order, each with the time it was asked to come
//     back by;
//   - every member of the group the pool holds (buffered, or in a drainer's
//     hand) is earlier in the group than every listed member, because a
//     member is admitted only when no listed member is ahead of it, and a
//     release lists everything the pool held for the group ahead of what was
//     already listed;
//   - an entry leaves the list when its member is admitted, or when it is
//     overdue by handbackReturnGrace (prune). A group whose list empties is
//     deleted, so the map holds only groups with a member out, and each
//     entry lives no longer than its due time plus the grace. The due time
//     of a capacity deferral is within the reservation horizon; of a release,
//     the outcome's own delay; of a gate hand-back, a spacing after the entry
//     ahead of it.
//
// What the gate does not cover: a member handed back by anything other than
// this pool's admission and release paths (a Manager-level nack, the stall
// detector, Stop's flush, a restart) is not listed, and comes back as a new
// member.
type handbackGate struct {
	groups map[string][]handback
	// nextSweep is when every group is next pruned (sweep), so that a group
	// whose members never come back does not keep its entry until the same
	// group id is seen again.
	nextSweep time.Time
}

// handback is one listed member: its message id and when it is due back.
type handback struct {
	id  string
	due time.Time
}

// clock is the pool's time: p.now when a test has set one, else time.Now.
func (p *Pool) clock() time.Time {
	if p.now != nil {
		return p.now()
	}
	return time.Now()
}

// gatingConsumer returns qm's source consumer when the hand-back gate applies
// to qm: an ordered message from a broker that does not keep a group in order
// across a hand-back (HonoursDelayedReturn false). Otherwise nil.
func (p *Pool) gatingConsumer(qm common.QueuedMessage) queue.Consumer {
	if !isOrdered(qm) {
		return nil
	}
	c := p.consumerFor(qm)
	if c == nil || c.HonoursDelayedReturn() {
		return nil
	}
	return c
}

// prune drops the group's entries overdue by more than handbackReturnGrace
// and returns what is left, deleting the group when nothing is.
func (g *handbackGate) prune(group string, now time.Time) []handback {
	list := g.groups[group]
	if len(list) == 0 {
		return nil
	}
	live := slices.DeleteFunc(list, func(h handback) bool { return now.After(h.due.Add(handbackReturnGrace)) })
	if len(live) == 0 {
		delete(g.groups, group)
		return nil
	}
	g.groups[group] = live
	return live
}

// sweep prunes every group, at most once per handbackReturnGrace.
func (g *handbackGate) sweep(now time.Time) {
	if len(g.groups) == 0 || now.Before(g.nextSweep) {
		return
	}
	for group := range g.groups {
		g.prune(group, now)
	}
	g.nextSweep = now.Add(handbackReturnGrace)
}

// set stores a group's list, deleting the group when it is empty.
func (g *handbackGate) set(group string, list []handback) {
	if len(list) == 0 {
		delete(g.groups, group)
		return
	}
	if g.groups == nil {
		g.groups = make(map[string][]handback)
	}
	g.groups[group] = list
}

func indexOf(list []handback, id string) int {
	return slices.IndexFunc(list, func(h handback) bool { return h.id == id })
}

// plan orders one poll batch's members of a group for admission and says how
// many of them, from the front, the gate lets in. Members that are listed
// (coming back from a hand-back) go first, in their listed order, and the
// rest after them in batch order: a member not listed was not handed back by
// this pool, so it is later than everything listed. A listed member may be
// admitted when every entry ahead of it is one of this batch's members
// admitted before it; an unlisted member only when every listed member is
// in this batch. Caller holds p.mu.
func (g *handbackGate) plan(group string, run []common.QueuedMessage, now time.Time) (order []common.QueuedMessage, open int) {
	g.sweep(now)
	list := g.prune(group, now)
	if len(list) == 0 {
		return run, len(run)
	}
	order = make([]common.QueuedMessage, 0, len(run))
	for _, m := range run {
		if indexOf(list, m.Message.ID) >= 0 {
			order = append(order, m)
		}
	}
	listed := len(order)
	slices.SortStableFunc(order, func(a, b common.QueuedMessage) int {
		return indexOf(list, a.Message.ID) - indexOf(list, b.Message.ID)
	})
	for _, m := range run {
		if indexOf(list, m.Message.ID) < 0 {
			order = append(order, m)
		}
	}
	for _, m := range order[:listed] {
		if indexOf(list, m.Message.ID) != open {
			return order, open // a member ahead of it is still out
		}
		open++
	}
	if open == len(list) {
		open = len(order) // nothing of the group is left out
	}
	return order, open
}

// gatedHandback is one member a gated admission hands back, with the least
// delay it is to be given before it is spaced behind the entry ahead of it.
type gatedHandback struct {
	qm    common.QueuedMessage
	least time.Duration
	// toHorizon caps the delay at the reservation horizon: a capacity
	// deferral is a reservation, and no reservation is booked past it.
	toHorizon bool
}

// admit records the outcome of a gated admission of a group: admitted came
// in, and back is going back to the broker, in group order. Admitted members
// leave the list; a listed member going back keeps its place, and an
// unlisted one joins the end. Each member going back is given a delay of at
// least its least, and at least deferralOrderedSpacing after the due time of
// the entry ahead of it, in whole seconds (what a broker is told), and is
// listed as due then. Returns the delays, in back's order. Caller holds p.mu.
func (g *handbackGate) admit(group string, admitted []common.QueuedMessage, back []gatedHandback, now time.Time, horizon time.Duration) []time.Duration {
	list := g.groups[group]
	if len(list) == 0 && len(back) == 0 {
		return nil
	}
	gone := func(h handback) bool {
		return slices.ContainsFunc(admitted, func(m common.QueuedMessage) bool { return m.Message.ID == h.id })
	}
	next := slices.DeleteFunc(slices.Clone(list), gone)
	for _, b := range back {
		if indexOf(next, b.qm.Message.ID) < 0 {
			next = append(next, handback{id: b.qm.Message.ID})
		}
	}
	delays := make([]time.Duration, len(back))
	var prev time.Time
	for i := range next {
		j := slices.IndexFunc(back, func(b gatedHandback) bool { return b.qm.Message.ID == next[i].id })
		if j >= 0 {
			d := back[j].least
			if !prev.IsZero() {
				d = max(d, prev.Sub(now)+deferralOrderedSpacing)
			}
			d = wholeSeconds(d)
			if back[j].toHorizon {
				d = min(d, horizon)
			}
			delays[j] = d
			next[i].due = now.Add(d)
		}
		prev = next[i].due
	}
	g.set(group, next)
	return delays
}

// release records a group handed back by a release: msgs, the head first if
// there is one and then what was buffered behind it, all earlier in the
// group than anything already listed, so they go to the front in order.
// delays are the delays the release asks for, one per message; each after the
// first is raised to at least deferralOrderedSpacing after the one before it
// (never lowered: a sibling stays no earlier than its head). Returns the
// delays to send. Caller holds p.mu.
func (g *handbackGate) release(group string, msgs []common.QueuedMessage, delays []*uint32, now time.Time) []*uint32 {
	if len(msgs) == 0 {
		return delays
	}
	out := make([]*uint32, len(msgs))
	front := make([]handback, 0, len(msgs))
	var prev time.Duration
	for i, m := range msgs {
		var d time.Duration
		if delays[i] != nil {
			d = time.Duration(*delays[i]) * time.Second
		}
		if i > 0 {
			d = max(d, prev+deferralOrderedSpacing)
		}
		prev = d
		secs := uint32(d / time.Second) //nolint:gosec // G115: a delay in seconds, far below 2^32
		if delays[i] == nil && i == 0 {
			out[i] = nil // the head's own "redeliver now" is kept as it was asked
		} else {
			out[i] = &secs
		}
		front = append(front, handback{id: m.Message.ID, due: now.Add(d)})
	}
	rest := slices.DeleteFunc(slices.Clone(g.groups[group]), func(h handback) bool { return indexOf(front, h.id) >= 0 })
	g.set(group, append(front, rest...))
	return out
}

// wholeSeconds rounds d up to a whole second: a broker is told a delay in
// seconds, and rounding down could put a member less than a spacing after
// the one ahead of it.
func wholeSeconds(d time.Duration) time.Duration {
	if r := d % time.Second; r != 0 {
		d += time.Second - r
	}
	return d
}

// submitRunGated is submitRun for a run from a source the hand-back gate
// applies to (c, whose broker does not keep a group in order). The gate's
// decision, the capacity check and the buffering of what is admitted happen
// in one hold of p.mu; what goes back is deferred after it is let go, each
// member with the delay the gate gave it.
//
// The run is admitted in group order as far as the gate and the pool's room
// allow, and the rest goes back: members with no room as capacity deferrals
// (booked on the admission schedule, as submitRun books them), and members
// with something of their group still out ahead of them as gate hand-backs
// (the least delay is deferralMinDelay: they are waiting for that member, not
// for room).
func (p *Pool) submitRunGated(ctx context.Context, c queue.Consumer, group string, run []common.QueuedMessage) {
	now := p.clock()
	p.mu.Lock()
	if p.stopped.Load() {
		p.mu.Unlock()
		p.events.reject(RejectStopped, len(run))
		for _, m := range run {
			p.nackMsg(ctx, m, new(uint32(10)), "pool stopped")
		}
		return
	}
	order, open := p.handbacks.plan(group, run, now)
	room := 0
	if size, capacity := p.queueSize.Load(), p.queueCapacity(); size < capacity {
		room = int(capacity - size)
	}
	admitted := order[:min(open, room)]
	back := make([]gatedHandback, 0, len(order)-len(admitted))
	for i, m := range order[len(admitted):] {
		if len(admitted)+i < open {
			back = append(back, gatedHandback{qm: m, least: p.admissionDelay(now, false, true), toHorizon: true})
		} else {
			back = append(back, gatedHandback{qm: m, least: deferralMinDelay})
		}
	}
	delays := p.handbacks.admit(group, admitted, back, now, p.deferralHorizon())
	if len(admitted) > 0 {
		gq := p.groupLocked(group)
		gq.gated = true
		gq.msgs = append(gq.msgs, admitted...)
		p.countBufferedLocked(len(admitted))
	}
	p.mu.Unlock()

	if len(admitted) > 0 {
		p.capacityChanged()
	}
	for i, b := range back {
		reason := "pool at capacity"
		if !b.toHorizon {
			reason = "an earlier member of its group is out at the broker"
		}
		// Counted as a capacity rejection: like one, it is a deferral to the
		// broker for the message's turn to be admitted.
		p.events.reject(RejectCapacity, 1)
		p.deferFor(ctx, c, b.qm, now, delays[i], reason)
	}
	if len(admitted) > 0 {
		p.tryDrainGroup(ctx, group)
	}
}

// takeGroupForRelease is takeGroup for a release that hands the group back to
// the broker: head (the message in the drainer's hand, nil when there is
// none) is going back with headDelay and the buffered messages with
// siblingDelay. It returns what was buffered and the delay each is to be
// nacked with.
//
// For a group the hand-back gate applies to (gq.gated: its members came from
// a broker that does not keep a group in order) the head and the buffered
// messages are listed at the front of the group's hand-backs in the same hold
// of p.mu as the take, and the siblings' delays are spaced after the head's.
// For any other group the delays are siblingDelay, as before.
func (p *Pool) takeGroupForRelease(group string, gq *groupQueue, head *common.QueuedMessage, headDelay, siblingDelay *uint32) ([]common.QueuedMessage, []*uint32) {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Nothing is recorded unless the take is the caller's (see takeGroup): a
	// drainer that panics after it has settled its group already recorded
	// that release.
	owns := gq != nil && p.groupQs[group] == gq
	buffered := p.takeGroupLocked(group, gq)
	if !owns || !gq.gated {
		return buffered, sameDelay(len(buffered), siblingDelay)
	}
	return buffered, p.recordReleaseLocked(group, head, headDelay, buffered, siblingDelay)
}

// recordReleaseLocked lists a gated group's released messages with the gate
// and returns the buffered messages' delays (see takeGroupForRelease). The
// head, already nacked with headDelay, is listed with it. Caller holds p.mu.
func (p *Pool) recordReleaseLocked(group string, head *common.QueuedMessage, headDelay *uint32, buffered []common.QueuedMessage, siblingDelay *uint32) []*uint32 {
	msgs := make([]common.QueuedMessage, 0, len(buffered)+1)
	delays := make([]*uint32, 0, len(buffered)+1)
	if head != nil {
		msgs = append(msgs, *head)
		delays = append(delays, headDelay)
	}
	msgs = append(msgs, buffered...)
	delays = append(delays, sameDelay(len(buffered), siblingDelay)...)
	out := p.handbacks.release(group, msgs, delays, p.clock())
	if head != nil {
		out = out[1:]
	}
	return out
}

func sameDelay(n int, d *uint32) []*uint32 {
	out := make([]*uint32, n)
	for i := range out {
		out[i] = d
	}
	return out
}

// deferralHorizon is the reservation horizon admissionDelay clamps to.
func (p *Pool) deferralHorizon() time.Duration {
	p.admission.mu.Lock()
	defer p.admission.mu.Unlock()
	if p.admission.maxDeferral > 0 {
		return p.admission.maxDeferral
	}
	return defaultMaxDeferral
}
