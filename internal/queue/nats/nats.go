// Package nats is the NATS JetStream queue backend.
//
//   - Continuous pull-based JetStream subscription (owner ruling), not a
//     per-poll Fetch: the client keeps one jetstream.Consumer.Consume
//     callback subscription open for the life of the consumer, bounded
//     client-side to about one in-flight batch (PullMaxMessages + nats.go's
//     own default threshold, half of max-messages, so the next batch is
//     requested while the current one is still half full rather than
//     after a broker round trip idles the workers — client-side
//     residency stays ≤1.5 batches, which is what "about one batch"
//     means in practice; see G15). The handler pushes each message into a
//     channel of capacity max-messages (blocking when full — the
//     back-pressure signal); Poll blocks on that channel rather than
//     issuing a broker round-trip.
//
//     G16 (owner ruling 2026-09-07): Consume, not Messages/Next. Both
//     multiplex the same underlying pull mechanics, but Messages/Next only
//     issues its next pull request from INSIDE a call to Next — so a
//     forwarder that calls Next only when it has downstream room (the
//     Messages-based design this replaced) stops asking the broker for
//     more the moment downstream is briefly saturated, even though
//     nothing else is wrong. On one CPU, 256 workers filling their buffer
//     was enough to park the poll loop, which stopped Next calls, which
//     stopped pull requests, until the outstanding one expired and the
//     per-Next heartbeat monitor timed out — five single-queue "no
//     heartbeat" recovery cycles at ~3k/s where 2 CPUs (workers draining
//     fast enough to keep calling Next) saw zero. Consume's pull-request
//     lifecycle, expiry handling and heartbeat monitoring run in the
//     library's own goroutine, entirely decoupled from whether OUR
//     handler is currently blocked on a full channel — matching how
//     nats.java and async-nats structure their consumers, which is why
//     they don't show this pathology on the same row.
//
//   - WorkQueue retention (messages removed after ack).
//
//   - Durable consumer auto-provisioned at startup.
//
//   - Receipt handles are `streamName:streamSequence` (the historical
//     receipt format).
//
//   - Defer maps to NAK-with-delay (same as Nack with delay >0).
//
// URI scheme: `nats://host:port` (optionally with comma-separated hosts).
// Stream + consumer + subject come from query params:
//
//	nats://localhost:4222?stream=FLOWCATALYST&consumer=fc-router&subject=flowcatalyst.>
//
// Defaults: stream=FLOWCATALYST, consumer=fc-router,
// subject=flowcatalyst.>, max-messages=10, poll-timeout=20s, ack-wait=120s,
// max-deliver=10, max-ack-pending=1000, storage=file, replicas=1,
// max-age-days=7.
//
// poll-timeout-ms is accepted for URI compatibility but UNUSED by this
// backend: Poll no longer issues a timed Fetch. It blocks untimed on the
// subscription's channel, bounded only by the CALLER's context — which is
// what "continuous subscription, not a poller" means in practice — see
// Poll below. Because it has no poll-timeout of its own, the caller's
// context deadline lapsing is Poll's only "nothing arrived" signal, and
// Poll reports that as a plain empty result (nil, nil), not an error —
// see G13 in Poll's doc comment for why that distinction matters to the
// router's poll loop and stall detector.
package nats

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	natsgo "github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

func init() {
	queue.RegisterConsumer("nats", consumerFactory)
	queue.RegisterPublisher("nats", publisherFactory)
}

// Config controls the NATS connection + stream provisioning.
type Config struct {
	Servers            string // comma-separated nats URLs
	StreamName         string
	ConsumerName       string
	Subject            string
	MaxMessagesPerPoll int
	PollTimeout        time.Duration
	AckWait            time.Duration
	MaxDeliver         int
	MaxAckPending      int
	Storage            string // "file" | "memory"
	Replicas           int
	MaxAge             time.Duration // 0 = unlimited
}

// DefaultConfig returns the standard NATS defaults.
func DefaultConfig() Config {
	return Config{
		Servers:            "nats://localhost:4222",
		StreamName:         "FLOWCATALYST",
		ConsumerName:       "fc-router",
		Subject:            "flowcatalyst.>",
		MaxMessagesPerPoll: 10,
		PollTimeout:        20 * time.Second,
		AckWait:            120 * time.Second,
		MaxDeliver:         10,
		MaxAckPending:      1000,
		Storage:            "file",
		Replicas:           1,
		MaxAge:             7 * 24 * time.Hour,
	}
}

func consumerFactory(ctx context.Context, cfg common.QueueConfig) (queue.Consumer, error) {
	q, err := newQueue(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return q, nil
}

func publisherFactory(ctx context.Context, cfg common.QueueConfig) (queue.Publisher, error) {
	q, err := newQueue(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return q, nil
}

// Queue is a NATS JetStream-backed queue (consumer + publisher).
type Queue struct {
	cfg        Config
	identifier string

	nc       *natsgo.Conn
	js       jetstream.JetStream
	consumer jetstream.Consumer

	// consumeCtxMu guards consumeCtx: handleConsumeErr replaces it on
	// resubscribe (from a goroutine it starts), Stop() reads it to unwind
	// the current one, and both can run concurrently.
	consumeCtxMu sync.Mutex
	// consumeCtx is the current Consume() subscription. The library's own
	// goroutines own its pull-request lifecycle end to end (issuing,
	// expiry, heartbeats) and invoke handleMsg/handleConsumeErr; nothing
	// here calls Next or otherwise drives it. Replaced (not reopened in
	// place) when handleConsumeErr sees a terminal error — see resubscribe
	// and G16 above.
	consumeCtx jetstream.ConsumeContext

	// resubscribe opens a fresh Consume() subscription with the same
	// options as the original. A field (not a hardcoded call) so tests can
	// substitute one that fails, to exercise the "can't recover" path
	// deterministically.
	resubscribe func() (jetstream.ConsumeContext, error)

	// resubscribing guards against handleConsumeErr starting more than one
	// concurrent resubscribe attempt — it can fire multiple times for the
	// same underlying failure (the library calls it once per error, and
	// errors can repeat while a subscription is going down).
	resubscribing atomic.Bool

	// healthy is false from the moment handleConsumeErr sees a terminal
	// error until a resubscribe succeeds. Poll consults it: a
	// caller-deadline lapse while healthy means "genuinely idle, nothing to
	// report" (nil, nil); while unhealthy it means "the subscription is
	// broken and a resubscribe is in progress", surfaced as an error so the
	// router's normal poll-error handling (and, if resubscribing keeps
	// failing, the stall watchdog) can see it. See G14; the trigger
	// condition is narrower now (terminal errors only — see
	// isTerminalConsumeErr) because Consume self-recovers from the
	// transient ones (a missed heartbeat, a reconnect) without ending the
	// subscription at all.
	healthy atomic.Bool
	// unhealthyMsg is the most recent terminal error, for Poll's error
	// text. Plain string (not the error itself) so a stale *error can't be
	// read back after being reused/wrapped elsewhere.
	unhealthyMsg atomic.Value

	// msgCh is fed by handleMsg (invoked by the library's own delivery
	// goroutine, one message at a time, serially) and drained by Poll. Its
	// capacity is max-messages: handleMsg's send blocks once it's full,
	// which is the back-pressure signal — with Consume, that blocks
	// handleMsg, not the pull-request lifecycle (G16), so a saturated
	// downstream no longer stops the subscription asking the broker for
	// more.
	//
	// G15 (owner ruling 2026-09-07): the pull threshold is nats.go's own
	// default — half of max-messages — not "wait until fully drained".
	// PullThresholdMessages(1) (the original choice here) minimises
	// client-side residency in the abstract, but it means the SERVER
	// round trip for the next batch doesn't even start until this one is
	// completely gone: on one CPU, with the workers otherwise idle
	// between batches, that round trip is pure dead time on every single
	// batch (measured: 10 messages per ~2.3ms RTT, 4,276/s). Requesting
	// the next batch once half of this one is consumed means the new
	// batch is usually already arriving by the time the old one runs out
	// — client-side residency rises to ≤1.5 batches instead of ≤1, which
	// is still what "about one batch, not an unbounded prefetch" means in
	// practice (see TestFullChannelStopsRequestingMoreBatches, whose
	// bound documents the same number). If nothing drains msgCh at all,
	// residency plateaus at two batches, exactly as before — the
	// threshold only changes WHEN the next request goes out, not the cap
	// on how many batches can ever be outstanding (msgCh's capacity is
	// still the only thing handleMsg will overrun before blocking).
	msgCh chan common.QueuedMessage

	// stopCh is closed by Stop to unblock a Poll parked on msgCh, or a
	// handleMsg call blocked sending to a full msgCh, without waiting for
	// the library to notice the subscription closed.
	stopCh   chan struct{}
	stopOnce sync.Once

	running atomic.Bool

	pendingMu sync.Mutex
	pending   map[string]jetstream.Msg

	totalPolled   atomic.Uint64
	totalAcked    atomic.Uint64
	totalNacked   atomic.Uint64
	totalDeferred atomic.Uint64
}

// pullThreshold mirrors nats.go's own default (jetstream.parseMessagesOpts:
// ceil(MaxMessages/2)) — see G15. Exported behaviour, not just an internal
// default: this is what "next batch requested at half-drained" means for
// any max-messages value, including the batch=1 edge case (threshold=1,
// i.e. every message is its own round trip — there's no "half" of one).
func pullThreshold(batch int) int {
	t := (batch + 1) / 2
	if t < 1 {
		t = 1
	}
	return t
}

func newQueue(ctx context.Context, qc common.QueueConfig) (*Queue, error) {
	cfg, err := parseURI(qc.URI)
	if err != nil {
		return nil, err
	}
	nc, err := natsgo.Connect(cfg.Servers,
		natsgo.Timeout(10*time.Second),
		natsgo.ReconnectWait(2*time.Second),
		natsgo.MaxReconnects(-1),
	)
	if err != nil {
		return nil, fmt.Errorf("nats: connect: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: jetstream: %w", err)
	}

	storage := jetstream.FileStorage
	if strings.EqualFold(cfg.Storage, "memory") {
		storage = jetstream.MemoryStorage
	}
	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      cfg.StreamName,
		Subjects:  []string{cfg.Subject},
		Retention: jetstream.WorkQueuePolicy,
		Storage:   storage,
		Replicas:  cfg.Replicas,
		MaxAge:    cfg.MaxAge,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: get/create stream %q: %w", cfg.StreamName, err)
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          cfg.ConsumerName,
		Durable:       cfg.ConsumerName,
		AckWait:       cfg.AckWait,
		MaxDeliver:    cfg.MaxDeliver,
		MaxAckPending: cfg.MaxAckPending,
		FilterSubject: cfg.Subject,
	})
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: get/create consumer %q: %w", cfg.ConsumerName, err)
	}

	batch := cfg.MaxMessagesPerPoll
	if batch <= 0 {
		batch = DefaultConfig().MaxMessagesPerPoll
	}
	// G15 (owner ruling 2026-09-07): use nats.go's own default threshold —
	// half of max-messages — rather than 1 (wait until fully drained). See
	// msgCh's doc comment for the measured reason (a 1-CPU single queue at
	// threshold=1 ran at 4,276/s: ten messages per broker round trip, then
	// idle workers waiting the RTT out). Requesting the next batch while
	// this one is still half full keeps client-side residency at ≤1.5
	// batches — still "about one batch", not an unbounded prefetch.
	threshold := pullThreshold(batch)

	q := &Queue{
		cfg:        cfg,
		identifier: cfg.StreamName + "/" + cfg.ConsumerName,
		nc:         nc,
		js:         js,
		consumer:   consumer,
		msgCh:      make(chan common.QueuedMessage, batch),
		stopCh:     make(chan struct{}),
		pending:    make(map[string]jetstream.Msg),
	}
	// A field, not a hardcoded call: the SAME closure is used for the
	// initial subscribe (right below) and every later resubscribe
	// (handleConsumeErr), so the options can never drift between the two.
	q.resubscribe = func() (jetstream.ConsumeContext, error) {
		return consumer.Consume(q.handleMsg,
			jetstream.PullMaxMessages(batch),
			jetstream.PullThresholdMessages(threshold),
			jetstream.ConsumeErrHandler(q.handleConsumeErr),
		)
	}
	consumeCtx, err := q.resubscribe()
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: open subscription: %w", err)
	}
	q.consumeCtx = consumeCtx
	q.healthy.Store(true)
	q.running.Store(true)
	return q, nil
}

// handleMsg is the Consume callback: the library's own delivery goroutine
// invokes it once per message, serially (never concurrently with itself).
// It converts the message and hands it to Poll via msgCh. A full msgCh
// blocks this call — the back-pressure signal — but that block is entirely
// ours: per G16, it does NOT reach into the library's pull-request
// lifecycle (issuing, expiry, heartbeats), which runs in Consume's own
// goroutine regardless of how long handleMsg takes to return.
func (q *Queue) handleMsg(msg jetstream.Msg) {
	qm, ok := q.toQueuedMessage(msg)
	if !ok {
		return
	}
	select {
	case q.msgCh <- qm:
	case <-q.stopCh:
	}
}

// isTerminalConsumeErr reports whether err means the ConsumeContext that
// reported it has stopped (or is about to) for good, rather than one of
// the errors Consume recovers from on its own (a missed heartbeat, a
// reconnect) without ending the subscription.
func isTerminalConsumeErr(err error) bool {
	return errors.Is(err, jetstream.ErrConnectionClosed) ||
		errors.Is(err, jetstream.ErrConsumerDeleted) ||
		errors.Is(err, jetstream.ErrBadRequest)
}

// handleConsumeErr is Consume's ConsumeErrHandler: the library calls it
// for every error it sees on the subscription, including ones it has
// already recovered from by the time this runs (a missed heartbeat
// triggers an automatic internal re-pull; a brief reconnect is likewise
// handled without our involvement). Only a TERMINAL error — one after
// which the library has stopped (or is stopping) this ConsumeContext for
// good — needs us to act: mark unhealthy and resubscribe with backoff in
// a fresh goroutine (never block here — this runs on the library's own
// event-loop goroutine, and blocking it would stall the ConsumeContext's
// own teardown). See G14 (why unhealthy must be surfaced through Poll) and
// G16 (why this trigger is narrower than the old Messages/Next one).
func (q *Queue) handleConsumeErr(_ jetstream.ConsumeContext, err error) {
	if !q.running.Load() {
		return // Stop() was called; nothing to recover.
	}
	if !isTerminalConsumeErr(err) {
		slog.Warn("nats: consume notice (self-recovering)", "queue", q.identifier, "err", err)
		return
	}
	slog.Error("nats: consume subscription terminated; resubscribing",
		"queue", q.identifier, "err", err)
	q.healthy.Store(false)
	q.unhealthyMsg.Store(err.Error())
	if !q.resubscribing.CompareAndSwap(false, true) {
		return // a resubscribe attempt is already in flight
	}
	go q.resubscribeUntilHealthy()
}

// resubscribeUntilHealthy retries resubscribe with capped exponential
// backoff until it succeeds (swapping in the new ConsumeContext and
// marking healthy again) or Stop is called.
func (q *Queue) resubscribeUntilHealthy() {
	defer q.resubscribing.Store(false)
	const maxBackoff = 5 * time.Second
	backoff := 200 * time.Millisecond
	for {
		select {
		case <-q.stopCh:
			return
		default:
		}
		newCtx, err := q.resubscribe()
		if err == nil {
			q.setConsumeCtx(newCtx)
			q.healthy.Store(true)
			slog.Info("nats: resubscribed; subscription healthy again", "queue", q.identifier)
			return
		}
		slog.Error("nats: resubscribe failed; retrying", "queue", q.identifier, "err", err, "backoff", backoff)
		select {
		case <-q.stopCh:
			return
		case <-time.After(backoff):
		}
		if backoff *= 2; backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (q *Queue) currentConsumeCtx() jetstream.ConsumeContext {
	q.consumeCtxMu.Lock()
	defer q.consumeCtxMu.Unlock()
	return q.consumeCtx
}

func (q *Queue) setConsumeCtx(cc jetstream.ConsumeContext) {
	q.consumeCtxMu.Lock()
	q.consumeCtx = cc
	q.consumeCtxMu.Unlock()
}

// toQueuedMessage converts a jetstream.Msg into a common.QueuedMessage,
// registering it in the pending-ack map. Returns ok=false for a message
// that was term'd on the spot (unreadable metadata / malformed body) and
// therefore never becomes a QueuedMessage.
func (q *Queue) toQueuedMessage(msg jetstream.Msg) (common.QueuedMessage, bool) {
	meta, err := msg.Metadata()
	if err != nil {
		// Can't track — term so the server stops redelivering.
		_ = msg.Term()
		return common.QueuedMessage{}, false
	}
	receipt := receiptFor(q.cfg.StreamName, meta.Sequence.Stream)
	var m common.Message
	if err := json.Unmarshal(msg.Data(), &m); err != nil {
		_ = msg.Term() // malformed
		return common.QueuedMessage{}, false
	}
	// A redelivery replaces the pending entry: same stream sequence, but
	// only the newest jetstream.Msg can be acked (the older delivery's ack
	// is stale), so the map must hold the newest.
	q.pendingMu.Lock()
	q.pending[receipt] = msg
	q.pendingMu.Unlock()
	return common.QueuedMessage{
		Message:         m,
		ReceiptHandle:   receipt,
		BrokerMessageID: brokerIDFor(meta.Sequence.Stream),
		QueueIdentifier: q.identifier,
	}, true
}

// parseURI accepts `nats://host:port[?stream=...&consumer=...&subject=...&...]`.
func parseURI(uri string) (Config, error) {
	cfg := DefaultConfig()
	u, err := url.Parse(uri)
	if err != nil {
		return cfg, fmt.Errorf("nats: parse URI: %w", err)
	}
	if u.Scheme != "nats" {
		return cfg, fmt.Errorf("nats: expected scheme nats://, got %q", u.Scheme)
	}
	// Rebuild server list from the URI host (and any commas inside).
	servers := u.Scheme + "://" + u.Host
	cfg.Servers = servers
	q := u.Query()
	if v := q.Get("stream"); v != "" {
		cfg.StreamName = v
	}
	if v := q.Get("consumer"); v != "" {
		cfg.ConsumerName = v
	}
	if v := q.Get("subject"); v != "" {
		cfg.Subject = v
	}
	if v := q.Get("max-messages"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxMessagesPerPoll = n
		}
	}
	if v := q.Get("poll-timeout-ms"); v != "" {
		if ms, err := strconv.Atoi(v); err == nil {
			cfg.PollTimeout = time.Duration(ms) * time.Millisecond
		}
	}
	if v := q.Get("ack-wait-secs"); v != "" {
		if s, err := strconv.Atoi(v); err == nil {
			cfg.AckWait = time.Duration(s) * time.Second
		}
	}
	if v := q.Get("max-deliver"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxDeliver = n
		}
	}
	if v := q.Get("max-ack-pending"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.MaxAckPending = n
		}
	}
	if v := q.Get("storage"); v != "" {
		cfg.Storage = v
	}
	if v := q.Get("replicas"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Replicas = n
		}
	}
	if v := q.Get("max-age-days"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			if n > 0 {
				cfg.MaxAge = time.Duration(n) * 24 * time.Hour
			} else {
				cfg.MaxAge = 0
			}
		}
	}
	return cfg, nil
}

// Identifier returns "stream/consumer" — the historical identifier format.
func (q *Queue) Identifier() string { return q.identifier }

// Poll takes up to max messages from the continuous subscription's buffer
// (see forward). It blocks on the first message for as long as ctx allows,
// then drains whatever is immediately available without waiting further,
// up to max.
//
// poll-timeout-ms plays no part here: this is not a broker round-trip, it's
// a channel read against a subscription that has been open all along.
//
// G13 (owner ruling 2026-09-07): if ctx's OWN deadline lapses before
// anything arrives, Poll returns (nil, nil) — a plain empty result, not an
// error. Poll has no timeout of its own; ctx's deadline is the only thing
// bounding how long it stays parked on a quiet queue, so hitting it means
// "nothing showed up in the window the caller gave me", which is exactly
// what an empty batch means for any other backend. The router's poll loop
// (manager.go) treats that as a successful, empty poll: it stamps a
// heartbeat and re-polls immediately (no added sleep, since the wait
// already happened) rather than logging a "poll error" and pausing. Before
// this, the caller's own bounding context was the thing that made a
// healthy blocking Poll look stalled to the restart watchdog — see
// TestPollDeadlineIsNotAnError and the manager-side test pinning that a
// long-blocking, message-free Poll is never flagged.
//
// A real Cancel (Stop() closing stopCh, or the caller cancelling ctx for
// shutdown) is NOT reclassified this way — it still returns a non-nil
// error (ErrStopped, or ctx.Err() for a plain cancel) so a caller whose
// loop exits on error actually exits, instead of reading a shutdown signal
// as "try again".
func (q *Queue) Poll(ctx context.Context, max uint32) ([]common.QueuedMessage, error) {
	if !q.running.Load() {
		return nil, queue.ErrStopped
	}
	limit := int(max)
	if limit <= 0 || limit > q.cfg.MaxMessagesPerPoll {
		limit = q.cfg.MaxMessagesPerPoll
	}

	var first common.QueuedMessage
	select {
	case qm, ok := <-q.msgCh:
		if !ok {
			return nil, queue.ErrStopped
		}
		first = qm
	case <-q.stopCh:
		return nil, queue.ErrStopped
	case <-ctx.Done():
		// G13 (owner ruling 2026-09-07): the caller's context expiring on
		// its OWN deadline — as opposed to being cancelled — means nothing
		// arrived within the window the caller was willing to wait, not
		// that anything is wrong. This backend blocks by contract (no
		// poll-timeout of its own; see the package doc), so the caller's
		// context is the only bound on how long a genuinely quiet queue
		// leaves Poll parked. Reporting that as a plain empty result (nil
		// error) rather than an error is what lets a healthy blocking Poll
		// look exactly like any other backend's fast "nothing queued"
		// answer to the router's poll loop: no error log, no backoff
		// pause, and — because it reaches the loop's success path — a
		// heartbeat stamp that keeps a merely-idle consumer from ever
		// looking stalled to the restart watchdog.
		//
		// A genuine Cancel (real shutdown, or Stop()/stopPoll() cancelling
		// the parent context) is NOT reclassified: it still returns
		// ctx.Err() so the caller's shutdown/restart path — which checks
		// for a non-nil error — actually observes it and exits its loop
		// rather than reading this as "queue's just quiet, keep going".
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// G14: "nothing arrived" is only a benign, plain-empty result
			// while the subscription is actually healthy. If forward() has
			// seen the iterator error and is mid-resubscribe, an empty
			// result here would be indistinguishable from a genuinely
			// quiet queue — exactly the bug that let a dead subscription
			// look like an idle one forever. Surface it as an error
			// instead: the router's ordinary poll-error handling applies
			// (logged, 1s backoff, no heartbeat stamp), and if
			// resubscribing keeps failing for long enough, the stall
			// watchdog rebuilds the whole Queue.
			if q.healthy.Load() {
				return nil, nil
			}
			reason, _ := q.unhealthyMsg.Load().(string)
			return nil, fmt.Errorf("nats: subscription unhealthy, resubscribing (last error: %s)", reason)
		}
		return nil, ctx.Err()
	}

	out := make([]common.QueuedMessage, 0, limit)
	out = append(out, first)
	q.totalPolled.Add(1)

	// Drain whatever is already sitting in the channel, up to limit. This
	// never blocks: an empty channel falls straight to default.
	for len(out) < limit {
		select {
		case qm, ok := <-q.msgCh:
			if !ok {
				return out, nil
			}
			out = append(out, qm)
			q.totalPolled.Add(1)
		default:
			return out, nil
		}
	}
	return out, nil
}

// Ack consumes the receipt and ACKs the underlying JetStream message.
func (q *Queue) Ack(_ context.Context, receipt string, _ string) error {
	msg := q.popPending(receipt)
	if msg == nil {
		return fmt.Errorf("nats: no pending message for receipt %q", receipt)
	}
	if err := msg.Ack(); err != nil {
		return fmt.Errorf("nats: ack: %w", err)
	}
	q.totalAcked.Add(1)
	return nil
}

// Nack NAKs with optional delay. Counts as a failure.
func (q *Queue) Nack(_ context.Context, receipt string, delaySeconds *uint32) error {
	msg := q.popPending(receipt)
	if msg == nil {
		return fmt.Errorf("nats: no pending message for receipt %q", receipt)
	}
	if err := q.nakWith(msg, delaySeconds); err != nil {
		return err
	}
	q.totalNacked.Add(1)
	return nil
}

// Defer NAKs with delay but doesn't count as a failure (backpressure /
// rate-limit signal). Same wire effect as Nack-with-delay in NATS.
func (q *Queue) Defer(_ context.Context, receipt string, delaySeconds *uint32) error {
	msg := q.popPending(receipt)
	if msg == nil {
		return fmt.Errorf("nats: no pending message for receipt %q", receipt)
	}
	if err := q.nakWith(msg, delaySeconds); err != nil {
		return err
	}
	q.totalDeferred.Add(1)
	return nil
}

func (q *Queue) nakWith(msg jetstream.Msg, delaySeconds *uint32) error {
	if delaySeconds != nil && *delaySeconds > 0 {
		return msg.NakWithDelay(time.Duration(*delaySeconds) * time.Second)
	}
	return msg.Nak()
}

// Healthy reports whether the consumer is running and the underlying
// connection is up.
func (q *Queue) Healthy() bool {
	if !q.running.Load() {
		return false
	}
	return q.nc.Status() == natsgo.CONNECTED
}

// Stop signals the consumer to wind down. In-flight messages are
// dropped from the local tracker; the server will redeliver after the
// ack-wait expires.
//
// Closing stopCh unblocks a Poll parked on msgCh (or a forward() blocked
// sending to a full msgCh) immediately, without waiting for msgsCtx.Stop to
// unwind the subscription — that's what keeps Stop's effect on a parked
// Poll bounded regardless of how long the subscription takes to tear down.
func (q *Queue) Stop() {
	q.stopOnce.Do(func() {
		q.running.Store(false)
		close(q.stopCh)
		q.currentConsumeCtx().Stop()
	})
	q.pendingMu.Lock()
	q.pending = make(map[string]jetstream.Msg)
	q.pendingMu.Unlock()
	q.nc.Close()
}

// Metrics queries JetStream for the consumer's pending + in-flight counts.
func (q *Queue) Metrics(ctx context.Context) (*queue.Metrics, error) {
	info, err := q.consumer.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("nats: consumer info: %w", err)
	}
	return &queue.Metrics{
		QueueIdentifier:  q.identifier,
		PendingMessages:  info.NumPending,
		InFlightMessages: uint64(info.NumAckPending),
		TotalPolled:      q.totalPolled.Load(),
		TotalAcked:       q.totalAcked.Load(),
		TotalNacked:      q.totalNacked.Load(),
		TotalDeferred:    q.totalDeferred.Load(),
	}, nil
}

// Counters returns the process-local counters (no broker round-trip).
func (q *Queue) Counters() *queue.Metrics {
	return &queue.Metrics{
		QueueIdentifier: q.identifier,
		TotalPolled:     q.totalPolled.Load(),
		TotalAcked:      q.totalAcked.Load(),
		TotalNacked:     q.totalNacked.Load(),
		TotalDeferred:   q.totalDeferred.Load(),
	}
}

// Publish marshals m to JSON and publishes to the configured subject.
// The returned id is the JetStream stream sequence.
func (q *Queue) Publish(ctx context.Context, m common.Message) (string, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("nats: marshal: %w", err)
	}
	ack, err := q.js.Publish(ctx, subjectFor(q.cfg.Subject, m), body)
	if err != nil {
		return "", fmt.Errorf("nats: publish: %w", err)
	}
	return strconv.FormatUint(ack.Sequence, 10), nil
}

// PublishBatch publishes each message sequentially. NATS doesn't have
// true batch publish; we accept the round-trip cost for simplicity.
func (q *Queue) PublishBatch(ctx context.Context, msgs []common.Message) ([]string, error) {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		id, err := q.Publish(ctx, m)
		if err != nil {
			return out, err
		}
		out = append(out, id)
	}
	return out, nil
}

// receiptFor and brokerIDFor derive a message's two identities from its
// STREAM sequence — the message's identity in the stream, which every
// redelivery of it shares.
//
// Neither may involve the CONSUMER sequence, which counts deliveries and so
// changes on every redelivery. Feeding that to the router's duplicate filter
// made each redelivery look like a different copy of the message (an external
// requeue), and the router ACK-deletes those — so JetStream destroyed its own
// copy every time the ack-wait lapsed, leaving the in-memory copy alone and
// silently losing the message if it was later released or flushed.
func receiptFor(stream string, streamSeq uint64) string {
	return stream + ":" + strconv.FormatUint(streamSeq, 10)
}

func brokerIDFor(streamSeq uint64) string {
	return strconv.FormatUint(streamSeq, 10)
}

func (q *Queue) popPending(receipt string) jetstream.Msg {
	q.pendingMu.Lock()
	defer q.pendingMu.Unlock()
	msg, ok := q.pending[receipt]
	if !ok {
		return nil
	}
	delete(q.pending, receipt)
	return msg
}

// subjectFor picks the subject for a published message. If the
// configured filter is a wildcard like `flowcatalyst.>`, we substitute
// the message's pool code for the wildcard part; otherwise we use the
// filter verbatim.
func subjectFor(filter string, m common.Message) string {
	if strings.HasSuffix(filter, ".>") || strings.HasSuffix(filter, ".*") {
		base := strings.TrimSuffix(strings.TrimSuffix(filter, ".>"), ".*")
		if m.PoolCode != "" {
			return base + "." + m.PoolCode
		}
		return base + ".default"
	}
	return filter
}
