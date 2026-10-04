// Package sqs is the AWS SQS-backed queue backend.
//
//   - 20s long-poll (AWS max) for low API-call rate.
//   - Visibility timeout configurable per queue.
//   - Pending-delete guard for at-least-once redeliveries: once we
//     successfully (or unsuccessfully) DeleteMessage for a MessageId,
//     subsequent redeliveries until pendingDeleteGrace after the delete
//     completed are deleted immediately on poll instead of being routed to
//     the mediator.
package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// pendingDeleteGrace is how long we keep remembering an acked MessageId AFTER
// its DeleteMessageBatch entry completed (success or failure), so a redelivery
// racing the delete (SQS is at-least-once) is short-circuited to DeleteMessage
// instead of delivered again. While the delete is still in flight the id is
// remembered however long the linger or call takes. This is a courtesy, not a
// guarantee: delivery endpoints are idempotent.
const pendingDeleteGrace = 5 * time.Second

// MaxVisibility is SQS's own ceiling on a message's total
// invisibility, counted from the ORIGINAL ReceiveMessage — not from any one
// ChangeMessageVisibility call (R3, owner ruling 2026-09-17,
// docs/spec/router-deferral-handback.md). Nack's clamp.
const MaxVisibility = 12 * time.Hour

// DefaultWaitSeconds is the long-poll wait time. AWS max is 20s.
const DefaultWaitSeconds = 20

func init() {
	queue.RegisterConsumer("sqs", consumerFactory)
	queue.RegisterPublisher("sqs", publisherFactory)
}

func consumerFactory(ctx context.Context, cfg common.QueueConfig) (queue.Consumer, error) {
	q, err := build(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return q, nil
}

func publisherFactory(ctx context.Context, cfg common.QueueConfig) (queue.Publisher, error) {
	q, err := build(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return q, nil
}

// apiCallTimeout bounds every SQS call other than the long-poll receive (which
// the router bounds itself). The SDK client has no timeout of its own, so a call
// on a stalled connection blocked its caller for good: an acknowledgement
// blocked a pool worker and the slot it held, and a hung call is exactly the
// kind of unbounded wait that froze the NATS consumer. 25s matches the other
// SQS consumers' operation timeout.
var apiCallTimeout = 25 * time.Second // a var only so tests can shorten it

// callCtx derives the context for one bounded SQS API call.
func callCtx(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(ctx, apiCallTimeout)
}

func build(ctx context.Context, cfg common.QueueConfig) (*Queue, error) {
	// The queue's own URL encodes its region (sqs.<region>.amazonaws.com), so
	// use it: an SQS queue must be reached in its own region, and this works
	// even when AWS_REGION/AWS_DEFAULT_REGION isn't set in the environment
	// (e.g. an ECS task def that doesn't export it). Falls back to the SDK's
	// default region chain when the URL isn't a recognisable SQS endpoint.
	var opts []func(*awsconfig.LoadOptions) error
	if region := regionFromSQSURL(cfg.URI); region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	client := newSQSClient(awsCfg)
	queueName := cfg.Name
	if queueName == "" {
		queueName = queueNameFromURL(cfg.URI)
	}
	vt := cfg.VisibilityTimeout
	if vt == 0 {
		vt = 30
	}
	q := &Queue{
		client:            client,
		queueURL:          cfg.URI,
		queueName:         queueName,
		visibilityTimeout: int32(vt),
		waitSeconds:       DefaultWaitSeconds,
		pendingDelete:     make(map[string]*pendingEntry),
		receiptPolledAt:   make(map[string]time.Time),
	}
	q.running.Store(true)
	return q, nil
}

// regionFromSQSURL extracts the AWS region from an SQS queue URL whose host is
// sqs.<region>.amazonaws.com (or sqs-fips.<region>.amazonaws.com[.cn]). Returns
// "" when uri is not a recognisable SQS endpoint (e.g. a non-AWS test URI).
func regionFromSQSURL(uri string) string {
	u, err := neturl.Parse(uri)
	if err != nil || u.Host == "" {
		return ""
	}
	parts := strings.Split(u.Host, ".")
	if len(parts) >= 4 && strings.HasPrefix(parts[0], "sqs") && parts[2] == "amazonaws" {
		return parts[1]
	}
	return ""
}

// NewClient builds an SQS client with the hardening every FlowCatalyst SQS
// user needs (the request-body workaround in plainBodyClient, no per-message
// MD5 validation). Other packages that talk to SQS directly, such as the
// dispatch scheduler's publisher, must use it instead of sqs.NewFromConfig.
func NewClient(awsCfg aws.Config) *sqs.Client { return newSQSClient(awsCfg) }

// newSQSClient builds the SDK client. The SDK's per-message MD5 check of every
// received body is switched off: it hashes each payload on the poll hot path
// purely for CPU's sake of detecting corruption the TLS transport already
// rules out, and a mismatch would only fail the poll for the whole batch.
func newSQSClient(awsCfg aws.Config) *sqs.Client {
	return sqs.NewFromConfig(awsCfg, func(o *sqs.Options) {
		o.DisableMessageChecksumValidation = true
		// See plainBodyClient: works around a lost-response race in the SDK's
		// request body handling.
		o.HTTPClient = plainBodyClient{inner: o.HTTPClient}
	})
}

func queueNameFromURL(url string) string {
	parts := strings.Split(url, "/")
	if len(parts) == 0 {
		return "unknown"
	}
	return parts[len(parts)-1]
}

// Queue is the SQS-backed queue. Implements both Consumer and Publisher.
type Queue struct {
	client            *sqs.Client
	queueURL          string
	queueName         string
	visibilityTimeout int32
	waitSeconds       int32

	mu            sync.Mutex
	pendingDelete map[string]*pendingEntry
	// pendingFIFO records pendingDelete insertions in time order so pruning
	// pops finished, expired entries from the front instead of scanning the
	// whole map on every poll. An id re-marked later has a new map entry and a
	// second FIFO entry; the older entry is skipped lazily on pop (see
	// evictExpiredPendingDeletesLocked). Guarded by mu; the zero value is usable.
	pendingFIFO []*pendingEntry
	pendingHead int
	// receiptPolledAt records when THIS consumer's ReceiveMessage first
	// handed out each currently-outstanding receipt handle — Nack's clamp
	// (R3) measures SQS's 12-hour ceiling from here, since SQS itself counts
	// it from the original receive, not from any one ChangeMessageVisibility
	// call. Entries are removed on Ack/Nack (the receipt is then spent —
	// this consumer never sees it again as a live handle) and swept for
	// staleness on every Poll, so growth is bounded by outstanding receipts,
	// not by every receipt ever issued.
	receiptPolledAt map[string]time.Time
	// lastReceiptPrune is when evictStaleReceiptTimestampsLocked last scanned
	// receiptPolledAt; the scan is O(map) so it runs at most once per
	// receiptPruneInterval rather than on every poll. Guarded by mu.
	lastReceiptPrune time.Time

	running atomic.Bool

	// Delete batching (see deletebatch.go). Created lazily; zero value usable.
	del deleteBatcher
	// Deferral/nack batching (see visibilitybatch.go). Same laziness.
	vis visibilityBatcher

	polled   atomic.Uint64
	acked    atomic.Uint64
	nacked   atomic.Uint64
	deferred atomic.Uint64
}

// Identifier returns the queue name.
func (q *Queue) Identifier() string { return q.queueName }

// Poll fetches up to maxMessages via ReceiveMessage with long-polling.
func (q *Queue) Poll(ctx context.Context, maxMessages uint32) ([]common.QueuedMessage, error) {
	if !q.running.Load() {
		return nil, queue.ErrStopped
	}
	max := min(int32(maxMessages),
		// SQS hard limit
		10)

	out, err := q.client.ReceiveMessage(ctx, &sqs.ReceiveMessageInput{
		QueueUrl:            aws.String(q.queueURL),
		MaxNumberOfMessages: max,
		VisibilityTimeout:   q.visibilityTimeout,
		WaitTimeSeconds:     q.waitSeconds,
		// No system or message attributes are requested: parseMessage reads
		// only the body, receipt handle and MessageId, and nothing else in the
		// package consumes a received attribute. Asking for "All" made SQS
		// build, and the SDK parse, attributes that were then discarded.
	})
	if err != nil {
		return nil, fmt.Errorf("sqs ReceiveMessage: %w", err)
	}
	if len(out.Messages) == 0 {
		return nil, nil
	}

	q.evictExpiredPendingDeletesLocked()
	q.evictStaleReceiptTimestampsLocked()
	results := make([]common.QueuedMessage, 0, len(out.Messages))
	for _, sm := range out.Messages {
		if sm.MessageId != nil {
			alreadyAcked := q.alreadyDeleted(*sm.MessageId)
			if alreadyAcked {
				// Redelivery of an acked message — delete immediately.
				if sm.ReceiptHandle != nil {
					dctx, cancel := callCtx(ctx)
					_, _ = q.client.DeleteMessage(dctx, &sqs.DeleteMessageInput{
						QueueUrl:      aws.String(q.queueURL),
						ReceiptHandle: sm.ReceiptHandle,
					})
					cancel()
				}
				continue
			}
		}

		msg, receipt, brokerID, perr := q.parseMessage(sm)
		if perr != nil {
			// Malformed — ACK it so it doesn't keep coming back.
			if sm.ReceiptHandle != nil {
				_ = q.Ack(ctx, *sm.ReceiptHandle, brokerIDOf(sm))
			}
			continue
		}
		q.recordPolled(receipt)
		results = append(results, common.QueuedMessage{
			Message:         msg,
			ReceiptHandle:   receipt,
			BrokerMessageID: brokerID,
			QueueIdentifier: q.queueName,
		})
	}

	q.polled.Add(uint64(len(results)))
	return results, nil
}

func (q *Queue) parseMessage(sm sqstypes.Message) (common.Message, string, string, error) {
	if sm.Body == nil {
		return common.Message{}, "", "", errors.New("empty body")
	}
	var m common.Message
	// Unmarshal only reads its input and copies whatever it keeps, so the body
	// is viewed as bytes rather than copied into a fresh []byte per message.
	if err := json.Unmarshal(unsafe.Slice(unsafe.StringData(*sm.Body), len(*sm.Body)), &m); err != nil {
		return common.Message{}, "", "", fmt.Errorf("unmarshal: %w", err)
	}
	if sm.ReceiptHandle == nil {
		return common.Message{}, "", "", errors.New("missing receipt handle")
	}
	brokerID := ""
	if sm.MessageId != nil {
		brokerID = *sm.MessageId
	}
	return m, *sm.ReceiptHandle, brokerID, nil
}

// Ack deletes the message and remembers its MessageId so a redelivery already
// in flight is short-circuited rather than processed twice.
//
// The id is supplied by the caller. It used to be recovered from a
// receipt→MessageId map populated at poll time, which silently failed whenever
// that entry had been evicted first (the map is pruned by age once it exceeds a
// size threshold). A message held longer than the eviction age before its ack —
// routine while a slow target is being retried — would then be deleted with NO
// pending-delete entry recorded, so the next redelivery of it was not
// recognised and got delivered to the target a second time. Passing the id in
// removes the lookup, and with it the failure mode.
//
// Retention stays bounded: an id is remembered while its delete is in flight
// and for pendingDeleteGrace after it completes (pruned on every poll), so
// memory is bounded by acks in flight plus a few seconds of throughput.
func (q *Queue) Ack(ctx context.Context, receipt string, brokerMessageID string) error {
	entry := q.markDeleted(brokerMessageID)
	q.forgetReceipt(receipt)

	ctx, cancel := callCtx(ctx)
	defer cancel()
	if err := q.del.delete(ctx, q, receipt, entry); err != nil {
		return fmt.Errorf("sqs DeleteMessage: %w", err)
	}
	return nil
}

// Nack honours the delay via ChangeMessageVisibility — R3 (owner ruling
// 2026-09-17, docs/spec/router-deferral-handback.md).
//
// This used to be a no-op: the router retried a failing message in-process,
// keeping it in its message-group pipeline with its own backoff rather than
// releasing it to the broker, so shortening SQS's own visibility timeout
// here would have let SQS redeliver the message while this process was
// still retrying it — a concurrent duplicate. That is no longer the shape
// of every call: a delay-bearing MediationDeferred (R1) is now handed back
// to the broker on its FIRST occurrence rather than retried, and every
// Pool.nackMsg call first removes the message's in-flight tracker entry
// before Nack ever runs — see nackMsg's doc comment — so by the time this
// method is called the router has already given up ownership of the
// message. A redelivery once the delay elapses is therefore a fresh
// delivery, not a duplicate of a retry still running here.
//
// delaySeconds is floored at zero and clamped to MaxVisibility,
// measured from when THIS consumer first polled the receipt (SQS counts its
// own 12-hour ceiling from the original ReceiveMessage, not from this call)
// — see remainingVisibilitySeconds. Best-effort, matching the
// queue.Consumer contract every backend's Nack already keeps: a failure
// (e.g. a stale or already-deleted receipt) is logged at WARN and
// swallowed rather than returned, so the message simply returns at its
// natural visibility timeout — the same outcome this method always had.
// The nacked counter is incremented regardless of the AWS call's outcome,
// same as before.
func (q *Queue) Nack(ctx context.Context, receipt string, delaySeconds *uint32) error {
	return q.returnAfter(ctx, receipt, delaySeconds, &q.nacked)
}

// Defer is the same ChangeMessageVisibility as Nack, counted as a deferral
// rather than a failure. It used to be a no-op, because nothing handed a
// backpressure signal to the broker: a consumer whose destination pool was
// full simply stopped polling. That pause is now reserved for the case
// where EVERY pool the queue feeds is full (Manager.hasCapacityFor); a
// message for one full pool among several is deferred here, for the delay
// the pool's admission schedule computed (Pool.deferMsg), so the rest of the
// queue keeps flowing past it.
//
// Same ownership contract as Nack: the caller has already dropped the
// message's in-flight tracker entry, so the redelivery is a fresh delivery.
// Each redelivery is one more ReceiveMessage, so it bumps the message's
// ApproximateReceiveCount — a redrive policy on the source queue counts
// deferrals against maxReceiveCount exactly as it counts failures.
func (q *Queue) Defer(ctx context.Context, receipt string, delaySeconds *uint32) error {
	return q.returnAfter(ctx, receipt, delaySeconds, &q.deferred)
}

// returnAfter is the shared body of Nack and Defer: make receipt visible
// again after delaySeconds (clamped — see clampToRemainingVisibility), and
// forget the receipt. Best-effort, per the queue.Consumer contract.
//
// The change is QUEUED to the visibility batcher (visibilitybatch.go) and this
// returns without waiting for the broker: a failure is logged there, and
// counter is incremented there once the broker has answered or failed. The
// only error is the caller's ctx ending while the (bounded, deliberately
// back-pressuring) queue is full.
func (q *Queue) returnAfter(ctx context.Context, receipt string, delaySeconds *uint32, counter *atomic.Uint64) error {
	defer q.forgetReceipt(receipt)

	var seconds uint32
	if delaySeconds != nil {
		seconds = *delaySeconds
	}
	clamped := q.clampToRemainingVisibility(receipt, seconds)
	return q.vis.enqueue(ctx, q, visibilityItem{receipt: receipt, seconds: clamped, counter: counter})
}

// HonoursDelayedReturn is true (R5, docs/spec/router-deferral-handback.md):
// Nack's ChangeMessageVisibility call (R3) really does hold the message
// back for delay.
func (q *Queue) HonoursDelayedReturn() bool { return true }

// clampToRemainingVisibility bounds requested (seconds) to what's left of
// SQS's MaxVisibility ceiling for receipt, floored at zero. AWS
// rejects a ChangeMessageVisibility that would push a message's total
// invisibility (from its ORIGINAL ReceiveMessage) past that ceiling, so the
// clamp is what makes a large requested delay (a target asking for longer
// than SQS allows) a successful, smaller Nack instead of a rejected one.
func (q *Queue) clampToRemainingVisibility(receipt string, requested uint32) int32 {
	remaining := q.remainingVisibilitySeconds(receipt)
	if int64(requested) < remaining {
		return int32(requested)
	}
	return int32(remaining)
}

// remainingVisibilitySeconds is how much of MaxVisibility receipt has
// left, counted from when THIS consumer polled it (receiptPolledAt). A
// receipt not currently recorded there — evicted for staleness, or never
// recorded (e.g. a receipt this process didn't poll itself) — gets the full
// ceiling: there is nothing here to say it should be any shorter.
func (q *Queue) remainingVisibilitySeconds(receipt string) int64 {
	q.mu.Lock()
	polledAt, ok := q.receiptPolledAt[receipt]
	q.mu.Unlock()
	if !ok {
		return int64(MaxVisibility / time.Second)
	}
	elapsed := time.Since(polledAt)
	remaining := MaxVisibility - elapsed
	if remaining < 0 {
		return 0
	}
	return int64(remaining / time.Second)
}

// Publish sends a single message via SendMessage.
func (q *Queue) Publish(ctx context.Context, m common.Message) (string, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	in := &sqs.SendMessageInput{
		QueueUrl:    aws.String(q.queueURL),
		MessageBody: aws.String(string(body)),
	}
	if m.MessageGroupID != nil {
		in.MessageGroupId = aws.String(*m.MessageGroupID)
	}
	out, err := q.client.SendMessage(ctx, in)
	if err != nil {
		return "", fmt.Errorf("sqs SendMessage: %w", err)
	}
	if out.MessageId == nil {
		return "", nil
	}
	return *out.MessageId, nil
}

// PublishBatch sends in batches of 10 (SQS hard limit).
func (q *Queue) PublishBatch(ctx context.Context, msgs []common.Message) ([]string, error) {
	ids := make([]string, 0, len(msgs))
	for start := 0; start < len(msgs); start += 10 {
		end := min(start+10, len(msgs))
		entries := make([]sqstypes.SendMessageBatchRequestEntry, 0, end-start)
		for i := start; i < end; i++ {
			body, err := json.Marshal(msgs[i])
			if err != nil {
				return ids, err
			}
			e := sqstypes.SendMessageBatchRequestEntry{
				Id:          aws.String(strconv.Itoa(i)),
				MessageBody: aws.String(string(body)),
			}
			if msgs[i].MessageGroupID != nil {
				e.MessageGroupId = aws.String(*msgs[i].MessageGroupID)
			}
			entries = append(entries, e)
		}
		out, err := q.client.SendMessageBatch(ctx, &sqs.SendMessageBatchInput{
			QueueUrl: aws.String(q.queueURL),
			Entries:  entries,
		})
		if err != nil {
			return ids, fmt.Errorf("sqs SendMessageBatch: %w", err)
		}
		for _, r := range out.Successful {
			if r.MessageId != nil {
				ids = append(ids, *r.MessageId)
			}
		}
		if len(out.Failed) > 0 {
			return ids, fmt.Errorf("sqs SendMessageBatch: %d failures", len(out.Failed))
		}
	}
	return ids, nil
}

// Healthy reports running state.
func (q *Queue) Healthy() bool { return q.running.Load() }

// Stop marks the consumer stopped.
//
// It also stops the delete-batch drainers: acks still waiting for a batch fail
// rather than hang. Queued deferrals/nacks are flushed best-effort (the router
// nacks buffered messages after stopping pools, i.e. around Stop), and a Defer
// or Nack after Stop is still sent.
func (q *Queue) Stop() {
	q.running.Store(false)
	q.del.stop()
	q.vis.stop()
}

// Metrics calls GetQueueAttributes for pending + in-flight counts.
func (q *Queue) Metrics(ctx context.Context) (*queue.Metrics, error) {
	ctx, cancel := callCtx(ctx)
	defer cancel()
	out, err := q.client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(q.queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{
			sqstypes.QueueAttributeNameApproximateNumberOfMessages,
			sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("sqs GetQueueAttributes: %w", err)
	}
	parse := func(k sqstypes.QueueAttributeName) uint64 {
		s, ok := out.Attributes[string(k)]
		if !ok {
			return 0
		}
		v, _ := strconv.ParseUint(s, 10, 64)
		return v
	}
	return &queue.Metrics{
		QueueIdentifier:  q.queueName,
		PendingMessages:  parse(sqstypes.QueueAttributeNameApproximateNumberOfMessages),
		InFlightMessages: parse(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible),
		TotalPolled:      q.polled.Load(),
		TotalAcked:       q.acked.Load(),
		TotalNacked:      q.nacked.Load(),
		TotalDeferred:    q.deferred.Load(),
	}, nil
}

// Counters returns process-local counters only.
func (q *Queue) Counters() *queue.Metrics {
	return &queue.Metrics{
		QueueIdentifier: q.queueName,
		TotalPolled:     q.polled.Load(),
		TotalAcked:      q.acked.Load(),
		TotalNacked:     q.nacked.Load(),
		TotalDeferred:   q.deferred.Load(),
	}
}

// pendingEntry is one remembered MessageId. It is in flight (done=false) from
// the moment the ack is taken until its DeleteMessageBatch entry completes
// (success or failure), then expires pendingDeleteGrace after doneAt.
type pendingEntry struct {
	id     string
	done   bool
	doneAt time.Time
}

// evictExpiredPendingDeletesLocked prunes finished entries older than
// pendingDeleteGrace, so the guard remembers recent deletes rather than every
// delete ever. Called at the top of each poll.
func (q *Queue) evictExpiredPendingDeletesLocked() {
	q.evictExpiredPendingDeletesAt(time.Now())
}

// evictExpiredPendingDeletesAt is the pruner with an explicit clock. Only the
// front of pendingFIFO (insertion order) is examined: pop while the entry is
// done and its grace has passed, stop at the first in-flight or fresh entry. An
// in-flight entry is never popped, however old. A popped entry deletes the map
// entry only if the map still holds that exact entry (an id re-marked since has
// a newer one with its own FIFO slot). Completion order can differ slightly from
// insertion order, so an entry behind an older in-flight one waits for it; the
// wait is bounded by the call timeout.
func (q *Queue) evictExpiredPendingDeletesAt(now time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.pendingHead < len(q.pendingFIFO) {
		e := q.pendingFIFO[q.pendingHead]
		if !e.done || now.Sub(e.doneAt) <= pendingDeleteGrace {
			break
		}
		if q.pendingDelete[e.id] == e {
			delete(q.pendingDelete, e.id)
		}
		q.pendingFIFO[q.pendingHead] = nil // release the entry
		q.pendingHead++
	}
	// Reclaim the consumed prefix once it dominates, keeping the slice bounded.
	if q.pendingHead > 1024 && q.pendingHead*2 >= len(q.pendingFIFO) {
		n := copy(q.pendingFIFO, q.pendingFIFO[q.pendingHead:])
		clear(q.pendingFIFO[n:])
		q.pendingFIFO = q.pendingFIFO[:n]
		q.pendingHead = 0
	}
}

// markDeleted records, as in flight, that this MessageId is being deleted, so a
// redelivery can be short-circuited. An empty id is ignored (nil returned) —
// there is nothing to key on, and inserting "" would match every id-less
// message. The caller must pass the returned entry to markDeleteDone when the
// delete completes (a nil entry is accepted there).
func (q *Queue) markDeleted(brokerMessageID string) *pendingEntry {
	if brokerMessageID == "" {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.markDeletedLocked(brokerMessageID)
}

func (q *Queue) markDeletedLocked(id string) *pendingEntry {
	e := &pendingEntry{id: id}
	q.pendingDelete[id] = e
	q.pendingFIFO = append(q.pendingFIFO, e)
	return e
}

// markDeleteDone records that e's delete finished (any outcome); it expires
// pendingDeleteGrace later.
func (q *Queue) markDeleteDone(e *pendingEntry) { q.markDeleteDoneAt(e, time.Now()) }

func (q *Queue) markDeleteDoneAt(e *pendingEntry, at time.Time) {
	if e == nil {
		return
	}
	q.mu.Lock()
	if !e.done {
		e.done, e.doneAt = true, at
	}
	q.mu.Unlock()
}

// alreadyDeleted reports whether this MessageId is in flight or was deleted
// within pendingDeleteGrace.
func (q *Queue) alreadyDeleted(brokerMessageID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	e, ok := q.pendingDelete[brokerMessageID]
	return ok && (!e.done || time.Since(e.doneAt) <= pendingDeleteGrace)
}

// recordPolled remembers when THIS consumer first received receipt — Nack's
// clamp (R3) reads it back via remainingVisibilitySeconds. Called once per
// delivered message, at the point Poll hands out its receipt handle.
func (q *Queue) recordPolled(receipt string) {
	q.mu.Lock()
	q.receiptPolledAt[receipt] = time.Now()
	q.mu.Unlock()
}

// forgetReceipt drops receipt's poll-time entry once it is spent (acked, or
// nacked — either way this consumer will never see this exact receipt handle
// live again; a redelivery gets a fresh one). Keeps receiptPolledAt bounded
// by outstanding receipts rather than every receipt ever issued.
func (q *Queue) forgetReceipt(receipt string) {
	q.mu.Lock()
	delete(q.receiptPolledAt, receipt)
	q.mu.Unlock()
}

// receiptPruneInterval spaces the receiptPolledAt scans. Entries live for
// MaxVisibility (hours), so pruning a little late changes nothing observable:
// remainingVisibilitySeconds applies the age clamp itself.
const receiptPruneInterval = 30 * time.Second

// evictStaleReceiptTimestampsLocked prunes receiptPolledAt entries older
// than MaxVisibility: a receipt that old is beyond SQS's own ceiling
// regardless (remainingVisibilitySeconds would return 0 for it anyway), and
// forgetReceipt alone cannot bound the map for a receipt this consumer polled
// but never itself acked or nacked (e.g. lost to a consumer restart). Called
// at the top of each poll, alongside evictExpiredPendingDeletesLocked.
func (q *Queue) evictStaleReceiptTimestampsLocked() {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	if now.Sub(q.lastReceiptPrune) < receiptPruneInterval {
		return
	}
	q.lastReceiptPrune = now
	for receipt, ts := range q.receiptPolledAt {
		if now.Sub(ts) > MaxVisibility {
			delete(q.receiptPolledAt, receipt)
		}
	}
}

// brokerIDOf is the message's SQS MessageId, or "" when absent — used on the
// malformed-message path, where parseMessage has already failed and so cannot
// supply it.
func brokerIDOf(sm sqstypes.Message) string {
	if sm.MessageId == nil {
		return ""
	}
	return *sm.MessageId
}
