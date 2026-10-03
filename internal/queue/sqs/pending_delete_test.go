package sqs

import (
	"strconv"
	"testing"
	"time"
)

func newGuardQueue() *Queue {
	return &Queue{pendingDelete: make(map[string]*pendingEntry)}
}

// TestAckRecordsTheDeleteFromTheSuppliedID is the regression guard. The id used
// to be recovered from a receipt→MessageId map populated at poll time; when that
// entry had already been evicted, the lookup missed, NO pending-delete was
// recorded, and the message was deleted anyway — silently. The id now comes
// from the caller, so no lookup can fail.
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

// An empty id would key every id-less message to the same entry.
func TestMarkDeletedIgnoresEmptyID(t *testing.T) {
	q := newGuardQueue()

	if e := q.markDeleted(""); e != nil {
		t.Error("empty id must yield no entry")
	}
	if len(q.pendingDelete) != 0 || q.alreadyDeleted("") {
		t.Errorf("empty id must not be recorded; map holds %d entries", len(q.pendingDelete))
	}
	q.markDeleteDone(nil) // nil entries are accepted
}

// An entry whose delete is still in flight survives pruning however long that
// takes, then expires pendingDeleteGrace after it completes.
func TestInFlightEntrySurvivesPruningThenExpiresGraceAfterCompletion(t *testing.T) {
	q := newGuardQueue()
	e := q.markDeleted("m")
	start := time.Now()

	q.evictExpiredPendingDeletesAt(start.Add(time.Hour)) // far past any grace
	if !q.alreadyDeleted("m") || len(q.pendingDelete) != 1 {
		t.Fatal("an in-flight entry must never be pruned")
	}

	done := start.Add(time.Hour)
	q.markDeleteDoneAt(e, done)
	q.evictExpiredPendingDeletesAt(done.Add(time.Second)) // well inside the grace
	if len(q.pendingDelete) != 1 {
		t.Fatal("entry must be kept for the grace after completion")
	}
	q.evictExpiredPendingDeletesAt(done.Add(pendingDeleteGrace)) // exactly at the boundary
	if len(q.pendingDelete) != 1 {
		t.Fatal("entry must be kept until strictly past the grace")
	}
	q.evictExpiredPendingDeletesAt(done.Add(pendingDeleteGrace + time.Millisecond))
	if len(q.pendingDelete) != 0 {
		t.Fatal("completed entry must expire pendingDeleteGrace after completion")
	}
}

// A failed delete completes the same way (the caller marks done on any outcome),
// so it is forgotten after the grace too.
func TestFailedDeleteIsForgottenAfterTheGrace(t *testing.T) {
	q := newGuardQueue()
	e := q.markDeleted("m")
	now := time.Now()
	q.markDeleteDoneAt(e, now)
	q.markDeleteDoneAt(e, now.Add(time.Hour)) // a second completion must not extend it
	q.evictExpiredPendingDeletesAt(now.Add(pendingDeleteGrace + time.Second))
	if q.alreadyDeleted("m") || len(q.pendingDelete) != 0 {
		t.Fatal("completed entry must be forgotten after the grace")
	}
}

// An in-flight entry at the FIFO front holds back later completed ones (front
// pops only), but nothing is lost early and all go once it completes.
func TestFIFOFrontPopsOnlyAndStopsAtInFlight(t *testing.T) {
	q := newGuardQueue()
	now := time.Now()
	slow := q.markDeleted("slow")
	fast := q.markDeleted("fast")
	q.markDeleteDoneAt(fast, now)

	q.evictExpiredPendingDeletesAt(now.Add(time.Minute))
	if q.pendingHead != 0 || len(q.pendingDelete) != 2 {
		t.Fatalf("pop must stop at the in-flight front, head=%d", q.pendingHead)
	}
	q.markDeleteDoneAt(slow, now)
	q.evictExpiredPendingDeletesAt(now.Add(time.Minute))
	if q.pendingHead != 2 || len(q.pendingDelete) != 0 {
		t.Fatalf("both must be pruned once completed, head=%d", q.pendingHead)
	}
}

// An id re-marked must survive its older entry expiring.
func TestFIFOPruneKeepsReinsertedID(t *testing.T) {
	q := newGuardQueue()
	now := time.Now()
	old := q.markDeleted("dup")
	q.markDeleteDoneAt(old, now)
	q.markDeleted("dup") // re-acked, in flight

	q.evictExpiredPendingDeletesAt(now.Add(time.Minute))
	if !q.alreadyDeleted("dup") {
		t.Fatal("re-inserted id was pruned by its stale entry")
	}
}

// The consumed FIFO prefix is reclaimed so the slice does not grow forever.
func TestFIFOPruneCompactsConsumedPrefix(t *testing.T) {
	q := newGuardQueue()
	now := time.Now()
	for i := range 3000 {
		q.markDeleteDoneAt(q.markDeleted("id"+strconv.Itoa(i)), now)
	}
	q.markDeleted("fresh")
	q.evictExpiredPendingDeletesAt(now.Add(time.Minute))
	if len(q.pendingFIFO) != 1 || q.pendingHead != 0 {
		t.Errorf("want compacted FIFO of 1, got len=%d head=%d", len(q.pendingFIFO), q.pendingHead)
	}
	if !q.alreadyDeleted("fresh") || len(q.pendingDelete) != 1 {
		t.Errorf("unexpected map size %d", len(q.pendingDelete))
	}
}
