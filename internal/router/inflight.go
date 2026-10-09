package router

import (
	"context"
	"hash/maphash"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// InFlightTracker records messages currently being processed across
// the entire router process. Used by:
//   - stall detection (force-NACK messages stuck longer than threshold)
//   - duplicate filter (SQS at-least-once redelivery during processing)
//   - graceful shutdown (drain to zero before exit)
//
// # Sharding
//
// Every message takes three exclusive lock acquisitions here (Register at
// route time, EnsureTracked at dispatch, Remove at ack). With one lock for the
// whole process that became a lock convoy once GOMAXPROCS > 1: at 4 CPUs about
// 2,000 goroutines were measured parked on it, and the router left ~17% of its
// CPU quota idle (docs/router-scaling-results.md). The tracker is therefore
// split into trackerShards independent shards, each with its own lock and its
// own pair of maps.
//
// INVARIANT: a message's entries in BOTH maps live in the shard chosen by its
// application message id (shardFor(MessageID)). That is what lets Register
// keep its single atomic check-and-insert across the two maps under one shard
// lock:
//
//   - byMessage is keyed by the message id, so it is in that shard by
//     definition.
//   - byBroker is keyed by (queue, broker id), but a broker message has exactly
//     one body and so exactly one application message id: every copy of the
//     same broker message — a visibility-timeout redelivery, a deferred copy
//     coming back — carries the same message id and lands in the same shard,
//     where its alias already lives.
//   - An external requeue (same message id, DIFFERENT broker id) also hashes
//     to the owner's shard, so it is classified against the owner under the
//     same lock exactly as before sharding.
//
// Two different message ids never need each other's entries: the only
// cross-message interaction the maps ever had was the bare-broker-id
// collision across queues, which brokerKey already scopes away.
//
// The whole-tracker views (Count, DeferredCount, CountForQueue*, Snapshot,
// Reap) visit the shards one at a time. They were never point-in-time
// consistent with concurrent route/ack traffic in any useful sense (an entry
// can be added or removed the instant the lock is released), and every caller
// treats them as gauges or periodic sweeps.
type InFlightTracker struct {
	seed   maphash.Seed
	shards [trackerShards]trackerShard
}

// trackerShards is the shard count: a power of two so the index is a mask.
// 64 keeps the chance of two of a handful of concurrently-locking goroutines
// colliding low at any plausible GOMAXPROCS, for ~8 KiB of padded shards.
const trackerShards = 64

// trackerShard is one lock and its two maps. Padded to a 128-byte boundary
// (the arm64 cache-line pad size; two 64-byte lines on amd64) so neighbouring
// shards' locks do not share a cache line and bounce it between cores.
type trackerShard struct {
	mu sync.Mutex
	// keyed by (QueueIdentifier, BrokerMessageID); the message itself doubles
	// as the lookup key when the broker ID is unavailable (Postgres-backed
	// queues etc.).
	//
	// The queue identifier is part of the key, not just the broker id, because
	// a broker message id is only unique WITHIN the queue that produced it —
	// NATS's <streamSeq>:<consumerSeq> broker id is unique per stream only, so
	// with several NATS streams configured, every stream's Nth message shares
	// the same bare id. Keying on the bare id let one queue's arriving message
	// look like a redelivery of a different queue's, swapping that entry's
	// receipt handle and producing an ACK on the wrong consumer with a foreign
	// receipt handle (the broker rejects it, or worse, deletes something else).
	byBroker  map[brokerKey]*common.InFlightMessage
	byMessage map[string]*common.InFlightMessage
	_         [trackerShardPad]byte
}

// trackerShardPad pads trackerShard (an 8-byte Mutex plus two map pointers)
// to 128 bytes; TestTrackerShardIsCacheLinePadded pins the arithmetic.
const trackerShardPad = 128 - 8 - 8 - 8

// shardFor returns the shard owning messageID — see the sharding invariant on
// InFlightTracker.
func (t *InFlightTracker) shardFor(messageID string) *trackerShard {
	return &t.shards[maphash.String(t.seed, messageID)&(trackerShards-1)]
}

// brokerKey scopes a broker message id to the queue that produced it — see
// InFlightTracker.byBroker. Every lookup and mutation of byBroker must go
// through this pair, never the bare broker id.
type brokerKey struct {
	queueID  string
	brokerID string
}

// NewInFlightTracker constructs an empty tracker.
func NewInFlightTracker() *InFlightTracker {
	t := &InFlightTracker{seed: maphash.MakeSeed()}
	for i := range t.shards {
		t.shards[i].byBroker = make(map[brokerKey]*common.InFlightMessage)
		t.shards[i].byMessage = make(map[string]*common.InFlightMessage)
	}
	return t
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
	s := t.shardFor(im.MessageID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if im.BrokerMessageID != "" {
		if prev, ok := s.byBroker[brokerKey{im.QueueIdentifier, im.BrokerMessageID}]; ok {
			if !prev.DeferredUntil.IsZero() {
				// The copy the router deferred is back from the broker: it
				// re-enters the pipeline as itself — fresh receipt, fresh
				// LastSeenAt, no longer deferred — and the caller submits it.
				prev.DeferredUntil = time.Time{}
				prev.UpdateReceiptHandle(im.ReceiptHandle)
				return RegisterNew
			}
			prev.UpdateReceiptHandle(im.ReceiptHandle)
			return RegisterRedelivery
		}
	}
	if prev, ok := s.byMessage[im.MessageID]; ok {
		if im.BrokerMessageID != "" && prev.BrokerMessageID != "" && prev.BrokerMessageID != im.BrokerMessageID {
			// Also the deferred case: a second broker copy of a message whose
			// first copy is parked is a duplicate the platform republished.
			// The caller deletes it; the parked copy comes back on schedule.
			return RegisterExternalRequeue
		}
		// Blank-broker-id redelivery: same logical message, adopt the handle.
		prev.UpdateReceiptHandle(im.ReceiptHandle)
		if !prev.DeferredUntil.IsZero() {
			prev.DeferredUntil = time.Time{}
			return RegisterNew
		}
		return RegisterRedelivery
	}
	if im.BrokerMessageID != "" {
		s.byBroker[brokerKey{im.QueueIdentifier, im.BrokerMessageID}] = im
	}
	s.byMessage[im.MessageID] = im
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
	return t.ensureTracked(im.MessageID, im.BrokerMessageID, im.QueueIdentifier, func() *common.InFlightMessage { return im })
}

// EnsureTrackedMessage is EnsureTracked for a caller that has not built the
// InFlightMessage yet: the entry is allocated only when there is none to find,
// so the common case (route time already registered it) allocates nothing.
func (t *InFlightTracker) EnsureTrackedMessage(m *common.Message, brokerID, queueID, batchID, receipt string) bool {
	return t.ensureTracked(m.ID, brokerID, queueID, func() *common.InFlightMessage {
		return common.NewInFlightMessage(m, brokerID, queueID, batchID, receipt)
	})
}

func (t *InFlightTracker) ensureTracked(messageID, brokerID, queueID string, mk func() *common.InFlightMessage) bool {
	s := t.shardFor(messageID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if brokerID != "" {
		if _, ok := s.byBroker[brokerKey{queueID, brokerID}]; ok {
			return true
		}
	}
	if prev, ok := s.byMessage[messageID]; ok {
		if brokerID != "" && prev.BrokerMessageID != "" && prev.BrokerMessageID != brokerID {
			return false
		}
		return true
	}
	im := mk()
	if brokerID != "" {
		s.byBroker[brokerKey{queueID, brokerID}] = im
	}
	s.byMessage[messageID] = im
	return true
}

// CurrentReceipt returns the freshest receipt handle for a tracked message —
// the handle to ACK with after a possible redelivery swap. Reports false when
// the message is no longer tracked.
//
// Looked up by message id alone (brokerID is accepted for API symmetry with
// Remove/MarkRetrying but not consulted): Register and EnsureTracked always
// keep byMessage's entry pointer identical to byBroker's for the same
// delivery, so a redelivery's handle swap (UpdateReceiptHandle) is visible
// through either map. Reading byBroker here would additionally require
// scoping the lookup to the message's own queue — its bare broker id is not
// globally unique (see brokerKey) — for no benefit over the message-id path.
func (t *InFlightTracker) CurrentReceipt(messageID, _ string) (string, bool) {
	s := t.shardFor(messageID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if im, ok := s.byMessage[messageID]; ok {
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
//
// Looked up by message id alone (brokerID is accepted for call-site symmetry
// but not consulted) — see CurrentReceipt's doc for why byMessage always has
// whatever byBroker would have found.
func (t *InFlightTracker) MarkRetrying(messageID, _ string) {
	s := t.shardFor(messageID)
	s.mu.Lock()
	defer s.mu.Unlock()
	im := s.byMessage[messageID]
	if im != nil {
		im.Attempts++
		im.LastRetryAt = time.Now()
	}
}

// Lookup returns a copy of the tracked entry for messageID (exact match).
func (t *InFlightTracker) Lookup(messageID string) (common.InFlightMessage, bool) {
	s := t.shardFor(messageID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if im, ok := s.byMessage[messageID]; ok {
		return *im, true
	}
	return common.InFlightMessage{}, false
}

// Remove clears the message from the tracker. Idempotent.
//
// The byBroker eviction uses the removed entry's OWN QueueIdentifier and
// BrokerMessageID to build the key, not the caller-supplied brokerID: since
// byBroker is now scoped to (queue, broker id), the entry's stored queue is
// the only reliable half of that pair, and only the entry we just removed
// (checked by pointer identity) may be evicted — a later copy may already
// have claimed that key, and evicting it would un-track a live delivery.
func (t *InFlightTracker) Remove(messageID, _ string) {
	s := t.shardFor(messageID)
	s.mu.Lock()
	defer s.mu.Unlock()
	im, ok := s.byMessage[messageID]
	delete(s.byMessage, messageID)
	if ok && im.BrokerMessageID != "" {
		key := brokerKey{im.QueueIdentifier, im.BrokerMessageID}
		if cur, exists := s.byBroker[key]; exists && cur == im {
			delete(s.byBroker, key)
		}
	}
}

// countWhere counts byMessage entries matching pred, visiting the shards one
// at a time (see the sharding note on InFlightTracker).
func (t *InFlightTracker) countWhere(pred func(*common.InFlightMessage) bool) int {
	n := 0
	for i := range t.shards {
		s := &t.shards[i]
		s.mu.Lock()
		for _, im := range s.byMessage {
			if pred(im) {
				n++
			}
		}
		s.mu.Unlock()
	}
	return n
}

// Count returns the number of in-flight messages.
func (t *InFlightTracker) Count() int {
	// Deferred copies are the broker's to hold, not this process's to
	// finish: they must not keep a drain waiting or inflate the in-flight
	// gauge. They are counted separately (DeferredCount).
	return t.countWhere(func(im *common.InFlightMessage) bool { return im.DeferredUntil.IsZero() })
}

// DeferredCount is how many tracked messages are currently parked on the
// broker by a capacity deferral, awaiting their scheduled return.
func (t *InFlightTracker) DeferredCount() int {
	return t.countWhere(func(im *common.InFlightMessage) bool { return !im.DeferredUntil.IsZero() })
}

// MarkDeferred keeps a message's entry across a capacity deferral, stamped
// with when the broker is expected to redeliver it. Until then a copy of the
// same message id under a different broker id is a duplicate (Register →
// RegisterExternalRequeue), and the reaper leaves the entry alone. A message
// the tracker does not know is ignored.
func (t *InFlightTracker) MarkDeferred(messageID string, until time.Time) {
	s := t.shardFor(messageID)
	s.mu.Lock()
	defer s.mu.Unlock()
	if im, ok := s.byMessage[messageID]; ok {
		im.DeferredUntil = until
		im.LastSeenAt = time.Now()
	}
}

// deferredReapGrace is how long past DeferredUntil a parked entry is kept
// before the idle rule may reap it: the broker's redelivery is not to the
// second, and a copy that never returns (past the broker's own ceiling) must
// still age out so a later republish is admitted.
const deferredReapGrace = 5 * time.Minute

// CountForQueue returns how many tracked entries originated from queueID
// (InFlightMessage.QueueIdentifier). Every message a pool is holding —
// buffered, mid-backoff, or actively mediating — has a live tracker entry
// for exactly as long as it does (route-time Register through the
// terminal ack/nack Remove), so zero here is the honest answer to "does
// anything in the pipeline still reference this queue?" without asking
// every pool to introspect its own buffers. Used by
// Manager.retireDetachedConsumers (X-11 / R-26/R-49: a removed/changed
// queue's consumer stays addressable for ack/nack until nothing
// references it any more).
func (t *InFlightTracker) CountForQueue(queueID string) int {
	return t.countWhere(func(im *common.InFlightMessage) bool { return im.QueueIdentifier == queueID })
}

// CountForQueueBefore is CountForQueue narrowed to entries whose StartedAt
// predates cutoff. Used by Manager.retireDetachedConsumers (X-11 / R-26/
// R-49): a detached consumer's replacement keeps polling and delivering
// FRESH traffic under the exact same queue identifier, and plain
// CountForQueue can't tell that traffic apart from the pre-detach messages
// the old consumer is actually waiting to see resolved — so it would never
// reach zero for a queue under continuous load, and the detached consumer
// (and its underlying broker connection) would leak forever. Passing the
// detaching consumer's own detachedAt as cutoff counts only entries that
// existed before it stopped polling — the ones it could actually have
// polled — so the replacement's ongoing traffic (StartedAt always at or
// after detachedAt, since it can only start polling once installed) never
// holds up retirement.
func (t *InFlightTracker) CountForQueueBefore(queueID string, cutoff time.Time) int {
	return t.countWhere(func(im *common.InFlightMessage) bool {
		return im.QueueIdentifier == queueID && im.StartedAt.Before(cutoff)
	})
}

// Snapshot returns the current in-flight messages (by copy). Each entry is
// copied under its shard's lock, so no copy is torn; the set as a whole is
// assembled shard by shard.
func (t *InFlightTracker) Snapshot() []common.InFlightMessage {
	var out []common.InFlightMessage
	for i := range t.shards {
		s := &t.shards[i]
		s.mu.Lock()
		out = slices.Grow(out, len(s.byMessage))
		for _, im := range s.byMessage {
			out = append(out, *im)
		}
		s.mu.Unlock()
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
	now := time.Now()
	var pastCeiling []common.InFlightMessage
	for i := range t.shards {
		var n int
		n, pastCeiling = t.shards[i].reap(now, maxAge, absoluteMaxAge, pastCeiling[:0])
		reaped += n
		// Logged after the shard's lock is released, so a burst of reaped
		// entries never holds route and ack traffic behind the log handler.
		for j := range pastCeiling {
			im := &pastCeiling[j]
			// Past the ceiling nothing exempts an entry — not a retry in
			// progress, not a redelivery that just refreshed LastSeenAt.
			// Reaching here always means something upstream failed to remove
			// the entry, so say so loudly enough to be chased.
			slog.Warn("in-flight entry passed the absolute age ceiling; reaping (its owner is gone)",
				"message_id", im.MessageID, "pool", im.PoolCode,
				"queue", im.QueueIdentifier, "group", im.MessageGroupID,
				"attempts", im.Attempts, "elapsed_s", im.ElapsedSeconds(),
				"ceiling", absoluteMaxAge)
		}
	}
	return reaped
}

// reap applies Reap's rules to one shard under its lock. Entries reaped by the
// absolute ceiling are appended (by copy) to pastCeiling for the caller to log.
func (s *trackerShard) reap(now time.Time, maxAge, absoluteMaxAge time.Duration,
	pastCeiling []common.InFlightMessage,
) (int, []common.InFlightMessage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reaped := 0
	for id, im := range s.byMessage {
		if now.Sub(im.StartedAt) > absoluteMaxAge {
			pastCeiling = append(pastCeiling, *im)
		} else {
			// A live in-pipeline retry is legitimately long-running: exempt it
			// from the idle bound, but only while its last attempt is recent
			// enough that the retry can still be running. Past the grace the
			// owner is gone and the entry is judged like any other.
			if im.Attempts > 0 && !im.LastRetryAt.IsZero() &&
				now.Sub(im.LastRetryAt) <= reapRetryGrace {
				continue
			}
			// A deferred copy is parked on the broker by design for up to the
			// deferral horizon — well past the idle bound. Keep it until it is
			// overdue; the ceiling above still bounds it absolutely.
			if !im.DeferredUntil.IsZero() && now.Before(im.DeferredUntil.Add(deferredReapGrace)) {
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
		delete(s.byMessage, id)
		if im.BrokerMessageID != "" {
			delete(s.byBroker, brokerKey{im.QueueIdentifier, im.BrokerMessageID})
		}
		reaped++
	}
	// Consistency sweep: drop any byBroker alias that no longer points at a
	// live byMessage entry (defensive — the two maps are updated together, and
	// by the sharding invariant an alias lives in its message's shard).
	for id, im := range s.byBroker {
		if cur, ok := s.byMessage[im.MessageID]; !ok || cur != im {
			delete(s.byBroker, id)
		}
	}
	return reaped, pastCeiling
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
	warnings *WarningService // never nil after NewStallDetector; see SetWarnings
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

// NewStallDetector wires a detector. notifier may be nil (warnings are still
// recorded, just never webhooked). nackFn may be nil, in which case the
// force-NACK path is skipped even when ForceNackStalled is set (warnings
// still fire).
//
// Stall reports are routed through a WarningService (X-04) rather than the
// notifier directly, so they reach /warnings, health counts and
// acknowledgement — not the webhook alone. Without this a message wedged for
// the better part of an hour raised nothing any UI could show.
// NewStallDetector provisions a private WarningService wired to notifier so
// that holds true even before SetWarnings wires in the process-wide store;
// SetWarnings then swaps in that shared instance without changing behaviour
// otherwise (same warnings, same forwarding to notifier — see
// WarningService.SetNotifier/Add).
func NewStallDetector(cfg StallConfig, tracker *InFlightTracker, notifier *Notifier, nackFn NackFunc) *StallDetector {
	ws := NewWarningService(WarningServiceConfig{})
	ws.SetNotifier(notifier)
	return &StallDetector{
		cfg:      cfg,
		tracker:  tracker,
		warnings: ws,
		nackFn:   nackFn,
		warned:   make(map[string]struct{}),
	}
}

// SetWarnings swaps in the process-wide WarningService (e.g. Server.Warnings)
// so stall reports share the same store — and the same
// acknowledgement/health-count surface — as every other emitter, instead of
// the private one NewStallDetector provisions. nil detaches (reports become
// log-only).
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

	if d.warnings != nil {
		d.warnings.Add(WarningCategoryStall, severity, message, "StallDetector")
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
			safely("stallDetector.tick", func() { d.tick(ctx) })
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
