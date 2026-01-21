/**
 * Outbox Database Schema
 *
 * Tables for CQRS read model projection using the outbox pattern.
 * Items are written to outbox tables on changes and processed asynchronously.
 */

import { pgTable, bigserial, varchar, jsonb, smallint, text, index } from 'drizzle-orm/pg-core';
import { rawTsidColumn, timestampColumn } from '@flowcatalyst/persistence';

/**
 * Outbox status values (following postbox-processor pattern).
 */
export const OutboxStatus = {
	PENDING: 0,
	SUCCESS: 1,
	BAD_REQUEST: 2,
	SERVER_ERROR: 3,
	IN_PROGRESS: 9,
} as const;

export type OutboxStatusValue = (typeof OutboxStatus)[keyof typeof OutboxStatus];

/**
 * Dispatch job outbox table.
 *
 * Payload-based outbox: captures state at write time.
 * - INSERT: full job payload
 * - UPDATE: patch with changed fields only
 * - DELETE: just the operation marker
 */
export const dispatchJobOutbox = pgTable(
	'dispatch_job_outbox',
	{
		id: bigserial('id', { mode: 'number' }).primaryKey(),
		dispatchJobId: rawTsidColumn('dispatch_job_id').notNull(),
		operation: varchar('operation', { length: 10 }).notNull(), // INSERT, UPDATE, DELETE
		payload: jsonb('payload').notNull(),
		createdAt: timestampColumn('created_at').notNull().defaultNow(),
		processed: smallint('processed').notNull().default(0),
		processedAt: timestampColumn('processed_at'),
		errorMessage: text('error_message'),
	},
	(table) => [
		// Index for polling unprocessed entries, ordered by job (message group) then sequence
		index('idx_dispatch_job_outbox_unprocessed')
			.on(table.dispatchJobId, table.id)
			.where({ processed: 0 } as never), // Partial index
		// Index for crash recovery (find in-progress entries)
		index('idx_dispatch_job_outbox_in_progress')
			.on(table.id)
			.where({ processed: 9 } as never), // Partial index
		// Index for cleanup of old processed entries
		index('idx_dispatch_job_outbox_processed_at')
			.on(table.processedAt)
			.where({ processed: 1 } as never), // Partial index
	],
);

export type DispatchJobOutboxRecord = typeof dispatchJobOutbox.$inferSelect;
export type NewDispatchJobOutboxRecord = typeof dispatchJobOutbox.$inferInsert;

/**
 * Event outbox table.
 *
 * Events are simpler (immutable), but using outbox for:
 * - Consistent processing pattern
 * - Crash recovery
 * - Error tracking
 */
export const eventOutbox = pgTable(
	'event_outbox',
	{
		id: bigserial('id', { mode: 'number' }).primaryKey(),
		eventId: rawTsidColumn('event_id').notNull(),
		payload: jsonb('payload').notNull(),
		createdAt: timestampColumn('created_at').notNull().defaultNow(),
		processed: smallint('processed').notNull().default(0),
		processedAt: timestampColumn('processed_at'),
		errorMessage: text('error_message'),
	},
	(table) => [
		// Index for polling unprocessed entries
		index('idx_event_outbox_unprocessed')
			.on(table.id)
			.where({ processed: 0 } as never), // Partial index
		// Index for crash recovery
		index('idx_event_outbox_in_progress')
			.on(table.id)
			.where({ processed: 9 } as never), // Partial index
	],
);

export type EventOutboxRecord = typeof eventOutbox.$inferSelect;
export type NewEventOutboxRecord = typeof eventOutbox.$inferInsert;
