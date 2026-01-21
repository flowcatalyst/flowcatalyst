/**
 * Applications Database Schema
 *
 * Tables for storing applications and their client configurations.
 */

import { pgTable, varchar, text, boolean, index, unique } from 'drizzle-orm/pg-core';
import { baseEntityColumns, tsidColumn } from '@flowcatalyst/persistence';

/**
 * Applications table - stores registered applications.
 */
export const applications = pgTable(
	'applications',
	{
		...baseEntityColumns,
		/** APPLICATION or INTEGRATION */
		type: varchar('type', { length: 50 }).notNull().default('APPLICATION'),
		code: varchar('code', { length: 50 }).notNull().unique(),
		name: varchar('name', { length: 255 }).notNull(),
		description: text('description'),
		iconUrl: varchar('icon_url', { length: 500 }),
		website: varchar('website', { length: 500 }),
		logo: text('logo'),
		logoMimeType: varchar('logo_mime_type', { length: 100 }),
		defaultBaseUrl: varchar('default_base_url', { length: 500 }),
		serviceAccountId: tsidColumn('service_account_id'),
		active: boolean('active').notNull().default(true),
	},
	(table) => [
		index('idx_applications_code').on(table.code),
		index('idx_applications_type').on(table.type),
		index('idx_applications_active').on(table.active),
	],
);

/**
 * Application client configs table - stores per-client application configurations.
 */
export const applicationClientConfigs = pgTable(
	'application_client_configs',
	{
		...baseEntityColumns,
		applicationId: tsidColumn('application_id').notNull(),
		clientId: tsidColumn('client_id').notNull(),
		enabled: boolean('enabled').notNull().default(true),
	},
	(table) => [
		index('idx_app_client_configs_application').on(table.applicationId),
		index('idx_app_client_configs_client').on(table.clientId),
		unique('uq_app_client_configs_app_client').on(table.applicationId, table.clientId),
	],
);

// Type inference
export type ApplicationRecord = typeof applications.$inferSelect;
export type NewApplicationRecord = typeof applications.$inferInsert;
export type ApplicationClientConfigRecord = typeof applicationClientConfigs.$inferSelect;
export type NewApplicationClientConfigRecord = typeof applicationClientConfigs.$inferInsert;
