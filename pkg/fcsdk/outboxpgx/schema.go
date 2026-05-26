// Package outboxpgx implements the consumer-facing Sink for use cases
// running against pgx. It writes domain events and audit logs to the
// outbox_messages table; the fc-outbox-processor polls that table and
// forwards items to the FlowCatalyst platform API.
//
// The Sink satisfies usecasepgx.Sink, so any use case that uses
// usecasepgx.Commit / CommitDelete / CommitAll / EmitEvent will emit
// events through the outbox transparently.
package outboxpgx

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CreateOutboxTableSQL is the schema for the outbox_messages table.
// Matches the columns expected by the fc-outbox-processor (also used by
// the TypeScript and Laravel SDKs). Safe to run multiple times.
const CreateOutboxTableSQL = `
CREATE TABLE IF NOT EXISTS outbox_messages (
    id VARCHAR(26) PRIMARY KEY,
    type VARCHAR(20) NOT NULL,
    message_group VARCHAR(255),
    payload TEXT NOT NULL,
    status SMALLINT NOT NULL DEFAULT 0,
    retry_count SMALLINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    error_message TEXT,
    client_id VARCHAR(26),
    payload_size INTEGER,
    headers JSONB
);

CREATE INDEX IF NOT EXISTS idx_outbox_messages_pending
    ON outbox_messages(status, message_group, created_at)
    WHERE status = 0;

CREATE INDEX IF NOT EXISTS idx_outbox_messages_stuck
    ON outbox_messages(status, created_at)
    WHERE status = 9;

CREATE INDEX IF NOT EXISTS idx_outbox_client_pending
    ON outbox_messages(client_id, status, created_at);
`

// InitSchema creates the outbox_messages table and its indexes on the
// given pool. Idempotent — safe to call on every boot. In production,
// run your real migration system instead and skip this helper.
func InitSchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, CreateOutboxTableSQL)
	return err
}
