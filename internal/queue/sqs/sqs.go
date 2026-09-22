// Package sqs is the AWS SQS-backed queue backend.
//
//   - 20s long-poll (AWS max) for low API-call rate.
//   - Visibility timeout configurable per queue.
//   - Pending-delete guard for at-least-once redeliveries: once we
//     successfully (or unsuccessfully) DeleteMessage for a MessageId,
//     subsequent redeliveries within PendingDeleteTTL are deleted
//     immediately on poll instead of being routed to the mediator.
package sqs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	neturl "net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// PendingDeleteTTL is how long we remember an acked MessageId so
// redeliveries (SQS standard queues are at-least-once) are
// short-circuited to DeleteMessage. 15 minutes.
const PendingDeleteTTL = 15 * time.Minute

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
	client := sqs.NewFromConfig(awsCfg)
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
		pendingDelete:     make(map[string]time.Time),
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
	pendingDelete map[string]time.Time
	// receiptPolledAt records when THIS consumer's ReceiveMessage first
	// handed out each currently-outstanding receipt handle — Nack's clamp
	// (R3) measures SQS's 12-hour ceiling from here, since SQS itself counts
	// it from the original receive, not from any one ChangeMessageVisibility
	// call. Entries are removed on Ack/Nack (the receipt is then spent —
	// this consumer never sees it again as a live handle) and swept for
	// staleness on every Poll, so growth is bounded by outstanding receipts,
	// not by every receipt ever issued.
	receiptPolledAt map[string]time.Time

	running atomic.Bool

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
		QueueUrl:                    aws.String(q.queueURL),
		MaxNumberOfMessages:         max,
		VisibilityTimeout:           q.visibilityTimeout,
		WaitTimeSeconds:             q.waitSeconds,
		MessageSystemAttributeNames: []sqstypes.MessageSystemAttributeName{sqstypes.MessageSystemAttributeNameAll},
		MessageAttributeNames:       []string{"All"},
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
					_, _ = q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
						QueueUrl:      aws.String(q.queueURL),
						ReceiptHandle: sm.ReceiptHandle,
					})
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
	if err := json.Unmarshal([]byte(*sm.Body), &m); err != nil {
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
// Retention stays bounded: entries are pruned by PendingDeleteTTL on every poll,
// so this remembers recent deletes, never all of them.
func (q *Queue) Ack(ctx context.Context, receipt string, brokerMessageID string) error {
	q.markDeleted(brokerMessageID)
	q.forgetReceipt(receipt)

	_, err := q.client.DeleteMessage(ctx, &sqs.DeleteMessageInput{
		QueueUrl:      aws.String(q.queueURL),
		ReceiptHandle: aws.String(receipt),
	})
	if err != nil {
		return fmt.Errorf("sqs DeleteMessage: %w", err)
	}
	q.acked.Add(1)
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
	defer q.nacked.Add(1)
	return q.returnAfter(ctx, receipt, delaySeconds)
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
	defer q.deferred.Add(1)
	return q.returnAfter(ctx, receipt, delaySeconds)
}

// returnAfter is the shared body of Nack and Defer: make receipt visible
// again after delaySeconds (clamped — see clampToRemainingVisibility), and
// forget the receipt. Best-effort, per the queue.Consumer contract.
func (q *Queue) returnAfter(ctx context.Context, receipt string, delaySeconds *uint32) error {
	defer q.forgetReceipt(receipt)

	var seconds uint32
	if delaySeconds != nil {
		seconds = *delaySeconds
	}
	clamped := q.clampToRemainingVisibility(receipt, seconds)

	_, err := q.client.ChangeMessageVisibility(ctx, &sqs.ChangeMessageVisibilityInput{
		QueueUrl:          aws.String(q.queueURL),
		ReceiptHandle:     aws.String(receipt),
		VisibilityTimeout: clamped,
	})
	if err != nil {
		slog.Warn("sqs ChangeMessageVisibility failed; message returns at its natural visibility timeout instead",
			"queue", q.queueName, "err", err)
	}
	return nil
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
func (q *Queue) Stop() { q.running.Store(false) }

// Metrics calls GetQueueAttributes for pending + in-flight counts.
func (q *Queue) Metrics(ctx context.Context) (*queue.Metrics, error) {
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

// evictExpiredPendingDeletesLocked prunes acked-MessageId entries older than
// PendingDeleteTTL, so the guard remembers recent deletes rather than every
// delete ever. Called at the top of each poll. Holds the lock.
func (q *Queue) evictExpiredPendingDeletesLocked() {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now()
	for id, ts := range q.pendingDelete {
		if now.Sub(ts) > PendingDeleteTTL {
			delete(q.pendingDelete, id)
		}
	}
}

// markDeleted records that this MessageId has been deleted, so a redelivery
// already in flight can be short-circuited. An empty id is ignored — there is
// nothing to key on, and inserting "" would match every id-less message.
func (q *Queue) markDeleted(brokerMessageID string) {
	if brokerMessageID == "" {
		return
	}
	q.mu.Lock()
	q.pendingDelete[brokerMessageID] = time.Now()
	q.mu.Unlock()
}

// alreadyDeleted reports whether this MessageId was deleted recently enough to
// still be remembered (see PendingDeleteTTL).
func (q *Queue) alreadyDeleted(brokerMessageID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.pendingDelete[brokerMessageID]
	return ok
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
