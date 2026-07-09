package scheduler

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
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
	publisher          queue.Publisher
	authSvc            *DispatchAuthService
	processingEndpoint string
}

// NewMessageGroupDispatcher wires the dispatcher.
func NewMessageGroupDispatcher(pool *pgxpool.Pool, publisher queue.Publisher, authSvc *DispatchAuthService, processingEndpoint string) *MessageGroupDispatcher {
	return &MessageGroupDispatcher{
		pool:               pool,
		publisher:          publisher,
		authSvc:            authSvc,
		processingEndpoint: processingEndpoint,
	}
}

// SubmitBatch publishes a batch of claimed jobs in one PublishBatch call. `toks`
// MUST already be in dispatch order (the poller claims them ordered by
// message_group, sequence, created_at); that order is preserved into the batch,
// and the SQS backend chunks it to SendMessageBatch's limit of 10.
//
// On a publish error the batch is reverted QUEUED→PENDING so the next poll
// re-dispatches it. The `status = 'QUEUED'` guard leaves alone any job that
// /api/dispatch/process has already advanced, and a re-published duplicate is
// harmless (FIFO content-dedup + the endpoint's terminal-status check). A crash
// between the caller's commit and this publish leaves rows QUEUED for stale
// recovery — the same failure mode the recovery loop already covers.
func (d *MessageGroupDispatcher) SubmitBatch(ctx context.Context, toks []DispatchJobToken) {
	if len(toks) == 0 {
		return
	}
	msgs := make([]common.Message, len(toks))
	for i, tok := range toks {
		msgs[i] = d.buildMessage(tok)
	}
	if _, err := d.publisher.PublishBatch(ctx, msgs); err != nil {
		ids := make([]string, len(toks))
		for i, tok := range toks {
			ids[i] = tok.JobID
		}
		slog.Warn("batch publish failed; reverting QUEUED→PENDING", "count", len(ids), "err", err)
		if _, err := d.pool.Exec(ctx,
			`UPDATE msg_dispatch_jobs SET status = 'PENDING', updated_at = NOW()
			  WHERE id = ANY($1) AND status = 'QUEUED'`, ids); err != nil {
			slog.Warn("batch revert failed", "err", err)
		}
	}
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
	}
	if tok.MessageGroup != "" {
		group := tok.MessageGroup // copy: don't alias the loop/param variable
		msg.MessageGroupID = &group
	}
	return msg
}
