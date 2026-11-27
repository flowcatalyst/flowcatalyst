package router

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// defaultPoolCode is the fallback pool for messages whose pool_code is empty
// or names a pool that isn't configured.
const defaultPoolCode = "DEFAULT-POOL"

// defaultPoolConcurrency is the default pool concurrency (20),
// used when the config doesn't define an explicit DEFAULT-POOL.
const defaultPoolConcurrency uint32 = 20

// consumerRestartDelay is the pause before re-spawning a stalled consumer —
// avoids a thundering-herd of reconnects when several stall at once (5s).
const consumerRestartDelay = 5 * time.Second

// consumerPollTimeout bounds a single consumer.Poll. It has to clear the
// backends' own long-poll wait — SQS caps at 20s (sqs.DefaultWaitSeconds) — by
// enough margin that a healthy slow poll never trips it, while staying well
// under any sane stall threshold so a hung poll surfaces as a retryable error
// long before the watchdog would tear the consumer down.
const consumerPollTimeout = 30 * time.Second

// restartRecord is one queue's restart history: how many consecutive attempts,
// and when the last one was. The timestamp is what makes the count meaningful —
// see the recovery rule in RestartStalledConsumers.
type restartRecord struct {
	attempts int
	last     time.Time
}

// consumerRebuildTimeout bounds building a replacement consumer. Building one
// is real network I/O — for SQS it loads AWS config and resolves credentials —
// and the SQS backend sets no client-level timeout, so an unbounded build can
// hang indefinitely. That matters more here than anywhere else: the rebuild
// runs synchronously inside the watchdog's own tick, so a hang takes down the
// loop whose job is to fix hangs, and nothing polls that queue again until the
// process restarts.
const consumerRebuildTimeout = 20 * time.Second

// capacityStuckWarnInterval is how often a consumer paused on backpressure
// re-reports that its destination pools are still full. Long enough not to
// flood /warnings, short enough that a wedged pool is never silent.
const capacityStuckWarnInterval = 5 * time.Minute

// consumerRestartCriticalAfter escalates a repeatedly-stalling consumer's
// warning to CRITICAL after this many restart attempts.
const consumerRestartCriticalAfter = 10

// Manager owns the running consumers and pools and the routing between them.
//
// Topology: N consumers (one per queue)
// each run a poll loop that hands batches to route(); route() assigns a
// batch id, drops external-requeue duplicates, then groups each message to
// the pool named by its pool_code (DEFAULT-POOL fallback) and submits it.
// Pools are passive — they do not own a queue. A pool processes messages
// routed from many queues, and ack/nack targets each message's SOURCE
// consumer (resolved by QueueIdentifier via resolveConsumer).
type Manager struct {
	mediator    Mediator
	tracker     *InFlightTracker
	poolResults atomic.Pointer[PoolResultRecorder] // optional; set via SetPoolResultRecorder.
	warnings    atomic.Pointer[WarningService]     // optional; set via SetWarnings. nil → no-op.

	// reconfigureMu serialises whole reconciles (Reconfigure, Shutdown) against
	// each other. It guards no state; the two data locks below do that, and are
	// taken in sequence rather than nested, so there is no lock ordering to get
	// wrong.
	reconfigureMu sync.Mutex

	// Pools and consumers are reconciled together but read apart, and the
	// routing hot path only ever needs pools, so they get a lock each.
	poolMu sync.RWMutex
	pools  map[string]*Pool // pool code → passive pool

	consumerMu sync.RWMutex
	consumers  map[string]*runningConsumer   // queue name (config) → consumer + poll loop
	queues     map[string]common.QueueConfig // queue name → cfg (for publishers)

	// consumersByID mirrors consumers, keyed by each consumer's OWN
	// Identifier() rather than the config queue name. The two differ for
	// NATS: its Identifier() is "<stream>/<consumer>", not the operator's
	// config-name label, and every QueuedMessage/InFlightMessage carries
	// QueueIdentifier — the broker identity, not the config name (see
	// resolveConsumer). Postgres and SQS happen to report the config name
	// as their Identifier() too, so this only changes behaviour for NATS,
	// but ack/nack resolution must always go through the identity the
	// backend itself reports, not an operator-chosen label it may not
	// share. Maintained in lockstep with consumers at every registration,
	// replacement and teardown — never read or written on its own.
	consumersByID map[string]*runningConsumer
	// missingQueues are configured queues the broker has no queue for yet (see
	// queue.ErrQueueMissing): no consumer, no poll loop, no warning. Guarded by
	// consumerMu. An entry is how the INFO line is logged once per streak
	// rather than on every recheck, and what RecheckMissingQueues walks.
	missingQueues map[string]common.QueueConfig
	// missingSince is when each missingQueues entry was first found missing, and
	// lastFullRecheck when RecheckMissingQueuesPaced last probed all of them.
	// Both guarded by consumerMu. They pace the recheck: a queue that only just
	// went missing (a new client's, created on its first publish) is probed on
	// every config tick, the rest on a slow cadence.
	missingSince    map[string]time.Time
	lastFullRecheck time.Time

	// drainingMu guards drainingPools: pools this Manager removed from
	// routing (m.pools, via Reconfigure) but which are still finishing their
	// buffer asynchronously (Pool.Drain — X-11: removals drain, never
	// flush). A separate lock from poolMu so the operator-facing traversal
	// accessors (AllPools) and the reaper's ReleaseParkedGroups sweep can
	// read it without contending with pool reconciliation. A pool leaves
	// this map only via its own drain-completion callback
	// (Manager.beginPoolDrain's onDone) — never removed by anything else.
	drainingMu    sync.Mutex
	drainingPools map[string]*Pool

	// detachMu guards detaching: consumers Reconfigure removed or
	// reconfigured out of m.consumers/m.queues but which still own buffered
	// or in-flight messages (X-11 / R-26/R-49). Poll is already stopped
	// (rc.stopPoll fired before the entry moved here); the consumer stays
	// resolvable via resolveConsumer's fallback so those messages can still
	// ack/nack. A slice, not a map: a CHANGED queue's outgoing instance and
	// a REMOVED queue's instance carry no natural exclusion against each
	// other by name (a queue can change more than once before the first
	// change has finished draining), and both keep the SAME
	// consumer.Identifier() as whatever replaced them, so keying by name
	// would let a later entry silently clobber an earlier one still
	// draining. retireDetachedConsumers (reaper tick) removes an entry once
	// InFlightTracker.CountForQueue reports nothing left referencing it.
	detachMu  sync.Mutex
	detaching []*runningConsumer

	// pollingStopped latches once StopPolling has run, so the stalled-consumer
	// watchdog doesn't helpfully respawn the poll loops a drain just stopped.
	pollingStopped atomic.Bool

	// consumerRoot parents every consumer's context, and only Shutdown cancels
	// it. Consumers must NOT inherit from whoever called Reconfigure: that is
	// a config-sync fetch context or a bootstrap context, and tying a poll
	// loop's life to a request-scoped deadline kills it the moment that
	// deadline passes — which is exactly what the embedded default-broker path
	// did, handing Reconfigure a 10s context and losing every consumer with it.
	rootMu     sync.Mutex
	root       context.Context
	rootCancel context.CancelFunc

	// consumerCounters maps a queue name to its *consumerEventCounters (see
	// event_counters.go): per-queue poll counters that outlive any one
	// consumer instance.
	consumerCounters sync.Map

	// wg counts the running poll loops (runConsumer). An idleGroup rather
	// than a sync.WaitGroup so Shutdown can wait on it with a deadline
	// without leaking a helper goroutine when the deadline wins.
	wg idleGroup

	// restartAttempts tracks consecutive restart attempts per stalled consumer
	// so a repeatedly-failing consumer escalates to a CRITICAL warning, and a
	// genuinely recovered one is cleared. Touched only by
	// RestartStalledConsumers, which the lifecycle watchdog calls from a single
	// goroutine — no lock needed.
	restartAttempts map[string]restartRecord

	batchCounter atomic.Uint64

	// restartDelay is the pause before rebuilding a stalled consumer. A field
	// rather than the constant so tests don't sit out five seconds per restart.
	restartDelay time.Duration

	// rebuildTimeout bounds each replacement-consumer build. A field rather
	// than the constant for the same reason as restartDelay.
	rebuildTimeout time.Duration

	// pollTimeout bounds each consumer.Poll call. A field for the same reason.
	pollTimeout time.Duration

	pubMu      sync.Mutex
	publishers map[string]queue.Publisher // queue name → publisher (lazy)

	// strictRouting gates R-13/R-16: when set, route() ACKs (never delivers,
	// never NACKs) a message with an empty pool_code, an empty/absent
	// dispatch_mode, or an ordered mode with no message_group_id, instead of
	// silently falling back (DEFAULT-POOL / DefaultDispatchMode / the shared
	// "" group). Off by default — see SetStrictRouting.
	strictRouting atomic.Bool

	// synthMu guards synthPools, which tracks only pools this Manager itself
	// synthesised on demand (see ensureFallbackPool) — never configured ones.
	// A separate lock from poolMu because the routing hot path touches it on
	// every message to a fallback pool (touchSynthPool), and that must not
	// contend with Reconfigure's pool reconciliation. Lock order, whenever
	// both are held: poolMu before synthMu.
	synthMu    sync.RWMutex
	synthPools map[string]*synthPoolState

	// settledReporter is handed to every pool this Manager creates (see
	// SetSettledReporter). Held here rather than passed to NewPool so there
	// is one wiring point for both pool-creation sites — configured pools in
	// Reconfigure and synthesised fallbacks in ensureFallbackPool. nil means
	// "no platform behind this router", which is the standalone default.
	srMu            sync.RWMutex
	settledReporter SettledReporter

	// capacityGate wakes every poll loop parked in awaitCapacity the moment
	// some pool's capacity might have changed (G12): a pool crossing back
	// under capacity (wired via Pool.SetCapacityFreed at every
	// pool-creation site), or a reconfigure that could change which pools
	// exist or feed a queue. Never nil after NewManager.
	capacityGate *capacityGate

	// deferralBudget and maxDeferral are the backpressure-deferral knobs
	// (pool_admission.go, deferral_ledger.go), zero meaning the package
	// defaults. Set before Reconfigure builds pools; maxDeferral is handed
	// to each pool at construction, deferralBudget is read per poll.
	deferralBudget atomic.Int64
	maxDeferral    atomic.Int64
}

// SetDeferralBudget bounds how many deferred messages one queue's consumer
// may have outstanding before it stops polling into full pools (see
// defaultDeferralBudget). Zero restores the default.
func (m *Manager) SetDeferralBudget(n int) { m.deferralBudget.Store(int64(n)) }

// SetMaxDeferral sets the reservation horizon handed to every pool built
// after this call (see defaultMaxDeferral). Zero restores the default.
func (m *Manager) SetMaxDeferral(d time.Duration) { m.maxDeferral.Store(int64(d)) }

func (m *Manager) deferralBudgetValue() int {
	if n := m.deferralBudget.Load(); n > 0 {
		return int(n)
	}
	return defaultDeferralBudget
}

// noteDeferral is every pool's deferral observer (Pool.SetDeferralObserver):
// it books the return time on the ledger of the consumer that owns queueID
// so that consumer's poll loop can count what is out and wake when it
// lands. A queue that has already been deregistered has no ledger to book
// on, and needs none — nothing polls it.
func (m *Manager) noteDeferral(queueID string, returnAt time.Time) {
	m.consumerMu.RLock()
	rc, ok := m.consumersByID[queueID]
	m.consumerMu.RUnlock()
	if ok {
		rc.deferrals.add(returnAt)
	}
}

// wirePool applies the Manager-level wiring every pool needs, whichever of
// the two creation sites (Reconfigure, ensureFallbackPool) built it.
func (m *Manager) wirePool(p *Pool) {
	p.SetSettledReporter(m.settledReporterRef())
	p.SetCapacityFreed(m.capacityGate.signal)
	p.SetDeferralObserver(m.noteDeferral)
	p.SetMaxDeferral(time.Duration(m.maxDeferral.Load()))
	p.results = m.recordPoolResult
}

// PoolResultRecorder receives each pool's delivery outcomes; HealthService
// implements it and turns them into the rolling pool success rate.
type PoolResultRecorder interface {
	RecordPoolResult(poolCode string, success bool)
}

// SetPoolResultRecorder wires the recorder every pool reports outcomes to.
// Opt-in; set once at startup before Start. Without it the pool success-rate
// clause of the health report never fires.
func (m *Manager) SetPoolResultRecorder(r PoolResultRecorder) { m.poolResults.Store(&r) }

func (m *Manager) recordPoolResult(poolCode string, success bool) {
	if r := m.poolResults.Load(); r != nil {
		(*r).RecordPoolResult(poolCode, success)
	}
}

// SetSettledReporter wires the T3/A-01 settled-message reporter that every
// pool this Manager creates will fire from ackBuffered when a
// BLOCK_ON_ERROR head fails terminally (see settled.go). Set once, before
// Reconfigure builds any pools; pools created afterwards — including
// synthesised per-client fallback pools — pick it up at construction, so
// there is exactly one wiring point rather than one per pool.
//
// nil is the correct default: a standalone router with no platform behind
// it stays completely silent, exactly as before this feature existed.
func (m *Manager) SetSettledReporter(r SettledReporter) {
	m.srMu.Lock()
	m.settledReporter = r
	m.srMu.Unlock()
}

// settledReporterRef reads the reporter for a pool being constructed. Both
// call sites already hold poolMu; this takes only srMu, which is never held
// while taking any other lock, so it introduces no ordering constraint.
func (m *Manager) settledReporterRef() SettledReporter {
	m.srMu.RLock()
	defer m.srMu.RUnlock()
	return m.settledReporter
}

// NewManager builds a manager. The mediator (which now owns the per-endpoint
// circuit breakers) is shared by all pools. tracker may be nil; if so, pools
// run without in-flight tracking.
func NewManager(mediator Mediator, tracker *InFlightTracker) *Manager {
	m := &Manager{
		mediator:        mediator,
		tracker:         tracker,
		pools:           make(map[string]*Pool),
		consumers:       make(map[string]*runningConsumer),
		consumersByID:   make(map[string]*runningConsumer),
		queues:          make(map[string]common.QueueConfig),
		drainingPools:   make(map[string]*Pool),
		publishers:      make(map[string]queue.Publisher),
		synthPools:      make(map[string]*synthPoolState),
		restartAttempts: make(map[string]restartRecord),
		restartDelay:    consumerRestartDelay,
		rebuildTimeout:  consumerRebuildTimeout,
		pollTimeout:     consumerPollTimeout,
		capacityGate:    newCapacityGate(),
	}
	m.root, m.rootCancel = context.WithCancel(context.Background())
	return m
}

// consumerRoot returns the current parent context for consumer goroutines.
func (m *Manager) consumerRoot() context.Context {
	m.rootMu.Lock()
	defer m.rootMu.Unlock()
	return m.root
}

// cancelConsumerRoot stops every consumer goroutine and installs a fresh root,
// because Shutdown is not necessarily terminal: in standby mode it runs on
// leadership LOSS and a later regain reconfigures the same Manager, which would
// otherwise spawn consumers into an already-cancelled context.
func (m *Manager) cancelConsumerRoot() {
	m.rootMu.Lock()
	defer m.rootMu.Unlock()
	m.rootCancel()
	m.root, m.rootCancel = context.WithCancel(context.Background())
}

// SetWarnings wires a WarningService so routing/capacity conditions surface on
// /warnings and into health. Opt-in; set once at startup before Start.
func (m *Manager) SetWarnings(ws *WarningService) { m.warnings.Store(ws) }

// SetStrictRouting toggles FC_ROUTER_STRICT_ROUTING (R-13/R-16): when true,
// route() ACKs a malformed message (empty pool_code, empty dispatch_mode, or
// an ordered mode with no message_group_id) instead of applying the usual
// fallback. Safe to call at any time; read on the routing hot path via an
// atomic, so a runtime toggle takes effect on the next batch.
func (m *Manager) SetStrictRouting(v bool) { m.strictRouting.Store(v) }

// resolveConsumer maps a message's origin queue to its consumer so a pool can
// ack/nack on the right queue. queueID is always a QueueIdentifier off a
// QueuedMessage or InFlightMessage — the queue backend's OWN identity (see
// consumersByID's doc comment), never the config queue name. Checks the
// active routing table first (by that identity, via consumersByID), then
// falls back to a lingering (removed/changed) consumer still detaching (X-11
// / R-26/R-49 — see the detaching field's doc comment): a message buffered
// before its queue dropped out of config must still resolve to SOMETHING,
// or its ack/nack is silently skipped, stranding both the tracker entry and
// the broker copy. Returns nil only once nothing — active or detaching —
// answers to queueID.
func (m *Manager) resolveConsumer(queueID string) queue.Consumer {
	m.consumerMu.RLock()
	if rc, ok := m.consumersByID[queueID]; ok {
		m.consumerMu.RUnlock()
		return rc.consumer
	}
	m.consumerMu.RUnlock()

	m.detachMu.Lock()
	defer m.detachMu.Unlock()
	for _, rc := range m.detaching {
		if rc.consumer.Identifier() == queueID {
			return rc.consumer
		}
	}
	return nil
}

// NackInFlight returns a stuck in-flight message to its source queue for
// redelivery, resolving the source consumer by queue identifier. Backs the
// StallDetector's force-NACK path (see StallConfig.ForceNackStalled). Errors if
// the source queue was deregistered.
func (m *Manager) NackInFlight(ctx context.Context, queueID, receiptHandle string, delaySeconds uint32) error {
	c := m.resolveConsumer(queueID)
	if c == nil {
		return fmt.Errorf("no consumer for queue %q", queueID)
	}
	return c.Nack(ctx, receiptHandle, &delaySeconds)
}

// ForceAckResult reports what ForceAckInFlight did: the entry as it stood
// when acked, and the outcome of the best-effort broker delete.
type ForceAckResult struct {
	Entry  common.InFlightMessage
	AckErr error
}

// ForceAckInFlight is the operator override behind the dashboard's force-ACK:
// it deletes the broker copy of a tracked message (using the freshest receipt
// handle from the tracker) and releases the tracker entry so future
// redeliveries or external requeues re-enter the pipeline fresh instead of
// being ACK-dropped as duplicates of a stuck/phantom owner. The broker ack is
// best-effort — an expired receipt handle still clears the tracker entry,
// which is the part that unblocks a phantom. It does NOT abort a worker that
// is currently mediating the message; that attempt runs to its own terminal
// state (its eventual ack/nack logs a warn against the now-stale handle).
func (m *Manager) ForceAckInFlight(ctx context.Context, messageID string) (ForceAckResult, bool) {
	if m.tracker == nil {
		return ForceAckResult{}, false
	}
	entry, ok := m.tracker.Lookup(messageID)
	if !ok {
		return ForceAckResult{}, false
	}
	res := ForceAckResult{Entry: entry}
	if c := m.resolveConsumer(entry.QueueIdentifier); c != nil {
		res.AckErr = c.Ack(ctx, entry.ReceiptHandle, entry.BrokerMessageID)
	} else {
		res.AckErr = fmt.Errorf("no consumer for queue %q", entry.QueueIdentifier)
	}
	m.tracker.Remove(entry.MessageID, entry.BrokerMessageID)
	slog.Warn("force-acked in-flight message (operator request)",
		"message_id", entry.MessageID, "queue", entry.QueueIdentifier,
		"elapsed_s", entry.ElapsedSeconds(), "attempts", entry.Attempts, "ack_err", res.AckErr)
	return res, true
}

// Consumers returns every running consumer (for the QueueHealthMonitor /
// metrics to call Metrics/Counters on).
func (m *Manager) Consumers() []queue.Consumer {
	m.consumerMu.RLock()
	defer m.consumerMu.RUnlock()
	out := make([]queue.Consumer, 0, len(m.consumers))
	for _, rc := range m.consumers {
		out = append(out, rc.consumer)
	}
	return out
}

// ConsumerStats reports each consumer's liveness for the health service.
//
// The Manager is the only honest source: membership of m.consumers is what
// "running" means, and rc.lastPoll is the same heartbeat the restart watchdog
// judges a consumer by. Serving health from anywhere else would let the two
// disagree about whether the same consumer is alive.
func (m *Manager) ConsumerStats() []ConsumerStat {
	m.consumerMu.RLock()
	defer m.consumerMu.RUnlock()
	out := make([]ConsumerStat, 0, len(m.consumers))
	for name, rc := range m.consumers {
		var last time.Time
		if lp := rc.lastPoll.Load(); lp != 0 {
			last = time.Unix(0, lp)
		}
		out = append(out, ConsumerStat{QueueName: name, Running: true, LastPoll: last})
	}
	return out
}

// PoolStats returns one snapshot per running pool (map iteration order).
func (m *Manager) PoolStats() []PoolStats {
	m.poolMu.RLock()
	defer m.poolMu.RUnlock()
	out := make([]PoolStats, 0, len(m.pools))
	for _, p := range m.pools {
		out = append(out, p.Stats())
	}
	return out
}

// MediatingSnapshot returns every message currently inside a pool worker,
// across all pools — the live, never-reaped "what is mediating right now" set.
func (m *Manager) MediatingSnapshot() []MediatingEntry {
	m.poolMu.RLock()
	pools := make([]*Pool, 0, len(m.pools))
	for _, p := range m.pools {
		pools = append(pools, p)
	}
	m.poolMu.RUnlock()
	// Snapshot each pool OUTSIDE the manager lock (each pool takes its own).
	var out []MediatingEntry
	for _, p := range pools {
		out = append(out, p.MediatingSnapshot()...)
	}
	return out
}

// PoolCodes returns the codes of all currently registered pools.
func (m *Manager) PoolCodes() []string {
	m.poolMu.RLock()
	defer m.poolMu.RUnlock()
	out := make([]string, 0, len(m.pools))
	for code := range m.pools {
		out = append(out, code)
	}
	return out
}

// Pool returns the running pool with the given code, or nil if absent.
func (m *Manager) Pool(code string) *Pool {
	m.poolMu.RLock()
	defer m.poolMu.RUnlock()
	return m.pools[code]
}

// AllPools returns every pool this Manager is tracking — actively routed
// (m.pools) plus any still finishing an asynchronous removal-drain
// (drainingPools; see its field doc comment) — for the operator-facing
// monitoring endpoints (R-52 flush-suppression listing, R-04 blocked-groups)
// that must keep showing a removed pool's live state for as long as it
// still holds any. The two locks are taken in sequence, never nested — the
// same pattern as every other accessor here. Order is unspecified (map
// iteration).
func (m *Manager) AllPools() []*Pool {
	m.poolMu.RLock()
	out := make([]*Pool, 0, len(m.pools))
	for _, p := range m.pools {
		out = append(out, p)
	}
	m.poolMu.RUnlock()

	m.drainingMu.Lock()
	for _, p := range m.drainingPools {
		out = append(out, p)
	}
	m.drainingMu.Unlock()
	return out
}

// QueueMetrics returns per-consumer broker attributes + counters. Calls
// Consumer.Metrics(ctx) which may do a broker round-trip.
func (m *Manager) QueueMetrics(ctx context.Context) []queue.Metrics {
	consumers := m.Consumers()
	out := make([]queue.Metrics, 0, len(consumers))
	for _, c := range consumers {
		mtr, err := c.Metrics(ctx)
		if err != nil {
			slog.Warn("queue metrics fetch failed", "queue", c.Identifier(), "err", err)
			continue
		}
		if mtr != nil {
			out = append(out, *mtr)
		}
	}
	return out
}

// QueueCounters returns process-local counters only (no broker round-trip).
func (m *Manager) QueueCounters() []queue.Metrics {
	consumers := m.Consumers()
	out := make([]queue.Metrics, 0, len(consumers))
	for _, c := range consumers {
		if mtr := c.Counters(); mtr != nil {
			out = append(out, *mtr)
		}
	}
	return out
}

// Publisher returns (and lazily caches) a publisher for manual/test message
// injection. The routing model publishes to a QUEUE (the message's pool_code
// then routes it), so `key` is resolved to a queue: a queue named `key` if
// one exists, else any registered queue (deterministic). Errors when no
// queue is registered.
func (m *Manager) Publisher(ctx context.Context, key string) (queue.Publisher, error) {
	qc, ok := m.queueForPublish(key)
	if !ok {
		return nil, fmt.Errorf("publisher: no queue registered for %q", key)
	}

	m.pubMu.Lock()
	if p, ok := m.publishers[qc.Name]; ok {
		m.pubMu.Unlock()
		return p, nil
	}
	m.pubMu.Unlock()

	pub, err := queue.NewPublisher(ctx, qc)
	if err != nil {
		return nil, fmt.Errorf("publisher: build for %q: %w", qc.Name, err)
	}
	m.pubMu.Lock()
	if existing, ok := m.publishers[qc.Name]; ok {
		m.pubMu.Unlock()
		return existing, nil
	}
	m.publishers[qc.Name] = pub
	m.pubMu.Unlock()
	return pub, nil
}

func (m *Manager) queueForPublish(key string) (common.QueueConfig, bool) {
	m.consumerMu.RLock()
	defer m.consumerMu.RUnlock()
	if qc, ok := m.queues[key]; ok {
		return qc, true
	}
	if len(m.queues) == 0 {
		return common.QueueConfig{}, false
	}
	return m.queues[slices.Min(slices.Collect(maps.Keys(m.queues)))], true
}

// UpdatePool applies a runtime config update to an existing pool. See the
// PUT /monitoring/pools/{poolCode} handler. Concurrency==0 leaves it
// unchanged; setRateLimit toggles whether rateLimitPerMinute is applied.
func (m *Manager) UpdatePool(code string, concurrency uint32, rateLimitPerMinute *uint32, setRateLimit bool) bool {
	pool := m.Pool(code)
	if pool == nil {
		return false
	}
	if concurrency != 0 {
		if !pool.UpdateConcurrency(concurrency) {
			return false
		}
	}
	if setRateLimit {
		pool.UpdateRateLimit(rateLimitPerMinute)
	}
	return true
}

// StopPolling ends every consumer's poll loop while leaving work already in the
// pipeline running. It is the first move of a graceful shutdown: intake stops,
// then the drain has something that can actually reach zero. Cancelling the
// consumers outright instead would abort the very deliveries the drain waits
// for, and the drain would then sit out its whole timeout on messages nothing
// was going to finish.
//
// Idempotent, and latched — the stalled-consumer watchdog must not respawn what
// this stopped.
func (m *Manager) StopPolling() {
	m.pollingStopped.Store(true)
	m.consumerMu.RLock()
	defer m.consumerMu.RUnlock()
	for _, rc := range m.consumers {
		rc.stopPoll()
	}
}

// Shutdown cancels all consumer poll loops, stops the pools, and waits for
// the poll loops to exit.
//
// Shutdown is not necessarily terminal: in standby/HA mode it runs on
// leadership LOSS, and a later regain calls Reconfigure on the same Manager.
// The maps are therefore re-made empty rather than nil'd — Reconfigure
// assigns into them, and writing to a nil map panics (in the Watch
// goroutine, taking the process down on the designed failover path).
func (m *Manager) Shutdown(ctx context.Context) error {
	// Held for the same reason Reconfigure holds it: a reconcile arriving
	// mid-shutdown must not start a consumer into the maps we are clearing.
	m.reconfigureMu.Lock()
	defer m.reconfigureMu.Unlock()

	// Pools first, while their consumers are still up: Stop nacks every
	// buffered message back to its broker, and a nack needs the source
	// consumer. Stopping consumers first (as this used to) left every
	// buffered message to its visibility timeout, its group stuck behind it.
	m.stopAllPools()

	m.consumerMu.Lock()
	for _, rc := range m.consumers {
		rc.cancel()
		rc.consumer.Stop()
	}
	m.consumers = make(map[string]*runningConsumer)
	m.consumersByID = make(map[string]*runningConsumer)
	m.queues = make(map[string]common.QueueConfig)
	m.consumerMu.Unlock()

	// Any consumer still detaching (X-11 removal/change-drain in progress)
	// gets the same hard teardown as an active one: process shutdown (or
	// leadership loss, which runs this same path) is past the graceful
	// drain window — StopPolling + the bounded wait already happened at the
	// Server level before Shutdown was ever called — so there is nothing
	// left to wait on here.
	m.detachMu.Lock()
	for _, rc := range m.detaching {
		rc.cancel()
		rc.consumer.Stop()
	}
	m.detaching = nil
	m.detachMu.Unlock()

	// Cancel anything still running under the root (and install a fresh one, so
	// a leadership regain can reconfigure this same Manager), then let polling
	// resume for that regain.
	m.cancelConsumerRoot()
	m.pollingStopped.Store(false)

	// Same "not necessarily terminal" reasoning as the pool maps: a later
	// regain re-synthesises fallback pools on demand, into a clean map
	// rather than one carrying entries for pools that no longer exist.
	m.synthMu.Lock()
	m.synthPools = make(map[string]*synthPoolState)
	m.synthMu.Unlock()

	// Wait for the poll loops to exit, or the caller's budget to run out.
	// No helper goroutine bridges a WaitGroup here: that helper leaked on
	// every timeout (once per leadership flap), parked in Wait until the
	// stragglers exited. idleGroup reports "none left" as a channel.
	select {
	case <-m.wg.Idle(): // every poll loop has exited
		return nil
	case <-ctx.Done(): // shutdown budget spent; stragglers exit on their own
		return ctx.Err()
	}
}

// stopAllPools stops every routed pool and every pool still draining after
// removal, and empties both maps. Shutdown is not necessarily terminal (see
// its doc comment), so the maps are re-made rather than nil'd.
//
// Same hard-teardown reasoning for a draining pool as for a detaching
// consumer: a pool mid-background-drain (Pool.Drain) is force-stopped now
// rather than left to finish on its own goroutine. Stop is idempotent, so
// this is safe even if that goroutine calls Stop again moments later.
//
// The pools are taken under their locks and stopped outside them: Stop
// nacks through the broker, and routing must not wait on that.
func (m *Manager) stopAllPools() {
	m.poolMu.Lock()
	stopping := make([]*Pool, 0, len(m.pools))
	for _, p := range m.pools {
		stopping = append(stopping, p)
	}
	m.pools = make(map[string]*Pool)
	m.poolMu.Unlock()

	m.drainingMu.Lock()
	for _, p := range m.drainingPools {
		stopping = append(stopping, p)
	}
	m.drainingPools = make(map[string]*Pool)
	m.drainingMu.Unlock()

	for _, p := range stopping {
		p.Stop()
	}
}

// PoolCount returns the count of running pools (for /health or /metrics).
func (m *Manager) PoolCount() int {
	m.poolMu.RLock()
	defer m.poolMu.RUnlock()
	return len(m.pools)
}
