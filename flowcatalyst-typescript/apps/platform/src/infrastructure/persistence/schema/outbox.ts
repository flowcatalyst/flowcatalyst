/**
 * Projection Feed Database Schema
 *
 * Tables for CQRS read model projection using the feed pattern.
 * Items are written to feed tables on changes and processed asynchronously.
 */

import { pgTable, bigserial, varchar, jsonb, smallint, text, index } from 'drizzle-orm/pg-core';
import { rawTsidColumn, timestampColumn } from '@flowcatalyst/persistence';

/**
 * Projection feed status values (following postbox-processor pattern).
 */
export const ProjectionFeedStatus = {
	PENDING: 0,
	SUCCESS: 1,
	BAD_REQUEST: 2,
	SERVER_ERROR: 3,
	IN_PROGRESS: 9,
} as const;

export type ProjectionFeedStatusValue = (typeof ProjectionFeedStatus)[keyof typeof ProjectionFeedStatus];

/**
 * Dispatch job projection feed table.
 *
 * Payload-based feed: captures state at write time.
 * - INSERT: full job payload
 * - UPDATE: patch with changed fields only
 * - DELETE: just the operation marker
 */
export const dispatchJobProjectionFeed = pgTable(
	'dispatch_job_projection_feed',
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
		index('idx_dj_projection_feed_unprocessed')
			.on(table.dispatchJobId, table.id)
			.where({ processed: 0 } as never), // Partial index
		// Index for crash recovery (find in-progress entries)
		index('idx_dj_projection_feed_in_progress')
			.on(table.id)
			.where({ processed: 9 } as never), // Partial index
		// Index for cleanup of old processed entries
		index('idx_dj_projection_feed_processed_at')
			.on(table.processedAt)
			.where({ processed: 1 } as never), // Partial index
	],
);

export type DispatchJobProjectionFeedRecord = typeof dispatchJobProjectionFeed.$inferSelect;
export type NewDispatchJobProjectionFeedRecord = typeof dispatchJobProjectionFeed.$inferInsert;

/**
 * Event projection feed table.
 *
 * Events are simpler (immutable), but using feed for:
 * - Consistent processing pattern
 * - Crash recovery
 * - Error tracking
 */
export const eventProjectionFeed = pgTable(
	'event_projection_feed',
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
		index('idx_event_projection_feed_unprocessed')
			.on(table.id)
			.where({ processed: 0 } as never), // Partial index
		// Index for crash recovery
		index('idx_event_projection_feed_in_progress')
			.on(table.id)
			.where({ processed: 9 } as never), // Partial index
	],
);

export type EventProjectionFeedRecord = typeof eventProjectionFeed.$inferSelect;
export type NewEventProjectionFeedRecord = typeof eventProjectionFeed.$inferInsert;
