package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/google/uuid"

	"github.com/flowcatalyst/flowcatalyst-go/internal/platform/dispatch"
)

// maxSQSBatchSize is SQS's hard cap on one SendMessageBatch — well below the
// poller's claim size, which is why publishing chunks.
const maxSQSBatchSize = 10

// maxDedupIDLength is SQS's hard cap on a MessageDeduplicationId.
const maxDedupIDLength = 128

// sqsSendClient is the slice of the SQS API this publisher uses, so tests can
// substitute one.
type sqsSendClient interface {
	SendMessageBatch(ctx context.Context, in *sqs.SendMessageBatchInput, optFns ...func(*sqs.Options)) (*sqs.SendMessageBatchOutput, error)
	CreateQueue(ctx context.Context, in *sqs.CreateQueueInput, optFns ...func(*sqs.Options)) (*sqs.CreateQueueOutput, error)
}

// SQSDispatchPublisher publishes each claimed job to its client's SQS FIFO
// queue for its dispatch priority, creating the queue on first use.
//
// # Chunking and per-queue grouping
//
// The poller claims up to 100 jobs; SQS caps a batch at 10 and one call
// addresses a single queue. Publish partitions the claim-ordered batch into one
// ordered list per destination, then chunks each list. Two jobs bound for
// different queues have no ordering relationship to preserve in the first
// place, so grouping by destination is not a behavioural change; within one
// destination the single forward pass preserves claim order exactly.
//
// # No two jobs of one group ever share a chunk
//
// This is load-bearing, not defensive. The claim orders by message_group, so
// two jobs of a group are adjacent and would routinely land in the same chunk.
// SQS reports batch failures PER ENTRY: if the earlier of the two fails and the
// later succeeds, the later is durably QUEUED while the earlier reverts to
// PENDING and is published again afterwards — the group is now delivered out of
// order. Two rules together prevent it:
//
//  1. A candidate whose group is already in the chunk being built closes that
//     chunk early, without being consumed. A group-less job uses its own job id
//     as its group, which nothing else shares, so group-less jobs still pack
//     fully.
//  2. A group that fails poisons its own later jobs for the rest of the call.
//     Any job whose group is already known to have failed is reported
//     unpublished without ever being sent. So a group is either delivered in
//     claim order, or truncated at its first failure with every later member
//     reverted alongside it — never reordered.
//
// # FIFO identifiers
//
// MessageGroupId is the job's message group, or its own job id when it has
// none: FIFO requires the field, and a singleton group imposes no ordering,
// which is exactly what a group-less job should have.
//
// MessageDeduplicationId is the job id plus a nonce generated fresh for every
// Publish call. That is the whole point rather than a formality: SQS FIFO drops
// a repeated dedup id within a five-minute window, and stale recovery reverts a
// stranded QUEUED job to PENDING for the ordinary path to publish again — a
// second, genuinely intended delivery. Reusing the job id would let the broker
// silently swallow it, stranding the job for good. The platform's own status
// and scheduled_for machinery stays the only deduplication authority; the
// broker-native id exists because FIFO demands one, and is deliberately unique
// per attempt so the broker can never deduplicate anything itself.
type SQSDispatchPublisher struct {
	client       sqsSendClient
	settings     dispatch.Settings
	destinations destinationResolver
}

// NewSQSDispatchPublisher builds a publisher against the SDK's default
// credential chain, in the region the settings resolved.
func NewSQSDispatchPublisher(ctx context.Context, settings dispatch.Settings, destinations destinationResolver) (*SQSDispatchPublisher, error) {
	var opts []func(*awsconfig.LoadOptions) error
	if settings.SQSRegion != "" {
		opts = append(opts, awsconfig.WithRegion(settings.SQSRegion))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("aws config: %w", err)
	}
	return &SQSDispatchPublisher{
		client:       sqs.NewFromConfig(awsCfg),
		settings:     settings,
		destinations: destinations,
	}, nil
}

// Publish sends every item to its destination queue, returning the job ids it
// did not publish.
func (p *SQSDispatchPublisher) Publish(ctx context.Context, items []PublishItem) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}

	order, byQueue, unpublished, lastErr := p.partition(ctx, items)

	// One nonce per Publish call, shared by every chunk: the same job
	// published on two different calls always gets two different dedup ids.
	nonce := uuid.NewString()
	// Scoped to this one call and shared across every destination and chunk.
	// A group id is a plain string that nothing else can share, so one flat
	// set is correct however many destinations are involved.
	failedGroups := make(map[string]bool)

	for _, queueName := range order {
		queued := byQueue[queueName]
		for i := 0; i < len(queued); {
			chunk, next := p.nextChunk(queued, i, failedGroups, &unpublished)
			i = next
			if len(chunk) == 0 {
				continue
			}
			failedIDs, err := p.sendChunk(ctx, queueName, chunk, nonce)
			if err != nil {
				// Every other chunk and every other group is still attempted:
				// one client's queue must not strand every other client's jobs
				// in the same claimed batch.
				lastErr = err
				for _, item := range chunk {
					unpublished = append(unpublished, item.JobID)
					failedGroups[groupIDFor(item)] = true
				}
				slog.Warn("sqs dispatch chunk failed; job(s) will revert to PENDING",
					"queue", queueName, "count", len(chunk), "err", err)
				continue
			}
			for _, item := range chunk {
				if failedIDs[item.JobID] {
					unpublished = append(unpublished, item.JobID)
					failedGroups[groupIDFor(item)] = true
				}
			}
		}
	}

	if len(unpublished) > 0 {
		return unpublished, fmt.Errorf("sqs dispatch publish failed for %d of %d job(s): %w",
			len(unpublished), len(items), lastErr)
	}
	return nil, nil
}

// partition groups the batch by destination queue, preserving claim order
// within each. An item whose destination cannot be composed is reported
// unpublished rather than failing the whole batch.
func (p *SQSDispatchPublisher) partition(ctx context.Context, items []PublishItem) (order []string, byQueue map[string][]PublishItem, unpublished []string, lastErr error) {
	byQueue = make(map[string][]PublishItem)
	for _, item := range items {
		name, err := p.destinations.Destination(ctx, item)
		if err != nil {
			lastErr = err
			unpublished = append(unpublished, item.JobID)
			slog.Warn("could not resolve a dispatch destination; job will revert to PENDING",
				"job_id", item.JobID, "err", err)
			continue
		}
		if _, seen := byQueue[name]; !seen {
			order = append(order, name)
		}
		byQueue[name] = append(byQueue[name], item)
	}
	return order, byQueue, unpublished, lastErr
}

// nextChunk builds the next chunk from queued[start:], returning it and the
// index to resume at. Poisoned jobs are appended to unpublished without being
// sent; a repeated group closes the chunk without consuming the candidate.
func (p *SQSDispatchPublisher) nextChunk(queued []PublishItem, start int, failedGroups map[string]bool, unpublished *[]string) ([]PublishItem, int) {
	chunk := make([]PublishItem, 0, maxSQSBatchSize)
	inChunk := make(map[string]bool, maxSQSBatchSize)
	i := start
	for i < len(queued) && len(chunk) < maxSQSBatchSize {
		candidate := queued[i]
		group := groupIDFor(candidate)
		if failedGroups[group] {
			// Never sent at all: it reverts alongside the sibling that
			// actually failed, keeping the group's relative order rather than
			// racing a later publish against whatever a real send might do.
			*unpublished = append(*unpublished, candidate.JobID)
			i++
			continue
		}
		if inChunk[group] {
			break // starts the next chunk, candidate unconsumed
		}
		chunk = append(chunk, candidate)
		inChunk[group] = true
		i++
	}
	return chunk, i
}

// sendChunk sends one chunk to one queue, creating the queue and retrying once
// when it does not exist yet. Returns the job ids SQS itself reported failed.
func (p *SQSDispatchPublisher) sendChunk(ctx context.Context, queueName string, chunk []PublishItem, nonce string) (map[string]bool, error) {
	queueURL := p.settings.QueueURIFor(queueName)
	entries := make([]sqstypes.SendMessageBatchRequestEntry, 0, len(chunk))
	for _, item := range chunk {
		body, err := json.Marshal(item.Message)
		if err != nil {
			return nil, fmt.Errorf("marshal dispatch message %q: %w", item.JobID, err)
		}
		entries = append(entries, sqstypes.SendMessageBatchRequestEntry{
			Id:                     aws.String(item.JobID),
			MessageBody:            aws.String(string(body)),
			MessageGroupId:         aws.String(groupIDFor(item)),
			MessageDeduplicationId: aws.String(dedupID(item.JobID, nonce)),
		})
	}
	in := &sqs.SendMessageBatchInput{QueueUrl: aws.String(queueURL), Entries: entries}

	out, err := p.client.SendMessageBatch(ctx, in)
	if err != nil {
		if !isQueueMissing(err) {
			return nil, err
		}
		if cerr := p.createQueue(ctx, queueName); cerr != nil {
			return nil, cerr
		}
		// The same request, with the same dedup ids: the first attempt never
		// reached a queue that could have deduplicated anything, so this is
		// still the one publish attempt, not a second.
		out, err = p.client.SendMessageBatch(ctx, in)
		if err != nil {
			return nil, err
		}
	}
	failed := make(map[string]bool, len(out.Failed))
	for _, f := range out.Failed {
		if f.Id != nil {
			failed[*f.Id] = true
		}
	}
	return failed, nil
}

// createQueue creates the FIFO queue. Content-based deduplication is OFF: this
// publisher always supplies its own dedup id, and content hashing could not see
// the per-attempt nonce that makes a genuine re-publish work.
func (p *SQSDispatchPublisher) createQueue(ctx context.Context, queueName string) error {
	_, err := p.client.CreateQueue(ctx, &sqs.CreateQueueInput{
		QueueName: aws.String(queueName),
		Attributes: map[string]string{
			string(sqstypes.QueueAttributeNameFifoQueue):                 "true",
			string(sqstypes.QueueAttributeNameContentBasedDeduplication): "false",
		},
	})
	if err != nil {
		// Another instance (or another chunk of this same call) created it
		// between our failed send and here. The queue exists now, which is all
		// this promises.
		if _, ok := errors.AsType[*sqstypes.QueueNameExists](err); ok {
			return nil
		}
		return fmt.Errorf("create dispatch queue %q: %w", queueName, err)
	}
	slog.Info("created dispatch queue on first publish", "queue", queueName)
	return nil
}

// isQueueMissing reports whether err is SQS's "that queue does not exist".
func isQueueMissing(err error) bool {
	var missing *sqstypes.QueueDoesNotExist
	return errors.As(err, &missing)
}

// groupIDFor is the FIFO MessageGroupId: the job's own message group, or its
// job id when it has none.
func groupIDFor(item PublishItem) string {
	if item.Message.MessageGroupID != nil && *item.Message.MessageGroupID != "" {
		return *item.Message.MessageGroupID
	}
	return item.JobID
}

// dedupID is the job id plus this call's nonce — never the bare job id. See the
// type's doc for why that distinction is the point.
func dedupID(jobID, nonce string) string {
	id := jobID + ":" + nonce
	// Defensive: a TSID job id plus a UUID nonce never approaches the limit,
	// but a future id scheme must not silently produce an invalid request.
	if len(id) > maxDedupIDLength {
		return id[:maxDedupIDLength]
	}
	return id
}
