package sqs

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

const (
	// visibilityBatchMax is SQS's hard limit on entries per ChangeMessageVisibilityBatch.
	visibilityBatchMax = 10
	// visibilityDrainers is how many drainers share a queue's waiting changes.
	visibilityDrainers = 4
	// visibilityQueueDepth is how many changes may wait for a drainer before
	// further enqueues block (honouring their context). The block is deliberate
	// back-pressure: it stops a poll loop deferring faster than SQS accepts.
	visibilityQueueDepth = 4096
)

// visibilityIdleExit is how long a drainer waits with nothing to send before it
// exits; the next enqueue starts drainers again. A var only so tests can shorten it.
var visibilityIdleExit = 30 * time.Second

// visibilityItem is one deferral or nack waiting to be sent.
type visibilityItem struct {
	receipt string
	seconds int32
	// counter is the caller's counter (deferred or nacked), incremented once the
	// broker has answered or the call has failed.
	counter *atomic.Uint64
}

// visibilityBatcher sends deferrals and nacks as ChangeMessageVisibilityBatch
// calls of up to ten without making the caller wait: the caller (a poll loop
// deferring a batch that found its pool full) has nothing to do with the
// answer, so enqueue only queues. As with deleteBatcher there is no fill
// window; a drainer takes whatever is already waiting and sends at once. The
// zero value is usable; everything is created lazily.
//
// Drainers exit when idle and are restarted by the next enqueue, so a queue
// that is no longer used needs no teardown. stop makes drainers flush what is
// queued and exit as soon as the queue is empty; it never rejects an enqueue
// (the router stops pools — which nack every buffered message — before
// consumers, and those nacks follow the stop), it only stops lingering.
type visibilityBatcher struct {
	initOnce sync.Once
	stopOnce sync.Once
	drainers sync.WaitGroup // lets tests wait for drainers to exit

	items  chan visibilityItem
	stopCh chan struct{}

	mu   sync.Mutex
	live int // drainers currently running; guarded by mu
}

func (b *visibilityBatcher) init() {
	b.initOnce.Do(func() {
		b.items = make(chan visibilityItem, visibilityQueueDepth)
		b.stopCh = make(chan struct{})
	})
}

// stop tells drainers to flush the queue and exit once it is empty.
func (b *visibilityBatcher) stop() {
	b.init()
	b.stopOnce.Do(func() { close(b.stopCh) })
}

func (b *visibilityBatcher) stopped() bool {
	select {
	case <-b.stopCh:
		return true
	default:
		return false
	}
}

// enqueue queues the change and returns without waiting for the broker. It
// blocks only while visibilityQueueDepth changes are already waiting, and then
// only until ctx is done.
func (b *visibilityBatcher) enqueue(ctx context.Context, q *Queue, it visibilityItem) error {
	b.init()
	b.ensureDrainers(q)
	select {
	case b.items <- it:
	case <-ctx.Done():
		return ctx.Err()
	}
	// A drainer may have decided to exit between the first ensure and the send.
	b.ensureDrainers(q)
	return nil
}

func (b *visibilityBatcher) ensureDrainers(q *Queue) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.startLocked(q)
}

func (b *visibilityBatcher) startLocked(q *Queue) {
	if b.live != 0 {
		return
	}
	b.live = visibilityDrainers
	for range visibilityDrainers {
		b.drainers.Add(1)
		go b.drain(q)
	}
}

func (b *visibilityBatcher) drain(q *Queue) {
	defer b.drainers.Done()
	idle := time.NewTimer(visibilityIdleExit)
	defer idle.Stop()
	for {
		var first visibilityItem
		if b.stopped() {
			// Stopped: flush what is queued, then go.
			select {
			case first = <-b.items:
			default:
				b.exit(q)
				return
			}
		} else {
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(visibilityIdleExit)
			select {
			case first = <-b.items:
			case <-b.stopCh:
				continue
			case <-idle.C:
				b.exit(q)
				return
			}
		}
		batch := make([]visibilityItem, 1, visibilityBatchMax)
		batch[0] = first
	fill:
		for len(batch) < visibilityBatchMax {
			select {
			case it := <-b.items:
				batch = append(batch, it)
			default:
				break fill
			}
		}
		q.sendVisibilityBatch(batch)
	}
}

// exit retires one drainer. The last one out restarts drainers if an enqueue
// slipped an item in after its final look at the queue.
func (b *visibilityBatcher) exit(q *Queue) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.live--
	if b.live == 0 && len(b.items) > 0 {
		b.startLocked(q)
	}
}

// sendVisibilityBatch issues one ChangeMessageVisibilityBatch (bounded by
// apiCallTimeout). The caller is gone, so a whole-call error and each Failed
// entry are logged; every entry's counter is incremented either way. Entry Ids
// are the batch index.
func (q *Queue) sendVisibilityBatch(batch []visibilityItem) {
	entries := make([]sqstypes.ChangeMessageVisibilityBatchRequestEntry, len(batch))
	for i, it := range batch {
		entries[i] = sqstypes.ChangeMessageVisibilityBatchRequestEntry{
			Id:                aws.String(strconv.Itoa(i)),
			ReceiptHandle:     aws.String(it.receipt),
			VisibilityTimeout: it.seconds,
		}
	}
	ctx, cancel := callCtx(context.Background())
	out, err := q.client.ChangeMessageVisibilityBatch(ctx, &sqs.ChangeMessageVisibilityBatchInput{
		QueueUrl: aws.String(q.queueURL),
		Entries:  entries,
	})
	cancel()
	if err != nil {
		slog.Warn("sqs ChangeMessageVisibilityBatch failed; messages return at their natural visibility timeout instead",
			"queue", q.queueName, "entries", len(batch), "err", err)
	} else {
		for _, f := range out.Failed {
			slog.Warn("sqs ChangeMessageVisibilityBatch entry failed; message returns at its natural visibility timeout instead",
				"queue", q.queueName, "id", aws.ToString(f.Id),
				"code", aws.ToString(f.Code), "message", aws.ToString(f.Message))
		}
	}
	for _, it := range batch {
		it.counter.Add(1)
	}
}
