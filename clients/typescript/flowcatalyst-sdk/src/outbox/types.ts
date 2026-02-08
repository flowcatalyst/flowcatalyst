/**
 * Outbox types for transactional outbox pattern.
 *
 * Schema must match the Java outbox-processor's expected format.
 * The processor reads from these tables and manages status transitions.
 */

/** Message types supported by the outbox. */
export type MessageType = 'EVENT' | 'DISPATCH_JOB' | 'AUDIT_LOG';

/**
 * Outbox status codes matching the Java outbox-processor.
 *
 * The processor uses SMALLINT status codes, NOT strings.
 * Only PENDING (0) is written by the SDK; all others are set by the processor.
 */
export const OutboxStatus = {
  /** Waiting to be processed. */
  PENDING: 0,
  /** Successfully sent to FlowCatalyst. */
  SUCCESS: 1,
  /** API returned 400 Bad Request (permanent failure). */
  BAD_REQUEST: 2,
  /** API returned 500 Internal Server Error (retryable). */
  INTERNAL_ERROR: 3,
  /** API returned 401 Unauthorized (retryable). */
  UNAUTHORIZED: 4,
  /** API returned 403 Forbidden (permanent failure). */
  FORBIDDEN: 5,
  /** API returned 502/503/504 Gateway Error (retryable). */
  GATEWAY_ERROR: 6,
  /** Currently being processed - crash recovery marker. */
  IN_PROGRESS: 9,
} as const;

export type OutboxStatusCode = (typeof OutboxStatus)[keyof typeof OutboxStatus];

/** An outbox message record to be persisted by the driver. */
export interface OutboxMessage {
  id: string;
  type: MessageType;
  message_group: string | null;
  payload: string;
  status: number;
  created_at: string;
  updated_at: string;
  /** SDK-specific: client identifier for multi-tenant routing. */
  client_id: string;
  /** SDK-specific: byte size of payload. */
  payload_size: number;
  /** SDK-specific: optional headers. */
  headers: Record<string, string> | null;
}

/**
 * Driver interface for outbox persistence.
 *
 * Users implement this with their own DB client to participate
 * in the same transaction as their business logic.
 *
 * @example
 * ```typescript
 * import { OutboxDriver, OutboxMessage } from '@flowcatalyst/sdk';
 * import { Pool } from 'pg';
 *
 * class PostgresOutboxDriver implements OutboxDriver {
 *   constructor(private pool: Pool) {}
 *
 *   async insert(message: OutboxMessage): Promise<void> {
 *     await this.pool.query(
 *       `INSERT INTO outbox_messages
 *        (id, type, message_group, payload, status, created_at, updated_at,
 *         client_id, payload_size, headers)
 *        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
 *       [
 *         message.id, message.type, message.message_group, message.payload,
 *         message.status, message.created_at, message.updated_at,
 *         message.client_id, message.payload_size,
 *         message.headers ? JSON.stringify(message.headers) : null,
 *       ]
 *     );
 *   }
 *
 *   async insertBatch(messages: OutboxMessage[]): Promise<void> {
 *     const client = await this.pool.connect();
 *     try {
 *       await client.query('BEGIN');
 *       for (const message of messages) {
 *         await this.insert(message);
 *       }
 *       await client.query('COMMIT');
 *     } catch (e) {
 *       await client.query('ROLLBACK');
 *       throw e;
 *     } finally {
 *       client.release();
 *     }
 *   }
 * }
 * ```
 */
export interface OutboxDriver {
  /** Insert a single message into the outbox. */
  insert(message: OutboxMessage): Promise<void>;

  /** Insert multiple messages into the outbox (batch). */
  insertBatch(messages: OutboxMessage[]): Promise<void>;
}
