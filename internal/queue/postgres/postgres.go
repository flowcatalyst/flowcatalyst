// Package postgres is the Postgres-backed queue backend. Mirrors the
// Rust fc-queue postgres feature: messages stored in a fc_queue_messages
// table with claim semantics via SELECT FOR UPDATE SKIP LOCKED.
//
// Schema (created by InitSchema):
//
//	CREATE TABLE fc_queue_messages (
//	    id              TEXT PRIMARY KEY,
//	    queue_name      TEXT NOT NULL,
//	    body            JSONB NOT NULL,
//	    visible_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
//	    receipt_handle  TEXT,
//	    received_count  INT NOT NULL DEFAULT 0,
//	    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
//	);
//	CREATE INDEX fc_queue_messages_pending_idx
//	    ON fc_queue_messages (queue_name, visible_at)
//	    WHERE receipt_handle IS NULL;
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/flowcatalyst/flowcatalyst-go/internal/common"
	"github.com/flowcatalyst/flowcatalyst-go/internal/queue"
)

func init() {
	queue.RegisterConsumer("postgres", consumerFactory)
	queue.RegisterPublisher("postgres", publisherFactory)
}

func consumerFactory(ctx context.Context, cfg common.QueueConfig) (queue.Consumer, error) {
	pool, err := pgxpool.New(ctx, cfg.URI)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	return &Queue{pool: pool, cfg: cfg}, nil
}

func publisherFactory(ctx context.Context, cfg common.QueueConfig) (queue.Publisher, error) {
	pool, err := pgxpool.New(ctx, cfg.URI)
	if err != nil {
		return nil, fmt.Errorf("pgxpool: %w", err)
	}
	return &Queue{pool: pool, cfg: cfg}, nil
}

// Queue is the Postgres-backed queue (both consumer + publisher).
type Queue struct {
	pool *pgxpool.Pool
	cfg  common.QueueConfig

	polled   atomic.Uint64
	acked    atomic.Uint64
	nacked   atomic.Uint64
	deferred atomic.Uint64
}

// Identifier returns the queue name.
func (q *Queue) Identifier() string { return q.cfg.Name }

// InitSchema creates the queue table and indexes.
func (q *Queue) InitSchema(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS fc_queue_messages (
    id             TEXT PRIMARY KEY,
    queue_name     TEXT NOT NULL,
    body           JSONB NOT NULL,
    visible_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    receipt_handle TEXT,
    received_count INT NOT NULL DEFAULT 0,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS fc_queue_messages_pending_idx
    ON fc_queue_messages (queue_name, visible_at)
    WHERE receipt_handle IS NULL;
`
	_, err := q.pool.Exec(ctx, ddl)
	return err
}

// Poll claims up to maxMessages messages from this queue.
func (q *Queue) Poll(ctx context.Context, maxMessages uint32) ([]common.QueuedMessage, error) {
	receipt := uuid.NewString()
	visibility := time.Duration(q.cfg.VisibilityTimeout) * time.Second
	if visibility <= 0 {
		visibility = 30 * time.Second
	}
	const sql = `
WITH claimed AS (
  SELECT id FROM fc_queue_messages
  WHERE queue_name = $1
    AND visible_at <= NOW()
    AND receipt_handle IS NULL
  ORDER BY visible_at
  LIMIT $2
  FOR UPDATE SKIP LOCKED
)
UPDATE fc_queue_messages m
   SET receipt_handle = $3,
       visible_at     = NOW() + $4 * INTERVAL '1 second',
       received_count = received_count + 1
  FROM claimed
 WHERE m.id = claimed.id
 RETURNING m.id, m.body
`
	rows, err := q.pool.Query(ctx, sql, q.cfg.Name, maxMessages, receipt, int(visibility.Seconds()))
	if err != nil {
		return nil, fmt.Errorf("postgres queue poll: %w", err)
	}
	defer rows.Close()

	var msgs []common.QueuedMessage
	for rows.Next() {
		var id string
		var body []byte
		if err := rows.Scan(&id, &body); err != nil {
			return nil, err
		}
		var m common.Message
		if err := json.Unmarshal(body, &m); err != nil {
			return nil, fmt.Errorf("unmarshal message %s: %w", id, err)
		}
		msgs = append(msgs, common.QueuedMessage{
			Message:         m,
			ReceiptHandle:   receipt + ":" + id,
			BrokerMessageID: id,
			QueueIdentifier: q.cfg.Name,
		})
	}
	q.polled.Add(uint64(len(msgs)))
	return msgs, rows.Err()
}

// Ack deletes the message permanently.
func (q *Queue) Ack(ctx context.Context, receipt string) error {
	id := messageIDFromReceipt(receipt)
	if id == "" {
		return errors.New("ack: malformed receipt handle")
	}
	_, err := q.pool.Exec(ctx, `DELETE FROM fc_queue_messages WHERE id = $1`, id)
	if err != nil {
		return err
	}
	q.acked.Add(1)
	return nil
}

// Nack restores visibility after delay; counted as a failure.
func (q *Queue) Nack(ctx context.Context, receipt string, delaySeconds *uint32) error {
	if err := q.makeVisible(ctx, receipt, delaySeconds); err != nil {
		return err
	}
	q.nacked.Add(1)
	return nil
}

// Defer restores visibility after delay; not counted as a failure.
func (q *Queue) Defer(ctx context.Context, receipt string, delaySeconds *uint32) error {
	if err := q.makeVisible(ctx, receipt, delaySeconds); err != nil {
		return err
	}
	q.deferred.Add(1)
	return nil
}

// ExtendVisibility prolongs the visibility window without releasing.
func (q *Queue) ExtendVisibility(ctx context.Context, receipt string, seconds uint32) error {
	id := messageIDFromReceipt(receipt)
	if id == "" {
		return errors.New("extend: malformed receipt handle")
	}
	_, err := q.pool.Exec(ctx,
		`UPDATE fc_queue_messages SET visible_at = NOW() + $1 * INTERVAL '1 second' WHERE id = $2`,
		seconds, id)
	return err
}

// Publish writes a single message.
func (q *Queue) Publish(ctx context.Context, m common.Message) (string, error) {
	body, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	id := uuid.NewString()
	_, err = q.pool.Exec(ctx,
		`INSERT INTO fc_queue_messages (id, queue_name, body) VALUES ($1, $2, $3::jsonb)`,
		id, q.cfg.Name, body)
	return id, err
}

// PublishBatch writes a batch of messages in one transaction.
func (q *Queue) PublishBatch(ctx context.Context, msgs []common.Message) ([]string, error) {
	tx, err := q.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	ids := make([]string, 0, len(msgs))
	batch := &pgx.Batch{}
	for _, m := range msgs {
		body, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		id := uuid.NewString()
		ids = append(ids, id)
		batch.Queue(
			`INSERT INTO fc_queue_messages (id, queue_name, body) VALUES ($1, $2, $3::jsonb)`,
			id, q.cfg.Name, body)
	}
	br := tx.SendBatch(ctx, batch)
	for range msgs {
		if _, err := br.Exec(); err != nil {
			_ = br.Close()
			return nil, err
		}
	}
	if err := br.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return ids, nil
}

// Healthy reports whether we can talk to Postgres.
func (q *Queue) Healthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return q.pool.Ping(ctx) == nil
}

// Stop closes the connection pool.
func (q *Queue) Stop() { q.pool.Close() }

// Metrics returns broker-side metrics. Reads counts from the table.
func (q *Queue) Metrics(ctx context.Context) (*queue.Metrics, error) {
	var pending, inflight uint64
	err := q.pool.QueryRow(ctx,
		`SELECT
		   COUNT(*) FILTER (WHERE receipt_handle IS NULL AND visible_at <= NOW()),
		   COUNT(*) FILTER (WHERE receipt_handle IS NOT NULL)
		 FROM fc_queue_messages WHERE queue_name = $1`,
		q.cfg.Name,
	).Scan(&pending, &inflight)
	if err != nil {
		return nil, err
	}
	return &queue.Metrics{
		QueueIdentifier:  q.cfg.Name,
		PendingMessages:  pending,
		InFlightMessages: inflight,
		TotalPolled:      q.polled.Load(),
		TotalAcked:       q.acked.Load(),
		TotalNacked:      q.nacked.Load(),
		TotalDeferred:    q.deferred.Load(),
	}, nil
}

// Counters returns process-local counters only.
func (q *Queue) Counters() *queue.Metrics {
	return &queue.Metrics{
		QueueIdentifier: q.cfg.Name,
		TotalPolled:     q.polled.Load(),
		TotalAcked:      q.acked.Load(),
		TotalNacked:     q.nacked.Load(),
		TotalDeferred:   q.deferred.Load(),
	}
}

func (q *Queue) makeVisible(ctx context.Context, receipt string, delaySeconds *uint32) error {
	id := messageIDFromReceipt(receipt)
	if id == "" {
		return errors.New("malformed receipt handle")
	}
	delay := uint32(0)
	if delaySeconds != nil {
		delay = *delaySeconds
	}
	_, err := q.pool.Exec(ctx,
		`UPDATE fc_queue_messages
		    SET receipt_handle = NULL,
		        visible_at = NOW() + $1 * INTERVAL '1 second'
		  WHERE id = $2`,
		delay, id)
	return err
}

func messageIDFromReceipt(r string) string {
	for i := range len(r) {
		if r[i] == ':' {
			return r[i+1:]
		}
	}
	return ""
}
