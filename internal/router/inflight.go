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
// bumping its attempt count, so the stall detector and the reaper leave it
// alone (it is legitimately retrying, not stuck). No-op when the entry is gone.
func (t *InFlightTracker) MarkRetrying(messageID, brokerID string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	im := t.byMessage[messageID]
	if im == nil && brokerID != "" {
		im = t.byBroker[brokerID]
	}
	if im != nil {
		im.Attempts++
	}
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

// Reaper periodically prunes entries older than maxAge. Wires together
// with the lifecycle reaper goroutine in cmd/fc-router.
func (t *InFlightTracker) Reap(maxAge time.Duration) (reaped int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for id, im := range t.byMessage {
		// Never reap a message that is actively being retried in-pipeline —
		// it is legitimately long-lived, not stuck, and dropping its entry
		// would blind the redelivery dedup.
		if im.Attempts > 0 {
			continue
		}
		// Age on LastSeenAt, not StartedAt: while the broker still holds the
		// message it keeps redelivering it, and every redelivery refreshes
		// LastSeenAt via the receipt-handle swap. A long-buffered message
		// (slow ordered group) therefore stays tracked — reaping it would
		// blind the dedup and re-admit duplicates. Only an entry the broker
		// has stopped redelivering (leaked/lost) ages out.
		if time.Since(im.LastSeenAt) > maxAge {
			delete(t.byMessage, id)
			if im.BrokerMessageID != "" {
				delete(t.byBroker, im.BrokerMessageID)
			}
			reaped++
		}
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

// DefaultStallConfig matches the Rust defaults (5 min threshold, force-nack off).
func DefaultStallConfig() StallConfig {
	return StallConfig{
		Enabled:               true,
		StallThresholdSeconds: 300,
		ForceNackStalled:      false,
		ForceNackAfterSeconds: 600,
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
	nackFn   NackFunc // optional; required for the force-NACK path
}

// NewStallDetector wires a detector. notifier may be nil. nackFn may be nil,
// in which case the force-NACK path is skipped even when ForceNackStalled is
// set (warnings still fire).
func NewStallDetector(cfg StallConfig, tracker *InFlightTracker, notifier *Notifier, nackFn NackFunc) *StallDetector {
	return &StallDetector{cfg: cfg, tracker: tracker, notifier: notifier, nackFn: nackFn}
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
	stalled := []common.InFlightMessage{}
	for _, im := range d.tracker.Snapshot() {
		// Messages being retried in-pipeline (Attempts>0) are not stalled —
		// they sit in-flight across backoff windows by design. Skip them so
		// they neither warn nor get force-NACKed out from under the retry.
		if im.Attempts > 0 {
			continue
		}
		if im.ElapsedSeconds() >= int64(d.cfg.StallThresholdSeconds) {
			stalled = append(stalled, im)
		}
	}
	if len(stalled) == 0 {
		return
	}
	slog.Warn("stalled messages detected", "count", len(stalled))
	for i := range stalled {
		im := stalled[i]
		if d.notifier != nil {
			d.notifier.Add(Warning{
				Category: WarningCategoryStall,
				Severity: WarningWarning,
				Message: "Message " + im.MessageID + " stalled for " +
					utoa(uint64(im.ElapsedSeconds())) + "s in pool " + im.PoolCode,
				Source: "StallDetector",
			})
		}
		// Force-NACK messages stuck well past the threshold back to their
		// source queue for redelivery, if enabled (default off). Mirrors the
		// Rust force-nack-stalled path. On success, drop the tracker entry so
		// it isn't re-NACKed every tick.
		if d.cfg.ForceNackStalled && d.nackFn != nil &&
			im.ElapsedSeconds() >= int64(d.cfg.ForceNackAfterSeconds) {
			if err := d.nackFn(ctx, im.QueueIdentifier, im.ReceiptHandle, d.cfg.NackDelaySeconds); err != nil {
				slog.Warn("force-nack stalled message failed",
					"msg", im.MessageID, "queue", im.QueueIdentifier, "err", err)
				continue
			}
			d.tracker.Remove(im.MessageID, im.BrokerMessageID)
			slog.Warn("force-nacked stalled message",
				"msg", im.MessageID, "elapsed_s", im.ElapsedSeconds(), "queue", im.QueueIdentifier)
		}
	}
}
