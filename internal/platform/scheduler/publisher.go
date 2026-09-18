package scheduler

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue/postgres"
)

// PublishItem is one claimed job on its way to a queue: the rendered message
// plus the two ids the destination is resolved from. A claimed job carries
// client_id and subscription_id but neither the client's identifier nor the
// subscription's priority — the claim query is deliberately join-free — so the
// destination is resolved from cached lookups at publish time.
type PublishItem struct {
	JobID          string
	ClientID       string
	SubscriptionID string
	// Queue is the job's OWN stored priority claim (msg_dispatch_jobs.queue,
	// raw and unresolved — "" when the job has none). DestinationResolver
	// consults this before the subscription lookup: see
	// docs/spec/dispatch-job-priority.md R4.
	Queue   string
	Message common.Message
}

// DispatchPublisher hands a claimed batch of dispatch jobs to the queues the
// router consumes from. The poller calls it once per tick, AFTER the claim
// transaction commits.
//
// Publishing is deliberately NOT all-or-nothing. SQS caps a batch send at 10
// while the poller claims up to 100, so an implementation chunks internally
// and a partial failure is the broker's normal operating mode, not an
// exceptional one. Publish returns the job ids it did NOT publish; the caller
// reverts exactly those to PENDING and leaves the rest QUEUED, because a job
// the broker accepted is legitimately queued and reverting it would deliver it
// twice. An implementation must never report a job unpublished that the broker
// accepted, or the reverse: the caller trusts this list exactly.
//
// On a clean run it returns (nil, nil). The error is for logging and carries
// no ids of its own.
type DispatchPublisher interface {
	Publish(ctx context.Context, items []PublishItem) (unpublished []string, err error)
}

// NoopDispatchPublisher drops everything on the floor. It exists for a
// deployment with no resolvable broker, where claimed jobs would otherwise
// block the poller; stale recovery reclaims them. Never silent — see the
// warning at its construction site.
type NoopDispatchPublisher struct{}

// Publish reports everything as published, which is the least-bad lie: the
// alternative reverts the whole batch every tick, so the poller spins on the
// same rows forever instead of letting stale recovery handle them.
func (NoopDispatchPublisher) Publish(context.Context, []PublishItem) ([]string, error) {
	return nil, nil
}

// PostgresDispatchPublisher publishes to the built-in Postgres broker — the
// same queue_messages table the router's own Postgres consumers claim from.
//
// It routes per (tenant, priority) exactly as the SQS publisher does, through
// the same resolver. That is not symmetry for its own sake: the moment the
// router consumes the platform's served document, which advertises composed
// names like platform-DEFAULT, a publisher writing to one fixed queue named
// after the database would put every dev dispatch job where nothing is
// listening.
//
// One statement per batch, so its failures stay all-or-nothing however many
// destination queues the batch spans.
type PostgresDispatchPublisher struct {
	pool         *pgxpool.Pool
	destinations destinationResolver
}

// destinationResolver is the publishers' view of DestinationResolver: the one
// question they ask it. Narrowed to an interface so the publish rules can be
// exercised without a database behind the resolver's caches.
type destinationResolver interface {
	Destination(ctx context.Context, item PublishItem) (string, error)
}

// NewPostgresDispatchPublisher wires the publisher and creates the queue table
// if it is missing.
//
// The platform owns that schema. A router process must NOT create it: doing DDL
// from a consumer build also fails wherever the shared pool cannot hand out a
// connection, and the platform is the only party that knows the table is on
// its own database.
func NewPostgresDispatchPublisher(ctx context.Context, pool *pgxpool.Pool, destinations destinationResolver) (*PostgresDispatchPublisher, error) {
	if err := postgres.InitSchema(ctx, pool); err != nil {
		return nil, fmt.Errorf("init dispatch queue schema: %w", err)
	}
	return &PostgresDispatchPublisher{pool: pool, destinations: destinations}, nil
}

// Publish writes every item as a queue row, each naming its own destination.
func (p *PostgresDispatchPublisher) Publish(ctx context.Context, items []PublishItem) ([]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	rows := make([]postgres.Row, 0, len(items))
	for _, item := range items {
		name, err := p.destinations.Destination(ctx, item)
		if err != nil {
			// A name that cannot be composed is a configuration fault, not a
			// transient one; report the whole batch unpublished so nothing is
			// silently dropped, and let the next poll retry it.
			return jobIDs(items), fmt.Errorf("resolve dispatch destination: %w", err)
		}
		rows = append(rows, postgres.Row{QueueName: name, Message: item.Message})
	}
	if err := postgres.InsertBatch(ctx, p.pool, rows); err != nil {
		return jobIDs(items), fmt.Errorf("postgres dispatch publish: %w", err)
	}
	slog.Debug("dispatch batch published to the built-in postgres broker", "count", len(rows))
	return nil, nil
}

// jobIDs is every job id in the batch — the "nothing was published" answer.
func jobIDs(items []PublishItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.JobID)
	}
	return ids
}
