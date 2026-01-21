/**
 * Outbox Tables Schema
 *
 * Outbox pattern for CQRS read model projection.
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
 * Outbox operation type.
 */
export type OutboxOperation = 'INSERT' | 'UPDATE' | 'DELETE';

/**
 * Outbox processed status.
 */
export type OutboxProcessedStatus = 0 | 1 | 2 | 3 | 9;

/**
 * Event outbox table schema.
 *
 * Used for projecting events to events_read table.
 * Events are immutable, so this is simpler than dispatch_job_outbox.
 */
export const eventOutbox = pgTable(
	'event_outbox',
	{
		id: bigserial('id', { mode: 'number' }).primaryKey(),
		eventId: rawTsidColumn('event_id').notNull(),
		payload: jsonb('payload').notNull(),
		createdAt: timestampColumn('created_at').notNull().defaultNow(),
		processed: smallint('processed').notNull().default(0).$type<OutboxProcessedStatus>(),
		processedAt: timestampColumn('processed_at'),
		errorMessage: text('error_message'),
	},
	(table) => [
		// Index for polling unprocessed entries
		index('idx_event_outbox_unprocessed').on(table.id).where({ processed: 0 } as any),
		// Index for crash recovery
		index('idx_event_outbox_in_progress').on(table.id).where({ processed: 9 } as any),
	],
);

/**
 * Dispatch job outbox table schema.
 *
 * Used for projecting dispatch_jobs to dispatch_jobs_read table.
 * Captures state at write time - INSERT has full payload, UPDATE has patch.
 * dispatch_job_id serves as message group for sequencing.
 */
export const dispatchJobOutbox = pgTable(
	'dispatch_job_outbox',
	{
		id: bigserial('id', { mode: 'number' }).primaryKey(),
		dispatchJobId: rawTsidColumn('dispatch_job_id').notNull(),
		operation: varchar('operation', { length: 10 }).notNull().$type<OutboxOperation>(),
		payload: jsonb('payload').notNull(), // Full job on INSERT, patch on UPDATE
		createdAt: timestampColumn('created_at').notNull().defaultNow(),
		processed: smallint('processed').notNull().default(0).$type<OutboxProcessedStatus>(),
		processedAt: timestampColumn('processed_at'),
		errorMessage: text('error_message'),
	},
	(table) => [
		// Index for polling unprocessed entries, ordered by job (message group) then sequence
		index('idx_dispatch_job_outbox_unprocessed').on(table.dispatchJobId, table.id).where({ processed: 0 } as any),
		// Index for crash recovery
		index('idx_dispatch_job_outbox_in_progress').on(table.id).where({ processed: 9 } as any),
		// Index for cleanup of old processed entries
		index('idx_dispatch_job_outbox_processed_at').on(table.processedAt).where({ processed: 1 } as any),
	],
);

/**
 * Event outbox entity type (select result).
 */
export type EventOutboxRecord = typeof eventOutbox.$inferSelect;

/**
 * New event outbox type (insert input).
 */
export type NewEventOutboxRecord = typeof eventOutbox.$inferInsert;

/**
 * Dispatch job outbox entity type (select result).
 */
export type DispatchJobOutboxRecord = typeof dispatchJobOutbox.$inferSelect;

/**
 * New dispatch job outbox type (insert input).
 */
export type NewDispatchJobOutboxRecord = typeof dispatchJobOutbox.$inferInsert;
