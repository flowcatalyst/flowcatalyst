package sqs

import (
	"context"
	"errors"
	"fmt"
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
	// deleteBatchMax is SQS's hard limit on entries per DeleteMessageBatch.
	deleteBatchMax = 10
	// deleteMaxHelpers is how many transient helper drainers may run beside the
	// one primary drainer, so a slow round trip does not cap a queue at
	// deleteBatchMax deletes per round trip. Helpers exist only under load.
	deleteMaxHelpers = 3
	// deleteQueueDepth is how many acks may wait for a drainer before further
	// acks block on enqueue (still honouring their context).
	deleteQueueDepth = 1024
)

// deleteLinger is how long the primary drainer waits, in total from the first
// ack of a batch, for more acks before sending a partial batch. Without it every
// drainer grabs each ack the instant it arrives (the runtime runs a just-woken
// goroutine next) and batches average 2-3 entries. It bounds the latency added
// to an ack. A var so tests can change it.
var deleteLinger = time.Millisecond

var errBatcherStopped = errors.New("sqs consumer stopped")

// deleteItem is one ack waiting for its DeleteMessageBatch entry's outcome.
type deleteItem struct {
	receipt string
	done    chan error // buffered(1): the drainer never blocks on a gone caller
}

// deleteBatcher coalesces concurrent acks into DeleteMessageBatch calls. One
// primary drainer per queue takes the first ack, then lingers up to
// deleteLinger (or until 10 are waiting) before sending. When acks pile up
// faster than that (a full batch already waiting), up to deleteMaxHelpers
// transient helpers drain immediately-available acks and exit once the channel
// is empty. Each caller still blocks until the broker answered for ITS receipt.
// The zero value is usable; everything is created lazily.
type deleteBatcher struct {
	initOnce  sync.Once
	startOnce sync.Once
	stopOnce  sync.Once
	drainers  sync.WaitGroup // lets tests wait for drainers to exit after stop
	helpers   atomic.Int32   // running helper drainers, <= deleteMaxHelpers

	items   chan *deleteItem
	stopCh  chan struct{}
	baseCtx context.Context // cancelled on stop; parents every batch call
	cancel  context.CancelFunc
}

func (b *deleteBatcher) init() {
	b.initOnce.Do(func() {
		b.items = make(chan *deleteItem, deleteQueueDepth)
		b.stopCh = make(chan struct{})
		b.baseCtx, b.cancel = context.WithCancel(context.Background())
	})
}

// stop ends the drainers and fails every ack still waiting.
func (b *deleteBatcher) stop() {
	b.init()
	b.stopOnce.Do(func() {
		close(b.stopCh)
		b.cancel()
	})
}

// delete enqueues receipt and waits for its entry's outcome, ctx, or stop.
func (b *deleteBatcher) delete(ctx context.Context, q *Queue, receipt string) error {
	b.init()
	b.startOnce.Do(func() {
		b.drainers.Add(1)
		go b.drain(q)
	})
	it := &deleteItem{receipt: receipt, done: make(chan error, 1)}
	select {
	case b.items <- it:
	case <-ctx.Done():
		return ctx.Err()
	case <-b.stopCh:
		return errBatcherStopped
	}
	b.maybeStartHelper(q)
	select {
	case err := <-it.done:
		return err
	case <-ctx.Done():
		return ctx.Err() // the drainer still completes the item
	case <-b.stopCh:
		// Prefer a real outcome if the drainer already answered.
		select {
		case err := <-it.done:
			return err
		default:
			return errBatcherStopped
		}
	}
}

// maybeStartHelper starts a transient helper when a full batch is already
// waiting and fewer than deleteMaxHelpers are running.
func (b *deleteBatcher) maybeStartHelper(q *Queue) {
	if len(b.items) < deleteBatchMax {
		return
	}
	select {
	case <-b.stopCh:
		return
	default:
	}
	for {
		n := b.helpers.Load()
		if n >= deleteMaxHelpers {
			return
		}
		if b.helpers.CompareAndSwap(n, n+1) {
			break
		}
	}
	b.drainers.Add(1)
	go b.helper(q)
}

// failWaiting answers every queued ack with errBatcherStopped.
func (b *deleteBatcher) failWaiting() {
	for {
		select {
		case it := <-b.items:
			it.done <- errBatcherStopped
		default:
			return
		}
	}
}

// drain is the primary drainer: block for the first ack, linger for more, send.
func (b *deleteBatcher) drain(q *Queue) {
	defer b.drainers.Done()
	for {
		select {
		case <-b.stopCh:
			b.failWaiting()
			return
		case first := <-b.items:
			batch := make([]*deleteItem, 1, deleteBatchMax)
			batch[0] = first
			timer := time.NewTimer(deleteLinger)
		fill:
			for len(batch) < deleteBatchMax {
				select {
				case it := <-b.items:
					batch = append(batch, it)
				case <-timer.C:
					break fill
				case <-b.stopCh:
					timer.Stop()
					for _, it := range batch {
						it.done <- errBatcherStopped
					}
					b.failWaiting()
					return
				}
			}
			timer.Stop()
			q.sendDeleteBatch(b.baseCtx, batch)
		}
	}
}

// helper drains immediately-available acks without lingering and exits when
// none are left (or on stop).
func (b *deleteBatcher) helper(q *Queue) {
	defer b.drainers.Done()
	defer b.helpers.Add(-1)
	for {
		select {
		case <-b.stopCh:
			b.failWaiting()
			return
		default:
		}
		var batch []*deleteItem
	fill:
		for len(batch) < deleteBatchMax {
			select {
			case it := <-b.items:
				batch = append(batch, it)
			default:
				break fill
			}
		}
		if len(batch) == 0 {
			return
		}
		q.sendDeleteBatch(b.baseCtx, batch)
	}
}

// sendDeleteBatch issues one DeleteMessageBatch (bounded by apiCallTimeout) and
// answers every item: a whole-call error fails all of them, a Failed entry fails
// just its own. Entry Ids are the batch index.
func (q *Queue) sendDeleteBatch(parent context.Context, batch []*deleteItem) {
	entries := make([]sqstypes.DeleteMessageBatchRequestEntry, len(batch))
	for i, it := range batch {
		entries[i] = sqstypes.DeleteMessageBatchRequestEntry{
			Id:            aws.String(strconv.Itoa(i)),
			ReceiptHandle: aws.String(it.receipt),
		}
	}
	ctx, cancel := callCtx(parent)
	out, err := q.client.DeleteMessageBatch(ctx, &sqs.DeleteMessageBatchInput{
		QueueUrl: aws.String(q.queueURL),
		Entries:  entries,
	})
	cancel()
	if err != nil {
		err = fmt.Errorf("DeleteMessageBatch: %w", err)
		slog.Warn("sqs DeleteMessageBatch failed", "queue", q.queueName, "entries", len(batch), "err", err)
		for _, it := range batch {
			it.done <- err
		}
		return
	}
	failed := make(map[string]sqstypes.BatchResultErrorEntry, len(out.Failed))
	for _, f := range out.Failed {
		if f.Id != nil {
			failed[*f.Id] = f
		}
	}
	for i, it := range batch {
		if f, bad := failed[strconv.Itoa(i)]; bad {
			slog.Warn("sqs DeleteMessageBatch entry failed", "queue", q.queueName,
				"code", aws.ToString(f.Code), "message", aws.ToString(f.Message))
			it.done <- fmt.Errorf("DeleteMessageBatch entry: %s: %s", aws.ToString(f.Code), aws.ToString(f.Message))
			continue
		}
		q.acked.Add(1)
		it.done <- nil
	}
}
