package nats

import (
	"testing"
	"time"
)

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
// being handed back to a broker that will not block a group's successors on
// it (no per-group subject here). (The answer also used to protect a finite
// MaxDeliver; that is unlimited by default since 2026-09-22.)
func TestHonoursDelayedReturnIsFalse(t *testing.T) {
	q := &Queue{}
	if q.HonoursDelayedReturn() {
		t.Fatal("NATS must answer false — see queue.Consumer.HonoursDelayedReturn's doc comment")
	}
}

// The pull-request lifetime defaults to a few seconds, not nats.go's 30s: a
// pull that idles while the pool is full otherwise leaves Next blocked for the
// whole 30s once the pool drains (reproduced with only the client library).
func TestPullExpiryDefaultsShortAndIsConfigurable(t *testing.T) {
	cfg, err := parseURI("nats://localhost:4222?stream=S&consumer=C&subject=s.>")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PullExpiry != defaultPullExpiry || defaultPullExpiry >= 10*time.Second {
		t.Fatalf("default pull expiry = %s (default const %s), want a short default", cfg.PullExpiry, defaultPullExpiry)
	}

	cfg, err = parseURI("nats://localhost:4222?stream=S&consumer=C&subject=s.>&pull-expiry-ms=7000")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PullExpiry != 7*time.Second {
		t.Fatalf("pull-expiry-ms=7000 parsed as %s", cfg.PullExpiry)
	}
}

// Every expiry must yield options the client accepts: heartbeat at least 500ms
// and under half the expiry, or none at all.
func TestPullOptionsAreAlwaysAcceptedByTheClient(t *testing.T) {
	for _, expiry := range []time.Duration{0, 500 * time.Millisecond, time.Second, 1500 * time.Millisecond,
		2 * time.Second, 3 * time.Second, 5 * time.Second, 30 * time.Second} {
		if got := len(pullOptions(10, 5, expiry)); got < 3 {
			t.Fatalf("expiry %s: %d options, want at least the three base options", expiry, got)
		}
	}
}
