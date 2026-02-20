/**
 * Events Read Schema
 *
 * Read-optimized projection of events for efficient querying.
 * Populated by the stream processor from event_projection_feed.
 *
 * Key differences from events table:
 * - Parsed application/subdomain/aggregate fields for filtering
 * - Data stored as TEXT (not JSONB) since we don't query inside it
 * - projected_at timestamp for tracking projection lag
 */

import { pgTable, varchar, text, index } from "drizzle-orm/pg-core";
import { tsidColumn, rawTsidColumn, timestampColumn } from "./common.js";

/**
 * Events read table schema.
 */
export const eventsRead = pgTable(
	"msg_events_read",
	{
		// Primary key (id IS the event id - 1:1 projection, unprefixed for performance)
		id: rawTsidColumn("id").primaryKey(),

		// CloudEvents fields
		specVersion: varchar("spec_version", { length: 20 }),
		type: varchar("type", { length: 200 }).notNull(),
		source: varchar("source", { length: 500 }).notNull(),
		subject: varchar("subject", { length: 500 }),
		time: timestampColumn("time").notNull(),

		// Event data as text (no JSONB querying needed)
		data: text("data"),

		// Tracing
		correlationId: varchar("correlation_id", { length: 100 }),
		causationId: varchar("causation_id", { length: 100 }),
		deduplicationId: varchar("deduplication_id", { length: 200 }),
		messageGroup: varchar("message_group", { length: 200 }),

		// Multi-tenant context
		clientId: tsidColumn("client_id"),

		// Parsed fields for efficient filtering (extracted from type/subject)
		application: varchar("application", { length: 100 }),
		subdomain: varchar("subdomain", { length: 100 }),
		aggregate: varchar("aggregate", { length: 100 }),

		// Projection tracking
		projectedAt: timestampColumn("projected_at").notNull().defaultNow(),
	},
	(table) => [
		index("idx_msg_events_read_type").on(table.type),
		index("idx_msg_events_read_client_id").on(table.clientId),
		index("idx_msg_events_read_time").on(table.time),
		index("idx_msg_events_read_application").on(table.application),
		index("idx_msg_events_read_subdomain").on(table.subdomain),
		index("idx_msg_events_read_aggregate").on(table.aggregate),
		index("idx_msg_events_read_correlation_id").on(table.correlationId),
	],
);

/**
 * Event read entity type (select result).
 */
export type EventReadRecord = typeof eventsRead.$inferSelect;

/**
 * New event read type (insert input).
 */
export type NewEventReadRecord = typeof eventsRead.$inferInsert;
