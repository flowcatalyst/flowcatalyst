/**
 * Principal Application Access Database Schema
 *
 * Junction table linking principals to the applications they can access.
 * Matches Java's principal_application_access table (V30 migration).
 */

import { pgTable, varchar, primaryKey, index } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn } from '@flowcatalyst/persistence';

/**
 * Principal application access junction table.
 * Tracks which applications a user principal can access.
 */
export const principalApplicationAccess = pgTable(
	'principal_application_access',
	{
		principalId: tsidColumn('principal_id').notNull(),
		applicationId: tsidColumn('application_id').notNull(),
		grantedAt: timestampColumn('granted_at').notNull().defaultNow(),
	},
	(table) => [
		primaryKey({ columns: [table.principalId, table.applicationId] }),
		index('idx_principal_app_access_app_id').on(table.applicationId),
	],
);

// Type inference
export type PrincipalApplicationAccessRecord = typeof principalApplicationAccess.$inferSelect;
export type NewPrincipalApplicationAccessRecord = typeof principalApplicationAccess.$inferInsert;
