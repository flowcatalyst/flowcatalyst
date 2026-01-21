import type { Database } from 'sql.js';

/**
 * SQL schema for the embedded queue
 * Matches Java implementation in EmbeddedQueueSchema.java
 */

const CREATE_QUEUE_TABLE = `
CREATE TABLE IF NOT EXISTS queue_messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    message_id TEXT UNIQUE NOT NULL,
    message_group_id TEXT NOT NULL,
    message_deduplication_id TEXT,
    message_json TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    visible_at INTEGER NOT NULL,
    receipt_handle TEXT UNIQUE NOT NULL,
    receive_count INTEGER DEFAULT 0,
    first_received_at INTEGER
)
`;

const CREATE_DEDUPLICATION_TABLE = `
CREATE TABLE IF NOT EXISTS message_deduplication (
    message_deduplication_id TEXT PRIMARY KEY,
    message_id TEXT NOT NULL,
    created_at INTEGER NOT NULL
)
`;

const CREATE_GROUP_VISIBILITY_INDEX = `
CREATE INDEX IF NOT EXISTS idx_group_visibility
ON queue_messages (message_group_id, visible_at, id)
`;

const CREATE_VISIBILITY_ID_INDEX = `
CREATE INDEX IF NOT EXISTS idx_visibility_id
ON queue_messages (visible_at, id)
`;

const CREATE_DEDUP_CREATED_INDEX = `
CREATE INDEX IF NOT EXISTS idx_dedup_created
ON message_deduplication (created_at)
`;

/**
 * Initialize the embedded queue schema
 */
export function initializeSchema(db: Database): void {
	db.run(CREATE_QUEUE_TABLE);
	db.run(CREATE_DEDUPLICATION_TABLE);
	db.run(CREATE_GROUP_VISIBILITY_INDEX);
	db.run(CREATE_VISIBILITY_ID_INDEX);
	db.run(CREATE_DEDUP_CREATED_INDEX);
}

/**
 * Queue message row from database
 */
export interface QueueMessageRow {
	id: number;
	message_id: string;
	message_group_id: string;
	message_deduplication_id: string | null;
	message_json: string;
	created_at: number;
	visible_at: number;
	receipt_handle: string;
	receive_count: number;
	first_received_at: number | null;
}

/**
 * Deduplication row from database
 */
export interface DeduplicationRow {
	message_deduplication_id: string;
	message_id: string;
	created_at: number;
}
