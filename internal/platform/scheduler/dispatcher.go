package scheduler

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
)

// MessageGroupDispatcher publishes claimed dispatch jobs to the message queue
// in batches (PublishBatch → one SQS SendMessageBatch per 10), so the scheduler
// makes ceil(N/10) round trips instead of N — it never waits on a per-message
// SQS round trip.
//
// Ordering is preserved by the caller + the queue, not by in-process
// serialization: the poller claims tokens in (message_group, sequence,
// created_at) order, this dispatcher keeps that order into the batch, and a
// FIFO queue maintains per-MessageGroupId delivery order across the batch
// chunks. A single active scheduler (leader-gated) keeps one publisher per
// group. Publishing a whole ordered batch is inherently in-order, so this is a
// strictly cheaper way to get the same guarantee the old per-group serial
// dispatcher provided.
type MessageGroupDispatcher struct {
	pool               *pgxpool.Pool
	publisher          DispatchPublisher
	authSvc            *DispatchAuthService
	processingEndpoint string
}

// NewMessageGroupDispatcher wires the dispatcher.
func NewMessageGroupDispatcher(pool *pgxpool.Pool, publisher DispatchPublisher, authSvc *DispatchAuthService, processingEndpoint string) *MessageGroupDispatcher {
	return &MessageGroupDispatcher{
		pool:               pool,
		publisher:          publisher,
		authSvc:            authSvc,
		processingEndpoint: processingEndpoint,
	}
}

// PublishClaim publishes a claimed batch in one Publish call and returns the
// ids the publisher reports it did NOT publish. `toks` MUST already be in
// dispatch order (the poller claims them ordered by message_group, sequence,
// created_at); that order is preserved into the batch, and the SQS backend
// chunks it to SendMessageBatch's limit of 10.
//
// The poller calls it while its claim transaction is still open, then marks
// exactly the published ids QUEUED and commits (see pollOnce). The
// unpublished ids need no revert: their QUEUED status is never written, so
// they stay PENDING for the next poll. The publisher's list is trusted
// exactly — a job the broker accepted must be marked QUEUED, or the next
// poll publishes it again.
func (d *MessageGroupDispatcher) PublishClaim(ctx context.Context, toks []DispatchJobToken) (unpublished []string) {
	if len(toks) == 0 {
		return nil
	}
	items := make([]PublishItem, len(toks))
	for i, tok := range toks {
		items[i] = PublishItem{
			JobID:          tok.JobID,
			ClientID:       tok.ClientID,
			SubscriptionID: tok.SubscriptionID,
			Queue:          tok.Queue,
			Message:        d.buildMessage(tok),
		}
	}
	unpublished, err := d.publisher.Publish(ctx, items)
	if err != nil {
		slog.Warn("dispatch publish failed; the unpublished jobs stay PENDING for the next poll",
			"unpublished", len(unpublished), "of", len(items), "err", err)
	}
	return unpublished
}

// buildMessage renders the queue message for a claimed job. mediation_target is
// the platform processing endpoint (NOT the subscriber URL): the router POSTs
// {messageId} there and that endpoint loads the job, delivers to
// job.target_url, records the attempt, and advances status. The signed token
// lets the endpoint verify the callback came from a job this scheduler queued.
func (d *MessageGroupDispatcher) buildMessage(tok DispatchJobToken) common.Message {
	authToken := d.authSvc.Sign(tok.JobID)
	msg := common.Message{
		ID:              tok.JobID,
		MediationType:   common.MediationTypeHTTP,
		MediationTarget: d.processingEndpoint,
		AuthToken:       &authToken,
		// PoolCode is the resolved, client-namespaced code. Setting it is what
		// puts a subscription's configured pool — its concurrency AND its rate
		// limit — into force; while it was empty every job landed in the
		// router's global DEFAULT-POOL regardless of configuration.
		PoolCode: tok.PoolCode,
		// DispatchMode decides whether the router honours the message group.
		// Omitting it made every job parse as IMMEDIATE, which takes the
		// concurrent path and bypasses the per-group FIFO buffer — the ordering
		// NEXT_ON_ERROR and BLOCK_ON_ERROR promise was lost at the router even
		// though MessageGroupID was set.
		DispatchMode: common.ParseDispatchMode(tok.Mode),
	}
	if tok.MessageGroup != "" {
		group := tok.MessageGroup // copy: don't alias the loop/param variable
		msg.MessageGroupID = &group
	}
	return msg
}
