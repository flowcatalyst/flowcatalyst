package router

// Deferral hand-back (owner ruling 2026-09-17, R1-R5,
// docs/spec/router-deferral-handback.md), ported from the Java reference
// implementation (flowcatalyst-javalin 5420b516 + 7759059a). T-numbers below
// are the doc's own numbering, carried through its "Go port" table.
//
// Go's DispositionOf is the single, pure decision function BOTH dispatch
// paths (runImmediate for IMMEDIATE messages, drainGroup for ordered groups)
// funnel through — unlike Java, which decides the ordered case in a
// separate OrderedGroups.onHeadFailure. That is what makes R2 ("the
// returned group's head carries its real delay, not a fixed value") fall
// out of R1 for free here: releaseGroup nacks the head with the exact same
// Disposition.RetryAfter runImmediate would have used for the identical
// outcome, because they are the same value, computed once. There is no
// separate "head gets a fixed 10s regardless of the outcome" bug to fix in
// Go the way there was in Java, so this file's ordered-path test
// (TestOrderedHeadDeferralWithDelayReturnsWholeGroupWithHeadsRealDelay)
// pins the observable consequence directly rather than a Go analogue of a
// defect Go's shape doesn't have.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// deferredWithDelay builds the MediationDeferred outcome a target's 2xx
// {"ack":false,"delaySeconds":N} produces (see mediator.go).
func deferredWithDelay(seconds int) *common.MediationOutcome {
	out := common.Deferred(seconds, "come back later")
	return &out
}

// ---- pure DispositionOf tests: exact decision, no I/O, no sleep ----------

// T1/T2: a delay-bearing deferral, on a broker that honours a delayed
// return, is released on ITS VERY FIRST occurrence (attempts=0 — no budget
// spent first) and carries the EXACT requested delay, never the deferred
// curve's floor-then-cap. Checked at attempts=0 AND near the budget's
// exhaustion to pin that the budget is skipped entirely, not merely
// deferred — a mutant that only removed the early-release but left
// retryOrRelease's own eventual release in place would still show
// BrokerRelease at attempts=9, so both rows must show the SAME exact
// RetryAfter for this to mean anything.
func TestDispositionOfDeferredWithDelayReleasesOnFirstOccurrenceWithExactDelay(t *testing.T) {
	outcome := *deferredWithDelay(600)

	for _, attempts := range []uint{0, maxInPipelineAttempts - 1} {
		d := DispositionOf(outcome, attempts, common.DispatchImmediate, true)

		assert.Equal(t, BrokerRelease, d.Action,
			"attempts=%d: a mutant restoring the in-memory retry would report BrokerRetry here", attempts)
		assert.Equal(t, GroupRelease, d.Group)
		assert.Equal(t, 600*time.Second, d.RetryAfter,
			"attempts=%d: T2 — a mutant routing the delay through the deferred curve/cap "+
				"would report 60s (deferredMaxDelay), not the exact 600s requested", attempts)
	}
}

// T3: a deferral naming NO delay is completely unaffected by R1 — it keeps
// retrying in-pipeline within the ordinary budget, exactly as before this
// ruling landed.
func TestDispositionOfDeferredWithNoDelayIsUnchanged(t *testing.T) {
	outcome := *deferredWithDelay(0)

	within := DispositionOf(outcome, 0, common.DispatchImmediate, true)
	assert.Equal(t, BrokerRetry, within.Action,
		"delaySeconds==0 must still retry in-pipeline — a mutant applying R1 regardless of delaySeconds "+
			"would report BrokerRelease here")

	spent := DispositionOf(outcome, maxInPipelineAttempts-1, common.DispatchImmediate, true)
	assert.Equal(t, BrokerRelease, spent.Action, "the ordinary budget still applies once exhausted")
}

// T12/T13 (decision level): R5 — a delay-bearing deferral is handed back
// ONLY when the source broker actually honours a delayed return. NATS does
// not (queue.Consumer.HonoursDelayedReturn() == false for NatsQueue), so
// there the deferral must stay on the in-memory curve exactly as it did
// before R1 existed — for BOTH dispatch paths, since both funnel through
// this one function.
func TestDispositionOfDeferredWithDelayStaysInMemoryWhenBrokerDoesNotHonourIt(t *testing.T) {
	outcome := *deferredWithDelay(600)

	for _, mode := range []common.DispatchMode{common.DispatchImmediate, common.DispatchBlockOnError} {
		d := DispositionOf(outcome, 0, mode, false)

		assert.Equal(t, BrokerRetry, d.Action,
			"mode=%s: a mutant dropping honoursDelayedReturn from the condition would report BrokerRelease here", mode)
	}
}

// ---- end-to-end tests: the observable broker effect, via a fake consumer -

// T1 (observable): an IMMEDIATE message whose target defers with a delay is
// hit exactly once — if R1 ever regressed to the in-memory retry, the
// mediator would be called repeatedly (up to maxInPipelineAttempts) before
// any nack, which is the actual bug this ruling exists to fix.
func TestImmediateDeferralWithDelayIsHandedBackOnce(t *testing.T) {
	cons := &cascadeConsumer{wantTotal: 1, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m1", failWith: deferredWithDelay(600)}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	pool.submit(context.Background(), common.QueuedMessage{
		Message: common.Message{
			ID: "m1", MediationType: common.MediationTypeHTTP,
			MediationTarget: "http://example.invalid", DispatchMode: common.DispatchImmediate,
		},
		ReceiptHandle: "rh-m1",
	})

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the message was never handed back to the broker")
	}

	med.mu.Lock()
	seen := len(med.seen)
	med.mu.Unlock()
	assert.Equal(t, 1, seen, "handed back on its first occurrence — never retried in-pipeline")

	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()
	assert.Equal(t, []string{"rh-m1"}, nacked)
	assert.Empty(t, acked)
	if delay := cons.nackDelayFor("rh-m1"); assert.NotNil(t, delay, "T2: a delay must have been supplied") {
		assert.Equal(t, uint32(600), *delay, "the exact requested delay, uncapped and uncurved")
	}
}

// T4 (observable): an ordered group's HEAD defers with a delay — the whole
// group (head + untried siblings) goes back to the broker on the head's
// FIRST attempt, the siblings are never delivered, and the head's own nack
// carries its real delay (R2 — see this file's header comment for why that
// falls out of R1 automatically in Go's design).
func TestOrderedHeadDeferralWithDelayReturnsWholeGroupWithHeadsRealDelay(t *testing.T) {
	group := "g"
	cons := &cascadeConsumer{wantTotal: 3, done: make(chan struct{})}
	med := &cascadeMediator{failID: "m0", failWith: deferredWithDelay(600)}
	pool := newCascadePool(med, func(string) queue.Consumer { return cons })

	pool.submit(context.Background(), mkOrdered("m0", &group))
	pool.submit(context.Background(), mkOrdered("m1", &group))
	pool.submit(context.Background(), mkOrdered("m2", &group))

	select {
	case <-cons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the group was never released")
	}

	med.mu.Lock()
	seen := append([]string(nil), med.seen...)
	med.mu.Unlock()
	assert.Equal(t, []string{"m0"}, seen, "siblings must never be attempted against a head already deferred")

	cons.mu.Lock()
	nacked := append([]string(nil), cons.nacked...)
	acked := append([]string(nil), cons.acked...)
	cons.mu.Unlock()
	assert.ElementsMatch(t, []string{"m0", "m1", "m2"}, nacked, "the whole group returns to the broker")
	assert.Empty(t, acked)

	if delay := cons.nackDelayFor("m0"); assert.NotNil(t, delay, "the head must carry a delay") {
		assert.Equal(t, uint32(600), *delay, "the head carries the deferral's exact delay, not a fixed value")
	}
}

// T5 (observable, Go-shaped): the ordered head's nack delay for an
// UNAVAILABLE target (not a deferral) is exactly what the unordered path
// would use for the SAME outcome — pinning, cross-path, that there is no
// separate fixed delay hard-coded for the ordered head the way Java's
// REJECTED_NACK_DELAY was. A future regression that special-cased the
// ordered head to some other constant would desynchronise these two values.
func TestHeadNackDelayMatchesWhatTheUnorderedPathWouldUseForTheSameOutcome(t *testing.T) {
	outcome := common.ErrorProcess(30, "unavailable")

	immediateCons := &cascadeConsumer{wantTotal: 1, done: make(chan struct{})}
	immediateMed := &cascadeMediator{failID: "solo", failWith: &outcome}
	immediatePool := newCascadePool(immediateMed, func(string) queue.Consumer { return immediateCons })
	immediatePool.submit(context.Background(), common.QueuedMessage{
		Message: common.Message{
			ID: "solo", MediationType: common.MediationTypeHTTP,
			MediationTarget: "http://example.invalid", DispatchMode: common.DispatchImmediate,
		},
		ReceiptHandle: "rh-solo",
	})
	select {
	case <-immediateCons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the immediate message was never released")
	}

	group := "g"
	orderedCons := &cascadeConsumer{wantTotal: 1, done: make(chan struct{})}
	orderedMed := &cascadeMediator{failID: "m0", failWith: &outcome}
	orderedPool := newCascadePool(orderedMed, func(string) queue.Consumer { return orderedCons })
	orderedPool.submit(context.Background(), mkOrdered("m0", &group))
	select {
	case <-orderedCons.done:
	case <-time.After(3 * time.Second):
		t.Fatal("the ordered head was never released")
	}

	immediateDelay := immediateCons.nackDelayFor("rh-solo")
	headDelay := orderedCons.nackDelayFor("m0")
	require.Equal(t, immediateDelay == nil, headDelay == nil, "both paths must agree on whether a delay was even sent")
	if immediateDelay != nil {
		assert.Equal(t, *immediateDelay, *headDelay,
			"the ordered head must carry exactly the delay the unordered path uses for this outcome")
	}
}
