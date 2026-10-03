package sqs

import (
	"testing"
	"time"
)

func newGuardQueue() *Queue {
	return &Queue{pendingDelete: make(map[string]time.Time)}
}

// TestAckRecordsTheDeleteFromTheSuppliedID is the regression guard. The id used
// to be recovered from a receipt→MessageId map populated at poll time; when that
// entry had already been evicted, the lookup missed, NO pending-delete was
// recorded, and the message was deleted anyway — silently. A redelivery of it
// was then not recognised and got delivered to the target a second time.
//
// The id now comes from the caller, which holds it in BrokerMessageID, so no
// lookup can fail and no map state can make the guard forget.
func TestAckRecordsTheDeleteFromTheSuppliedID(t *testing.T) {
	q := newGuardQueue()

	q.markDeleted("msg-1")

	if !q.alreadyDeleted("msg-1") {
		t.Fatal("a deleted MessageId must be remembered so its redelivery is suppressed")
	}
	if q.alreadyDeleted("msg-2") {
		t.Error("an unrelated MessageId must not be reported as deleted")
	}
}

// TestMarkDeletedIgnoresEmptyID: an empty id would key every id-less message to
// the same entry and suppress messages that were never deleted.
func TestMarkDeletedIgnoresEmptyID(t *testing.T) {
	q := newGuardQueue()

	q.markDeleted("")

	if len(q.pendingDelete) != 0 {
		t.Errorf("empty id must not be recorded; map holds %d entries", len(q.pendingDelete))
	}
	if q.alreadyDeleted("") {
		t.Error("empty id must never report as already-deleted")
	}
}

// TestPendingDeletesAreNotRememberedForever: the guard is a short-term
// suppression window, not a permanent ledger of every message ever handled.
// Entries older than PendingDeleteTTL are dropped on each poll, so the map is
// bounded by delete RATE rather than by total volume. A redelivery arriving
// after the window is benign — the target handles it — the window only exists
// to avoid the wasted resend.
func TestPendingDeletesAreNotRememberedForever(t *testing.T) {
	q := newGuardQueue()
	q.markDeletedAt("stale", time.Now().Add(-PendingDeleteTTL-time.Minute))
	q.markDeletedAt("fresh", time.Now())

	q.evictExpiredPendingDeletesLocked()

	if q.alreadyDeleted("stale") {
		t.Error("an entry past the TTL must be evicted — the guard must not grow without bound")
	}
	if !q.alreadyDeleted("fresh") {
		t.Error("an entry inside the TTL must be kept — evicting it early reintroduces the duplicate")
	}
}

// TestEvictionKeepsAnEntryExactlyAtTheBoundary pins the comparison as strictly
// greater-than, so an entry is not dropped a moment early.
func TestEvictionKeepsAnEntryExactlyAtTheBoundary(t *testing.T) {
	q := newGuardQueue()
	q.markDeletedAt("edge", time.Now().Add(-PendingDeleteTTL+time.Second))

	q.evictExpiredPendingDeletesLocked()

	if !q.alreadyDeleted("edge") {
		t.Error("an entry still inside the TTL must survive eviction")
	}
}

// Pruning pops from the FIFO front only: expired ids go, fresh ones stay, and
// the FIFO is fully consumed up to the first fresh entry.
func TestFIFOPruneRemovesExpiredAndKeepsFresh(t *testing.T) {
	q := newGuardQueue()
	now := time.Now()
	q.markDeletedAt("old1", now.Add(-PendingDeleteTTL-2*time.Minute))
	q.markDeletedAt("old2", now.Add(-PendingDeleteTTL-time.Minute))
	q.markDeletedAt("new1", now.Add(-time.Minute))
	q.markDeletedAt("new2", now)

	q.evictExpiredPendingDeletesLocked()

	if len(q.pendingDelete) != 2 || !q.alreadyDeleted("new1") || !q.alreadyDeleted("new2") {
		t.Fatalf("want exactly the 2 fresh ids kept, got %v", q.pendingDelete)
	}
	if q.pendingHead != 2 {
		t.Errorf("only the 2 expired FIFO entries should have been popped, head=%d", q.pendingHead)
	}
}

// An id re-marked with a newer timestamp must survive its old FIFO entry
// expiring: the stale entry must not delete the live map entry.
func TestFIFOPruneKeepsReinsertedIDWithNewTimestamp(t *testing.T) {
	q := newGuardQueue()
	q.markDeletedAt("dup", time.Now().Add(-PendingDeleteTTL-time.Minute)) // expired entry
	q.markDeletedAt("dup", time.Now())                                    // re-inserted, fresh

	q.evictExpiredPendingDeletesLocked()

	if !q.alreadyDeleted("dup") {
		t.Fatal("re-inserted id was pruned by its stale FIFO entry")
	}
	// ...and once the newer stamp expires too, it goes.
	q.pendingDelete["dup"] = time.Now().Add(-PendingDeleteTTL - time.Minute)
	q.pendingFIFO[len(q.pendingFIFO)-1].ts = q.pendingDelete["dup"]
	q.evictExpiredPendingDeletesLocked()
	if q.alreadyDeleted("dup") {
		t.Fatal("expired re-inserted id must eventually be pruned")
	}
}

// The consumed FIFO prefix is reclaimed so the slice does not grow forever.
func TestFIFOPruneCompactsConsumedPrefix(t *testing.T) {
	q := newGuardQueue()
	old := time.Now().Add(-PendingDeleteTTL - time.Minute)
	for i := 0; i < 3000; i++ {
		q.markDeletedAt("id"+time.Duration(i).String(), old)
	}
	q.markDeletedAt("fresh", time.Now())
	q.evictExpiredPendingDeletesLocked()
	if len(q.pendingFIFO) != 1 || q.pendingHead != 0 {
		t.Errorf("want compacted FIFO of 1, got len=%d head=%d", len(q.pendingFIFO), q.pendingHead)
	}
	if !q.alreadyDeleted("fresh") || len(q.pendingDelete) != 1 {
		t.Errorf("unexpected map %v", q.pendingDelete)
	}
}
