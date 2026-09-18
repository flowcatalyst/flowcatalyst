package nats

import "testing"

// A message's identity must be its STREAM sequence, which every delivery of it
// shares. Deriving either identity from the consumer sequence — which counts
// deliveries — made the router see each redelivery as a different copy of the
// message and ACK-delete it, so JetStream destroyed its own copy every time the
// ack-wait lapsed.
func TestIdentitiesAreStableAcrossRedeliveries(t *testing.T) {
	const streamSeq = 42

	// The same stream message, delivered three times: consumer sequence 1, 2, 3.
	first := brokerIDFor(streamSeq)
	for _, delivery := range []uint64{2, 3} {
		if got := brokerIDFor(streamSeq); got != first {
			t.Fatalf("delivery %d: broker id %q != %q — a redelivery must carry the same broker id",
				delivery, got, first)
		}
	}

	if got, want := brokerIDFor(streamSeq), "42"; got != want {
		t.Fatalf("broker id = %q, want %q (the stream sequence alone)", got, want)
	}
	if got, want := receiptFor("FLOWCATALYST", streamSeq), "FLOWCATALYST:42"; got != want {
		t.Fatalf("receipt = %q, want %q", got, want)
	}
}

// Distinct stream messages must stay distinguishable, or the router would treat
// unrelated messages as copies of one another.
func TestDistinctMessagesGetDistinctBrokerIDs(t *testing.T) {
	if brokerIDFor(1) == brokerIDFor(2) {
		t.Fatal("different stream sequences must yield different broker ids")
	}
}

// T14 (docs/spec/router-deferral-handback.md, R5): NATS answers false — a
// delay-bearing deferral must stay on the in-memory retry curve rather than
// being handed back to a broker that will neither block a group's
// successors on it (no per-group subject here) nor honour the delay before
// spending one of MaxDeliver's limited redeliveries on it.
func TestHonoursDelayedReturnIsFalse(t *testing.T) {
	q := &Queue{}
	if q.HonoursDelayedReturn() {
		t.Fatal("NATS must answer false — see queue.Consumer.HonoursDelayedReturn's doc comment")
	}
}
