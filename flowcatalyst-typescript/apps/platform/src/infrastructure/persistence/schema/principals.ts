/**
 * Principals Database Schema
 *
 * Tables for storing principals (users and service accounts).
 * User identity fields are flattened directly into the principals table.
 * Roles are stored in the separate principal_roles junction table.
 */

import { pgTable, varchar, boolean, jsonb, index, uniqueIndex } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn, baseEntityColumns } from '@flowcatalyst/persistence';

/**
 * Service account data stored in JSONB (for SERVICE type principals).
 */
export interface ServiceAccountJson {
	code: string;
	description: string | null;
	whAuthType: string | null;
	whAuthTokenRef: string | null;
	whSigningSecretRef: string | null;
	whSigningAlgorithm: string | null;
	whCredentialsCreatedAt: string | null;
	whCredentialsRegeneratedAt: string | null;
	lastUsedAt: string | null;
}

/**
 * Principals table - stores users and service accounts.
 *
 * User identity fields (email, email_domain, etc.) are stored as flat columns.
 * Service account data (for SERVICE type) is stored as JSONB.
 *
 * Note: Roles are stored in the principal_roles junction table, not here.
 */
export const principals = pgTable(
	'principals',
	{
		...baseEntityColumns,
		type: varchar('type', { length: 20 }).notNull(), // 'USER' | 'SERVICE'
		scope: varchar('scope', { length: 20 }), // 'ANCHOR' | 'PARTNER' | 'CLIENT' | null
		clientId: tsidColumn('client_id'),
		applicationId: tsidColumn('application_id'),
		name: varchar('name', { length: 255 }).notNull(),
		active: boolean('active').notNull().default(true),

		// Flattened user identity fields (for USER type)
		email: varchar('email', { length: 255 }),
		emailDomain: varchar('email_domain', { length: 100 }),
		idpType: varchar('idp_type', { length: 50 }), // 'INTERNAL' | 'OIDC'
		externalIdpId: varchar('external_idp_id', { length: 255 }),
		passwordHash: varchar('password_hash', { length: 255 }),
		lastLoginAt: timestampColumn('last_login_at'),

		// Service account data (for SERVICE type)
		serviceAccount: jsonb('service_account').$type<ServiceAccountJson>(),
	},
	(table) => [
		index('idx_principals_type').on(table.type),
		index('idx_principals_client_id').on(table.clientId),
		index('idx_principals_active').on(table.active),
		uniqueIndex('idx_principals_email').on(table.email),
		index('idx_principals_email_domain').on(table.emailDomain),
	],
);

// Type inference
export type PrincipalRecord = typeof principals.$inferSelect;
export type NewPrincipalRecord = typeof principals.$inferInsert;
