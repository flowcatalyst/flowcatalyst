package router

// Guardrail suite for the message router. Run with: go test -race ./internal/router/ -run Guardrail
//
// These encode the operational contract the router must preserve and the
// concurrency invariants Go cannot enforce at compile time. The breaker now lives in the mediator, so the
// breaker-accounting guardrails target that layer; the pool guardrails assert
// resolution always fires (incl. panic) and that concurrent submit is race-free
// (`-race` turns a dropped lock into a hard failure here).

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// grConsumer is a thread-safe fake queue.Consumer that records terminal actions.
type grConsumer struct {
	id            string
	acks          atomic.Int64
	nacks         atomic.Int64
	defers        atomic.Int64
	lastNackDelay atomic.Pointer[uint32]
}

func (c *grConsumer) Identifier() string { return c.id }
func (c *grConsumer) Poll(ctx context.Context, max uint32) ([]common.QueuedMessage, error) {
	return nil, nil
}

func (c *grConsumer) Ack(ctx context.Context, receipt string, _ string) error {
	c.acks.Add(1)
	return nil
}

func (c *grConsumer) Nack(ctx context.Context, receipt string, delay *uint32) error {
	c.nacks.Add(1)
	c.lastNackDelay.Store(delay)
	return nil
}

func (c *grConsumer) Defer(ctx context.Context, receipt string, delay *uint32) error {
	c.defers.Add(1)
	return nil
}

func (c *grConsumer) Healthy() bool                                       { return true }
func (c *grConsumer) Stop()                                               {}
func (c *grConsumer) Metrics(ctx context.Context) (*queue.Metrics, error) { return nil, nil }
func (c *grConsumer) Counters() *queue.Metrics                            { return nil }
func (c *grConsumer) total() int64                                        { return c.acks.Load() + c.nacks.Load() + c.defers.Load() }

// grMediator is a Mediator that returns a fixed outcome (or panics). It does NOT
// consult a breaker — breaker behaviour is tested against the real HTTPMediator.
type grMediator struct {
	outcome  common.MediationOutcome
	panicMsg string
	called   atomic.Bool
}

func (m *grMediator) Mediate(ctx context.Context, msg *common.Message) common.MediationOutcome {
	m.called.Store(true)
	if m.panicMsg != "" {
		panic(m.panicMsg)
	}
	return m.outcome
}

func grPool(med Mediator, c queue.Consumer) *Pool {
	cfg := common.PoolConfig{Code: "TEST", Concurrency: 8}
	return NewPool(cfg, med, NewInFlightTracker(), func(string) queue.Consumer { return c })
}

func grMsg(id, endpoint string) common.QueuedMessage {
	return common.QueuedMessage{
		Message:         common.Message{ID: id, MediationTarget: endpoint, DispatchMode: common.DispatchImmediate},
		ReceiptHandle:   "rh-" + id,
		BrokerMessageID: "bk-" + id,
		QueueIdentifier: "q1",
	}
}

func grWaitFor(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !cond() {
		t.Fatalf("condition not met within %s", timeout)
	}
}

// --- Pool: resolution follows the in-pipeline-retry contract ---
//
// Terminal outcomes (2xx success, 4xx) ACK once and clear the in-flight entry.
// In-pipeline-retryable outcomes (429, deferred, panic) do NOT touch the
// broker — the message is retried in-pipeline — so processOne returns a
// BrokerRetry Disposition with a backoff and the consumer sees zero terminal
// actions. Unreachable-target outcomes (5xx/timeout, connection, circuit-open)
// RELEASE the message (and its group) back to the broker instead — see the
// tests below for those.

func TestGuardrail_ResolutionOnSuccess(t *testing.T) {
	c := &grConsumer{id: "q1"}
	p := grPool(&grMediator{outcome: common.Success(http.StatusOK)}, c)
	d := p.processOne(context.Background(), grMsg("evt_ok", "http://t/ok"))
	if d.Action != BrokerAck || d.Metric != MetricSuccess || c.acks.Load() != 1 || c.nacks.Load() != 0 || c.defers.Load() != 0 {
		t.Fatalf("success must ACK exactly once and report success; got action=%v metric=%v acks=%d nacks=%d defers=%d",
			d.Action, d.Metric, c.acks.Load(), c.nacks.Load(), c.defers.Load())
	}
}

// TestGuardrail_DiscardOn500 — a plain 500 is the app rejecting this message,
// not the app being unavailable. Under the R-57 boundary the mediator
// classifies every 5xx other than 502/503/504 as ErrorConfig (permanent,
// single attempt, no burst), so it is ACKed away and can be re-sent once
// whatever is wrong is resolved.
func TestGuardrail_DiscardOn500(t *testing.T) {
	c := &grConsumer{id: "q1"}
	out := common.ErrorConfig(http.StatusInternalServerError, "HTTP 500: Server error")
	p := grPool(&grMediator{outcome: out}, c)

	d := p.processOne(context.Background(), grMsg("evt_500", "http://t/500"))

	if d.Action != BrokerAck || d.Metric != MetricFailure {
		t.Fatalf("a 500 surviving the burst must be a terminal FAILURE (ack + MetricFailure), "+
			"which is what lets an ordered group tell BLOCK_ON_ERROR from NEXT_ON_ERROR; got action=%v metric=%v", d.Action, d.Metric)
	}
	if c.acks.Load() != 1 || c.nacks.Load() != 0 {
		t.Fatalf("a 500 must ACK exactly once and never NACK; got acks=%d nacks=%d",
			c.acks.Load(), c.nacks.Load())
	}
}

// TestGuardrail_AckOn501 — 501 is the deliberate exception among 5xx codes. The
// others mean "couldn't reach a working app" and are released to be retried
// until it recovers; 501 means the app WAS reached and does not implement this
// endpoint, which retrying cannot fix. So it is terminal like a 4xx.
//
// The mediator classifies it as ErrorConfig, which is what makes it ACK — this
// pins that, so a future rework of the 5xx branch can't quietly fold 501 into
// the release path and leave a permanently-failing message cycling forever.
func TestGuardrail_AckOn501(t *testing.T) {
	c := &grConsumer{id: "q1"}
	p := grPool(&grMediator{outcome: common.ErrorConfig(http.StatusNotImplemented, "HTTP 501: Not implemented")}, c)

	d := p.processOne(context.Background(), grMsg("evt_501", "http://t/501"))

	if d.Action != BrokerAck || d.Metric != MetricFailure {
		t.Fatalf("501 must be a terminal failure, not released or retried; got action=%v metric=%v", d.Action, d.Metric)
	}
	if c.acks.Load() != 1 || c.nacks.Load() != 0 {
		t.Fatalf("501 must ACK exactly once and never NACK; got acks=%d nacks=%d",
			c.acks.Load(), c.nacks.Load())
	}
}

// TestGuardrail_ReleaseOnUnreachable — 502/503/504 and any 5xx that isn't a
// plain 500 mean we never reached a working app. Nothing about the message is
// wrong, so it goes back to the broker rather than being discarded or pinned
// in-process for the length of the outage.
func TestGuardrail_ReleaseOnUnreachable(t *testing.T) {
	for _, status := range []int{http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout} {
		c := &grConsumer{id: "q1"}
		out := common.ErrorProcess(30, "gateway")
		out.StatusCode = status
		p := grPool(&grMediator{outcome: out}, c)

		d := p.processOne(context.Background(), grMsg("evt_5xx", "http://t/5xx"))

		if d.Action != BrokerRelease {
			t.Fatalf("status %d must release to the broker; got action=%v", status, d.Action)
		}
		if c.acks.Load() != 0 {
			t.Fatalf("status %d must never ACK — the message is not resolved; got acks=%d",
				status, c.acks.Load())
		}
	}
}

// TestGuardrail_ReleaseOnConnectionError — a transport failure is the same class
// as a gateway error: the target is down and the message is blameless.
func TestGuardrail_ReleaseOnConnectionError(t *testing.T) {
	c := &grConsumer{id: "q1"}
	p := grPool(&grMediator{outcome: common.ErrorConnection("dial tcp: refused")}, c)

	d := p.processOne(context.Background(), grMsg("evt_conn", "http://t/conn"))

	if d.Action != BrokerRelease {
		t.Fatalf("connection error must release to the broker; got action=%v", d.Action)
	}
	if c.acks.Load() != 0 {
		t.Fatalf("connection error must never ACK; got acks=%d", c.acks.Load())
	}
}

// TestGuardrail_ReleaseOnCircuitOpen — circuit-open MUST release, exactly as a
// transport failure does. The breaker opens almost immediately in a sustained
// outage, so retrying it in-pipeline would mean: first failure releases the
// group, the broker redelivers, the redelivery meets an open breaker and is
// pinned in memory again — reinstating the pinning the release exists to
// prevent, in the very scenario it exists for.
func TestGuardrail_ReleaseOnCircuitOpen(t *testing.T) {
	c := &grConsumer{id: "q1"}
	p := grPool(&grMediator{outcome: common.CircuitOpen(5)}, c)

	d := p.processOne(context.Background(), grMsg("evt_cb", "http://t/cb"))

	if d.Action != BrokerRelease {
		t.Fatalf("circuit-open must release to the broker; got action=%v", d.Action)
	}
	if c.acks.Load() != 0 {
		t.Fatalf("circuit-open must never ACK; got acks=%d", c.acks.Load())
	}
}

// TestGuardrail_RateLimitStillRetriesInPipeline — 429 and 2xx+ack=false are the
// outcomes that still retry in place: the target answered, so it is reachable
// and healthy, just asking for later. Releasing those would surrender our
// backoff curve to the broker for no reason.
func TestGuardrail_RateLimitStillRetriesInPipeline(t *testing.T) {
	c := &grConsumer{id: "q1"}
	p := grPool(&grMediator{outcome: common.RateLimited(5)}, c)

	d := p.processOne(context.Background(), grMsg("evt_429", "http://t/429"))

	if d.Action != BrokerRetry || c.total() != 0 {
		t.Fatalf("429 must retry in-pipeline with no broker action; got action=%v terminal=%d",
			d.Action, c.total())
	}
	if d.RetryAfter < 5*time.Second {
		t.Fatalf("429 backoff must honour the Retry-After floor; got %v", d.RetryAfter)
	}
}

func TestGuardrail_RetryOnPanic(t *testing.T) {
	// A panic mid-mediation must be recovered, NOT crash the process, and be
	// retried in-pipeline (the in-flight entry is kept) — processOne recovers
	// internally and returns BrokerRetry with no broker action.
	c := &grConsumer{id: "q1"}
	p := grPool(&grMediator{panicMsg: "boom"}, c)
	d := p.processOne(context.Background(), grMsg("evt_panic", "http://t/panic"))
	if d.Action != BrokerRetry {
		t.Fatalf("a panic mid-mediation must be recovered and retried in-pipeline; got action=%v", d.Action)
	}
	if c.total() != 0 {
		t.Fatalf("a recovered panic must not produce a terminal broker action; got %d", c.total())
	}
}

// --- Pool (marquee): the data-race surface under contention ---

// Hammer submit() from many goroutines across both dispatch paths (IMMEDIATE
// goroutine-per-message + ordered per-group drainers) and overlapping groups.
// Exercises groupQs (p.mu), the concurrency semaphore and the mediating set
// concurrently. Under -race, any future edit that drops a lock fails here (or
// panics on concurrent map write). All messages succeed here, so the invariant
// is: every submitted message is ACKed exactly once (no loss, no double-ack).
func TestGuardrail_ConcurrentSubmitNoRaceAndResolvesEach(t *testing.T) {
	const n = 600
	c := &grConsumer{id: "q1"}
	p := grPool(&grMediator{outcome: common.Success(http.StatusOK)}, c)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m := grMsg(fmt.Sprintf("evt_%d", i), "http://t/x")
			m.BatchID = fmt.Sprintf("b_%d", i%8)
			if i%2 == 0 {
				g := fmt.Sprintf("grp_%d", i%5)
				m.Message.MessageGroupID = &g
				m.Message.DispatchMode = common.DispatchMode("BLOCK_ON_ERROR") // ordered path
			}
			p.submit(ctx, m)
		}(i)
	}
	wg.Wait()

	grWaitFor(t, func() bool { return c.total() == n }, 10*time.Second)
	if c.total() != int64(n) {
		t.Fatalf("every message must resolve exactly once; got %d of %d", c.total(), n)
	}
}

// --- Mediator: breaker accounting now lives here ---

// A 4xx means the endpoint is reachable, so it must record a circuit-breaker
// SUCCESS (it must not trip the breaker, and must let an open breaker recover).
// Centralising the recording in the mediator removes the bug class where a pool
// switch arm forgot to record.
func TestGuardrail_BreakerRecordsSuccessOn4xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	br := NewBreakerRegistry(DefaultBreakerConfig())
	med := NewHTTPMediator(DevMediatorConfig(), br)
	defer med.Close()

	out := med.Mediate(context.Background(),
		&common.Message{ID: "evt_4xx", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL})

	if out.Result != common.MediationErrorConfig {
		t.Fatalf("404 must classify as ErrorConfig; got %v", out.Result)
	}
	if s := br.Get(srv.URL).Stats().Successes; s != 1 {
		t.Fatalf("ErrorConfig (4xx) must record a circuit-breaker success (reachable); got successes=%d", s)
	}
}

// An open breaker short-circuits in the mediator: no HTTP is attempted and a
// MediationCircuitOpen outcome is returned for the pool to DEFER.
func TestGuardrail_MediatorShortCircuitsWhenOpen(t *testing.T) {
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	br := NewBreakerRegistry(DefaultBreakerConfig())
	for i := 0; i < 10; i++ { // trip: 10 failures @ 100% >= 0.5 threshold, MinCalls 10
		br.Get(srv.URL).RecordFailure()
	}
	med := NewHTTPMediator(DevMediatorConfig(), br)
	defer med.Close()

	out := med.Mediate(context.Background(),
		&common.Message{ID: "evt_open", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL})

	if out.Result != common.MediationCircuitOpen {
		t.Fatalf("open breaker must yield MediationCircuitOpen; got %v", out.Result)
	}
	if hits.Load() != 0 {
		t.Fatalf("open breaker must NOT hit the endpoint; got %d hits", hits.Load())
	}
}

// Config-class responses (400/401/403/404 → Error, 501 → Critical) must surface
// as Configuration warnings on the WarningService when one is wired, so they
// appear on /warnings and a 501 degrades health.
func TestGuardrail_ConfigErrorsSurfaceAsWarnings(t *testing.T) {
	cases := []struct {
		status int
		sev    WarningSeverity
	}{
		{http.StatusNotFound, WarningError},
		{http.StatusNotImplemented, WarningCritical},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))
		ws := NewWarningService(DefaultWarningServiceConfig())
		med := NewHTTPMediator(DevMediatorConfig(), NewBreakerRegistry(DefaultBreakerConfig()))
		med.SetWarnings(ws)

		med.Mediate(context.Background(),
			&common.Message{ID: "m", MediationType: common.MediationTypeHTTP, MediationTarget: srv.URL})

		found := false
		for _, w := range ws.BySeverity(tc.sev) {
			if w.Category == WarningCategoryConfiguration {
				found = true
			}
		}
		if !found {
			t.Fatalf("status %d must record a %s Configuration warning", tc.status, tc.sev)
		}
		med.Close()
		srv.Close()
	}
}
