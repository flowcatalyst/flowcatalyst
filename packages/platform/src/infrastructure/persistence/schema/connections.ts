/**
 * Connection Schema
 *
 * Table definitions for connection management.
 * Connections group endpoint + credentials between ServiceAccount and Subscription.
 */

import {
	pgTable,
	varchar,
	index,
	uniqueIndex,
} from "drizzle-orm/pg-core";
import { baseEntityColumns, tsidColumn } from "@flowcatalyst/persistence";

/**
 * Connections - named endpoint + credential groupings.
 */
export const connections = pgTable(
	"msg_connections",
	{
		...baseEntityColumns,
		code: varchar("code", { length: 100 }).notNull(),
		name: varchar("name", { length: 255 }).notNull(),
		description: varchar("description", { length: 500 }),
		endpoint: varchar("endpoint", { length: 500 }).notNull(),
		externalId: varchar("external_id", { length: 100 }),
		status: varchar("status", { length: 20 }).notNull().default("ACTIVE"),
		serviceAccountId: tsidColumn("service_account_id").notNull(),
		clientId: tsidColumn("client_id"),
		clientIdentifier: varchar("client_identifier", { length: 100 }),
	},
	(table) => ({
		codeClientIdx: uniqueIndex("idx_msg_connections_code_client").on(
			table.code,
			table.clientId,
		),
		statusIdx: index("idx_msg_connections_status").on(table.status),
		clientIdIdx: index("idx_msg_connections_client_id").on(table.clientId),
		serviceAccountIdx: index("idx_msg_connections_service_account").on(
			table.serviceAccountId,
		),
	}),
);

export type ConnectionRecord = typeof connections.$inferSelect;
export type NewConnectionRecord = typeof connections.$inferInsert;
