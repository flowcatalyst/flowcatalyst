package scheduler

import (
	"context"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

// MessageGroupDispatcher publishes claimed dispatch jobs to the message
// queue, enforcing per-message-group FIFO ordering and a global concurrency
// cap. Mirrors fc-platform/src/scheduler/dispatcher.rs.
type MessageGroupDispatcher struct {
	pool      *pgxpool.Pool
	publisher queue.Publisher
	authSvc   *DispatchAuthService
	sem       chan struct{}

	// processingEndpoint is the mediation_target every message carries: the
	// router POSTs {messageId} there and the platform delivers the webhook.
	processingEndpoint string

	mu     sync.Mutex
	groups map[string]*groupQueue
}

type groupQueue struct {
	pending []DispatchJobToken
	running bool
}

// NewMessageGroupDispatcher wires the dispatcher.
func NewMessageGroupDispatcher(pool *pgxpool.Pool, publisher queue.Publisher, authSvc *DispatchAuthService, maxInFlight int, processingEndpoint string) *MessageGroupDispatcher {
	if maxInFlight <= 0 {
		maxInFlight = 1000
	}
	return &MessageGroupDispatcher{
		pool:               pool,
		publisher:          publisher,
		authSvc:            authSvc,
		sem:                make(chan struct{}, maxInFlight),
		processingEndpoint: processingEndpoint,
		groups:             make(map[string]*groupQueue),
	}
}

// Submit enqueues a job for dispatch. Same-group jobs run serially;
// different-group jobs run concurrently under the global semaphore cap.
func (d *MessageGroupDispatcher) Submit(ctx context.Context, tok DispatchJobToken) {
	group := tok.MessageGroup
	if group == "" {
		// No group → fire immediately under the global semaphore.
		go d.dispatch(ctx, tok)
		return
	}
	d.mu.Lock()
	g, ok := d.groups[group]
	if !ok {
		g = &groupQueue{}
		d.groups[group] = g
	}
	g.pending = append(g.pending, tok)
	shouldDrain := !g.running
	if shouldDrain {
		g.running = true
	}
	d.mu.Unlock()

	if shouldDrain {
		go d.drainGroup(ctx, group)
	}
}

func (d *MessageGroupDispatcher) drainGroup(ctx context.Context, group string) {
	for {
		d.mu.Lock()
		g := d.groups[group]
		if g == nil || len(g.pending) == 0 {
			// Fully drained — drop the entry so `groups` doesn't accumulate
			// one empty groupQueue per message-group ID ever seen (unbounded
			// growth with high-cardinality groups). A later Submit re-creates
			// it.
			delete(d.groups, group)
			d.mu.Unlock()
			return
		}
		tok := g.pending[0]
		g.pending = g.pending[1:]
		d.mu.Unlock()

		d.dispatch(ctx, tok)
	}
}

func (d *MessageGroupDispatcher) dispatch(ctx context.Context, tok DispatchJobToken) {
	// Acquire the global semaphore so we never exceed MaxInFlight.
	select {
	case <-ctx.Done():
		return
	case d.sem <- struct{}{}:
	}
	defer func() { <-d.sem }()

	authToken := d.authSvc.Sign(tok.JobID)
	// mediation_target is the platform's processing endpoint, NOT the
	// subscriber URL: the router POSTs {messageId} here and the endpoint
	// loads the job, delivers to job.target_url, records the attempt and
	// advances the status. The signed token lets the endpoint verify the
	// callback really came from a job this scheduler queued.
	msg := common.Message{
		ID:              tok.JobID,
		MediationType:   common.MediationTypeHTTP,
		MediationTarget: d.processingEndpoint,
		AuthToken:       &authToken,
	}
	if tok.MessageGroup != "" {
		msg.MessageGroupID = &tok.MessageGroup
	}
	if _, err := d.publisher.Publish(ctx, msg); err != nil {
		slog.Warn("publish failed; reverting status to PENDING", "job_id", tok.JobID, "err", err)
		// Revert the status so the next poll cycle picks the job up again.
		if _, err := d.pool.Exec(ctx,
			`UPDATE msg_dispatch_jobs SET status = 'PENDING', updated_at = NOW()
			  WHERE id = $1 AND status = 'QUEUED'`, tok.JobID); err != nil {
			slog.Warn("revert status failed", "job_id", tok.JobID, "err", err)
		}
	}
}
