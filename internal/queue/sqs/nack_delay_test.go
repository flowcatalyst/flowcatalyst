//go:build integration

package sqs

// Nack honours the delay (R3, owner ruling 2026-09-17,
// docs/spec/router-deferral-handback.md), ported from the Java reference
// (flowcatalyst-javalin 5420b516, SqsQueueTest#nackChangesVisibilityToTheRequestedDelay
// / #nackClampsToTwelveHoursSinceTheOriginalPoll / #nackNeverThrowsOnChangeVisibilityFailure).
//
// These drive a real SQS round trip against LocalStack (see
// sqs_localstack_test.go's newLocalstackQueueVis) rather than a fake client:
// this package has no injectable AWS client (the field is the concrete SDK
// *sqs.Client, matching this codebase's existing choice), so — as with
// every other SQS behaviour test here — the observable effect is asserted
// against the real service rather than a mocked call.
//
// Every test below uses a queue with default VisibilityTimeout=nackTestQueueDefaultVisibility
// seconds — deliberately much larger than anything a delay-value assertion
// here waits for, and deliberately NOT 0 (a message received with
// VisibilityTimeout=0 has no invisibility window to extend in the first
// place, so ChangeMessageVisibility on that receipt is a no-op regardless of
// what this code does — that shape cannot tell Nack apart from a no-op
// Nack). With a large-but-finite default, "the message reappeared well
// before the default would have lapsed" is only true because Nack's
// requested delay — not the receive-time default — governs, which is
// exactly what R3 claims.
//
// The 12-hour clamp is exercised by directly backdating receiptPolledAt
// (white-box, same style as pending_delete_test.go's TTL tests) rather than
// injecting a fake clock: this package has no Clock abstraction, and Go's
// existing tests here already manipulate map timestamps directly.

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

func secs(n uint32) *uint32 { return &n }

// nackTestQueueDefaultVisibility is comfortably longer than any window these
// tests poll within, so "it reappeared" can only mean Nack's own requested
// delay governed, not the receive-time default lapsing on its own.
const nackTestQueueDefaultVisibility = 20

// T6: Nack must actually change the message's visibility rather than being a
// no-op. Nacking with a 2s delay must make the message reappear at ~2s —
// far short of the queue's 20s default — which a no-op Nack (the pre-R3
// behaviour) could not produce: the message would stay hidden for the full
// 20s default instead.
func TestNackChangesVisibilityToTheRequestedDelay(t *testing.T) {
	ctx := context.Background()
	q := newLocalstackQueueVis(t, "fc-nack-delay-test", nackTestQueueDefaultVisibility)

	_, err := q.Publish(ctx, common.Message{
		ID: "evt-nack", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/nack",
	})
	require.NoError(t, err)

	msgs := pollUntil(t, q, 1, 5*time.Second)
	require.Len(t, msgs, 1)
	qm := msgs[0]

	require.NoError(t, q.Nack(ctx, qm.ReceiptHandle, secs(2)))

	reappeared := pollUntil(t, q, 1, 4*time.Second)
	require.Len(t, reappeared, 1,
		"the message must reappear at ~2s — a no-op Nack would leave it hidden for the queue's 20s default instead")

	require.Equal(t, uint64(1), q.Counters().TotalNacked, "the nacked counter still increments, same as before R3")
}

// T7: the delay is clamped to SQS's 12-hour ceiling, measured from when THIS
// consumer originally polled the receipt — not from this Nack call. Backdate
// the receipt's recorded poll time so only ~2s of the ceiling remains, then
// request a delay far larger than that (also far larger than
// ChangeMessageVisibility's own 43,200s parameter range, so a dropped clamp
// either gets rejected outright by AWS's own validation or, if accepted,
// keeps the message invisible far longer than this test waits — either way
// it does NOT reappear within the ~2s the clamp actually allows).
func TestNackClampsToTheSQSCeilingMeasuredFromTheOriginalPoll(t *testing.T) {
	ctx := context.Background()
	q := newLocalstackQueueVis(t, "fc-nack-clamp-test", nackTestQueueDefaultVisibility)

	_, err := q.Publish(ctx, common.Message{
		ID: "evt-clamp", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/clamp",
	})
	require.NoError(t, err)

	msgs := pollUntil(t, q, 1, 5*time.Second)
	require.Len(t, msgs, 1)
	qm := msgs[0]

	q.mu.Lock()
	q.receiptPolledAt[qm.ReceiptHandle] = time.Now().Add(-(MaxVisibility - 2*time.Second))
	q.mu.Unlock()

	require.NoError(t, q.Nack(ctx, qm.ReceiptHandle, secs(50_000)))

	reappeared := pollUntil(t, q, 1, 4*time.Second)
	require.Len(t, reappeared, 1,
		"the clamp must have reduced the effective delay to ~2s remaining on the 12h ceiling, not the raw 50,000s requested")
}

// T7b: a receipt this consumer never recorded a poll time for (evicted, or
// never seen) gets the FULL ceiling rather than an arbitrarily short one —
// there is nothing here to say it should be shorter. An "absent entry means
// ZERO remaining" bug would clamp even a modest requested delay down to an
// immediate ChangeMessageVisibility(0) — the 300ms check below, well short
// of the 2s actually requested, is what would catch exactly that (0s and 2s
// are both far shorter than the 20s default, so only comparing against the
// REQUESTED value, not the default, can tell them apart).
func TestNackWithNoRecordedPollTimeGetsTheFullCeilingNotZero(t *testing.T) {
	ctx := context.Background()
	q := newLocalstackQueueVis(t, "fc-nack-no-poll-record-test", nackTestQueueDefaultVisibility)

	_, err := q.Publish(ctx, common.Message{
		ID: "evt-no-record", MediationType: common.MediationTypeHTTP, MediationTarget: "http://t/no-record",
	})
	require.NoError(t, err)

	msgs := pollUntil(t, q, 1, 5*time.Second)
	require.Len(t, msgs, 1)
	qm := msgs[0]

	// Simulate an evicted/never-recorded entry.
	q.mu.Lock()
	delete(q.receiptPolledAt, qm.ReceiptHandle)
	q.mu.Unlock()

	require.NoError(t, q.Nack(ctx, qm.ReceiptHandle, secs(2)))

	time.Sleep(300 * time.Millisecond)
	stillHidden, err := q.Poll(ctx, 10)
	require.NoError(t, err)
	require.Empty(t, stillHidden,
		"an unrecorded receipt clamped to ZERO remaining would already be visible at 300ms, well short of the requested 2s")

	reappeared := pollUntil(t, q, 1, 4*time.Second)
	require.Len(t, reappeared, 1, "the message must still reappear once its requested 2s delay elapses")
}

// T8: Nack must never return an error, even when the underlying
// ChangeMessageVisibility call fails (e.g. a receipt handle SQS no longer
// recognises) — the Acknowledger/Consumer contract every backend keeps is
// best-effort: log and swallow, so the message simply returns at its
// natural visibility timeout. The nacked counter still increments.
func TestNackNeverErrorsWhenChangeMessageVisibilityFails(t *testing.T) {
	ctx := context.Background()
	q := newLocalstackQueueVis(t, "fc-nack-invalid-receipt-test", nackTestQueueDefaultVisibility)

	before := q.Counters().TotalNacked

	err := q.Nack(ctx, "garbage-receipt-handle-does-not-exist", secs(30))
	require.NoError(t, err, "Nack must swallow a ChangeMessageVisibility failure, not return it")

	require.Equal(t, before+1, q.Counters().TotalNacked, "the counter increments regardless of the AWS call's outcome")
}
