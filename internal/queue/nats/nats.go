// Package nats is the NATS JetStream queue backend.
//
//   - Continuous pull-based JetStream subscription (owner ruling), not a
//     per-poll Fetch: the client keeps one jetstream.Messages() iterator
//     open for the life of the consumer, bounded client-side to one
//     in-flight batch (PullMaxMessages + PullThresholdMessages(1), so the
//     next batch isn't requested until the current one is fully drained).
//     A background goroutine forwards the iterator into a channel of
//     capacity max-messages; Poll blocks on that channel rather than
//     issuing a broker round-trip.
//   - WorkQueue retention (messages removed after ack).
//   - Durable consumer auto-provisioned at startup.
//   - Receipt handles are `streamName:streamSequence` (the historical
//     receipt format).
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

	// msgsCtx is the one continuous pull subscription open for the life of
	// the Queue. forward() drains it into msgCh; nothing else calls Next.
	msgsCtx jetstream.MessagesContext

	// msgCh is fed by forward() and drained by Poll. Its capacity is
	// max-messages: forward()'s send blocks once it's full, which is what
	// stops the subscription from being drained further (Next isn't called
	// again until there's room). Combined with PullThresholdMessages(1),
	// the subscription only asks for a fresh batch once the previous one
	// is fully drained out of the library and into msgCh — so with a
	// consumer actively draining msgCh (the normal case) at most one
	// batch's worth of ack-wait is ticking at a time. If nothing drains
	// msgCh at all, one extra batch can land just as the first finishes
	// draining (the request that empties the library's own buffer fires
	// before forward() notices msgCh is full) — client-side residency
	// then plateaus at two batches, never growing further, because that
	// next send blocks forward() before a third batch is ever requested.
	msgCh chan common.QueuedMessage

	// stopCh is closed by Stop to unblock a Poll parked on msgCh, or a
	// forward() blocked sending to a full msgCh, without waiting for
	// forward() to notice the subscription closed.
	stopCh   chan struct{}
	stopOnce sync.Once

	// forwardDone closes once forward() has returned (subscription closed
	// or Stop called). Not waited on by Stop (which must return promptly);
	// tests use it to confirm the goroutine actually exited.
	forwardDone chan struct{}

	running atomic.Bool

	pendingMu sync.Mutex
	pending   map[string]jetstream.Msg

	totalPolled   atomic.Uint64
	totalAcked    atomic.Uint64
	totalNacked   atomic.Uint64
	totalDeferred atomic.Uint64
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
	// PullThresholdMessages(1): don't request the next batch until the
	// current one is fully drained (pending < 1, i.e. 0). The library's
	// own default threshold (half of MaxMessages) would start a second
	// batch while the first is still half full, letting up to ~1.5
	// batches sit client-side with their ack-wait ticking — the owner's
	// ruling is one batch at a time.
	msgsCtx, err := consumer.Messages(
		jetstream.PullMaxMessages(batch),
		jetstream.PullThresholdMessages(1),
	)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("nats: open subscription: %w", err)
	}

	q := &Queue{
		cfg:         cfg,
		identifier:  cfg.StreamName + "/" + cfg.ConsumerName,
		nc:          nc,
		js:          js,
		consumer:    consumer,
		msgsCtx:     msgsCtx,
		msgCh:       make(chan common.QueuedMessage, batch),
		stopCh:      make(chan struct{}),
		forwardDone: make(chan struct{}),
		pending:     make(map[string]jetstream.Msg),
	}
	q.running.Store(true)
	go q.forward()
	return q, nil
}

// forward is the one goroutine that ever calls msgsCtx.Next. It blocks
// until a message is available (or the subscription is stopped/drained),
// converts it, and hands it to Poll via msgCh. A full msgCh blocks the
// send — and therefore blocks the next Next call — which is the
// back-pressure that keeps the subscription from requesting more than one
// batch ahead.
func (q *Queue) forward() {
	defer close(q.forwardDone)
	for {
		msg, err := q.msgsCtx.Next()
		if err != nil {
			// ErrMsgIteratorClosed: Stop() or Drain() was called (including
			// our own Stop). ErrConnectionClosed / any other error: the
			// connection is gone. Either way there is nothing left to
			// forward — exit and let msgCh's lack of further sends (plus
			// stopCh, closed by Stop) unblock any parked Poll.
			return
		}
		qm, ok := q.toQueuedMessage(msg)
		if !ok {
			continue
		}
		select {
		case q.msgCh <- qm:
		case <-q.stopCh:
			return
		}
	}
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
			return nil, nil
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
		q.msgsCtx.Stop()
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
