/**
 * Projection Feed Tables Schema
 *
 * Feed tables for CQRS read model projection.
 * These tables capture changes at write time for later projection to read models.
 *
 * Processed values (following postbox-processor pattern):
 *   0 = pending
 *   1 = success
 *   2 = bad request (permanent failure)
 *   3 = server error (retriable)
 *   9 = in-progress (crash recovery marker)
 */

import { pgTable, bigserial, varchar, smallint, text, jsonb, index } from 'drizzle-orm/pg-core';
import { rawTsidColumn, timestampColumn } from './common.js';

/**
 * Projection feed operation type.
 */
export type ProjectionFeedOperation = 'INSERT' | 'UPDATE' | 'DELETE';

/**
 * Projection feed processed status.
 */
export type ProjectionFeedProcessedStatus = 0 | 1 | 2 | 3 | 9;

/**
 * Event projection feed table schema.
 *
 * Used for projecting events to events_read table.
 * Events are immutable, so this is simpler than dispatch_job_projection_feed.
 */
export const eventProjectionFeed = pgTable(
  'msg_event_projection_feed',
  {
    id: bigserial('id', { mode: 'number' }).primaryKey(),
    eventId: rawTsidColumn('event_id').notNull(),
    payload: jsonb('payload').notNull(),
    createdAt: timestampColumn('created_at').notNull().defaultNow(),
    processed: smallint('processed').notNull().default(0).$type<ProjectionFeedProcessedStatus>(),
    processedAt: timestampColumn('processed_at'),
    errorMessage: text('error_message'),
  },
  (table) => [
    // Index for polling unprocessed entries
    index('idx_msg_event_projection_feed_unprocessed')
      .on(table.id)
      .where({ processed: 0 } as any),
    // Index for crash recovery
    index('idx_msg_event_projection_feed_in_progress')
      .on(table.id)
      .where({ processed: 9 } as any),
  ],
);

/**
 * Dispatch job projection feed table schema.
 *
 * Used for projecting dispatch_jobs to dispatch_jobs_read table.
 * Captures state at write time - INSERT has full payload, UPDATE has patch.
 * dispatch_job_id serves as message group for sequencing.
 */
export const dispatchJobProjectionFeed = pgTable(
  'msg_dispatch_job_projection_feed',
  {
    id: bigserial('id', { mode: 'number' }).primaryKey(),
    dispatchJobId: rawTsidColumn('dispatch_job_id').notNull(),
    operation: varchar('operation', { length: 10 }).notNull().$type<ProjectionFeedOperation>(),
    payload: jsonb('payload').notNull(), // Full job on INSERT, patch on UPDATE
    createdAt: timestampColumn('created_at').notNull().defaultNow(),
    processed: smallint('processed').notNull().default(0).$type<ProjectionFeedProcessedStatus>(),
    processedAt: timestampColumn('processed_at'),
    errorMessage: text('error_message'),
  },
  (table) => [
    // Index for polling unprocessed entries, ordered by job (message group) then sequence
    index('idx_msg_dj_projection_feed_unprocessed')
      .on(table.dispatchJobId, table.id)
      .where({ processed: 0 } as any),
    // Index for crash recovery
    index('idx_msg_dj_projection_feed_in_progress')
      .on(table.id)
      .where({ processed: 9 } as any),
    // Index for cleanup of old processed entries
    index('idx_msg_dj_projection_feed_processed_at')
      .on(table.processedAt)
      .where({ processed: 1 } as any),
  ],
);

/**
 * Event projection feed entity type (select result).
 */
export type EventProjectionFeedRecord = typeof eventProjectionFeed.$inferSelect;

/**
 * New event projection feed type (insert input).
 */
export type NewEventProjectionFeedRecord = typeof eventProjectionFeed.$inferInsert;

/**
 * Dispatch job projection feed entity type (select result).
 */
export type DispatchJobProjectionFeedRecord = typeof dispatchJobProjectionFeed.$inferSelect;

/**
 * New dispatch job projection feed type (insert input).
 */
export type NewDispatchJobProjectionFeedRecord = typeof dispatchJobProjectionFeed.$inferInsert;
