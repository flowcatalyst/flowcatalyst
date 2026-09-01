package router

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// InFlightTracker records messages currently being processed across
// the entire router process. Used by:
//   - stall detection (force-NACK messages stuck longer than threshold)
//   - duplicate filter (SQS at-least-once redelivery during processing)
//   - graceful shutdown (drain to zero before exit)
type InFlightTracker struct {
	mu sync.RWMutex
	// keyed by broker message ID; the message itself doubles as the lookup
	// key when the broker ID is unavailable (Postgres-backed queues etc.).
	byBroker  map[string]*common.InFlightMessage
	byMessage map[string]*common.InFlightMessage
}

// NewInFlightTracker constructs an empty tracker.
func NewInFlightTracker() *InFlightTracker {
	return &InFlightTracker{
		byBroker:  make(map[string]*common.InFlightMessage),
		byMessage: make(map[string]*common.InFlightMessage),
	}
}

// RegisterOutcome is Register's verdict on a newly-polled copy of a message.
type RegisterOutcome int

const (
	// RegisterNew — first copy of this message; it was inserted and now OWNS
	// the pipeline. The caller submits it to a pool.
	RegisterNew RegisterOutcome = iota
	// RegisterRedelivery — a copy of a message already in the pipeline: the
	// same broker MessageId (SQS visibility-timeout redelivery), or the same
	// app message ID when broker ids are blank (Postgres-style queues). The
	// owner's receipt handle was swapped to this fresher one; the caller
	// drops this copy (nothing to release — SQS Nack is a no-op).
	RegisterRedelivery
	// RegisterExternalRequeue — the same app message ID is in the pipeline
	// under a DIFFERENT broker id: an external process requeued a message it
	// thought was lost while the original is still being processed/retried.
	// Nothing was inserted and the owner's receipt handle was NOT touched
	// (handles are per-broker-message; adopting this one would make the
	// owner's eventual ACK delete the wrong SQS message). The caller must
	// ACK this copy so the requeued duplicate is deleted from the broker.
	RegisterExternalRequeue
)

// Register claims pipeline ownership for a polled copy of a message, or
// classifies it as a duplicate of the current owner. Called by the Manager at
// ROUTE time — before the message is buffered or dispatched — so ordered-group
// buffering windows are covered: a copy that is merely queued behind a slow
// group head still dedupes redeliveries and external requeues. The single
// atomic check-and-insert replaces the old separate swap/requeue probes.
func (t *InFlightTracker) Register(im *common.InFlightMessage) RegisterOutcome {
	t.mu.Lock()
	defer t.mu.Unlock()
	if im.BrokerMessageID != "" {
		if prev, ok := t.byBroker[im.BrokerMessageID]; ok {
			prev.UpdateReceiptHandle(im.ReceiptHandle)
			return RegisterRedelivery
		}
	}
	if prev, ok := t.byMessage[im.MessageID]; ok {
		if im.BrokerMessageID != "" && prev.BrokerMessageID != "" && prev.BrokerMessageID != im.BrokerMessageID {
			return RegisterExternalRequeue
		}
		// Blank-broker-id redelivery: same logical message, adopt the handle.
		prev.UpdateReceiptHandle(im.ReceiptHandle)
		return RegisterRedelivery
	}
	if im.BrokerMessageID != "" {
		t.byBroker[im.BrokerMessageID] = im
	}
	t.byMessage[im.MessageID] = im
	return RegisterNew
}

// EnsureTracked is the process-time backstop behind Register: the pool calls
// it on first dispatch. Normally the route-time entry is found and the message
// proceeds; if the entry was lost (e.g. reaped while the message sat buffered
// for a very long time) it is restored. It never swaps receipt handles — the
// existing entry may hold a FRESHER handle from a redelivery swap, and this
// copy's route-time handle must not regress it. Returns false when a DIFFERENT
// copy of the app message owns the pipeline (an external requeue that slipped
// past route-time dedup, e.g. across a reap): the caller ACK-drops this copy
// with its own receipt handle and must not touch the owner's entry.
func (t *InFlightTracker) EnsureTracked(im *common.InFlightMessage) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if im.BrokerMessageID != "" {
		if _, ok := t.byBroker[im.BrokerMessageID]; ok {
			return true
		}
	}
	if prev, ok := t.byMessage[im.MessageID]; ok {
		if im.BrokerMessageID != "" && prev.BrokerMessageID != "" && prev.BrokerMessageID != im.BrokerMessageID {
			return false
		}
		return true
	}
	if im.BrokerMessageID != "" {
		t.byBroker[im.BrokerMessageID] = im
	}
	t.byMessage[im.MessageID] = im
	return true
}

// CurrentReceipt returns the freshest receipt handle for a tracked message
// (broker id preferred, message id fallback) — the handle to ACK with after a
// possible redelivery swap. Reports false when the message is no longer tracked.
func (t *InFlightTracker) CurrentReceipt(messageID, brokerID string) (string, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if brokerID != "" {
		if im, ok := t.byBroker[brokerID]; ok {
			return im.ReceiptHandle, true
		}
	}
	if im, ok := t.byMessage[messageID]; ok {
		return im.ReceiptHandle, true
	}
	return "", false
}

// MarkRetrying records that a tracked message is being retried in-pipeline by
// bumping its attempt count and stamping LastRetryAt, so the stall detector
// and the reaper leave it alone while the retry is live (it is legitimately
// retrying, not stuck). No-op when the entry is gone.
//
// The stamp is not optional bookkeeping: it is the clock the reaper's
// exemption expires on. Bumping Attempts without it would restore the
// unbounded exemption this pair exists to close.
func (t *InFlightTracker) MarkRetrying(messageID, brokerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	im := t.byMessage[messageID]
	if im == nil && brokerID != "" {
		im = t.byBroker[brokerID]
	}
	if im != nil {
		im.Attempts++
		im.LastRetryAt = time.Now()
	}
}

// Lookup returns a copy of the tracked entry for messageID (exact match).
func (t *InFlightTracker) Lookup(messageID string) (common.InFlightMessage, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if im, ok := t.byMessage[messageID]; ok {
		return *im, true
	}
	return common.InFlightMessage{}, false
}

// Remove clears the message from the tracker. Idempotent.
func (t *InFlightTracker) Remove(messageID, brokerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.byMessage, messageID)
	if brokerID != "" {
		delete(t.byBroker, brokerID)
	}
}

// Count returns the number of in-flight messages.
func (t *InFlightTracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.byMessage)
}

// Snapshot returns the current in-flight messages (by copy).
func (t *InFlightTracker) Snapshot() []common.InFlightMessage {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]common.InFlightMessage, 0, len(t.byMessage))
	for _, im := range t.byMessage {
		out = append(out, *im)
	}
	return out
}

// reapRetryGrace bounds the reaper's Attempts>0 exemption: past it, an entry
// that has recorded no new attempt is an orphan rather than a live retry.
//
// It is sized against the MEDIATOR'S per-attempt timeout, not the backoff
// between attempts. A single delivery may legitimately occupy its worker for
// DefaultMediatorConfig().Timeout — 15 minutes, the platform's processing
// contract, matching AWS Lambda's ceiling — and records no new attempt for the
// whole of it. A grace shorter than that (the first version used
// 2*retryMaxDelay, ten minutes) would let the reaper drop the tracker entry of
// a delivery still in progress, after which the next broker redelivery is
// admitted as a fresh copy: a DOUBLE DELIVERY of a message that was being
// processed successfully. Doubling the attempt timeout leaves room for the
// dispatch either side of the call.
var reapRetryGrace = 2 * DefaultMediatorConfig().Timeout

// defaultAbsoluteMaxAgeFactor derives Reap's absolute ceiling from its idle
// bound when the caller supplies none (15m idle -> 2h ceiling).
const defaultAbsoluteMaxAgeFactor = 8

// Reap prunes stale entries under TWO independent bounds, because neither one
// alone guarantees the tracker ever lets go of an entry.
//
//	maxAge         — the IDLE bound, measured on LastSeenAt. Catches entries
//	                 the broker has stopped redelivering (leaked / lost).
//	absoluteMaxAge — the HARD ceiling, measured on StartedAt. Catches
//	                 everything else, including what the idle bound can never
//	                 see. Non-positive derives it from maxAge.
//
// The ceiling is what makes reaping guaranteed rather than best-effort. The
// idle bound is circular: every redelivery refreshes LastSeenAt (via
// UpdateReceiptHandle) — including the redeliveries that Register drops as
// duplicates of this very entry. An entry whose owner has died therefore
// resets its own idle clock on every redelivery and can never age out, while
// the message it keeps dropping cycles on the broker untouched until
// retention and, on a FIFO queue, holds the head of its message group for
// that entire time. Ageing on StartedAt as well is what breaks the loop.
//
// The ceiling can prune an entry whose message is still legitimately buffered
// in a very slow ordered group. That is deliberate and survivable: the pool's
// EnsureTracked backstop re-registers it on dispatch, and the worst case is a
// duplicate delivery on a broker that is at-least-once anyway. A permanently
// stuck message blocking a whole FIFO group is strictly worse.
func (t *InFlightTracker) Reap(maxAge, absoluteMaxAge time.Duration) (reaped int) {
	if absoluteMaxAge <= 0 {
		absoluteMaxAge = defaultAbsoluteMaxAgeFactor * maxAge
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	for id, im := range t.byMessage {
		if now.Sub(im.StartedAt) > absoluteMaxAge {
			// Past the ceiling nothing exempts an entry — not a retry in
			// progress, not a redelivery that just refreshed LastSeenAt.
			// Reaching here always means something upstream failed to remove
			// the entry, so say so loudly enough to be chased.
			slog.Warn("in-flight entry passed the absolute age ceiling; reaping (its owner is gone)",
				"message_id", im.MessageID, "pool", im.PoolCode,
				"queue", im.QueueIdentifier, "group", im.MessageGroupID,
				"attempts", im.Attempts, "elapsed_s", im.ElapsedSeconds(),
				"ceiling", absoluteMaxAge)
		} else {
			// A live in-pipeline retry is legitimately long-running: exempt it
			// from the idle bound, but only while its last attempt is recent
			// enough that the retry can still be running. Past the grace the
			// owner is gone and the entry is judged like any other.
			if im.Attempts > 0 && !im.LastRetryAt.IsZero() &&
				now.Sub(im.LastRetryAt) <= reapRetryGrace {
				continue
			}
			// Age on LastSeenAt, not StartedAt: while the broker still holds
			// the message it keeps redelivering it, and every redelivery
			// refreshes LastSeenAt via the receipt-handle swap. A
			// long-buffered message (slow ordered group) therefore stays
			// tracked — reaping it here would blind the dedup and re-admit
			// duplicates for no reason. The ceiling above is what stops that
			// refresh being unbounded.
			if now.Sub(im.LastSeenAt) <= maxAge {
				continue
			}
		}
		delete(t.byMessage, id)
		if im.BrokerMessageID != "" {
			delete(t.byBroker, im.BrokerMessageID)
		}
		reaped++
	}
	// Consistency sweep: drop any byBroker alias that no longer points at a
	// live byMessage entry (defensive — the two maps are updated together).
	for id, im := range t.byBroker {
		if cur, ok := t.byMessage[im.MessageID]; !ok || cur != im {
			delete(t.byBroker, id)
		}
	}
	return
}

// StallConfig configures the stall detector.
type StallConfig struct {
	Enabled               bool
	StallThresholdSeconds uint64
	ForceNackStalled      bool
	ForceNackAfterSeconds uint64
	NackDelaySeconds      uint32
	CheckInterval         time.Duration
}

// DefaultStallConfig returns the defaults. The thresholds are derived from the
// mediation contract, not chosen freely: a single delivery attempt may
// legitimately run for DefaultMediatorConfig().Timeout (15 minutes, matching
// Lambda's ceiling), and a message may take three of them.
//
// The old values — 300s to warn, 600s to force-NACK — predate that contract and
// were actively wrong under it. 300s flagged every legitimate long delivery as
// stalled, and 600s would have force-NACKed one that was still running,
// handing the broker a second copy of a message being processed successfully.
// That last one was inert only because ForceNackStalled defaults to false; it
// must stay false unless ForceNackAfterSeconds clears the full three-attempt
// budget.
func DefaultStallConfig() StallConfig {
	attempt := DefaultMediatorConfig().Timeout
	return StallConfig{
		Enabled: true,
		// One attempt plus a minute of dispatch slack: below this a message may
		// simply be inside a long call that is going to succeed.
		StallThresholdSeconds: uint64(attempt.Seconds()) + 60,
		ForceNackStalled:      false,
		// Clear of the whole budget (3 attempts) with room to spare, so
		// enabling it can never duplicate a live delivery.
		ForceNackAfterSeconds: uint64(4 * attempt.Seconds()),
		NackDelaySeconds:      30,
		CheckInterval:         60 * time.Second,
	}
}

// NackFunc returns a stuck in-flight message to its source queue for
// redelivery (resolving the queue by identifier). The Manager implements it.
type NackFunc func(ctx context.Context, queueID, receiptHandle string, delaySeconds uint32) error

// StallDetector watches the in-flight tracker for messages stuck longer
// than the threshold. Emits warnings and optionally force-NACKs.
type StallDetector struct {
	cfg      StallConfig
	tracker  *InFlightTracker
	notifier *Notifier
	warnings *WarningService // optional; nil → webhook-only, as before
	nackFn   NackFunc        // optional; required for the force-NACK path

	// warned remembers which message ids have already been reported in the
	// current stall episode, cleared when a message leaves the tracker.
	//
	// Without it the detector re-reports every stalled message on every tick.
	// That was survivable while these warnings went only to the webhook, but
	// they now also reach the WarningService, which drives the active-warning
	// count and the DEGRADED threshold (20) — so a handful of long deliveries
	// re-reported each minute would put the router into DEGRADED purely for
	// doing its job.
	warnedMu sync.Mutex
	warned   map[string]struct{}
}

// NewStallDetector wires a detector. notifier may be nil. nackFn may be nil,
// in which case the force-NACK path is skipped even when ForceNackStalled is
// set (warnings still fire).
func NewStallDetector(cfg StallConfig, tracker *InFlightTracker, notifier *Notifier, nackFn NackFunc) *StallDetector {
	return &StallDetector{
		cfg:      cfg,
		tracker:  tracker,
		notifier: notifier,
		nackFn:   nackFn,
		warned:   make(map[string]struct{}),
	}
}

// SetWarnings routes stall reports to the WarningService as well as the
// webhook notifier, so they reach /warnings, the dashboard and the health
// report. Without it a message wedged for the better part of an hour raised
// nothing any UI could show.
func (d *StallDetector) SetWarnings(w *WarningService) { d.warnings = w }

// report emits a stall warning once per message per episode. Returns false if
// this message has already been reported and nothing was emitted.
func (d *StallDetector) report(id string, severity WarningSeverity, message string) bool {
	d.warnedMu.Lock()
	if _, seen := d.warned[id]; seen {
		d.warnedMu.Unlock()
		return false
	}
	d.warned[id] = struct{}{}
	d.warnedMu.Unlock()

	w := Warning{Category: WarningCategoryStall, Severity: severity, Message: message, Source: "StallDetector"}
	// WarningService forwards to the notifier itself, so route through it when
	// present and fall back to the raw notifier when it is not — never both, or
	// every stall is webhooked twice.
	if d.warnings != nil {
		d.warnings.Add(w.Category, w.Severity, w.Message, w.Source)
		return true
	}
	if d.notifier != nil {
		d.notifier.Add(w)
	}
	return true
}

// forgetResolved drops warned-entries for messages no longer in the pipeline,
// so a later stall of the same id reports again rather than being silenced for
// the life of the process.
func (d *StallDetector) forgetResolved(live map[string]struct{}) {
	d.warnedMu.Lock()
	defer d.warnedMu.Unlock()
	for id := range d.warned {
		if _, ok := live[id]; !ok {
			delete(d.warned, id)
		}
	}
}

// Watch runs the periodic check until ctx is cancelled.
func (d *StallDetector) Watch(ctx context.Context) {
	if !d.cfg.Enabled {
		return
	}
	tick := time.NewTicker(d.cfg.CheckInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			d.tick(ctx)
		}
	}
}

func (d *StallDetector) tick(ctx context.Context) {
	// Two populations, and the distinction is about what may be DONE to them,
	// not whether they are worth reporting:
	//
	//   stalled  — nothing is happening to them. Warn, and force-NACK if that
	//              is enabled.
	//   retrying — a worker owns them and is retrying in-pipeline. Warn, but
	//              never force-NACK: yanking a message out from under a live
	//              retry hands the broker a copy while this one is still
	//              running, which is a double delivery.
	//
	// Retrying entries used to be skipped outright, which meant the one
	// failure mode that can genuinely persist — an endpoint deferring every
	// attempt — was the one nothing ever reported. The retry budget bounds it
	// now (see maxInPipelineAttempts), but a message burning that budget for
	// minutes is exactly what an operator wants to hear about.
	var stalled, retrying []common.InFlightMessage
	live := make(map[string]struct{})
	for _, im := range d.tracker.Snapshot() {
		live[im.MessageID] = struct{}{}
		if im.ElapsedSeconds() < int64(d.cfg.StallThresholdSeconds) {
			continue
		}
		if im.Attempts > 0 {
			retrying = append(retrying, im)
			continue
		}
		stalled = append(stalled, im)
	}
	// Messages that have left the pipeline can report again if they ever stall
	// afresh.
	d.forgetResolved(live)

	for i := range retrying {
		im := retrying[i]
		if !d.report(im.MessageID, WarningWarning,
			"Message "+im.MessageID+" has been retrying in-pipeline for "+
				utoa(uint64(im.ElapsedSeconds()))+"s ("+utoa(uint64(im.Attempts))+
				" attempts) in pool "+im.PoolCode) {
			continue
		}
		slog.Warn("message retrying in-pipeline past the stall threshold",
			"message_id", im.MessageID, "attempts", im.Attempts,
			"elapsed_s", im.ElapsedSeconds(), "pool", im.PoolCode)
	}
	if len(stalled) == 0 {
		return
	}
	slog.Warn("stalled messages detected", "count", len(stalled))
	for i := range stalled {
		im := stalled[i]
		if d.report(im.MessageID, WarningWarning,
			"Message "+im.MessageID+" stalled for "+
				utoa(uint64(im.ElapsedSeconds()))+"s in pool "+im.PoolCode) {
			slog.Warn("stalled message",
				"message_id", im.MessageID, "elapsed_s", im.ElapsedSeconds(), "pool", im.PoolCode)
		}
		// Force-NACK messages stuck well past the threshold back to their
		// source queue for redelivery, if enabled (default off). On
		// success, drop the tracker entry so it isn't re-NACKed every tick.
		if d.cfg.ForceNackStalled && d.nackFn != nil &&
			im.ElapsedSeconds() >= int64(d.cfg.ForceNackAfterSeconds) {
			if err := d.nackFn(ctx, im.QueueIdentifier, im.ReceiptHandle, d.cfg.NackDelaySeconds); err != nil {
				slog.Warn("force-nack stalled message failed",
					"message_id", im.MessageID, "queue", im.QueueIdentifier, "err", err)
				continue
			}
			d.tracker.Remove(im.MessageID, im.BrokerMessageID)
			slog.Warn("force-nacked stalled message",
				"message_id", im.MessageID, "elapsed_s", im.ElapsedSeconds(), "queue", im.QueueIdentifier)
		}
	}
}
