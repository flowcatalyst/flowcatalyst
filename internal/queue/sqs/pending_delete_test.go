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
	q.pendingDelete["stale"] = time.Now().Add(-PendingDeleteTTL - time.Minute)
	q.pendingDelete["fresh"] = time.Now()

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
	q.pendingDelete["edge"] = time.Now().Add(-PendingDeleteTTL + time.Second)

	q.evictExpiredPendingDeletesLocked()

	if !q.alreadyDeleted("edge") {
		t.Error("an entry still inside the TTL must survive eviction")
	}
}
