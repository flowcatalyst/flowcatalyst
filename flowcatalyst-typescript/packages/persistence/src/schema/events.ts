/**
 * Events Schema
 *
 * Database schema for domain events (CloudEvents-based).
 * Events are immutable records of state changes in the system.
 */

import { pgTable, varchar, timestamp, jsonb, index, uniqueIndex } from 'drizzle-orm/pg-core';
import { tsidColumn, rawTsidColumn, timestampColumn } from './common.js';

/**
 * Context data stored with events for filtering and search.
 */
export interface EventContextData {
	readonly key: string;
	readonly value: string;
}

/**
 * Events table schema (CloudEvents-based).
 */
export const events = pgTable(
	'events',
	{
		// Primary key (unprefixed TSID for high-volume performance)
		id: rawTsidColumn('id').primaryKey(),

		// CloudEvents required fields
		specVersion: varchar('spec_version', { length: 20 }).notNull().default('1.0'),
		type: varchar('type', { length: 200 }).notNull(),
		source: varchar('source', { length: 500 }).notNull(),
		subject: varchar('subject', { length: 500 }),
		time: timestampColumn('time').notNull(),

		// Event data as JSONB
		data: jsonb('data'),

		// Tracing fields
		correlationId: varchar('correlation_id', { length: 100 }),
		causationId: varchar('causation_id', { length: 100 }),

		// Idempotency and ordering
		deduplicationId: varchar('deduplication_id', { length: 200 }),
		messageGroup: varchar('message_group', { length: 200 }),

		// Multi-tenant scoping
		clientId: tsidColumn('client_id'),

		// Additional context for filtering
		contextData: jsonb('context_data').$type<EventContextData[]>(),

		// Metadata
		createdAt: timestampColumn('created_at').notNull().defaultNow(),
	},
	(table) => [
		// Index for event type queries
		index('idx_events_type').on(table.type),
		// Index for client-scoped queries
		index('idx_events_client_type').on(table.clientId, table.type),
		// Index for chronological queries
		index('idx_events_time').on(table.time),
		// Index for correlation tracing
		index('idx_events_correlation').on(table.correlationId),
		// Unique index for idempotency
		uniqueIndex('idx_events_deduplication').on(table.deduplicationId),
	],
);

/**
 * Event entity type (select result).
 */
export type Event = typeof events.$inferSelect;

/**
 * New event type (insert input).
 */
export type NewEvent = typeof events.$inferInsert;
