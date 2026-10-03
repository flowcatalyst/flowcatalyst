package sqs

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

const (
	// deleteBatchMax is SQS's hard limit on entries per DeleteMessageBatch.
	deleteBatchMax = 10
	// deleteDrainers is how many long-lived drainers a queue runs, so one slow
	// round trip does not cap the queue at deleteBatchMax deletes per round trip.
	deleteDrainers = 4
	// deleteQueueDepth is how many acks may wait for a drainer before further
	// acks block on enqueue (still honouring their context).
	deleteQueueDepth = 1024
)

var errBatcherStopped = errors.New("sqs consumer stopped")

// deleteItem is one ack waiting for its DeleteMessageBatch entry's outcome.
type deleteItem struct {
	receipt string
	done    chan error // buffered(1): the drainer never blocks on a gone caller
}

// deleteBatcher coalesces concurrent acks into DeleteMessageBatch calls. There
// is no fill window: a drainer takes whatever is already waiting (up to 10) and
// sends at once, so a lone ack deletes with no added latency. Each caller still
// blocks until the broker answered for ITS receipt. The zero value is usable;
// everything is created lazily.
type deleteBatcher struct {
	initOnce  sync.Once
	startOnce sync.Once
	stopOnce  sync.Once
	drainers  sync.WaitGroup // lets tests wait for drainers to exit after stop

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
		for range deleteDrainers {
			b.drainers.Add(1)
			go b.drain(q)
		}
	})
	it := &deleteItem{receipt: receipt, done: make(chan error, 1)}
	select {
	case b.items <- it:
	case <-ctx.Done():
		return ctx.Err()
	case <-b.stopCh:
		return errBatcherStopped
	}
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

func (b *deleteBatcher) drain(q *Queue) {
	defer b.drainers.Done()
	for {
		select {
		case <-b.stopCh:
			for {
				select {
				case it := <-b.items:
					it.done <- errBatcherStopped
				default:
					return
				}
			}
		case first := <-b.items:
			batch := make([]*deleteItem, 1, deleteBatchMax)
			batch[0] = first
		fill:
			for len(batch) < deleteBatchMax {
				select {
				case it := <-b.items:
					batch = append(batch, it)
				default:
					break fill
				}
			}
			q.sendDeleteBatch(b.baseCtx, batch)
		}
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
