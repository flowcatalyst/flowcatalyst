/**
 * Client Auth Configs Database Schema
 *
 * Tables for storing client authentication configuration.
 */

import { pgTable, varchar, boolean, jsonb, index } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn } from '@flowcatalyst/persistence';

/**
 * Client auth configs table - authentication configuration per email domain.
 */
export const clientAuthConfigs = pgTable(
	'client_auth_configs',
	{
		id: tsidColumn('id').primaryKey(),
		emailDomain: varchar('email_domain', { length: 255 }).notNull().unique(),
		configType: varchar('config_type', { length: 50 }).notNull(),
		primaryClientId: tsidColumn('primary_client_id'),
		additionalClientIds: jsonb('additional_client_ids').$type<string[]>().notNull().default([]),
		grantedClientIds: jsonb('granted_client_ids').$type<string[]>().notNull().default([]),
		authProvider: varchar('auth_provider', { length: 50 }).notNull(),
		oidcIssuerUrl: varchar('oidc_issuer_url', { length: 500 }),
		oidcClientId: varchar('oidc_client_id', { length: 255 }),
		oidcMultiTenant: boolean('oidc_multi_tenant').notNull().default(false),
		oidcIssuerPattern: varchar('oidc_issuer_pattern', { length: 500 }),
		oidcClientSecretRef: varchar('oidc_client_secret_ref', { length: 1000 }),
		createdAt: timestampColumn('created_at').notNull().defaultNow(),
		updatedAt: timestampColumn('updated_at').notNull().defaultNow(),
	},
	(table) => [
		index('client_auth_configs_email_domain_idx').on(table.emailDomain),
		index('client_auth_configs_config_type_idx').on(table.configType),
		index('client_auth_configs_primary_client_id_idx').on(table.primaryClientId),
	],
);

// Type inference
export type ClientAuthConfigRecord = typeof clientAuthConfigs.$inferSelect;
export type NewClientAuthConfigRecord = typeof clientAuthConfigs.$inferInsert;
