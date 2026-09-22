package router

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// Backpressure deferral — owner ruling 2026-09-22 on head-of-line blocking.
//
// A queue feeds many pools. When one of them is full the consumer used to
// stop polling the queue altogether, so a dedicated pool at concurrency 1
// handed 10,000 five-second messages starved every other pool on that
// client's queue for the fourteen hours it took to drain. The consumer now
// keeps polling while ANY of the queue's pools has room
// (Manager.hasCapacityFor), and a message for a full pool is handed back to
// the broker here with a delay — via Defer, not Nack: it has not failed.
//
// The delay is a RESERVATION, not an estimate. The pool measures its own
// completion rate and keeps a cursor, nextReturn, of when the last deferred
// message was told to come back. Each deferral is booked one slot after
// that cursor (or after the time the current buffer needs to drain,
// whichever is later), so deferred messages return spaced at the pool's
// pace, in the order they were deferred, and each of them bounces about
// once — instead of everyone returning at the same moment to be bounced
// again in a herd. There is deliberately no "come back early" hedge: an
// early return finds the pool still full and has to re-book at the BACK of
// the schedule, which both wastes the round-trip and pushes the cursor one
// slot further from reality every time it happens.
//
// The cost of a reservation is that it is not recalled: raise the pool's
// concurrency tenfold mid-backlog and messages already deferred still return
// on the old schedule, so the pool idles until they do. maxDeferral bounds
// that idle at an hour by default (owner: someone who set concurrency 1 is
// not about to change it drastically; the edge case is worth it). Beyond
// the horizon a reservation is clamped, with a little jitter so the tail of
// a large backlog trickles back over the last quarter-hour of the horizon
// instead of arriving as one wave.
//
// A bounce is a redelivery, and redeliveries are counted: SQS bumps
// ApproximateReceiveCount (a redrive policy counts it), and NATS spends a
// MaxDeliver attempt (unlimited by default, for exactly this reason — see
// that package's doc).

const (
	// deferralMinDelay floors a reservation. Anything shorter is a hot loop
	// against the broker for a pool that has not had time to free a slot.
	deferralMinDelay = 5 * time.Second

	// deferralFallbackWait is the buffer-drain estimate when the pool has no
	// completion in the rate window to measure from (just created, or every
	// delivery in it is still running). Wrong-early costs one bounce; by the
	// time it lands there is usually a rate.
	deferralFallbackWait = 30 * time.Second

	// deferralRateWindow is how far back CompletionRate looks.
	deferralRateWindow = 5 * time.Minute

	// deferralOrderedSpacing is the minimum gap between consecutive
	// reservations on a broker that does not itself keep a deferred group
	// in order (NATS: HonoursDelayedReturn false). Redelivery timers are not
	// sub-second precise, so two reservations a few milliseconds apart could
	// come back swapped; a second apart they cannot. Only matters on fast
	// pools — a slot on a 5s-per-message pool is already 5s wide.
	deferralOrderedSpacing = time.Second

	// defaultMaxDeferral is the reservation horizon (ServerConfig
	// DeferralMaxDelay overrides it).
	defaultMaxDeferral = time.Hour

	// deferralJitterFraction is the share of maxDeferral over which a
	// clamped reservation is spread backwards.
	deferralJitterFraction = 0.25
)

// admissionSchedule is a pool's deferral bookkeeping. Its own lock, not
// p.mu: deferMsg runs on the routing path and must not queue behind the
// drainers' buffer operations.
type admissionSchedule struct {
	mu         sync.Mutex
	nextReturn time.Time
	// maxDeferral is the horizon; zero means defaultMaxDeferral. Set once at
	// construction (SetMaxDeferral), read under mu.
	maxDeferral time.Duration
	// deferred counts lifetime deferrals for PoolStats.
	deferred uint64
	// observer, when set, is told of every deferral's source queue and
	// return time (Manager.noteDeferral books it on that queue's ledger).
	// nil is checked, not called: a pool built outside a Manager never
	// reports.
	observer func(queueID string, returnAt time.Time)
}

// SetMaxDeferral sets the reservation horizon. Zero restores the default.
func (p *Pool) SetMaxDeferral(d time.Duration) {
	p.admission.mu.Lock()
	p.admission.maxDeferral = d
	p.admission.mu.Unlock()
}

// SetDeferralObserver registers the callback deferMsg reports each
// deferral's return time to — set once by the Manager at construction.
func (p *Pool) SetDeferralObserver(fn func(queueID string, returnAt time.Time)) {
	p.admission.mu.Lock()
	p.admission.observer = fn
	p.admission.mu.Unlock()
}

// Deferred is how many messages this pool has handed back for capacity
// since it was created.
func (p *Pool) Deferred() uint64 {
	p.admission.mu.Lock()
	defer p.admission.mu.Unlock()
	return p.admission.deferred
}

// deferMsg hands a message the pool has no room for back to its source
// broker, to be redelivered when the pool's admission schedule expects to
// have a slot for it. The message is leaving the pipeline, so its tracker
// entry goes first, exactly as in nackMsg — a lingering entry would drop the
// redelivery as a duplicate and the message would never be delivered.
//
// A source with no consumer (deregistered between routing and here) is
// logged and skipped, as nackMsg does: the broker redelivers at its own
// visibility timeout, which is the worse-but-safe outcome.
func (p *Pool) deferMsg(ctx context.Context, qm common.QueuedMessage, reason string) {
	c := p.consumerFor(qm)
	if c == nil {
		if p.tracker != nil {
			p.tracker.Remove(qm.Message.ID, qm.BrokerMessageID)
		}
		slog.Warn("defer: no consumer for queue", "queue", qm.QueueIdentifier, "message_id", qm.Message.ID, "reason", reason)
		return
	}
	now := time.Now()
	delay := p.admissionDelay(now, c.HonoursDelayedReturn())
	seconds := uint32(delay / time.Second)
	// The tracker entry is KEPT and marked, not removed: the copy is coming
	// back, and until it does a second copy of the same message id (the
	// platform republishing a job it thinks is stranded) must be deleted as a
	// duplicate rather than deferred beside it. Marked even when the broker
	// call fails — the message then returns at its natural visibility, still
	// as itself.
	if p.tracker != nil {
		p.tracker.MarkDeferred(qm.Message.ID, now.Add(time.Duration(seconds)*time.Second))
	}
	if err := c.Defer(ctx, qm.ReceiptHandle, &seconds); err != nil {
		slog.Warn("defer failed", "reason", reason, "message_id", qm.Message.ID, "err", err)
		return
	}
	p.admission.mu.Lock()
	observer := p.admission.observer
	p.admission.mu.Unlock()
	if observer != nil {
		observer(qm.QueueIdentifier, now.Add(time.Duration(seconds)*time.Second))
	}
	slog.Debug("deferred message for pool capacity",
		"pool", p.cfg.Code, "message_id", qm.Message.ID, "queue", qm.QueueIdentifier,
		"delay_seconds", seconds, "queued", p.queueSize.Load(), "reason", reason)
}

// admissionDelay books the next reservation and returns how long from now
// it is. brokerOrders is the source's HonoursDelayedReturn: when false the
// slot spacing is floored at deferralOrderedSpacing (see that constant).
//
// Arithmetic: with the pool draining at rate r and q messages buffered,
// the buffer needs q/r to clear, so nothing deferred can be admitted before
// now+q/r. The reservation is one slot (1/r) after the later of that and
// the previous reservation, and the cursor moves to it. A cursor in the
// past is simply overtaken — no reset is needed, and none is done, because
// the crossing back under capacity happens on every single completion of a
// pool that is being kept full and would reset the schedule constantly.
func (p *Pool) admissionDelay(now time.Time, brokerOrders bool) time.Duration {
	wait := deferralFallbackWait
	slot := deferralOrderedSpacing
	if rate, ok := p.metrics.CompletionRate(deferralRateWindow); ok && rate > 0 {
		wait = time.Duration(float64(p.queueSize.Load()) / rate * float64(time.Second))
		slot = time.Duration(float64(time.Second) / rate)
	}
	if !brokerOrders {
		slot = max(slot, deferralOrderedSpacing)
	}

	p.admission.mu.Lock()
	horizon := p.admission.maxDeferral
	if horizon <= 0 {
		horizon = defaultMaxDeferral
	}
	earliest := now.Add(wait)
	if p.admission.nextReturn.After(earliest) {
		earliest = p.admission.nextReturn
	}
	reserved := earliest.Add(slot)
	p.admission.nextReturn = reserved
	p.admission.deferred++
	p.admission.mu.Unlock()

	delay := reserved.Sub(now)
	if delay > horizon {
		// Past the horizon: spread the tail back over the last quarter of it
		// rather than booking everything for the same instant.
		delay = horizon - time.Duration(rand.Float64()*deferralJitterFraction*float64(horizon)) //nolint:gosec // G404: jitter spreads a herd; it is not a secret
	}
	return max(delay, deferralMinDelay)
}
