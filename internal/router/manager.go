package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strconv"
	"strings"
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
	mediator Mediator
	tracker  *InFlightTracker
	warnings atomic.Pointer[WarningService] // optional; set via SetWarnings. nil → no-op.

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
	consumers  map[string]*runningConsumer   // queue name → consumer + poll loop
	queues     map[string]common.QueueConfig // queue name → cfg (for publishers)

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

	wg sync.WaitGroup

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

// synthPoolState is the idle tracker for one synthesised fallback pool. Only
// lastRouted is mutated after creation, and it's atomic so the routing hot
// path (touchSynthPool) never blocks behind the eviction sweep.
type synthPoolState struct {
	lastRouted atomic.Int64 // unix-nano of the most recently routed message
}

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

// NewManager builds a manager. The mediator (which now owns the per-endpoint
// circuit breakers) is shared by all pools. tracker may be nil; if so, pools
// run without in-flight tracking.
func NewManager(mediator Mediator, tracker *InFlightTracker) *Manager {
	m := &Manager{
		mediator:        mediator,
		tracker:         tracker,
		pools:           make(map[string]*Pool),
		consumers:       make(map[string]*runningConsumer),
		queues:          make(map[string]common.QueueConfig),
		drainingPools:   make(map[string]*Pool),
		publishers:      make(map[string]queue.Publisher),
		synthPools:      make(map[string]*synthPoolState),
		restartAttempts: make(map[string]restartRecord),
		restartDelay:    consumerRestartDelay,
		rebuildTimeout:  consumerRebuildTimeout,
		pollTimeout:     consumerPollTimeout,
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
// ack/nack on the right queue. Checks the active routing table first, then
// falls back to a lingering (removed/changed) consumer still detaching (X-11
// / R-26/R-49 — see the detaching field's doc comment): a message buffered
// before its queue dropped out of config must still resolve to SOMETHING,
// or its ack/nack is silently skipped, stranding both the tracker entry and
// the broker copy. Returns nil only once nothing — active or detaching —
// answers to queueID.
func (m *Manager) resolveConsumer(queueID string) queue.Consumer {
	m.consumerMu.RLock()
	if rc, ok := m.consumers[queueID]; ok {
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
	names := make([]string, 0, len(m.queues))
	for n := range m.queues {
		names = append(names, n)
	}
	sort.Strings(names)
	return m.queues[names[0]], true
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

// route handles one poll batch from a consumer.
// It assigns the batch id, registers each message with the in-flight tracker
// (claiming pipeline ownership BEFORE buffering/dispatch, so ordered-group
// buffering windows dedupe too), drops broker redeliveries and ACK-drops
// external-requeue duplicates, then routes each surviving message to the pool
// named by its pool_code (DEFAULT-POOL fallback) and submits it. ack/nack of
// the eventual outcome is the pool's job, against the message's source
// consumer.
// Returns the codes of the pools it submitted to, which the poll loop keeps as
// this queue's destination set for backpressure.
func (m *Manager) route(ctx context.Context, msgs []common.QueuedMessage, source queue.Consumer) []string {
	if len(msgs) == 0 {
		return nil
	}
	batchID := strconv.FormatUint(m.batchCounter.Add(1), 10)
	var fed []string

	for i := range msgs {
		msg := msgs[i]
		msg.BatchID = batchID

		if m.tracker != nil {
			im := common.NewInFlightMessage(&msg.Message, msg.BrokerMessageID, msg.QueueIdentifier, msg.BatchID, msg.ReceiptHandle)
			switch m.tracker.Register(im) {
			case RegisterRedelivery:
				// A copy of a message already in the pipeline (processing,
				// retrying, or buffered in an ordered group). The owner adopted
				// this fresher receipt handle; drop this copy — nothing to
				// release, SQS Nack is a no-op. If the owner sits in an ordered
				// group whose drainer died with its consumer (restart /
				// reconfigure), this redelivery is the resume signal: kick the
				// group so a fresh drainer picks the buffer back up.
				slog.Debug("broker redelivery of in-flight message; swapped receipt handle, dropped copy",
					"message_id", msg.Message.ID, "queue", source.Identifier())
				if group := msg.Message.GroupID(); group != "" && msg.Message.DispatchMode.RequiresOrdering() {
					if pool := m.poolByCode(msg.Message.PoolCode); pool != nil {
						pool.tryDrainGroup(ctx, group)
					}
				}
				continue

			case RegisterExternalRequeue:
				// The same application message ID is in flight under a
				// DIFFERENT broker id: an external process requeued a message
				// it thought was lost while the original still owns the work.
				// ACK this copy so the duplicate is DELETED from the broker —
				// otherwise it redelivers forever. The owner's entry (and
				// receipt handle) was deliberately left untouched.
				slog.Info("external requeue detected; ACKing duplicate", "message_id", msg.Message.ID, "queue", source.Identifier())
				if err := source.Ack(ctx, msg.ReceiptHandle, msg.BrokerMessageID); err != nil {
					slog.Warn("ack (external requeue) failed", "message_id", msg.Message.ID, "err", err)
				}
				continue

			case RegisterNew:
				// This copy owns the pipeline; fall through to submit.
			}
		}

		if m.strictRouting.Load() {
			if reason := malformedRoutingReason(&msg.Message); reason != "" {
				// None of these are fixable by the usual fallback (DEFAULT-POOL,
				// DefaultDispatchMode, the shared "" group) — under strict
				// routing they are a producer bug, not something to paper over.
				// ACK only: it must never be delivered, and it must never be
				// NACKed back onto the broker's redelivery clock either, since
				// nothing about a retry would fix a malformed message.
				slog.Warn("strict routing: malformed message; acking without delivery",
					"message_id", msg.Message.ID, "queue", source.Identifier(), "reason", reason)
				if w := m.warnings.Load(); w != nil {
					w.Add(WarningCategoryConfiguration, WarningWarning,
						fmt.Sprintf("malformed message %s on queue %s: %s", msg.Message.ID, source.Identifier(), reason),
						"router")
				}
				if m.tracker != nil {
					m.tracker.Remove(msg.Message.ID, msg.BrokerMessageID)
				}
				if err := source.Ack(ctx, msg.ReceiptHandle, msg.BrokerMessageID); err != nil {
					slog.Warn("ack (strict routing malformed) failed", "message_id", msg.Message.ID, "err", err)
				}
				continue
			}
		}

		pool := m.poolForMessage(msg)
		if pool == nil {
			// No pool at all (not even DEFAULT-POOL configured) — NACK so the
			// message is redelivered once a pool exists. It is leaving the
			// pipeline, so release its just-claimed tracker entry; a lingering
			// entry would classify every future redelivery as a duplicate and
			// the message would never be processed.
			slog.Warn("no pool available for message; nacking", "message_id", msg.Message.ID, "pool_code", msg.Message.PoolCode)
			if m.tracker != nil {
				m.tracker.Remove(msg.Message.ID, msg.BrokerMessageID)
			}
			if err := source.Nack(ctx, msg.ReceiptHandle, ptrU32(5)); err != nil {
				slog.Warn("nack (no pool) failed", "message_id", msg.Message.ID, "err", err)
			}
			continue
		}
		if code := pool.Identifier(); !slices.Contains(fed, code) {
			fed = append(fed, code)
		}
		pool.submit(ctx, msg)
	}
	return fed
}

// poolByCode resolves a pool by code with the DEFAULT-POOL fallback, without
// the routing warning poolForMessage emits — used on the redelivery-resume
// path, which fires repeatedly for the same message.
func (m *Manager) poolByCode(code string) *Pool {
	m.poolMu.RLock()
	defer m.poolMu.RUnlock()
	if code != "" {
		if p, ok := m.pools[code]; ok {
			return p
		}
	}
	return m.pools[defaultPoolCode]
}

// poolForMessage resolves the destination pool for a message: the pool named
// by pool_code, or DEFAULT-POOL when pool_code is empty or unknown (with a
// routing warning for the unknown case). Returns nil only if even
// DEFAULT-POOL is absent.
//
// This runs for every routed message, so the common case — a configured pool —
// is a read-locked map lookup and nothing else. Only the miss paths go
// further, and only one of them writes.
func (m *Manager) poolForMessage(msg common.QueuedMessage) *Pool {
	code := msg.Message.PoolCode
	if code == "" {
		return m.poolByCode("")
	}
	m.poolMu.RLock()
	p, ok := m.pools[code]
	m.poolMu.RUnlock()
	if ok {
		if isSynthPoolCode(code) {
			// Reset the idle clock EvictIdleSynthPools judges this pool by. A
			// no-op if code isn't tracked as synthesised (a configured pool of
			// the same name — Reconfigure removed its entry — must not be kept
			// eviction-eligible by traffic that no longer matters to that).
			m.touchSynthPool(code)
		}
		return p
	}
	// A per-client fallback pool ({identifier}-DEFAULT-POOL) will not be in the
	// config: nothing emits processingPools — the router polls an external
	// config service — so these codes only ever arrive from the scheduler.
	// Synthesise on demand with the same defaults as the global DEFAULT-POOL,
	// exactly as Reconfigure auto-adds that one. Without this every such
	// message would take the unknown-code path below: routed to the shared
	// DEFAULT-POOL with a warning apiece, losing the per-client isolation the
	// namespacing exists to create.
	if isDefaultPoolCode(code) {
		return m.ensureFallbackPool(code)
	}
	// Unknown pool code → DEFAULT-POOL, surfaced as a Routing warning.
	slog.Warn("no pool found for pool_code; routing to DEFAULT-POOL",
		"message_id", msg.Message.ID, "pool_code", code, "default_pool", defaultPoolCode)
	if w := m.warnings.Load(); w != nil {
		w.Add(WarningCategoryRouting, WarningWarning,
			fmt.Sprintf("no pool for pool_code %q; routed to %s", code, defaultPoolCode), "router")
	}
	return m.poolByCode("")
}

// ensureFallbackPool returns the per-client fallback pool for code, creating it
// if this is the first message to name it. Creation is the one write the
// routing path performs, so it is double-checked under the write lock: two
// messages for a new client arriving together must not end up in two different
// pools, each with its own concurrency cap.
//
// Config always wins: this only runs on a miss, and Reconfigure overwrites a
// synthesised pool with a configured one of the same code.
func (m *Manager) ensureFallbackPool(code string) *Pool {
	m.poolMu.Lock()
	defer m.poolMu.Unlock()
	if p, ok := m.pools[code]; ok {
		m.touchSynthPool(code)
		return p
	}
	p := NewPool(
		common.PoolConfig{Code: code, Concurrency: defaultPoolConcurrency},
		m.mediator, m.tracker, m.resolveConsumer,
	)
	p.SetSettledReporter(m.settledReporterRef())
	m.pools[code] = p
	m.trackSynthPool(code)
	slog.Info("synthesised per-client fallback pool",
		"pool_code", code, "concurrency", defaultPoolConcurrency)
	return p
}

// trackSynthPool registers code as eviction-eligible (see
// EvictIdleSynthPools), with its idle clock starting now. Called only from
// ensureFallbackPool at creation, under poolMu — lock order poolMu before
// synthMu, consistent with every other caller that holds both.
func (m *Manager) trackSynthPool(code string) {
	st := &synthPoolState{}
	st.lastRouted.Store(time.Now().UnixNano())
	m.synthMu.Lock()
	m.synthPools[code] = st
	m.synthMu.Unlock()
}

// touchSynthPool resets code's idle clock — a message just routed to it. A
// no-op if code isn't currently tracked as synthesised: a configured pool of
// the same code (Reconfigure) removes its entry, and traffic to it must not
// re-arm an eviction that no longer applies.
func (m *Manager) touchSynthPool(code string) {
	m.synthMu.RLock()
	st, ok := m.synthPools[code]
	m.synthMu.RUnlock()
	if ok {
		st.lastRouted.Store(time.Now().UnixNano())
	}
}

// forgetSynthPool removes code from eviction tracking — called whenever
// config takes ownership of the code (Reconfigure) or the pool is torn down
// outright (Shutdown), so a stale entry never points at a pool that is gone
// or no longer synthesised.
func (m *Manager) forgetSynthPool(code string) {
	m.synthMu.Lock()
	delete(m.synthPools, code)
	m.synthMu.Unlock()
}

// EvictIdleSynthPools stops and removes every synthesised per-client
// fallback pool (ensureFallbackPool) that has routed no message for at least
// ttl. Configured pools are never touched: Reconfigure removes a code from
// the eviction-eligible set the moment config defines it (see wantPools in
// Reconfigure), so only pools this Manager created on demand are ever
// candidates here. Uses Pool.Stop — the same drain path Reconfigure calls
// when a pool drops out of config — so a buffered message is flushed and its
// tracker entry released for broker redelivery rather than lost; the pool is
// re-synthesised on demand by the next message that names its code. Wired
// onto the router's reaper tick (see Server.reapInFlight). Returns the
// number of pools evicted.
func (m *Manager) EvictIdleSynthPools(ttl time.Duration) int {
	if ttl <= 0 {
		return 0
	}
	cutoff := time.Now().Add(-ttl).UnixNano()

	m.synthMu.RLock()
	var idle []string
	for code, st := range m.synthPools {
		if st.lastRouted.Load() < cutoff {
			idle = append(idle, code)
		}
	}
	m.synthMu.RUnlock()
	if len(idle) == 0 {
		return 0
	}

	evicted := 0
	m.poolMu.Lock()
	m.synthMu.Lock()
	for _, code := range idle {
		// Re-check under the write locks: a message may have routed to it
		// (bumping lastRouted) since the read-locked scan above, or
		// Reconfigure may have promoted/removed it in the meantime.
		st, tracked := m.synthPools[code]
		if !tracked || st.lastRouted.Load() >= cutoff {
			continue
		}
		if p, ok := m.pools[code]; ok {
			p.Stop()
			delete(m.pools, code)
			evicted++
		}
		delete(m.synthPools, code)
	}
	m.synthMu.Unlock()
	m.poolMu.Unlock()
	return evicted
}

// isDefaultPoolCode reports whether code names a fallback pool — the global
// DEFAULT-POOL or a per-client {identifier}-DEFAULT-POOL.
//
// A suffix test is the ONLY safe structural read of a composed pool code.
// The scheduler composes {clientIdentifier}-{poolCode} and both halves may
// contain hyphens, so the string can never be split back into its parts; the
// suffix is unambiguous only because the literal is fixed.
func isDefaultPoolCode(code string) bool {
	return code == defaultPoolCode || strings.HasSuffix(code, "-"+defaultPoolCode)
}

// isSynthPoolCode reports whether code names a per-client fallback pool —
// eligible for on-demand synthesis (ensureFallbackPool) and, if it is one,
// for idle eviction (EvictIdleSynthPools). Excludes the global DEFAULT-POOL
// itself, which Reconfigure always ensures explicitly and is therefore never
// synthesised or evicted.
func isSynthPoolCode(code string) bool {
	return code != defaultPoolCode && isDefaultPoolCode(code)
}

// malformedRoutingReason reports why msg is malformed under strict routing
// (FC_ROUTER_STRICT_ROUTING; see Manager.SetStrictRouting), or "" if it's
// well-formed. Checked once per message at route time, before pool
// resolution — every condition here is exactly the case non-strict routing
// papers over with a fallback (DEFAULT-POOL, DefaultDispatchMode, the shared
// "" group), so under strict routing none of them may be silently repaired.
func malformedRoutingReason(msg *common.Message) string {
	switch {
	case msg.PoolCode == "":
		return "empty pool_code"
	case msg.DispatchMode == "":
		return "empty dispatch_mode"
	case msg.DispatchMode.RequiresOrdering() && msg.GroupID() == "":
		return "ordered dispatch_mode with no message_group_id"
	default:
		return ""
	}
}

// runConsumer is the per-consumer poll loop. ctx is the consumer's POLL
// context: cancelling it stops intake, while messages already routed continue
// under rc.workCtx. It pauses when the pools this queue feeds are at capacity
// to avoid a hot poll-defer loop, polls up to 10, routes the batch, and paces
// itself by batch fullness.
func (m *Manager) runConsumer(ctx context.Context, rc *runningConsumer) {
	defer m.wg.Done()
	const maxPoll = 10
	wasFull := false
	var fullSince, nextFullWarn time.Time
	for {
		if ctx.Err() != nil {
			return
		}
		// Backpressure: if the pools this queue feeds are full, wait rather than
		// poll. Surface the transition into full as a PoolCapacity warning (once
		// per full period, not every tick, to avoid flooding /warnings), then
		// re-warn on a slow interval for as long as it lasts.
		if !m.hasCapacityFor(rc) {
			now := time.Now()
			switch {
			case !wasFull:
				wasFull = true
				fullSince = now
				nextFullWarn = now.Add(capacityStuckWarnInterval)
				if w := m.warnings.Load(); w != nil {
					w.Add(WarningCategoryPoolCapacity, WarningWarning,
						fmt.Sprintf("destination pools at capacity; pausing %s", rc.consumer.Identifier()), "router")
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
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
			continue
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
		msgs, err := rc.consumer.Poll(pollCtx, maxPoll)
		cancelPoll()
		rc.pollsReturned.Add(1)
		if err != nil {
			if ctx.Err() != nil {
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
		rc.lastPoll.Store(time.Now().UnixNano())

		if len(msgs) == 0 {
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

		// Full batch → re-poll immediately (more likely waiting). Partial →
		// brief pause (queue draining).
		if len(msgs) < maxPoll {
			select {
			case <-ctx.Done():
				return
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
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

// hasCapacityFor reports whether this consumer should keep polling: whether the
// pools its own last batch fed still have room in their pre-dispatch buffers.
//
// It used to ask whether ANY pool in the process had room, which meant a single
// idle pool kept every consumer polling — messages for a saturated pool were
// fetched and immediately bounced (a NACK that SQS ignores outright), burning
// round-trips and receive counts to no end. Judging a queue by the pools it
// actually feeds makes the pause mean something.
//
// A queue feeding several pools pauses when ANY of them is full: a batch cannot
// be split, so the alternative is fetching messages we know we would bounce.
// The pause is 2s and draining does not depend on polling, so this cannot
// deadlock — the pool empties and the queue resumes.
func (m *Manager) hasCapacityFor(rc *runningConsumer) bool {
	m.poolMu.RLock()
	defer m.poolMu.RUnlock()
	if len(m.pools) == 0 {
		return false // nothing to route to
	}
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
		if p.QueueSize() >= p.queueCapacity() {
			return false
		}
	}
	return true
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

// beginPoolDrain registers a pool Reconfigure just removed as draining (so
// ReleaseParkedGroups and the operator-facing traversal accessors —
// AllPools — keep sweeping/showing it until it finishes; see drainingPools'
// field doc comment) and starts its asynchronous drain. The pool has
// already left m.pools by the time this is called (caller holds poolMu over
// that removal), so Reconfigure never blocks on the drain completing.
func (m *Manager) beginPoolDrain(code string, p *Pool) {
	m.drainingMu.Lock()
	m.drainingPools[code] = p
	m.drainingMu.Unlock()
	p.Drain(func() {
		m.drainingMu.Lock()
		delete(m.drainingPools, code)
		m.drainingMu.Unlock()
	})
}

// Reconfigure applies a new RouterConfig: reconciles pools (by code) and
// consumers (by queue name), starting/stopping/updating as needed. A
// DEFAULT-POOL is always ensured. Hot-reloadable.
func (m *Manager) Reconfigure(ctx context.Context, cfg common.RouterConfig) error {
	// One reconcile at a time. The config watcher and POST /config/reload can
	// both land here, and Shutdown (leadership loss) takes this too, so those
	// never interleave. This is an admission lock only — poolMu and consumerMu
	// are what routing and ack/nack contend on, and neither is now held across
	// a broker connect.
	m.reconfigureMu.Lock()
	defer m.reconfigureMu.Unlock()

	wantPools := make(map[string]common.PoolConfig, len(cfg.ProcessingPools)+1)
	for _, p := range cfg.ProcessingPools {
		wantPools[p.Code] = p
	}
	if _, ok := wantPools[defaultPoolCode]; !ok {
		wantPools[defaultPoolCode] = common.PoolConfig{Code: defaultPoolCode, Concurrency: defaultPoolConcurrency}
	}
	wantQueues := make(map[string]common.QueueConfig, len(cfg.Queues))
	for _, q := range cfg.Queues {
		wantQueues[q.Name] = q
	}

	// Pools and consumers are reconciled under their own locks, one after the
	// other — never nested — so a consumer being rebuilt cannot hold up a
	// routing lookup.
	m.poolMu.Lock()
	// Pools: stop removed, update existing, start new. (Pools are passive —
	// stopping just flips the flag so in-flight submits NACK.)
	for code, p := range m.pools {
		if _, ok := wantPools[code]; !ok {
			// X-11: a removal drains, it does not flush. The pool leaves the
			// routing map right here (nothing routes to it again after this
			// point), but its buffer keeps draining in the background — see
			// beginPoolDrain / Pool.Drain.
			slog.Info("manager: removing pool; draining its buffer in the background", "code", code)
			delete(m.pools, code)
			m.forgetSynthPool(code)
			m.beginPoolDrain(code, p)
		}
	}
	for code, pc := range wantPools {
		// Config always wins: a code config now defines is never
		// eviction-eligible, whether it was previously synthesised or is
		// brand new — see EvictIdleSynthPools.
		m.forgetSynthPool(code)
		if p, ok := m.pools[code]; ok {
			rate := uint32(0)
			if pc.RateLimitPerMinute != nil {
				rate = *pc.RateLimitPerMinute
			}
			p.SetRateLimit(rate)
			if pc.Concurrency != 0 {
				p.UpdateConcurrency(pc.Concurrency)
			}
			continue
		}
		np := NewPool(pc, m.mediator, m.tracker, m.resolveConsumer)
		np.SetSettledReporter(m.settledReporterRef())
		m.pools[code] = np
	}
	m.poolMu.Unlock()

	// Consumers: deregister removed/changed and note the missing ones, then do
	// the connection work — tearing down and building both talk to the broker —
	// outside the lock. Every ack and nack in the process resolves its consumer
	// under consumerMu, so it must not be held across a connect.
	// A queue config change (URI/connections/visibility) restarts that consumer.
	m.consumerMu.Lock()
	var stale []*runningConsumer
	for name, rc := range m.consumers {
		if wq, ok := wantQueues[name]; !ok || wq != rc.queueCfg {
			slog.Info("manager: detaching consumer (queue removed or changed)", "queue", name)
			stale = append(stale, rc)
			delete(m.consumers, name)
			delete(m.queues, name)
		}
	}
	missing := make([]common.QueueConfig, 0, len(wantQueues))
	for name, qc := range wantQueues {
		if _, ok := m.consumers[name]; !ok {
			missing = append(missing, qc)
		}
	}
	m.consumerMu.Unlock()

	// X-11 / R-26/R-49: stop POLLING immediately, never cancel workCtx here.
	// stopPoll ends the poll loop only (runningConsumer's doc comment); any
	// delivery already in flight, and any ordered-group drainer, keeps
	// running under workCtx to its own conclusion — neither is touched by
	// this consumer leaving m.consumers/m.queues. The consumer itself stays
	// addressable for ack/nack via resolveConsumer's fallback until
	// retireDetachedConsumers (reaper tick) finds nothing left referencing
	// its queue and finishes the teardown (cancel + consumer.Stop()). For a
	// CHANGED queue, the loop below already builds and starts polling a
	// replacement under the same name — see missing's computation just
	// above, which runs after this entry was deleted from m.consumers.
	if len(stale) > 0 {
		now := time.Now()
		m.detachMu.Lock()
		for _, rc := range stale {
			rc.stopPoll()
			rc.detachedAt = now
			m.detaching = append(m.detaching, rc)
		}
		m.detachMu.Unlock()
	}
	for _, qc := range missing {
		// ctx bounds the CONNECT only. The consumer itself lives under the
		// manager's root — see the field comment: a caller's context is not a
		// lifetime.
		consumer, err := queue.NewConsumer(ctx, qc)
		if err != nil {
			return fmt.Errorf("build consumer for queue %s: %w", qc.Name, err)
		}
		rc, pollCtx := newRunningConsumer(m.consumerRoot(), consumer, qc)

		m.consumerMu.Lock()
		if _, taken := m.consumers[qc.Name]; taken {
			// The stalled-consumer watchdog respawned this queue while we were
			// connecting. It holds the live entry; discard ours.
			m.consumerMu.Unlock()
			rc.cancel()
			consumer.Stop()
			continue
		}
		m.consumers[qc.Name] = rc
		m.queues[qc.Name] = qc
		m.consumerMu.Unlock()

		m.wg.Add(1)
		go m.runConsumer(pollCtx, rc)
	}
	return nil
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

	m.consumerMu.Lock()
	for _, rc := range m.consumers {
		rc.cancel()
		rc.consumer.Stop()
	}
	m.consumers = make(map[string]*runningConsumer)
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

	m.poolMu.Lock()
	for _, p := range m.pools {
		p.Stop()
	}
	m.pools = make(map[string]*Pool)
	m.poolMu.Unlock()

	// Same hard-teardown reasoning as detaching above: a pool mid-background
	// -drain (Pool.Drain) gets force-stopped now rather than left to finish
	// on its own goroutine — Stop is idempotent, so this is safe even if
	// that goroutine is mid-flight and calls Stop again moments later.
	m.drainingMu.Lock()
	for _, p := range m.drainingPools {
		p.Stop()
	}
	m.drainingPools = make(map[string]*Pool)
	m.drainingMu.Unlock()

	// Same "not necessarily terminal" reasoning as m.pools above: a later
	// regain re-synthesises fallback pools on demand, into a clean map
	// rather than one carrying entries for pools that no longer exist.
	m.synthMu.Lock()
	m.synthPools = make(map[string]*synthPoolState)
	m.synthMu.Unlock()

	done := make(chan struct{})
	go func() { m.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// RestartStalledConsumers re-spawns any consumer whose poll loop has not
// completed a poll within threshold — a wedged loop (stuck inside
// consumer.Poll) leaves its lastPoll stale. The replacement is built first and
// swapped in, and only then is the stalled one retired, so a rebuild that
// fails or hangs can never leave the queue with no consumer at all. Returns
// the number restarted.
//
// Runs synchronously on the watchdog tick and from that single goroutine only
// (which is what lets restartAttempts go unlocked), so every step inside the
// loop has to be bounded — see consumerRebuildTimeout.
func (m *Manager) RestartStalledConsumers(ctx context.Context, threshold time.Duration) int {
	if threshold <= 0 {
		return 0
	}
	if m.pollingStopped.Load() {
		// Draining. Every poll loop is stopped on purpose and every lastPoll is
		// therefore stale — restarting here would re-open intake mid-shutdown.
		return 0
	}
	cutoff := time.Now().Add(-threshold).UnixNano()

	type candidate struct {
		name string
		qc   common.QueueConfig
		old  *runningConsumer
	}
	// pollState turns "stalled" into a cause. See runningConsumer.pollsStarted.
	pollState := func(rc *runningConsumer) string {
		started, returned := rc.pollsStarted.Load(), rc.pollsReturned.Load()
		switch {
		case started == 0:
			return "never polled (loop never reached consumer.Poll)"
		case started > returned:
			return "inside consumer.Poll (poll is hung)"
		default:
			return "between polls (loop is not calling consumer.Poll)"
		}
	}
	m.consumerMu.RLock()
	var stalled []candidate
	for name, rc := range m.consumers {
		if lp := rc.lastPoll.Load(); lp != 0 && lp < cutoff {
			stalled = append(stalled, candidate{name: name, qc: rc.queueCfg, old: rc})
		}
	}
	m.consumerMu.RUnlock()

	// Clear restart-attempt counters for consumers that have genuinely
	// recovered, so a transient stall doesn't escalate a later, unrelated one.
	//
	// "Recovered" has to mean QUIET FOR A WHILE, not merely "not stalled at
	// this instant". Restarting a consumer stamps its lastPoll fresh, so on the
	// next tick it is never stalled — not because it recovered but because it
	// was just rebuilt. Clearing on that basis reset the count every single
	// cycle, which is why a consumer restarted every 90 seconds for hours
	// reported "attempt 1" every time and could never reach the CRITICAL
	// escalation this counter exists to drive. A consumer that manages one poll
	// between restarts is flapping, not healthy, and must still accumulate.
	//
	// The window is derived from the caller's threshold so it scales with it:
	// the restart cadence is at most threshold plus a couple of watchdog ticks,
	// so 3x threshold is comfortably longer than a loop yet short enough that a
	// one-off stall is forgotten.
	recoveryWindow := 3 * threshold
	stalledSet := make(map[string]struct{}, len(stalled))
	for _, c := range stalled {
		stalledSet[c.name] = struct{}{}
	}
	now := time.Now()
	for name, rec := range m.restartAttempts {
		if _, stillStalled := stalledSet[name]; stillStalled {
			continue
		}
		if now.Sub(rec.last) <= recoveryWindow {
			continue
		}
		delete(m.restartAttempts, name)
	}

	if len(stalled) == 0 {
		return 0
	}

	restarted := 0
	swept := 0
	for _, c := range stalled {
		attempts := m.restartAttempts[c.name].attempts
		// Escalate to CRITICAL once a consumer keeps stalling across many
		// restarts (Critical after 10 attempts).
		severity := WarningWarning
		if attempts >= consumerRestartCriticalAfter {
			severity = WarningCritical
		}
		cause := pollState(c.old)
		if w := m.warnings.Load(); w != nil {
			w.Add(WarningCategoryConsumerHealth, severity,
				fmt.Sprintf("Consumer %s is stalled (%s), restart attempt %d", c.name, cause, attempts+1),
				"router")
		}
		slog.Warn("stalled consumer detected, attempting restart",
			"queue", c.name, "attempt", attempts+1, "stalled_threshold", threshold,
			"cause", cause,
			"polls_started", c.old.pollsStarted.Load(),
			"polls_returned", c.old.pollsReturned.Load())

		// Brief pause before reconnecting — avoids a thundering herd when
		// several consumers stall together. Only BETWEEN consumers: delaying
		// ahead of the first buys nothing and just makes the sweep (which runs
		// inside the watchdog tick) longer for the common single-queue case.
		// Abort cleanly on shutdown.
		if swept > 0 {
			select {
			case <-ctx.Done():
				return restarted
			case <-time.After(m.restartDelay):
			}
		}
		swept++

		// Build the replacement BEFORE detaching the old consumer, under a
		// bounded context.
		//
		// The old order tore down first and built second, so a build that
		// failed or hung left the queue with NO consumer at all — a stalled
		// consumer silently downgraded into an unconsumed queue. On a failure
		// that was recoverable only if the cause was transient (the dead entry
		// stays in the map, stale, and is retried next tick); on a hang it was
		// not recoverable at all, because this runs synchronously in the
		// watchdog's tick and took the whole loop with it. Building first makes
		// a failed rebuild leave things exactly as it found them.
		buildCtx, cancelBuild := context.WithTimeout(ctx, m.rebuildTimeout)
		consumer, err := queue.NewConsumer(buildCtx, c.qc)
		cancelBuild()
		if err != nil {
			// Count the failure. A rebuild that keeps failing leaves the
			// consumer stalled, so it stays in the stalled set and its counter
			// survives the recovery sweep above — which is what lets a queue
			// that can never be rebuilt escalate to CRITICAL instead of
			// re-reporting attempt 1 for ever.
			slog.Error("failed to rebuild stalled consumer; leaving the existing entry in place",
				"queue", c.name, "attempt", m.bumpRestart(c.name), "err", err)
			continue
		}
		rc, pollCtx := newRunningConsumer(m.consumerRoot(), consumer, c.qc)

		m.consumerMu.Lock()
		// Only replace if the entry is still the one we found stalled — a
		// concurrent Reconfigure may have already swapped or removed it.
		cur, ok := m.consumers[c.name]
		if !ok || cur != c.old {
			m.consumerMu.Unlock()
			rc.cancel()
			consumer.Stop()
			continue
		}
		// Still stalled? Detection ran on a snapshot taken before the herd
		// pause, before every earlier consumer in this sweep, and before a
		// build that may have taken seconds. A consumer that completed a poll
		// in that window has recovered, and replacing it would be pure harm:
		// the teardown cancels workCtx, which parks its ordered-group drainers.
		// Checked here, under the same lock as the identity check, so the whole
		// window is covered rather than just part of it.
		if lp := c.old.lastPoll.Load(); lp >= time.Now().Add(-threshold).UnixNano() {
			m.consumerMu.Unlock()
			rc.cancel()
			consumer.Stop()
			slog.Info("stalled consumer recovered before its restart; leaving it alone",
				"queue", c.name)
			continue
		}
		m.consumers[c.name] = rc
		m.consumerMu.Unlock()

		// R-26/R-49: detach, don't abort — the same machinery
		// Reconfigure's consumer-removal/change path uses (see the
		// detaching field's doc comment). "Stalled" here is a judgement
		// about the POLL loop (lastPoll went stale), and says nothing about
		// whether the consumer is currently carrying an in-flight delivery
		// or holding buffered ordered-group messages. The 2026-09-01
		// incident this watchdog exists to fix (a queue backed up to 27k
		// messages over 51 minutes of restart-every-90s) was made WORSE by
		// the old cancel-on-restart behaviour, not better: perfectly
		// healthy in-flight work was killed on every single restart,
		// alongside the one wedged poll loop that was the actual problem.
		// stopPoll ends only the poll loop; workCtx — and whatever it is
		// carrying — is left alone. The old consumer moves to detaching and
		// stays addressable for ack/nack via resolveConsumer's fallback
		// until retireDetachedConsumers (reaper tick) finds nothing left
		// referencing its queue. Even in the case this watchdog is FOR — a
		// genuinely dead broker connection — an ack attempted through it
		// simply fails and the message redelivers on the broker's own
		// clock, which is strictly better than cancelling a delivery that
		// might otherwise have succeeded.
		c.old.stopPoll()
		m.detachMu.Lock()
		c.old.detachedAt = time.Now()
		m.detaching = append(m.detaching, c.old)
		m.detachMu.Unlock()

		m.wg.Add(1)
		go m.runConsumer(pollCtx, rc)
		m.bumpRestart(c.name)
		restarted++
	}
	return restarted
}

// ReleaseParkedGroups sweeps every pool for message groups left buffered with
// no drainer for longer than minAge and hands them back to the broker. Wired
// onto the router's reaper tick. Returns the number of messages released.
//
// See Pool.ReleaseParkedGroups for why an unclaimed park cannot be left to
// resolve itself.
func (m *Manager) ReleaseParkedGroups(ctx context.Context, minAge time.Duration) int {
	// Sweeps AllPools — actively routed pools AND anything still draining
	// after a removal (drainingPools) — not just m.pools. Without the
	// latter, a group that parks (ctx cancellation) inside a draining pool
	// would never be found: the pool already left m.pools, so nothing would
	// ever release it back to the broker, QueueSize would never reach zero,
	// and Pool.Drain's background goroutine would sit out its full budget
	// for no reason. This IS Drain's real safety net (see poolDrainBudget's
	// doc comment) — this sweep should retire a genuinely stuck group long
	// before that budget ever matters.
	released := 0
	for _, p := range m.AllPools() {
		released += p.ReleaseParkedGroups(ctx, minAge)
	}
	return released
}

// retireDetachedConsumers finishes the teardown of any consumer Reconfigure
// or RestartStalledConsumers detached (queue removed/changed, or a wedged
// poll loop replaced — see the detaching field's doc comment) once nothing
// the OLD consumer could have polled still references its queue.
// "Nothing references it" is judged by the in-flight tracker
// (InFlightTracker.CountForQueueBefore, cutoff = the consumer's own
// detachedAt) rather than by asking every pool to introspect its buffers
// directly: every message a pool is holding — buffered, mid-backoff, or
// actively mediating — has a live tracker entry for exactly as long as it
// does, so a zero count of entries STARTED BEFORE the detach IS "drained"
// for this purpose.
//
// The cutoff is the whole point, not a refinement: plain CountForQueue
// counts every tracked entry for the queue identifier, including the
// REPLACEMENT consumer's own healthy ongoing traffic — which shares the
// identifier (see resolveConsumer's doc comment) and can arrive
// continuously the moment the replacement starts polling. Without the
// cutoff, a queue that stays busy after a restart or a config change would
// never let its old consumer retire: the count would never reach zero, and
// the stale runningConsumer — and whatever broker connection it holds open
// — would leak for the life of the process. Every entry the replacement
// creates has StartedAt >= detachedAt (it cannot poll before it exists), so
// the cutoff excludes it categorically, not by timing luck.
//
// When the tracker is nil (in-flight tracking disabled — NewManager's
// tracker parameter, never true for a production router: Server always
// wires one), there is no signal to wait on at all, so a detached consumer
// is retired immediately rather than lingering forever with no way to know
// it is safe.
//
// Wired onto the router's reaper tick (see Server.reapInFlight). Returns the
// number retired.
func (m *Manager) retireDetachedConsumers() int {
	m.detachMu.Lock()
	var kept []*runningConsumer
	retired := 0
	for _, rc := range m.detaching {
		if m.tracker != nil && m.tracker.CountForQueueBefore(rc.consumer.Identifier(), rc.detachedAt) > 0 {
			kept = append(kept, rc)
			continue
		}
		rc.cancel()
		rc.consumer.Stop()
		retired++
		slog.Info("manager: retired detached consumer; nothing pre-dating its detach still references its queue",
			"queue", rc.consumer.Identifier())
	}
	m.detaching = kept
	m.detachMu.Unlock()
	return retired
}

// bumpRestart records one more restart attempt for a queue and returns the new
// count. Called only from RestartStalledConsumers (single goroutine).
func (m *Manager) bumpRestart(name string) int {
	rec := m.restartAttempts[name]
	rec.attempts++
	rec.last = time.Now()
	m.restartAttempts[name] = rec
	return rec.attempts
}

// PoolCount returns the count of running pools (for /health or /metrics).
func (m *Manager) PoolCount() int {
	m.poolMu.RLock()
	defer m.poolMu.RUnlock()
	return len(m.pools)
}
