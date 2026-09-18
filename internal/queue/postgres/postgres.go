// Package postgres is the Postgres-backed queue backend. It is wire- and
// schema-compatible with existing deployments, so a Go router can be
// dropped in and drain the SAME queue_messages table that the existing
// producers write to (and vice versa).
//
// Schema (created by InitSchema; matches the pre-existing layout):
//
//	CREATE TABLE queue_messages (
//	    id               TEXT NOT NULL,
//	    queue_name       TEXT NOT NULL,
//	    message_group_id TEXT,
//	    receipt_handle   TEXT,
//	    visible_at       BIGINT NOT NULL,   -- unix epoch seconds
//	    payload          TEXT NOT NULL,     -- JSON-encoded common.Message
//	    created_at       BIGINT NOT NULL,   -- unix epoch seconds
//	    receive_count    INTEGER DEFAULT 0,
//	    PRIMARY KEY (queue_name, id)
//	);
//	CREATE INDEX idx_queue_visible
//	    ON queue_messages (queue_name, visible_at, message_group_id);
//
// Semantics:
//   - Claim is keyed on visible_at, NOT on receipt_handle being NULL. A
//     claimed message becomes eligible again once its visibility window
//     lapses, so a crashed consumer's messages are redelivered (at-least-once)
//     without an explicit NACK.
//   - FIFO per message group: only the earliest visible message of each
//     group (COALESCE(message_group_id, id)) is eligible at a time, so a
//     NULL group behaves as a singleton keyed by the message id.
package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
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

	// stopped is set by Stop before the pool is closed, so a Poll racing the
	// shutdown returns queue.ErrStopped (terminal) rather than an opaque
	// closed-pool error the router's poll loop would treat as transient.
	stopped atomic.Bool
}

// Identifier returns the queue name.
func (q *Queue) Identifier() string { return q.cfg.Name }

// InitSchema creates the queue table and index (idempotent). The DDL
// matches the pre-existing layout exactly so it is a no-op when the table
// was already provisioned by the existing system.
func (q *Queue) InitSchema(ctx context.Context) error { return InitSchema(ctx, q.pool) }

// InitSchema creates the queue tables on an existing pool. Exported for the
// party that OWNS the database the queue lives on — the platform, whose
// dispatch scheduler publishes there. A router process must not call it
// against the platform's database: the platform owns that schema, and running
// DDL from a consumer build fails wherever the shared pool cannot hand out a
// connection.
func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS queue_messages (
    id               TEXT NOT NULL,
    queue_name       TEXT NOT NULL,
    message_group_id TEXT,
    receipt_handle   TEXT,
    visible_at       BIGINT NOT NULL,
    payload          TEXT NOT NULL,
    created_at       BIGINT NOT NULL,
    receive_count    INTEGER DEFAULT 0,
    PRIMARY KEY (queue_name, id)
);
CREATE INDEX IF NOT EXISTS idx_queue_visible
    ON queue_messages (queue_name, visible_at, message_group_id);

-- Quarantine for rows whose payload cannot be parsed. Without it a single
-- malformed row stops its queue forever: the claim commits before the payload
-- is parsed, so the failure leaves the row claimed, it becomes visible again,
-- is re-claimed, and fails identically on every subsequent poll.
--
-- The name is shared with the other implementation of this queue on purpose.
-- Two names would mean two places for an operator to look, and switching
-- implementations either way would silently move where quarantined rows land —
-- making everything written before the switch invisible to the tooling that
-- runs after it. That is a migration hazard, not a naming preference.
CREATE TABLE IF NOT EXISTS queue_messages_failed (
    id               TEXT NOT NULL,
    queue_name       TEXT NOT NULL,
    message_group_id TEXT,
    payload          TEXT NOT NULL,
    error_message    TEXT NOT NULL,
    receive_count    INTEGER,
    created_at       BIGINT NOT NULL,
    failed_at        BIGINT NOT NULL,
    PRIMARY KEY (queue_name, id)
);

-- Carry over anything quarantined under the previous name, so an upgrade does
-- not strand the rows an operator is most likely to be looking for. Cheap and
-- self-limiting: the old table exists only where it was already created, and
-- once drained it is dropped.
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.tables
                WHERE table_name = 'queue_message_errors') THEN
        INSERT INTO queue_messages_failed
            (id, queue_name, message_group_id, payload, error_message,
             receive_count, created_at, failed_at)
        SELECT id, queue_name, message_group_id, payload, error,
               receive_count, created_at, failed_at
          FROM queue_message_errors
        ON CONFLICT (queue_name, id) DO NOTHING;
        DROP TABLE queue_message_errors;
    END IF;
END $$;
`
	_, err := pool.Exec(ctx, ddl)
	return err
}

// Poll claims up to maxMessages eligible messages from this queue.
//
// Eligibility = visible_at <= now AND the message is the earliest ELIGIBLE
// message in its group (COALESCE(message_group_id, id)). Each claimed row
// gets a unique receipt handle (<pollUUID>:<id>) and its visibility window
// is pushed out by the configured timeout. We use a correlated NOT EXISTS
// rather than a windowed CTE because FOR UPDATE SKIP LOCKED cannot be
// applied over a ROW_NUMBER() result under Postgres CTE inlining; the
// resulting claim set is equivalent.
//
// Two NOT EXISTS clauses gate "earliest eligible in its group", not one:
//   - an earlier row that is CLAIMED (receipt_handle IS NOT NULL) does NOT
//     block — an in-flight head's cross-poll ordering is still not enforced
//     by the broker, same as always;
//   - an earlier row that was NACKED WITH A DELAY (R4, owner ruling
//     2026-09-17, docs/spec/router-deferral-handback.md: receipt_handle IS
//     NULL AND visible_at > now) DOES block. Before R4 this state was
//     treated the same as "claimed" — not visible, so not blocking — which
//     let a returned group head's own successors overtake it on the very
//     next poll, exactly the ordering violation an ordered group exists to
//     prevent. A delay-bearing MediationDeferred hand-back (R1) sits in
//     precisely this state, which is what made the gap load-bearing rather
//     than academic.
func (q *Queue) Poll(ctx context.Context, maxMessages uint32) ([]common.QueuedMessage, error) {
	if q.stopped.Load() {
		return nil, queue.ErrStopped
	}
	visibility := time.Duration(q.cfg.VisibilityTimeout) * time.Second
	if visibility <= 0 {
		visibility = 30 * time.Second
	}
	now := time.Now().Unix()
	newVisibleAt := now + int64(visibility.Seconds())
	receipt := uuid.NewString()

	const sql = `
WITH claimed AS (
  SELECT m.id
    FROM queue_messages m
   WHERE m.queue_name = $1
     AND m.visible_at <= $2
     AND NOT EXISTS (
           SELECT 1 FROM queue_messages e
            WHERE e.queue_name = m.queue_name
              AND COALESCE(e.message_group_id, e.id) = COALESCE(m.message_group_id, m.id)
              AND e.visible_at <= $2
              AND (e.created_at < m.created_at
                   OR (e.created_at = m.created_at AND e.id < m.id))
         )
     AND NOT EXISTS (
           -- R4 (owner ruling 2026-09-17, docs/spec/router-deferral-handback.md):
           -- an earlier row of the same group RETURNED with a delay (nacked —
           -- receipt_handle cleared — but not yet visible) blocks this one. A
           -- CLAIMED earlier row (receipt_handle IS NOT NULL) is deliberately
           -- excluded here — see the Poll doc comment above.
           SELECT 1 FROM queue_messages e
            WHERE e.queue_name = m.queue_name
              AND COALESCE(e.message_group_id, e.id) = COALESCE(m.message_group_id, m.id)
              AND e.receipt_handle IS NULL
              AND e.visible_at > $2
              AND (e.created_at < m.created_at
                   OR (e.created_at = m.created_at AND e.id < m.id))
         )
   ORDER BY m.created_at, m.id
   LIMIT $3
   FOR UPDATE SKIP LOCKED
)
UPDATE queue_messages t
   SET receipt_handle = $4 || ':' || t.id,
       visible_at     = $5,
       receive_count  = t.receive_count + 1
  FROM claimed
 WHERE t.queue_name = $1
   AND t.id = claimed.id
 RETURNING t.id, t.payload, t.message_group_id, t.created_at, t.receive_count
`
	rows, err := q.pool.Query(ctx, sql, q.cfg.Name, now, int64(maxMessages), receipt, newVisibleAt)
	if err != nil {
		return nil, fmt.Errorf("postgres queue poll: %w", err)
	}
	defer rows.Close()

	var msgs []common.QueuedMessage
	type poison struct {
		id, payload, reason string
		group               *string
		createdAt           int64
		receiveCount        *int32
	}
	var quarantine []poison

	for rows.Next() {
		var id string
		var payload string
		var group *string
		var createdAt int64
		var receiveCount *int32
		if err := rows.Scan(&id, &payload, &group, &createdAt, &receiveCount); err != nil {
			// A scan failure is an infrastructure fault, not a bad row — the
			// whole poll is suspect, so surface it rather than quarantining.
			return nil, err
		}
		var m common.Message
		if err := json.Unmarshal([]byte(payload), &m); err != nil {
			// Poison row. The claim above has already committed, so returning
			// here would leave it claimed, visible again after the timeout,
			// re-claimed, and failing identically forever — taking the rest of
			// this batch down with it every time. Quarantine it after the scan
			// loop (the rows cursor is still open) and carry on.
			quarantine = append(quarantine, poison{
				id: id, payload: payload, reason: err.Error(),
				group: group, createdAt: createdAt, receiveCount: receiveCount,
			})
			continue
		}
		msgs = append(msgs, common.QueuedMessage{
			Message:         m,
			ReceiptHandle:   receipt + ":" + id,
			BrokerMessageID: id,
			QueueIdentifier: q.cfg.Name,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close() // done with the cursor; the moves below need the connection

	for _, p := range quarantine {
		if err := q.quarantine(ctx, p.id, p.payload, p.reason, p.group, p.createdAt, p.receiveCount); err != nil {
			// Couldn't move it — log and leave it claimed. It will come back on
			// the next poll and we'll try again; the rest of this batch is
			// already returned, so one unmovable row no longer costs the queue.
			slog.Warn("postgres queue: could not quarantine malformed message",
				"queue", q.cfg.Name, "message_id", p.id, "err", err)
			continue
		}
		slog.Warn("postgres queue: malformed message moved to queue_messages_failed",
			"queue", q.cfg.Name, "message_id", p.id, "reason", p.reason)
	}

	q.polled.Add(uint64(len(msgs)))
	return msgs, nil
}

// maxQuarantineErrorLen bounds the stored failure text. A pathological payload
// can produce an arbitrarily long parse error, and it would be stored verbatim
// once per quarantined row.
const maxQuarantineErrorLen = 1000

// quarantine moves one unparseable row out of the queue and into
// queue_messages_failed in a single statement, so the row can never be both
// places or neither.
//
// A repeat quarantine keeps the LATEST failure. A row that fails, is requeued
// and fails again is almost always being worked on — someone changed the
// payload, the schema, or the consumer — so the most recent failure is the one
// that describes what is wrong now. Keeping the first pins the record to the
// original attempt and discards every later diagnosis, which is backwards for
// the case this table exists to serve.
func (q *Queue) quarantine(ctx context.Context, id, payload, reason string, group *string, createdAt int64, receiveCount *int32) error {
	if len(reason) > maxQuarantineErrorLen {
		reason = reason[:maxQuarantineErrorLen]
	}
	_, err := q.pool.Exec(ctx, `
WITH moved AS (
  DELETE FROM queue_messages
   WHERE queue_name = $1 AND id = $2
  RETURNING id, queue_name, message_group_id, payload, receive_count, created_at
)
INSERT INTO queue_messages_failed
    (id, queue_name, message_group_id, payload, error_message, receive_count, created_at, failed_at)
SELECT id, queue_name, message_group_id, payload, $3, receive_count, created_at, $4
  FROM moved
ON CONFLICT (queue_name, id) DO UPDATE SET
    payload       = EXCLUDED.payload,
    error_message = EXCLUDED.error_message,
    receive_count = EXCLUDED.receive_count,
    failed_at     = EXCLUDED.failed_at`,
		q.cfg.Name, id, reason, time.Now().Unix())
	return err
}

// Ack deletes the message permanently. brokerMessageID is unused: a Postgres
// receipt handle embeds the row id and the row is deleted outright, so there is
// no at-least-once redelivery to guard against.
func (q *Queue) Ack(ctx context.Context, receipt string, _ string) error {
	tag, err := q.pool.Exec(ctx,
		`DELETE FROM queue_messages WHERE receipt_handle = $1 AND queue_name = $2`,
		receipt, q.cfg.Name)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("ack: %w", errNotFound(receipt))
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

// HonoursDelayedReturn is true (R5, docs/spec/router-deferral-handback.md):
// Nack's visible_at update really does hold the row back for delay, and the
// claim query's second NOT EXISTS clause (see Poll's doc comment) blocks a
// nacked-with-delay group head's successors from claiming ahead of it.
func (q *Queue) HonoursDelayedReturn() bool { return true }

// Publish writes a single message. Uses ON CONFLICT DO NOTHING so a
// duplicate id is a no-op (at-least-once publish semantics).
func (q *Queue) Publish(ctx context.Context, m common.Message) (string, error) {
	if err := InsertBatch(ctx, q.pool, []Row{{QueueName: q.cfg.Name, Message: m}}); err != nil {
		return "", err
	}
	return m.ID, nil
}

// PublishBatch writes a batch of messages, all bound for this queue, in one
// statement.
func (q *Queue) PublishBatch(ctx context.Context, msgs []common.Message) ([]string, error) {
	rows := make([]Row, 0, len(msgs))
	ids := make([]string, 0, len(msgs))
	for _, m := range msgs {
		rows = append(rows, Row{QueueName: q.cfg.Name, Message: m})
		ids = append(ids, m.ID)
	}
	if err := InsertBatch(ctx, q.pool, rows); err != nil {
		return nil, err
	}
	return ids, nil
}

// Row is one message bound for one named queue — the unit InsertBatch writes,
// so a single statement can span several destination queues. The dispatch
// scheduler publishes a claimed batch that way: one row-queue per (tenant,
// priority), all in this one table.
type Row struct {
	QueueName string
	Message   common.Message
}

// InsertBatch writes one row per Row in a single statement, so a failure never
// partially applies. Each row may name a different queue.
//
// Exported so the dispatch scheduler's Postgres publisher writes through the
// same column mapping this backend's own publisher uses: two mappings for one
// table is how a producer and its consumer drift apart. ON CONFLICT DO NOTHING
// keeps at-least-once publish semantics — re-publishing a message still
// sitting on the queue is a no-op rather than a duplicate.
func InsertBatch(ctx context.Context, pool *pgxpool.Pool, rows []Row) error {
	if len(rows) == 0 {
		return nil
	}
	ids := make([]string, len(rows))
	names := make([]string, len(rows))
	groups := make([]*string, len(rows))
	payloads := make([]string, len(rows))
	for i, r := range rows {
		payload, err := json.Marshal(r.Message)
		if err != nil {
			return fmt.Errorf("marshal queue message %q: %w", r.Message.ID, err)
		}
		ids[i] = r.Message.ID
		names[i] = r.QueueName
		groups[i] = r.Message.MessageGroupID
		payloads[i] = string(payload)
	}
	// UNNEST zips the four arrays positionally; visible_at and created_at are
	// the same instant for every row in the batch.
	_, err := pool.Exec(ctx,
		`INSERT INTO queue_messages
		     (id, queue_name, message_group_id, visible_at, payload, created_at)
		 SELECT u.id, u.queue_name, u.message_group_id, $5::bigint, u.payload, $5::bigint
		   FROM UNNEST($1::text[], $2::text[], $3::text[], $4::text[])
		     AS u(id, queue_name, message_group_id, payload)
		 ON CONFLICT (queue_name, id) DO NOTHING`,
		ids, names, groups, payloads, time.Now().Unix())
	return err
}

// Healthy reports whether we can talk to Postgres.
func (q *Queue) Healthy() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return q.pool.Ping(ctx) == nil
}

// Stop closes the connection pool.
func (q *Queue) Stop() {
	q.stopped.Store(true)
	q.pool.Close()
}

// Metrics returns broker-side metrics. Reads counts from the table.
func (q *Queue) Metrics(ctx context.Context) (*queue.Metrics, error) {
	now := time.Now().Unix()
	var pending, inflight uint64
	err := q.pool.QueryRow(ctx,
		`SELECT
		   COUNT(*) FILTER (WHERE receipt_handle IS NULL AND visible_at <= $2),
		   COUNT(*) FILTER (WHERE receipt_handle IS NOT NULL)
		 FROM queue_messages WHERE queue_name = $1`,
		q.cfg.Name, now,
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
	delay := int64(0)
	if delaySeconds != nil {
		delay = int64(*delaySeconds)
	}
	newVisibleAt := time.Now().Unix() + delay
	_, err := q.pool.Exec(ctx,
		`UPDATE queue_messages
		    SET receipt_handle = NULL,
		        visible_at = $1
		  WHERE receipt_handle = $2 AND queue_name = $3`,
		newVisibleAt, receipt, q.cfg.Name)
	return err
}

func errNotFound(receipt string) error {
	return fmt.Errorf("receipt handle not found: %s", receipt)
}
