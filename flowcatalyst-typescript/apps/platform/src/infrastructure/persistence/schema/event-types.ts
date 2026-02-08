/**
 * Event Types Database Schema
 *
 * Tables for storing event type definitions and their spec versions (schemas).
 * Event type codes follow the format: {application}:{subdomain}:{aggregate}:{event}
 */

import { pgTable, varchar, text, boolean, index, unique, jsonb } from 'drizzle-orm/pg-core';
import { baseEntityColumns, tsidColumn } from '@flowcatalyst/persistence';

/**
 * Event types table - stores registered event type definitions.
 */
export const eventTypes = pgTable(
	'event_types',
	{
		...baseEntityColumns,
		/** Unique event type code: {app}:{subdomain}:{aggregate}:{event} */
		code: varchar('code', { length: 255 }).notNull().unique(),
		/** Display name */
		name: varchar('name', { length: 255 }).notNull(),
		/** Optional description */
		description: text('description'),
		/** CURRENT or ARCHIVED */
		status: varchar('status', { length: 20 }).notNull().default('CURRENT'),
		/** CODE, API, or UI */
		source: varchar('source', { length: 20 }).notNull().default('UI'),
		/** Whether events of this type are client-scoped */
		clientScoped: boolean('client_scoped').notNull().default(false),
		/** Application segment from code */
		application: varchar('application', { length: 100 }).notNull(),
		/** Subdomain segment from code */
		subdomain: varchar('subdomain', { length: 100 }).notNull(),
		/** Aggregate segment from code */
		aggregate: varchar('aggregate', { length: 100 }).notNull(),
	},
	(table) => [
		index('idx_event_types_code').on(table.code),
		index('idx_event_types_status').on(table.status),
		index('idx_event_types_source').on(table.source),
		index('idx_event_types_application').on(table.application),
		index('idx_event_types_subdomain').on(table.subdomain),
		index('idx_event_types_aggregate').on(table.aggregate),
	],
);

/**
 * Event type spec versions table - stores schema versions for event types.
 * Schema content is stored as native jsonb for direct query and API use.
 */
export const eventTypeSpecVersions = pgTable(
	'event_type_spec_versions',
	{
		...baseEntityColumns,
		/** Parent event type ID */
		eventTypeId: tsidColumn('event_type_id').notNull(),
		/** Version string: "MAJOR.MINOR" (e.g., "1.0", "2.1") */
		version: varchar('version', { length: 20 }).notNull(),
		/** MIME type (e.g., "application/json") */
		mimeType: varchar('mime_type', { length: 100 }).notNull(),
		/** Schema content as native jsonb (JSON Schema, Proto def, etc.) */
		schemaContent: jsonb('schema_content'),
		/** JSON_SCHEMA, PROTO, or XSD */
		schemaType: varchar('schema_type', { length: 20 }).notNull(),
		/** FINALISING, CURRENT, or DEPRECATED */
		status: varchar('status', { length: 20 }).notNull().default('FINALISING'),
	},
	(table) => [
		index('idx_spec_versions_event_type').on(table.eventTypeId),
		index('idx_spec_versions_status').on(table.status),
		unique('uq_spec_versions_event_type_version').on(table.eventTypeId, table.version),
	],
);

// Type inference
export type EventTypeRecord = typeof eventTypes.$inferSelect;
export type NewEventTypeRecord = typeof eventTypes.$inferInsert;
export type EventTypeSpecVersionRecord = typeof eventTypeSpecVersions.$inferSelect;
export type NewEventTypeSpecVersionRecord = typeof eventTypeSpecVersions.$inferInsert;
