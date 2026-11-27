package router

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// synthPoolState is the idle tracker for one synthesised fallback pool. Only
// lastRouted is mutated after creation, and it's atomic so the routing hot
// path (touchSynthPool) never blocks behind the eviction sweep.
type synthPoolState struct {
	lastRouted atomic.Int64 // unix-nano of the most recently routed message
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
//
// Ordered messages are not submitted as they are routed. Each group's
// members in the batch are held until the whole batch has been routed and
// are then admitted together (Pool.submitRun), so the group's drainer never
// sees the head of a batch without the members behind it. Submitted one at a
// time, the head could be attempted and fail before its successors arrived;
// the release or the BLOCK_ON_ERROR ack then settled a group that did not
// yet hold them, and the successors started a fresh drainer and were
// delivered behind a head the broker would redeliver, or past a failure
// meant to block them.
func (m *Manager) route(ctx context.Context, msgs []common.QueuedMessage, source queue.Consumer) []string {
	if len(msgs) == 0 {
		return nil
	}
	batchID := strconv.FormatUint(m.batchCounter.Add(1), 10)
	var fed []string
	var runs []groupRun

	for i := range msgs {
		msg := msgs[i]
		msg.BatchID = batchID
		if code := m.routeOne(ctx, msg, source, &runs); code != "" && !slices.Contains(fed, code) {
			fed = append(fed, code)
		}
	}
	for _, run := range runs {
		m.admitRun(ctx, run, source)
	}
	return fed
}

// groupRun is the members of one ordered group in one poll batch, in batch
// order, bound for one pool.
type groupRun struct {
	pool  *Pool
	group string
	msgs  []common.QueuedMessage
}

// holdForRun adds msg to its group's run in runs, starting the run if this is
// the group's first member in the batch.
func holdForRun(runs []groupRun, pool *Pool, msg common.QueuedMessage) []groupRun {
	group := msg.Message.GroupID()
	for i := range runs {
		if runs[i].pool == pool && runs[i].group == group {
			runs[i].msgs = append(runs[i].msgs, msg)
			return runs
		}
	}
	return append(runs, groupRun{pool: pool, group: group, msgs: []common.QueuedMessage{msg}})
}

// admitRun submits one group's run. It is the panic boundary for the run, as
// routeOne is for a single message: a panic is logged, and every member's
// tracker entry is released and the member handed back to the broker. Every
// member got here as a fresh registration, so every one is this run's to
// hand back.
func (m *Manager) admitRun(ctx context.Context, run groupRun, source queue.Consumer) {
	defer func() {
		if r := recover(); r != nil {
			logRecovered("manager.route", r, messageAttrs(run.msgs[0].Message.PoolCode, run.msgs[0])...)
			for _, msg := range run.msgs {
				if m.tracker != nil {
					m.tracker.Remove(msg.Message.ID, msg.BrokerMessageID)
				}
				// At-least-once, as in routeOne: a member the pool had already
				// buffered before the panic is delivered and redelivered.
				safely("manager.route nack", func() {
					if err := source.Nack(ctx, msg.ReceiptHandle, new(siblingNackDelaySeconds)); err != nil {
						slog.Warn("nack (after routing panic) failed", logKeyMessageID, msg.Message.ID, "err", err)
					}
				}, logKeyMessageID, msg.Message.ID)
			}
		}
	}()
	run.pool.submitRun(ctx, run.group, run.msgs)
}

// routeOne routes one polled message: dedup against the in-flight tracker,
// the strict-routing check, then submission to its pool. An ordered message
// is not submitted here but added to its group's run in runs, which route
// admits once the whole batch has been routed. Returns the code of the pool
// it was submitted or held for, or "" when it was neither.
//
// It is a panic boundary for this one message. A panic here would otherwise
// unwind the consumer's poll loop and take the process down with the rest of
// the batch in hand; instead it is logged with its stack, the message's
// tracker entry is released and the message handed back to the broker, and
// the next message in the batch is routed as usual.
func (m *Manager) routeOne(ctx context.Context, msg common.QueuedMessage, source queue.Consumer, runs *[]groupRun) (fedCode string) {
	// ownsEntry is whether this copy registered the message's in-flight
	// entry. Only then is the message this copy's to hand back: a panic while
	// handling a redelivery or an external requeue must leave the owning
	// copy's entry (Remove is by message id) and its receipt handle alone.
	ownsEntry := m.tracker == nil
	defer func() {
		if r := recover(); r != nil {
			logRecovered("manager.route", r, messageAttrs(msg.Message.PoolCode, msg)...)
			fedCode = ""
			if !ownsEntry {
				return
			}
			if m.tracker != nil {
				m.tracker.Remove(msg.Message.ID, msg.BrokerMessageID)
			}
			// At-least-once: if the panic came after the pool had already
			// buffered the message, this nack makes a second copy. The
			// platform's /process claim delivers a dispatch job once.
			safely("manager.route nack", func() {
				if err := source.Nack(ctx, msg.ReceiptHandle, new(siblingNackDelaySeconds)); err != nil {
					slog.Warn("nack (after routing panic) failed", logKeyMessageID, msg.Message.ID, "err", err)
				}
			}, logKeyMessageID, msg.Message.ID)
		}
	}()

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
			return ""

		case RegisterExternalRequeue:
			// The same application message ID is in flight under a
			// DIFFERENT broker id: an external process requeued a message
			// it thought was lost while the original still owns the work.
			// ACK this copy so the duplicate is DELETED from the broker —
			// otherwise it redelivers forever. The owner's entry (and
			// receipt handle) was deliberately left untouched.
			slog.Info("external requeue detected; ACKing duplicate", "message_id", msg.Message.ID, "queue", source.Identifier())
			// Urgent: this ack runs on the consumer's poll loop, and a batching
			// broker (SQS) would otherwise hold it for its delete linger (up to
			// 5 s) — so a duplicate storm would stall this queue's intake for
			// that long per duplicate. See pollLoopAck.
			if err := source.Ack(pollLoopAck(ctx), msg.ReceiptHandle, msg.BrokerMessageID); err != nil {
				slog.Warn("ack (external requeue) failed", "message_id", msg.Message.ID, "err", err)
			}
			return ""

		case RegisterNew:
			// This copy owns the pipeline; fall through to submit.
			ownsEntry = true
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
			if err := source.Ack(pollLoopAck(ctx), msg.ReceiptHandle, msg.BrokerMessageID); err != nil {
				slog.Warn("ack (strict routing malformed) failed", "message_id", msg.Message.ID, "err", err)
			}
			return ""
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
		if err := source.Nack(ctx, msg.ReceiptHandle, new(uint32(5))); err != nil {
			slog.Warn("nack (no pool) failed", "message_id", msg.Message.ID, "err", err)
		}
		return ""
	}
	if isOrdered(msg) {
		*runs = holdForRun(*runs, pool, msg)
		return pool.Identifier()
	}
	pool.submit(ctx, msg)
	return pool.Identifier()
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
	m.wirePool(p)
	m.pools[code] = p
	m.trackSynthPool(code)
	slog.Info("synthesised per-client fallback pool",
		"pool_code", code, "concurrency", defaultPoolConcurrency)
	// A new pool existing at all is itself a capacity event: a consumer
	// parked in awaitCapacity because its last batch's dests pointed at a
	// now-gone pool re-evaluates on the fallback ("any pool has room") path
	// and would otherwise wait for an unrelated pool's queue to drain.
	m.capacityGate.signal()
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

	var stopping []*Pool
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
			stopping = append(stopping, p)
			delete(m.pools, code)
		}
		delete(m.synthPools, code)
	}
	m.synthMu.Unlock()
	m.poolMu.Unlock()
	// Stopped outside the locks: Stop nacks anything still buffered through
	// the broker, and routing must not wait on that.
	for _, p := range stopping {
		p.Stop()
	}
	return len(stopping)
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
// pollLoopAck marks an ack that routeOne issues itself — on the consumer's poll
// goroutine, before the message ever reaches a pool — as urgent. Ack blocks
// until the broker has answered, and a batching broker holds a non-urgent ack
// for its linger (SQS: up to 5 s, waiting for a full batch). Nothing else is
// polled from this queue meanwhile, so without the mark a storm of external
// requeues or malformed messages throttles the queue's intake to about one
// message per linger.
func pollLoopAck(ctx context.Context) context.Context { return queue.WithUrgentAck(ctx) }

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
