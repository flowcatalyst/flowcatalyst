/**
 * Client Access Grants Database Schema
 *
 * Table for storing client access grants for users.
 */

import { pgTable, index, unique, timestamp } from 'drizzle-orm/pg-core';
import { baseEntityColumns, tsidColumn } from '@flowcatalyst/persistence';

/**
 * Client access grants table - stores grants of client access to users.
 */
export const clientAccessGrants = pgTable(
	'client_access_grants',
	{
		...baseEntityColumns,
		principalId: tsidColumn('principal_id').notNull(),
		clientId: tsidColumn('client_id').notNull(),
		grantedBy: tsidColumn('granted_by').notNull(),
		grantedAt: timestamp('granted_at').notNull().defaultNow(),
	},
	(table) => [
		index('idx_client_access_grants_principal').on(table.principalId),
		index('idx_client_access_grants_client').on(table.clientId),
		unique('uq_client_access_grants_principal_client').on(table.principalId, table.clientId),
	],
);

// Type inference
export type ClientAccessGrantRecord = typeof clientAccessGrants.$inferSelect;
export type NewClientAccessGrantRecord = typeof clientAccessGrants.$inferInsert;
