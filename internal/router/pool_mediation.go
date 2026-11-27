package router

import (
	"context"
	"runtime/pprof"
	"runtime/trace"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// beginMediating records a message as actively inside a worker and returns the
// key to end it with. Called at the top of processOne, paired with
// endMediating on exit — the pair is also what makes ActiveWorkers true, so
// neither half is optional.
func (p *Pool) beginMediating(qm common.QueuedMessage) uint64 {
	worker := p.workerSeq.Add(1)
	p.mediatingMu.Lock()
	p.mediating[worker] = MediatingEntry{
		MessageID:  qm.Message.ID,
		PoolCode:   p.cfg.Code,
		Group:      qm.Message.GroupID(),
		Queue:      qm.QueueIdentifier,
		Target:     qm.Message.MediationTarget,
		Attempts:   qm.Attempts,
		MediatedAt: time.Now(),
	}
	p.mediatingMu.Unlock()
	return worker
}

func (p *Pool) endMediating(worker uint64) {
	p.mediatingMu.Lock()
	delete(p.mediating, worker)
	p.mediatingMu.Unlock()
}

// BrokerAction is what a Disposition does to a message at the broker — one
// of the three broker actions the router's design supports (see
// docs/router-architecture.md, "Delivery outcomes"): acknowledge, retry in
// place, or release.
type BrokerAction int

const (
	// BrokerAck settles the message — it is gone from the broker, whether
	// because it succeeded or because it failed in a way retrying cannot fix.
	// processOne has already ACKed it (or, for a deduplicated copy, ACKed it
	// under its own receipt handle) by the time a Disposition carrying this
	// is returned; the caller has nothing further to do at the broker.
	BrokerAck BrokerAction = iota
	// BrokerRetry keeps the message in-flight; the caller re-dispatches after
	// Disposition.RetryAfter (front of the group for ordered, delayed
	// re-spawn for IMMEDIATE). Never touches the broker.
	BrokerRetry
	// BrokerRelease hands the message back to the broker — the TARGET is
	// unreachable (transport failure, 502/503/504, or an open breaker), so
	// nothing about the message is wrong and retrying it in-process would pin
	// it, and its whole group, in memory for the duration of an outage. This
	// is what makes "retry until the broker expires it" true: an in-pipeline
	// retry never returns the message, so the broker's expiry and DLQ can
	// never act on it. Disposition.RetryAfter (when non-zero) becomes the
	// requested redelivery delay.
	BrokerRelease
)

// GroupEffect is what a Disposition means for the REST of an ordered message
// group — meaningless for IMMEDIATE dispatch, which has no group buffer, and
// for BrokerRetry, which blocks the group on this same message until it
// resolves rather than doing anything to the others.
type GroupEffect int

const (
	// GroupContinue: the drainer moves on to the next buffered message (or
	// exits idle if there is none). Covers a success, a discarded failure
	// under NEXT_ON_ERROR/IMMEDIATE, and a discarded deduplicated copy.
	GroupContinue GroupEffect = iota
	// GroupBlock is BLOCK_ON_ERROR's defining behaviour: the head failed
	// terminally, so the drainer stops advancing and ACKs away everything
	// still buffered behind it rather than delivering successors past a
	// failure the mode exists to stop at.
	GroupBlock
	// GroupRelease: the drainer hands the whole group — this message plus
	// everything still buffered behind it — back to the broker. Releasing
	// only the head while successors stayed buffered would put the head
	// behind them on redelivery, reordering a group whose entire purpose is
	// ordering.
	GroupRelease
)

// DispositionMetric names which PoolMetricsCollector method a Disposition's
// outcome records, as DATA rather than a call — the reason DispositionOf can
// stay pure. recordMetric is the single place that turns this into an actual
// Record* call.
type DispositionMetric int

const (
	// MetricNone: nothing recorded — a deduplicated copy, a suppressed-flush
	// ACK, a circuit-open release (no delivery was attempted, so there is
	// nothing to measure), or a retry/release from a path with no mediation
	// outcome at all (a cancelled rate-limiter wait, a recovered panic).
	MetricNone DispositionMetric = iota
	MetricSuccess
	MetricFailure
	MetricTransient
	MetricRateLimited
)

// Disposition is processOne's pure verdict for a mediation outcome: what
// happens to THIS message at the broker (Action), what that means for the
// rest of an ordered group (Group), the metric to record, and the backoff to
// apply for a retry or the redelivery hint to carry on a release
// (RetryAfter).
//
// It replaces the old flat processResult enum, which encoded broker action
// and group effect in the SAME five values positionally — see
// docs/router-go-idiom-plan.md item 5 ("Restructure the outcome enum before
// it grows again"). DispositionOf is exported specifically so a test can
// call it directly: mediation_conformance_test.go used to be unable to
// assert message fate at all, because "the decision lives in an inline
// switch inside pool.go's delivery loop with nothing a test can call".
type Disposition struct {
	Action     BrokerAction
	Group      GroupEffect
	Metric     DispositionMetric
	RetryAfter time.Duration

	// budgetExhausted marks a BrokerRelease produced by retryOrRelease when
	// the in-pipeline retry budget (maxInPipelineAttempts) is spent, as
	// opposed to one produced directly by DispositionOf because the target is
	// unreachable. Unexported: it exists only so settleRetry can log the
	// distinct "budget exhausted" warning in one place: the mediation-outcome
	// path (DispositionOf) and the two paths that retry without ever
	// reaching a mediation outcome (a cancelled rate-limiter wait, a
	// recovered panic) all funnel through it.
	budgetExhausted bool
}

// maxInPipelineAttempts bounds how many times a message may be retried IN
// PLACE before it is handed back to the broker instead.
//
// It has to be bounded. An in-pipeline retry never returns the message, so
// while it loops the broker's expiry, redelivery count and DLQ can never act on
// it — and the stall detector deliberately leaves retrying entries alone, so
// nothing warns either. An endpoint answering 429 or `ack:false` for ever would
// otherwise pin the message, its whole ordered group, and its tracker entry
// (exempt from reaping) in memory for the life of the process, invisibly.
//
// On exhaustion the message is RELEASED, which is the same treatment an
// unreachable target gets and the only outcome that restores the broker's
// authority over it. The attempts spent here are on top of the mediator's own
// bounded burst inside each attempt.
const maxInPipelineAttempts uint = 10

const (
	// retryMinDelay / retryMaxDelay bound the in-pipeline backoff; panicRetryDelay
	// is the fixed backoff after a recovered panic.
	retryMinDelay   = 100 * time.Millisecond
	retryMaxDelay   = 5 * time.Minute
	panicRetryDelay = 10 * time.Second

	// Deferred (2xx + ack=false) gets its own backoff curve: the target is
	// healthy and answering cheap 200s, so recovery latency matters more
	// than politeness — shorter start, 1-minute ceiling.
	deferredMinDelay = 5 * time.Second
	deferredMaxDelay = time.Minute
)

// retryDelay computes the in-pipeline backoff before the next attempt:
// exponential in the attempt count (starting at retryMinDelay), with any
// server-requested delay (Retry-After on 429, the breaker reset on circuit-open,
// the 5xx retry hint) applied as a floor, capped at retryMaxDelay.
func retryDelay(attempts uint, outcomeDelaySec int) time.Duration {
	return backoffDelay(attempts, outcomeDelaySec, retryMinDelay, retryMaxDelay)
}

// deferredDelay is the backoff for 200+ack=false deferrals — same shape as
// retryDelay but on the deferred curve (5s start, 60s cap), still flooring at
// whatever delaySeconds the target requested.
func deferredDelay(attempts uint, outcomeDelaySec int) time.Duration {
	return backoffDelay(attempts, outcomeDelaySec, deferredMinDelay, deferredMaxDelay)
}

func backoffDelay(attempts uint, floorSec int, minDelay, maxDelay time.Duration) time.Duration {
	shift := min(attempts,
		// cap the shift so the bit-shift can't overflow
		12)
	d := minDelay << shift
	if floor := time.Duration(floorSec) * time.Second; d < floor {
		d = floor
	}
	if d > maxDelay {
		d = maxDelay
	}
	return d
}

// DispositionOf maps a mediation outcome to its Disposition. Pure: no I/O, no
// metrics, no ack/flush/tracker calls — everything needed is passed in, so a
// test can call it directly instead of simulating a broker and a group
// buffer to observe the decision.
//
// attempts is qm.Attempts, the count of PRIOR in-pipeline attempts: it gates
// maxInPipelineAttempts for the two retryable outcomes (429, ack=false),
// past which they convert to a release instead of retrying forever — see
// maxInPipelineAttempts' doc comment.
//
// mode is the message's DispatchMode: only BLOCK_ON_ERROR changes the Group
// effect of a discarded failure, from GroupContinue to GroupBlock.
//
// honoursDelayedReturn is whether the message's SOURCE broker actually
// enforces a Nack's delay before redelivering — R5 (owner ruling
// 2026-09-17, docs/spec/router-deferral-handback.md, second unit). It
// gates R1's hand-back for a delay-bearing MediationDeferred: SQS and
// Postgres both honour it (true), NATS does not (false, see
// queue.Consumer.HonoursDelayedReturn's doc comment). It is meaningless for
// every other outcome, which is why it is threaded through as a single bool
// rather than the whole Consumer — DispositionOf stays pure and testable
// without a queue.
func DispositionOf(outcome common.MediationOutcome, attempts uint, mode common.DispatchMode, honoursDelayedReturn bool) Disposition {
	switch outcome.Result {
	case common.MediationSuccess:
		return Disposition{Action: BrokerAck, Group: GroupContinue, Metric: MetricSuccess}

	case common.MediationErrorConfig:
		// Permanent ACK-drop: a 4xx, or (R-57) any 5xx the mediator has
		// already classified as "the app answered" rather than "unavailable"
		// (see mediator.go's status-code switch — every 5xx other than
		// 502/503/504 lands here). The mediator already recorded the breaker
		// success (reachable). BLOCK_ON_ERROR must stop the group at this
		// failure rather than deliver successors past it; every other mode
		// moves on to the next buffered message.
		group := GroupContinue
		if mode == common.DispatchBlockOnError {
			group = GroupBlock
		}
		return Disposition{Action: BrokerAck, Group: group, Metric: MetricFailure}

	case common.MediationErrorProcess:
		// Pre-classified by the mediator (R-57): reaching this case at all
		// means 502/503/504 — "never reached a working app", verdict AFTER
		// the mediator's bounded retry burst — so release rather than retry
		// in place. The status code is deliberately not inspected here: the
		// allowlist decision already happened once, in mediator.go, and a
		// second check here would just be a second place for it to drift
		// from the ruling.
		//
		// The release carries the outcome's own delay (30s from the
		// mediator). Owner decision #41 (2026-09-26): a release with no delay
		// makes the message visible again at once, so during an outage the
		// router hot-loops redeliveries against a target it already knows is
		// down, spending one SQS receive count toward the DLQ on every lap.
		return Disposition{Action: BrokerRelease, Group: GroupRelease, Metric: MetricTransient, RetryAfter: outcomeDelay(outcome)}

	case common.MediationErrorConnection:
		// Transport failure / unreachable host / timeout — the target is down.
		// Released with the outcome's delay (30s), for the reason above.
		return Disposition{Action: BrokerRelease, Group: GroupRelease, Metric: MetricFailure, RetryAfter: outcomeDelay(outcome)}

	case common.MediationRateLimited:
		// 429 — retry in-pipeline honouring Retry-After; NOT a breaker failure.
		return retryOrRelease(attempts, retryDelay(attempts, outcome.DelaySeconds), MetricRateLimited)

	case common.MediationDeferred:
		// 2xx + ack=false — the target explicitly deferred this message
		// (e.g. a blocked record). Not a failure, and the mediator skipped
		// its in-pipeline retries.
		//
		// R1 (owner ruling 2026-09-17, docs/spec/router-deferral-handback.md):
		// when the target NAMED a delay and the source broker actually
		// honours a delayed return (R5), there is nothing to learn from
		// retrying this in-pipeline first — the target told us exactly how
		// long to wait, and the broker (not this process) is where that
		// wait belongs. So it skips the retry budget entirely and releases
		// on this, its FIRST, occurrence, carrying that EXACT delay — no
		// curve, no cap. This applies identically to both dispatch paths:
		// runImmediate nacks just this message with RetryAfter (via
		// nackDelay); the ordered drainer's releaseGroup nacks the HEAD
		// with the very same RetryAfter and only the untried siblings keep
		// their own fixed delay — one Disposition, shared by both callers,
		// is what makes R2 (the head's real delay) fall out of R1 for free
		// rather than needing a second decision point the way a design with
		// separate ordered/unordered outcome handling would.
		//
		// delaySeconds == 0 (nothing specific requested), or a broker that
		// does NOT honour a delayed return (R5: NATS — a hand-back there
		// would spend one of its limited redeliveries for nothing, since
		// nothing will actually hold the message back), falls through
		// unchanged to the pre-ruling behaviour: requeue on the deferred
		// curve (5s start, 60s cap) flooring at any requested delay, within
		// the ordinary in-pipeline retry budget.
		if outcome.DelaySeconds > 0 && honoursDelayedReturn {
			return Disposition{
				Action:     BrokerRelease,
				Group:      GroupRelease,
				Metric:     MetricTransient,
				RetryAfter: time.Duration(outcome.DelaySeconds) * time.Second,
			}
		}
		if outcome.DelaySeconds > 0 {
			// A broker that does not honour a delayed return (NATS) keeps the
			// message in memory, but the wait is still the one the target
			// named — not the curve, which would come back up to a minute late
			// for a short remaining delay (a not-before deferral answers the
			// exact time left). Capped at the deferred ceiling: a longer wait
			// is served as several, the target re-answering the time left.
			return retryOrRelease(attempts, min(time.Duration(outcome.DelaySeconds)*time.Second, deferredMaxDelay), MetricTransient)
		}
		return retryOrRelease(attempts, deferredDelay(attempts, outcome.DelaySeconds), MetricTransient)

	case common.MediationCircuitOpen:
		// Breaker open (decided by the mediator): no delivery was attempted,
		// so there is nothing to measure and the target is by definition
		// unavailable — same class as a transport failure, released to the
		// broker for the same reason.
		//
		// This MUST match the ErrorConnection disposition. The breaker opens
		// almost immediately during a sustained outage, so retrying
		// circuit-open in-pipeline would mean: first failure releases the
		// group, the broker redelivers, the redelivery finds an open breaker
		// and is pinned in-process again — reinstating the memory pinning
		// this release exists to prevent, in exactly the scenario it exists
		// for.
		//
		// The release carries the breaker's reset timeout (the outcome's
		// delay): redelivering sooner could only find the same open breaker
		// again (owner decision #41).
		return Disposition{Action: BrokerRelease, Group: GroupRelease, Metric: MetricNone, RetryAfter: outcomeDelay(outcome)}
	}
	// Unreachable: MediationResult is a closed enum handled exhaustively
	// above. Same silent fallback the original inline switch had.
	return Disposition{Action: BrokerAck, Group: GroupContinue}
}

// outcomeDelay is the delay a mediation outcome asks for, as a duration. A
// negative or zero delay is none.
func outcomeDelay(outcome common.MediationOutcome) time.Duration {
	if outcome.DelaySeconds <= 0 {
		return 0
	}
	return time.Duration(outcome.DelaySeconds) * time.Second
}

// siblingNackDelaySeconds is the least a message is held back when it goes
// back to the broker only because the message in front of it in its group
// did: it was never attempted.
const siblingNackDelaySeconds uint32 = 10

// siblingNackDelay is the nack delay for the untried messages behind a
// released head: never shorter than the head's own, so no sibling can become
// visible before the head it queued behind. With a shorter (or no) delay the
// group's order rests on the broker refusing a group's later messages while
// its head is held back. SQS FIFO does that; LocalStack's long poll does not,
// and nor does a broker without group locks (delivery run 5, platform-down:
// Go delivered g1's seq 6 after 7-10 because head and siblings came back
// visible at once).
func siblingNackDelay(head *uint32) *uint32 {
	d := siblingNackDelaySeconds
	if head != nil && *head > d {
		d = *head
	}
	return &d
}

// retryOrRelease applies the maxInPipelineAttempts budget: within budget,
// retry after delay; past it, release with delay as the broker's redelivery
// hint instead — the only way "retry until the broker expires it" stays true
// when a target answers 429/ack=false forever. Shared by DispositionOf (the
// two outcome-driven retryable results) and Pool.retryAfter (a cancelled
// rate-limiter wait, a recovered panic — paths that retry without ever
// reaching a mediation outcome, and so share the same budget and the same
// pure decision).
func retryOrRelease(attempts uint, delay time.Duration, metric DispositionMetric) Disposition {
	if attempts+1 >= maxInPipelineAttempts {
		return Disposition{Action: BrokerRelease, Group: GroupRelease, Metric: metric, RetryAfter: delay, budgetExhausted: true}
	}
	return Disposition{Action: BrokerRetry, Group: GroupContinue, Metric: metric, RetryAfter: delay}
}

// recordMetric applies a Disposition's Metric — data computed by
// DispositionOf/retryOrRelease — to the PoolMetricsCollector. The single
// place that turns "which metric" into an actual Record* call, so
// DispositionOf itself never touches p.metrics.
func (p *Pool) recordMetric(m DispositionMetric, durationMs uint64) {
	switch m {
	case MetricSuccess:
		p.metrics.RecordSuccess(durationMs)
		p.recordResult(true)
	case MetricFailure:
		p.metrics.RecordFailure(durationMs)
		p.recordResult(false)
	case MetricTransient:
		p.metrics.RecordTransient(durationMs)
	case MetricRateLimited:
		p.metrics.RecordRateLimited()
	case MetricNone:
		// Nothing to record — see MetricNone's doc comment.
	}
}

// recordResult forwards a success/failure outcome to the health recorder.
func (p *Pool) recordResult(success bool) {
	if p.results != nil {
		p.results(p.cfg.Code, success)
	}
}

// processOne runs the per-message pipeline: track (first dispatch only), rate
// limit, mediate, and resolve by disposition. It does NOT release messages to
// the broker on failure — a retryable disposition keeps the in-flight entry
// and returns BrokerRetry so the caller retries in-pipeline (preserving order
// for grouped messages). Only a terminal 2xx/4xx ACKs and clears the entry.
//
// The decision itself lives in DispositionOf (pure); everything here is the
// side effects around it — tracking, rate limiting, mediating, metrics,
// ack/flush.
func (p *Pool) processOne(ctx context.Context, qm common.QueuedMessage) Disposition {
	return p.processOneReleasing(ctx, qm, nil)
}

// slotRelease gives back a pool concurrency slot at most once, so a worker can
// release it early (before a broker acknowledgement) and still have its
// deferred release be a no-op. A nil *slotRelease releases nothing. Used from
// one goroutine only.
type slotRelease struct {
	p    *Pool
	done bool
}

func (s *slotRelease) release() {
	if s == nil || s.done {
		return
	}
	s.done = true
	s.p.sem.release()
}

// processOneReleasing is processOne for a worker that holds a concurrency slot:
// it gives the slot back just BEFORE the broker acknowledgement rather than
// after it. The slot bounds concurrent deliveries to the target, and a broker
// ack is not one of them. For NATS an ack is a fire-and-forget publish, so
// holding the slot through it cost nothing; for SQS it is a network round trip
// (DeleteMessage, ~10-30ms on real SQS), and holding a pool's slots through it
// capped the pool near concurrency / (delivery time + ack time) however fast
// the target was. A nil slot (callers that hold none) releases nothing.
func (p *Pool) processOneReleasing(ctx context.Context, qm common.QueuedMessage, slot *slotRelease) (result Disposition) {
	worker := p.beginMediating(qm)
	defer p.endMediating(worker)

	// One logger for everything said about this message from here down,
	// carried on ctx so the mediator's lines carry the same correlation set
	// (message id, group, pool, queue) without knowing the pool or queue.
	log := p.logger(qm)
	ctx = withMessageLogger(ctx, log)

	// Profiler and tracer correlation, stdlib only. The pprof labels name
	// the message this goroutine is working on, so a goroutine dump taken
	// through /debug/pprof/goroutine?debug=1 (and every CPU profile sample)
	// says which message and group each stuck worker holds. The labels are
	// restored on the way out: a drainer goroutine goes on to the next
	// message of its group. An execution trace (/debug/pprof/trace) shows
	// each delivery as a "router.dispatch" task.
	//
	// The labels cost an allocation set plus two label swaps per message and
	// are only ever read through the debug/pprof surface, so they are on only
	// when ProfilerLabelsEnabled says so (default on; the server turns them
	// off when the debug endpoints are not mounted).
	if profilerLabels.Load() {
		labelled := pprof.WithLabels(ctx, pprof.Labels(
			logKeyMessageID, qm.Message.ID, logKeyGroup, qm.Message.GroupID(),
			logKeyPool, p.cfg.Code, logKeyQueue, qm.QueueIdentifier))
		pprof.SetGoroutineLabels(labelled)
		defer pprof.SetGoroutineLabels(ctx)
		ctx = labelled
	}
	if trace.IsEnabled() {
		var task *trace.Task
		ctx, task = trace.NewTask(ctx, "router.dispatch")
		defer task.End()
		trace.Log(ctx, logKeyMessageID, qm.Message.ID)
	}

	// Panic isolation: a panic mid-mediation must not crash the process (an
	// unrecovered panic in a goroutine takes down the program) or strand the
	// message. Recover and retry in-pipeline — the in-flight entry is kept, so
	// the redelivery dedup stays intact and the worker survives. Named returns
	// let the deferred recover set the verdict.
	defer func() {
		if r := recover(); r != nil {
			logRecovered("pool.processOne (retrying in-pipeline)", r, p.msgAttrs(qm)...)
			// Through the same funnel as every other retry: a target that
			// panics us on every attempt must not be retried for ever either.
			result = p.retryAfter(qm, panicRetryDelay)
		}
	}()

	// The message was registered in-flight at ROUTE time; first dispatch
	// re-asserts the entry as a backstop (restores it if the reaper pruned it
	// during a very long buffer wait). A retry re-dispatch (Attempts>0) is
	// already tracked — keep the existing entry (which may have had its
	// receipt handle swapped by a redelivery) and skip. EnsureTracked never
	// swaps handles: the entry's handle may be fresher than this copy's.
	if p.tracker != nil && qm.Attempts == 0 {
		if !p.tracker.EnsureTrackedMessage(&qm.Message, qm.BrokerMessageID, qm.QueueIdentifier, qm.BatchID, qm.ReceiptHandle) {
			// A different copy of this app message owns the pipeline (external
			// requeue that slipped past route-time dedup). ACK-delete THIS
			// copy with its own receipt handle — leaving it un-acked would let
			// it redeliver forever — and leave the owner's entry alone.
			log.Info("external requeue duplicate (process-time backstop); ACKing copy")
			slot.release()
			if c := p.consumerFor(qm); c != nil {
				if err := c.Ack(ctx, qm.ReceiptHandle, qm.BrokerMessageID); err != nil {
					log.Warn("ack (requeue duplicate) failed", "err", err)
				}
			}
			return Disposition{Action: BrokerAck, Group: GroupContinue}
		}
	}

	// Flushed group: the target already told us it cannot take this group
	// right now and asked us to ACK the siblings rather than deliver them.
	// Checked BEFORE the rate limiter so a flushed group spends neither a
	// token nor a concurrency slot — that saving is the whole point. Safe
	// only because the target asked: it owns the records and re-drives them
	// itself. Suppression is TTL-bounded, so once the window lapses the next
	// message probes the target again.
	if group := qm.Message.GroupID(); group != "" && p.flushes.Suppressed(group) {
		log.Debug("message group flushed; ACKing without delivery")
		// R-53: a suppressed ACK previously left no pool metric at all, so a
		// pool suppressing a whole flushed group read idle rather than
		// busy-but-suppressed. No mediation happened (no HTTP call, no
		// duration), so this bypasses the DispositionOf/recordMetric funnel
		// that ties a metric to a mediation outcome — same reason the
		// duplicate-copy path above records nothing either.
		p.metrics.RecordSuppressed()
		p.events.reject(RejectSuppressed, 1)
		slot.release()
		p.ackTracked(ctx, qm)
		return Disposition{Action: BrokerAck, Group: GroupContinue}
	}

	// Rate limit (per-pool token bucket). Record a rate-limited event when the
	// limiter actually held us back (current tokens exhausted).
	if p.limiter.IsLimited() {
		p.metrics.RecordRateLimited()
	}
	if err := p.limiter.Wait(ctx); err != nil {
		// Context cancelled mid-wait — keep the entry and retry in-pipeline.
		return p.retry(qm, 5)
	}

	start := time.Now()
	outcome := p.mediator.Mediate(ctx, &qm.Message)
	durationMs := uint64(time.Since(start).Milliseconds())

	d := p.settleRetry(qm, DispositionOf(outcome, qm.Attempts, qm.Message.DispatchMode, p.honoursDelayedReturn(qm)))
	p.recordMetric(d.Metric, durationMs)
	p.events.processedOutcome(outcome.Result, d.Metric)

	// A deferral is the one delivery outcome the mediator logs nothing for
	// (it is a 2xx), yet it can hold a message back for as long as the target
	// names — so say who asked, and for how long.
	if outcome.Result == common.MediationDeferred {
		log.Info("target deferred message (ack=false)",
			"delay_seconds", outcome.DelaySeconds, "status", outcome.StatusCode)
	}

	if d.Action == BrokerAck {
		// A 2xx carrying {"flushGroup": true} delivered normally but asks us
		// to suppress the rest of its group. DelaySeconds (when the target
		// sent one) sizes the window; the registry clamps it. Only reachable
		// alongside MediationSuccess — a target that wants the message back
		// (a discarded failure) cannot also discard the group.
		if outcome.Result == common.MediationSuccess && outcome.FlushGroup {
			if group := qm.Message.GroupID(); group != "" {
				ttl := time.Duration(outcome.DelaySeconds) * time.Second
				if p.flushes.Flush(group, ttl) {
					log.Info("message group flushed by target", "delay_seconds", outcome.DelaySeconds)
				}
			} else {
				log.Warn("flushGroup ignored: message has no message group")
			}
		}
		slot.release()
		p.ackTracked(ctx, qm)
	}
	return d
}

// retry marks the in-flight entry as retrying (so the stall detector / reaper
// skip it) and returns the retry-or-release Disposition for the computed
// backoff. Metric is always MetricNone: neither caller (a cancelled
// rate-limiter wait, a recovered panic) reflects a mediation outcome, so
// there is nothing to record.
func (p *Pool) retry(qm common.QueuedMessage, outcomeDelaySec int) Disposition {
	return p.retryAfter(qm, retryDelay(qm.Attempts, outcomeDelaySec))
}

// retryAfter is retry with a pre-computed backoff (used by the deferred path,
// which runs on its own delay curve, and by panic recovery). It funnels
// through retryOrRelease — the same budget DispositionOf's two retryable
// outcomes use — via settleRetry, which applies the side effects the pure
// decision can't make itself.
func (p *Pool) retryAfter(qm common.QueuedMessage, delay time.Duration) Disposition {
	return p.settleRetry(qm, retryOrRelease(qm.Attempts, delay, MetricNone))
}

// settleRetry applies retryOrRelease's side effects to a Disposition however
// it was produced — from DispositionOf's own retryable outcomes, or from
// retryAfter's two paths that retry without a mediation outcome — so the
// budget-exhausted warning and the tracker's retrying mark happen in exactly
// one place. A Disposition that isn't a retry-or-release verdict (success,
// discard, a release the target itself caused) passes through untouched.
func (p *Pool) settleRetry(qm common.QueuedMessage, d Disposition) Disposition {
	if d.budgetExhausted {
		p.logger(qm).Warn("in-pipeline retry budget exhausted; releasing to broker",
			"attempts", qm.Attempts+1, "max_attempts", maxInPipelineAttempts,
			"redelivery_delay", d.RetryAfter)
		return d
	}
	if d.Action == BrokerRetry && p.tracker != nil {
		p.tracker.MarkRetrying(qm.Message.ID, qm.BrokerMessageID)
	}
	return d
}

// nackDelay turns a computed backoff into a broker redelivery delay: whole
// seconds, at least one, or nil when there is nothing to wait out.
func nackDelay(d time.Duration) *uint32 {
	if d <= 0 {
		return nil
	}
	secs := uint32(d / time.Second)
	if secs == 0 {
		secs = 1
	}
	return &secs
}
