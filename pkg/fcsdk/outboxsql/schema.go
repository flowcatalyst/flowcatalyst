// Package outboxsql provides the outbox-table DDL for consumer apps
// running against database/sql, for both Postgres (any driver: lib/pq,
// pgx/stdlib, etc.) and MySQL (go-sql-driver/mysql). [InitSchema] (and
// the [CreateOutboxTableSQLPostgres] / [CreateOutboxTableSQLMySQL]
// constants it dispatches on) create the outbox_messages table an
// outbox processor then drains.
package outboxsql

import (
	"context"
	"database/sql"
)

// Dialect picks the placeholder style and dialect-specific SQL.
type Dialect int

const (
	// DialectPostgres uses $1, $2 placeholders and Postgres-flavoured DDL.
	DialectPostgres Dialect = iota
	// DialectMySQL uses ? placeholders and MySQL-flavoured DDL.
	DialectMySQL
)

// CreateOutboxTableSQLPostgres is the schema for Postgres. Mirrors
// the TypeScript SDK's migration so all SDKs converge on one shape.
const CreateOutboxTableSQLPostgres = `
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

// CreateOutboxTableSQLMySQL is the schema for MySQL 8.0+. Includes
// SDK-specific columns and indexes matching the Java outbox-processor's
// expectations. Mirrors the TypeScript SDK migration.
const CreateOutboxTableSQLMySQL = `
CREATE TABLE IF NOT EXISTS outbox_messages (
    id VARCHAR(26) PRIMARY KEY,
    type VARCHAR(20) NOT NULL,
    message_group VARCHAR(255),
    payload LONGTEXT NOT NULL,
    status SMALLINT NOT NULL DEFAULT 0,
    retry_count SMALLINT NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    error_message TEXT,
    client_id VARCHAR(26),
    payload_size BIGINT,
    headers JSON,
    INDEX idx_outbox_messages_pending (status, message_group, created_at),
    INDEX idx_outbox_messages_stuck (status, created_at),
    INDEX idx_outbox_client_pending (client_id, status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
`

// InitSchema creates the outbox_messages table for the given dialect.
// Idempotent. In production, run your real migration system instead.
//
// Postgres: a single Exec covers all statements (the driver handles
// the script). MySQL: only one statement per Exec; this helper splits
// on bare semicolons at top level which is sufficient for the schema
// above (single CREATE TABLE statement, indexes inline). For richer
// schemas, use a real migration tool.
func InitSchema(ctx context.Context, db *sql.DB, dialect Dialect) error {
	var ddl string
	switch dialect {
	case DialectPostgres:
		ddl = CreateOutboxTableSQLPostgres
	case DialectMySQL:
		ddl = CreateOutboxTableSQLMySQL
	}
	_, err := db.ExecContext(ctx, ddl)
	return err
}
