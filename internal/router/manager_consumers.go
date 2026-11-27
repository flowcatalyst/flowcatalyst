package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// runningConsumer is one queue's consumer plus its poll loop, and it carries
// TWO cancellations because "stop taking new work" and "stop working" are
// different events:
//
//   - stopPoll ends the poll loop only. Messages already routed keep their
//     context, so deliveries in flight run to their own conclusion. This is
//     what a graceful drain wants.
//   - cancel tears the consumer down: it also cancels workCtx, which parks
//     ordered groups and aborts in-flight deliveries. Used when the consumer
//     itself is going away (removed by config, restarted after a stall, or the
//     whole manager shutting down), where finishing the work would be
//     pointless — nothing could ack it afterwards.
type runningConsumer struct {
	consumer queue.Consumer
	workCtx  context.Context // parent of everything the routed messages do
	cancel   context.CancelFunc
	stopPoll context.CancelFunc
	queueCfg common.QueueConfig
	// lastPoll is the unix-nano of the most recent completed poll; a poll
	// loop wedged inside consumer.Poll leaves it stale, which the
	// consumer-restart watchdog (RestartStalledConsumers) detects.
	lastPoll atomic.Int64

	// pollsStarted / pollsReturned bracket every consumer.Poll call, and exist
	// to make a stalled consumer say WHY it is stalled. A stale lastPoll has
	// several silent causes that look identical from outside — the loop never
	// reached the poll (backpressure pause, dead context), or it entered one
	// and never came back — and telling them apart from logs alone cost real
	// production time. started == returned means the loop is between polls;
	// started > returned means it is INSIDE one right now, so a stall with that
	// gap is a hung poll and nothing else.
	pollsStarted  atomic.Uint64
	pollsReturned atomic.Uint64

	// pools records the pool codes the most recent batch routed to, so the
	// poll loop can push back on the pools this queue actually feeds rather
	// than on the process as a whole. Guarded by its own mutex: written by the
	// poll loop, read by nothing else today, but cheap to keep safe.
	poolsMu sync.Mutex
	pools   []string

	// deferrals is when the messages this consumer's pools handed back for
	// capacity (Pool.deferMsg) are due to be redelivered. hasCapacityFor
	// reads it to keep polling into full pools only while the deferral
	// budget lasts, and awaitCapacity to wake when the next one lands.
	deferrals deferralLedger

	// detachedAt is stamped once, under Manager.detachMu, the moment this
	// consumer moves into Manager.detaching (X-11 / R-26/R-49 — a queue
	// removed/changed under Reconfigure, or a wedged poll loop replaced by
	// RestartStalledConsumers). Zero while the consumer is still active.
	// retireDetachedConsumers compares it against each tracked message's
	// StartedAt (InFlightTracker.CountForQueueBefore) so only messages this
	// consumer could actually have polled — never the replacement's own
	// ongoing traffic under the same queue identifier — hold up retirement.
	detachedAt time.Time
}

// newRunningConsumer wires a consumer's two contexts under parent.
func newRunningConsumer(parent context.Context, c queue.Consumer, qc common.QueueConfig) (*runningConsumer, context.Context) {
	workCtx, cancel := context.WithCancel(parent)
	pollCtx, stopPoll := context.WithCancel(workCtx)
	r := &runningConsumer{
		consumer: c, workCtx: workCtx, cancel: cancel, stopPoll: stopPoll, queueCfg: qc,
	}
	r.lastPoll.Store(time.Now().UnixNano())
	return r, pollCtx
}

// runConsumer is the per-consumer poll loop. ctx is the consumer's POLL
// context: cancelling it stops intake, while messages already routed continue
// under rc.workCtx. It pauses when the pools this queue feeds are at capacity
// to avoid a hot poll-defer loop, polls up to 10, routes the batch, and paces
// itself by batch fullness.
func (m *Manager) runConsumer(ctx context.Context, rc *runningConsumer) {
	defer m.wg.Done()
	// Last-resort panic boundary for the loop itself. Polls and per-message
	// routing have their own (pollSafely, routeOne), so reaching this means
	// the loop's own code panicked; nothing is in hand. The loop ends, its
	// lastPoll goes stale, and the restart watchdog builds a replacement.
	defer func() {
		if r := recover(); r != nil {
			logRecovered("manager.runConsumer (poll loop ended; the watchdog rebuilds it)", r,
				logKeyQueue, rc.consumer.Identifier())
		}
	}()
	const maxPoll = 10
	wasFull := false
	var fullSince, nextFullWarn time.Time
	for {
		if ctx.Err() != nil {
			return
		}
		// Backpressure: if every pool this queue feeds is full AND the queue's
		// deferral budget is spent, wait rather than poll (one full pool among
		// several is not a pause any more — its messages are deferred and the
		// rest flow; see hasCapacityFor). Surface the transition into full as
		// a PoolCapacity warning (once per full period, not every tick, to
		// avoid flooding /warnings), then re-warn on a slow interval for as
		// long as it lasts.
		if !m.hasCapacityFor(rc) {
			now := time.Now()
			switch {
			case !wasFull:
				wasFull = true
				fullSince = now
				nextFullWarn = now.Add(capacityStuckWarnInterval)
				if w := m.warnings.Load(); w != nil {
					w.Add(WarningCategoryPoolCapacity, WarningWarning,
						fmt.Sprintf("destination pools at capacity and %d deferrals outstanding (budget %d); pausing %s — "+
							"a slow pool's backlog larger than the budget blocks the rest of this queue: raise "+
							"FC_ROUTER_DEFERRAL_BUDGET (SQS FIFO allows 20k in flight) or give that job its own queue",
							rc.deferrals.outstanding(now), m.deferralBudgetValue(), rc.consumer.Identifier()), "router")
				}
			case now.After(nextFullWarn):
				// A pool that has been full this long is not absorbing a burst,
				// it is stuck. Say so again, louder. Without this the condition
				// warns once and then goes silent for ever — and since the pause
				// below now keeps the consumer OUT of the restart watchdog, this
				// is the only thing left that reports it.
				nextFullWarn = now.Add(capacityStuckWarnInterval)
				if w := m.warnings.Load(); w != nil {
					w.Add(WarningCategoryPoolCapacity, WarningError,
						fmt.Sprintf("destination pools for %s have been at capacity for %s; intake is stalled",
							rc.consumer.Identifier(), now.Sub(fullSince).Round(time.Second)), "router")
				}
				slog.Warn("destination pools still at capacity; intake stalled",
					"queue", rc.consumer.Identifier(), "full_for", now.Sub(fullSince).Round(time.Second))
			}
			// A consumer pausing for backpressure is doing its job, not wedging,
			// so keep its poll heartbeat fresh. Letting it go stale here made the
			// restart watchdog (RestartStalledConsumers) rebuild a perfectly
			// healthy consumer, and the rebuild cancelled workCtx — which parked
			// the ordered-group drainers that were the only thing draining the
			// buffer that was full in the first place. The buffer then never
			// emptied, so capacity never returned, so this branch never exited:
			// a permanent deadlock built entirely out of healthy parts, in which
			// the watchdog re-fired every 30s for ever.
			rc.lastPoll.Store(now.UnixNano())
			// G12 (owner ruling 2026-09-07): no timed wait here. Park
			// untimed on the capacity gate — some pool crossing back under
			// capacity, or a reconfigure, wakes this loop the instant
			// there is real room, instead of a fixed 2s poll that left the
			// router at 22% CPU with workers idle while a fast broker's
			// shared buffer refilled and drained many times over within
			// one sleep. Still exits promptly on ctx cancellation.
			if !m.awaitCapacity(ctx, rc) {
				return
			}
			continue
		}
		if wasFull {
			// The resume half of the PoolCapacity warning above: an
			// operator who saw "pausing" deserves to see when intake
			// actually picked back up, not silence until the next
			// unrelated log line.
			slog.Info("destination pools have capacity again; resuming intake",
				"queue", rc.consumer.Identifier())
		}
		wasFull = false

		// Bound every poll. The backends take the caller's context and add no
		// deadline of their own — the SQS client is built with no HTTP or
		// request timeout — so a ReceiveMessage that never returns parks this
		// loop indefinitely: no heartbeat, no error, no log, until the restart
		// watchdog notices a minute later and rebuilds a consumer that does the
		// same thing. A deadline turns that into an ordinary poll error, logged
		// and retried a second later.
		rc.pollsStarted.Add(1)
		pollCtx, cancelPoll := context.WithTimeout(ctx, m.pollTimeout)
		msgs, err := pollSafely(pollCtx, rc.consumer, maxPoll)
		cancelPoll()
		rc.pollsReturned.Add(1)
		counters := m.consumerCountersFor(rc.queueCfg.Name)
		counters.polls.Add(1)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			if errors.Is(err, errPollPanicked) {
				counters.pollPanics.Add(1)
			} else {
				counters.pollErrors.Add(1)
			}
			// The queue vanished under a running consumer: not a fault. Retire the
			// consumer; RecheckMissingQueues brings it back if the queue returns.
			if errors.Is(err, queue.ErrQueueMissing) {
				m.detachMissingConsumer(rc)
				return
			}
			// A stopped consumer never resumes: Stop() was called but this poll
			// loop wasn't torn down (e.g. a stop path that didn't also cancel our
			// context). Exit so the restart watchdog (RestartStalledConsumers)
			// respawns a fresh consumer, instead of spinning on the dead one
			// ~once a second forever — the "Error polling: Queue is stopped" flood.
			if errors.Is(err, queue.ErrStopped) {
				slog.Warn("consumer stopped; exiting poll loop for respawn", "queue", rc.consumer.Identifier())
				return
			}
			slog.Warn("consumer poll error", "queue", rc.consumer.Identifier(), "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}
		// Heartbeat only on a SUCCESSFUL poll (empty or not). Stamping it on an
		// errored poll keeps a wedged, error-spinning consumer looking alive to
		// the restart watchdog, so it is never rebuilt.
		//
		// G13 (owner ruling 2026-09-07): a backend whose Poll blocks by
		// contract (NATS's continuous subscription) reports "no messages
		// yet" by letting pollCtx's own deadline lapse — Poll returns
		// (nil, nil), not an error, once it recognises that (see the nats
		// backend). That is a SUCCESSFUL, empty poll, so it lands here and
		// gets exactly the same heartbeat treatment as any other backend's
		// instant "queue's empty" answer. This is what keeps a healthy
		// blocking Poll's lastPoll fresh every pollTimeout — well inside
		// any sane stall threshold — without the restart watchdog ever
		// needing to know which backend blocks and which doesn't.
		rc.lastPoll.Store(time.Now().UnixNano())

		if len(msgs) == 0 {
			// pollCtx already having reached ITS OWN deadline (as opposed to
			// returning empty well within it) means this call already spent
			// up to pollTimeout waiting — that wait already paced the loop,
			// so re-polling immediately isn't a hot loop. Without this
			// check, a blocking backend got pollTimeout of real waiting PLUS
			// an extra fixed 1s on top of it, and — worse — that extra
			// fixed sleep is exactly the "resurrected timed cadence" this
			// design deliberately avoids. A backend that returns empty fast
			// (SQS/Postgres with nothing queued) still gets the 1s pause:
			// it hasn't waited at all, and hammering an empty queue at full
			// loop speed is the hot-spin this pause exists to prevent.
			if errors.Is(pollCtx.Err(), context.DeadlineExceeded) {
				continue
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Second):
			}
			continue
		}

		// Route under the WORK context, not the poll context: stopping intake
		// must not abort deliveries already under way, which is what makes a
		// graceful drain graceful.
		rc.setPools(m.route(rc.workCtx, msgs, rc.consumer))

		// G12 (owner ruling 2026-09-07): a partial batch used to pause
		// 500ms on the theory that the queue was draining — but a partial
		// batch says nothing about the QUEUE, only about what this one
		// poll happened to return; on a fast broker it is routine and the
		// pause did nothing but idle the workers between batches. Re-poll
		// immediately, same as a full batch; the empty-poll branch above
		// is what actually paces an idle queue.
	}
}

// errPollPanicked marks a poll error that was a recovered panic.
var errPollPanicked = errors.New("poll panicked")

// pollSafely is consumer.Poll behind a panic boundary: a broker client that
// panics mid-receive becomes an ordinary poll error (logged with its stack),
// retried like any other, instead of killing the process. Nothing was handed
// to the router yet, so nothing is in hand to account for.
func pollSafely(ctx context.Context, c queue.Consumer, maxMessages uint32) (msgs []common.QueuedMessage, err error) {
	defer func() {
		if r := recover(); r != nil {
			logRecovered("consumer.Poll", r, logKeyQueue, c.Identifier())
			msgs, err = nil, fmt.Errorf("%w: %v", errPollPanicked, r)
		}
	}()
	return c.Poll(ctx, maxMessages)
}

// setPools records the destination pools of the batch just routed. An empty
// batch leaves the previous set in place — a quiet poll says nothing about
// where this queue's traffic goes.
func (rc *runningConsumer) setPools(codes []string) {
	if len(codes) == 0 {
		return
	}
	rc.poolsMu.Lock()
	rc.pools = codes
	rc.poolsMu.Unlock()
}

func (rc *runningConsumer) destPools() []string {
	rc.poolsMu.Lock()
	defer rc.poolsMu.Unlock()
	return rc.pools
}

// hasCapacityFor reports whether this consumer should keep polling: whether
// ANY of the pools its own last batch fed still has room in its pre-dispatch
// buffer.
//
// It is judged by the pools this queue actually feeds, not by every pool in
// the process — a single idle pool elsewhere must not keep a consumer polling
// into pools that are all saturated.
//
// It used to pause when ANY of those pools was full, on the reasoning that a
// batch cannot be split so the alternative was fetching messages we knew we
// would bounce — and, at the time, SQS ignored the bounce outright. That was
// the head-of-line block of 2026-09-22: one dedicated pool at concurrency 1
// with a 10k backlog parked its client's whole queue for the fourteen hours
// it took to drain, starving every other pool on it. A bounce is now a
// scheduled Defer on every backend (Pool.deferMsg), so polling into one full
// pool among several is the right trade: the full pool's messages come back
// at its own pace and everything else on the queue keeps flowing.
//
// When every pool this queue is known to feed is full, it still polls while
// its deferral budget lasts. "Known to feed" is only the last batch's
// destinations — a queue whose last ten messages all named the full pool
// may well have another pool's traffic queued behind them, and the only way
// to find out is to poll and defer the ones in the way. The budget
// (defaultDeferralBudget) is what stops that from being unbounded: with
// every pool full AND the budget spent there is nothing a poll could do but
// bounce, so the loop parks until a pool frees or a deferred message comes
// due (awaitCapacity).
func (m *Manager) hasCapacityFor(rc *runningConsumer) bool {
	m.poolMu.RLock()
	defer m.poolMu.RUnlock()
	if len(m.pools) == 0 {
		return false // nothing to route to
	}
	if m.destHasRoom(rc) {
		return true
	}
	return rc.deferrals.outstanding(time.Now()) < m.deferralBudgetValue()
}

// destHasRoom reports whether any pool this consumer's last batch fed has
// room. Caller holds poolMu.
func (m *Manager) destHasRoom(rc *runningConsumer) bool {
	dests := rc.destPools()
	if len(dests) == 0 {
		// Nothing routed yet (first poll, or every message was deduped): fall
		// back to "somewhere has room", which is what admits the first batch.
		return m.anyPoolHasRoom()
	}
	for _, code := range dests {
		p, ok := m.pools[code]
		if !ok {
			// The pool went away under a reconfigure; the next batch re-learns
			// where this queue's traffic goes.
			return m.anyPoolHasRoom()
		}
		if p.QueueSize() < p.queueCapacity() {
			return true
		}
	}
	return false
}

// anyPoolHasRoom reports whether at least one pool has room. Caller holds
// poolMu.
func (m *Manager) anyPoolHasRoom() bool {
	for _, p := range m.pools {
		if p.QueueSize() < p.queueCapacity() {
			return true
		}
	}
	return false
}

// awaitCapacity blocks the calling poll loop, UNTIMED, until hasCapacityFor
// reports true or ctx is cancelled — the G12 replacement for the fixed
// time.Sleep(2s) runConsumer used to fall back on whenever every pool this
// queue feeds was full.
//
// Snapshots m.capacityGate BEFORE checking hasCapacityFor, never after: a
// pool crossing back under capacity between the check and the wait would
// otherwise be a signal raised into a gap nothing is listening in yet, lost
// for good — see capacityGate's doc. Returns false only on ctx cancellation,
// so runConsumer can exit its poll loop promptly instead of parking through
// a shutdown.
//
// The second wake-up is the deferral ledger: a loop parked because its
// budget is spent gets budget back when the earliest deferred message comes
// due, and no pool signals that.
func (m *Manager) awaitCapacity(ctx context.Context, rc *runningConsumer) bool {
	for {
		sig := m.capacityGate.snapshot()
		if m.hasCapacityFor(rc) {
			return true
		}
		var due <-chan time.Time
		var timer *time.Timer
		if at, ok := rc.deferrals.earliest(); ok {
			timer = time.NewTimer(time.Until(at))
			due = timer.C
		}
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return false
		case <-sig:
			if timer != nil {
				timer.Stop()
			}
		case <-due:
		}
	}
}
