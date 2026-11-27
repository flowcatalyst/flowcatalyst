package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

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
		m.wirePool(np)
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
			delete(m.consumersByID, rc.consumer.Identifier())
			delete(m.queues, name)
		}
	}
	// A queue that left the config, or whose config changed, is no longer
	// "missing" as it was; the loop below re-judges it against the broker.
	for name, qc := range m.missingQueues {
		if wq, ok := wantQueues[name]; !ok || wq != qc {
			delete(m.missingQueues, name)
			delete(m.missingSince, name)
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
	// One queue whose consumer cannot be built (a broker that is down, a bad
	// URI) must not cost every queue after it: the rest are still built, and
	// the failures are reported together at the end (as Java's RouterServer
	// does). A failed queue is simply absent from m.consumers, so the next
	// Reconfigure — the watcher retries a failed apply — builds it again.
	failed := m.startConsumers(ctx, missing)
	// A reconfigure can add, remove, resize or repoint any pool a parked
	// consumer's next hasCapacityFor check depends on — wake every waiter
	// so it re-evaluates against the new topology instead of sitting out
	// however long is left on a queueDec that may never come from a pool
	// that no longer exists.
	m.capacityGate.signal()
	if len(failed) > 0 {
		err := &ReconfigureError{Failed: failed}
		// Running with less than the configuration asks for is an
		// operator-visible condition, not a log line: those queues are not
		// being consumed at all.
		if w := m.warnings.Load(); w != nil {
			w.Add(WarningCategoryConfiguration, WarningError, err.Error(), "router")
		}
		return err
	}
	return nil
}

// startConsumers builds and starts a consumer for each of qcs. A queue whose
// consumer cannot be built is returned as a failure and the rest are still
// started. A queue the broker has no queue for yet (queue.ErrQueueMissing) is
// not a failure: it is remembered in missingQueues, logged once per streak and
// otherwise left alone until RecheckMissingQueues (or the next Reconfigure)
// finds it.
func (m *Manager) startConsumers(ctx context.Context, qcs []common.QueueConfig) []QueueFailure {
	var failed []QueueFailure
	for _, qc := range qcs {
		// ctx bounds the CONNECT only. The consumer itself lives under the
		// manager's root — see the field comment: a caller's context is not a
		// lifetime.
		consumer, err := buildConsumerSafely(ctx, qc)
		if err != nil {
			if errors.Is(err, queue.ErrQueueMissing) {
				m.noteQueueMissing(qc)
				continue
			}
			slog.Error("manager: could not build consumer; continuing with the other queues",
				logKeyQueue, qc.Name, "err", err)
			failed = append(failed, QueueFailure{Queue: qc.Name, Err: err})
			continue
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
		m.consumersByID[consumer.Identifier()] = rc
		m.queues[qc.Name] = qc
		wasMissing := false
		if _, ok := m.missingQueues[qc.Name]; ok {
			wasMissing = true
			delete(m.missingQueues, qc.Name)
			delete(m.missingSince, qc.Name)
		}
		m.consumerMu.Unlock()
		if wasMissing {
			slog.Info("manager: queue now exists; consuming it", logKeyQueue, qc.Name)
		}

		m.wg.Add(1)
		go m.runConsumer(pollCtx, rc)
	}
	return failed
}

// noteQueueMissing records qc as configured-but-absent and logs the one INFO
// line for the transition into that state; a recheck that still finds it
// missing is silent.
func (m *Manager) noteQueueMissing(qc common.QueueConfig) {
	m.consumerMu.Lock()
	if m.missingQueues == nil {
		m.missingQueues = make(map[string]common.QueueConfig)
	}
	_, already := m.missingQueues[qc.Name]
	m.missingQueues[qc.Name] = qc
	m.markMissingSinceLocked(qc.Name)
	m.consumerMu.Unlock()
	if !already {
		slog.Info("manager: queue does not exist yet; not consuming it (rechecked periodically)",
			logKeyQueue, qc.Name)
	}
}

// RecheckMissingQueues starts a consumer for every configured queue that was
// missing at the last look and has appeared since. The platform creates a
// dispatch queue on the first message published to it, which can happen long
// after the config naming it was applied — and an unchanged config is never
// re-applied, so without this a queue created later would not be consumed
// until the next config change. With nothing missing it does no work at all.
func (m *Manager) RecheckMissingQueues(ctx context.Context) {
	m.recheckMissing(ctx, func(time.Time) bool { return true })
}

// Pacing of the recheck on the config tick. The document lists a queue for
// every client and lane, but only the ones that have published exist, so most
// missing queues stay missing; probing all of them on every (now frequent)
// tick would be a broker call per unused queue per tick. A queue that has been
// missing for less than youngMissingQueueAge is the one a new client is about
// to publish to, so it is probed on every tick; every queue is probed once per
// fullMissingRecheckEvery, which is the cadence the router used for all of
// them before F29.
const (
	youngMissingQueueAge    = 10 * time.Minute
	fullMissingRecheckEvery = 5 * time.Minute
)

// RecheckMissingQueuesPaced is the recheck the config tick runs: the young
// missing queues now, all of them when fullMissingRecheckEvery has passed.
func (m *Manager) RecheckMissingQueuesPaced(ctx context.Context) {
	now := time.Now()
	m.consumerMu.Lock()
	full := now.Sub(m.lastFullRecheck) >= fullMissingRecheckEvery
	if full {
		m.lastFullRecheck = now
	}
	m.consumerMu.Unlock()
	m.recheckMissing(ctx, func(since time.Time) bool {
		return full || now.Sub(since) < youngMissingQueueAge
	})
}

// recheckMissing starts consumers for the missing queues whose first-missing
// time passes want.
func (m *Manager) recheckMissing(ctx context.Context, want func(since time.Time) bool) {
	m.consumerMu.Lock()
	qcs := make([]common.QueueConfig, 0, len(m.missingQueues))
	for name, qc := range m.missingQueues {
		if _, running := m.consumers[name]; running {
			continue
		}
		if want(m.missingSince[name]) {
			qcs = append(qcs, qc)
		}
	}
	m.consumerMu.Unlock()
	if len(qcs) == 0 {
		return
	}
	// Failures other than "missing" are logged by startConsumers; the next
	// recheck (or a Reconfigure) tries them again.
	m.startConsumers(ctx, qcs)
}

// markMissingSinceLocked stamps name's first-missing time, if it has none.
// consumerMu must be held.
func (m *Manager) markMissingSinceLocked(name string) {
	if m.missingSince == nil {
		m.missingSince = make(map[string]time.Time)
	}
	if _, ok := m.missingSince[name]; !ok {
		m.missingSince[name] = time.Now()
	}
}

// detachMissingConsumer retires a consumer whose queue disappeared from under
// it: it leaves the active maps and stops polling exactly as a queue removed by
// Reconfigure does (messages already in flight finish and can still ack or nack
// through the detaching list), and the queue goes back to the missing set so
// RecheckMissingQueues brings it back when it exists again.
func (m *Manager) detachMissingConsumer(rc *runningConsumer) {
	name := rc.queueCfg.Name
	m.consumerMu.Lock()
	if cur, ok := m.consumers[name]; !ok || cur != rc {
		m.consumerMu.Unlock()
		return
	}
	delete(m.consumers, name)
	delete(m.consumersByID, rc.consumer.Identifier())
	delete(m.queues, name)
	if m.missingQueues == nil {
		m.missingQueues = make(map[string]common.QueueConfig)
	}
	m.missingQueues[name] = rc.queueCfg
	m.markMissingSinceLocked(name)
	m.consumerMu.Unlock()

	rc.stopPoll()
	m.detachMu.Lock()
	rc.detachedAt = time.Now()
	m.detaching = append(m.detaching, rc)
	m.detachMu.Unlock()
	slog.Info("manager: queue no longer exists; stopped consuming it (rechecked periodically)", logKeyQueue, name)
}

// QueueFailure is one queue a Reconfigure could not start a consumer for.
type QueueFailure struct {
	Queue string
	Err   error
}

// ReconfigureError reports the queues a Reconfigure could not start. Every
// other part of the configuration (pools, the remaining queues) was applied;
// the caller retries the whole apply, which builds only what is missing.
type ReconfigureError struct {
	Failed []QueueFailure
}

func (e *ReconfigureError) Error() string {
	parts := make([]string, 0, len(e.Failed))
	for _, f := range e.Failed {
		parts = append(parts, fmt.Sprintf("%s (%v)", f.Queue, f.Err))
	}
	return fmt.Sprintf("router is running without %d configured queue(s): %s",
		len(e.Failed), strings.Join(parts, ", "))
}

// Unwrap exposes each queue's error to errors.Is / errors.As.
func (e *ReconfigureError) Unwrap() []error {
	errs := make([]error, 0, len(e.Failed))
	for _, f := range e.Failed {
		errs = append(errs, f.Err)
	}
	return errs
}

// buildConsumerSafely is queue.NewConsumer behind a panic boundary: a broker
// client that panics while connecting fails that one queue, like any other
// build error, rather than the whole reconfigure (or the process).
func buildConsumerSafely(ctx context.Context, qc common.QueueConfig) (c queue.Consumer, err error) {
	defer func() {
		if r := recover(); r != nil {
			logRecovered("queue.NewConsumer", r, logKeyQueue, qc.Name)
			c, err = nil, fmt.Errorf("consumer build panicked: %v", r)
		}
	}()
	return queue.NewConsumer(ctx, qc)
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
		consumer, err := buildConsumerSafely(buildCtx, c.qc)
		cancelBuild()
		if errors.Is(err, queue.ErrQueueMissing) {
			// The queue is gone, not the consumer wedged: retire it quietly.
			m.detachMissingConsumer(c.old)
			continue
		}
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
		m.consumersByID[consumer.Identifier()] = rc
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
