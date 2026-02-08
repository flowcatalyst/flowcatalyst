/**
 * Service Accounts Database Schema
 *
 * Normalized table for service account data, matching Java's ServiceAccountJpaEntity.
 * Has a 1:1 relationship with the principals table (same ID).
 */

import { pgTable, varchar, boolean, index, uniqueIndex } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn } from '@flowcatalyst/persistence';

/**
 * Service accounts table - stores service account data separately from principals.
 * The ID is the same as the principal ID (1:1 relationship).
 */
export const serviceAccounts = pgTable(
	'service_accounts',
	{
		id: tsidColumn('id').primaryKey(),
		code: varchar('code', { length: 100 }).notNull(),
		name: varchar('name', { length: 200 }).notNull(),
		description: varchar('description', { length: 500 }),
		applicationId: tsidColumn('application_id'),
		active: boolean('active').notNull().default(true),
		whAuthType: varchar('wh_auth_type', { length: 50 }),
		whAuthTokenRef: varchar('wh_auth_token_ref', { length: 500 }),
		whSigningSecretRef: varchar('wh_signing_secret_ref', { length: 500 }),
		whSigningAlgorithm: varchar('wh_signing_algorithm', { length: 50 }),
		whCredentialsCreatedAt: timestampColumn('wh_credentials_created_at'),
		whCredentialsRegeneratedAt: timestampColumn('wh_credentials_regenerated_at'),
		lastUsedAt: timestampColumn('last_used_at'),
		createdAt: timestampColumn('created_at').notNull().defaultNow(),
		updatedAt: timestampColumn('updated_at').notNull().defaultNow(),
	},
	(table) => [
		uniqueIndex('idx_service_accounts_code').on(table.code),
		index('idx_service_accounts_application_id').on(table.applicationId),
		index('idx_service_accounts_active').on(table.active),
	],
);

// Type inference
export type ServiceAccountRecord = typeof serviceAccounts.$inferSelect;
export type NewServiceAccountRecord = typeof serviceAccounts.$inferInsert;
