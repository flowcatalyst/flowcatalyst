/**
 * Roles Database Schema
 *
 * Tables for storing roles and permissions.
 */

import { pgTable, varchar, text, boolean, index, jsonb } from 'drizzle-orm/pg-core';
import { baseEntityColumns, tsidColumn } from '@flowcatalyst/persistence';

/**
 * Auth roles table - stores role definitions.
 *
 * Roles can come from three sources:
 * - CODE: Defined in code, synced to DB at startup
 * - DATABASE: Created by administrators through the UI
 * - SDK: Registered by external applications via the SDK API
 */
export const authRoles = pgTable(
	'auth_roles',
	{
		...baseEntityColumns,
		/** The application this role belongs to (ID reference) */
		applicationId: tsidColumn('application_id'),
		/** The application code (denormalized for queries) */
		applicationCode: varchar('application_code', { length: 50 }),
		/** Full role name with application prefix (e.g., "platform:tenant-admin") */
		name: varchar('name', { length: 255 }).notNull().unique(),
		/** Human-readable display name (e.g., "Tenant Administrator") */
		displayName: varchar('display_name', { length: 255 }).notNull(),
		description: text('description'),
		permissions: jsonb('permissions').notNull().$type<string[]>().default([]),
		/** Source of this role: CODE, DATABASE, or SDK */
		source: varchar('source', { length: 50 }).notNull().default('DATABASE'),
		/** If true, this role syncs to IDPs configured for client-managed roles */
		clientManaged: boolean('client_managed').notNull().default(false),
	},
	(table) => [
		index('idx_auth_roles_name').on(table.name),
		index('idx_auth_roles_application_id').on(table.applicationId),
		index('idx_auth_roles_application_code').on(table.applicationCode),
		index('idx_auth_roles_source').on(table.source),
		index('idx_auth_roles_client_managed').on(table.clientManaged),
	],
);

/**
 * Auth permissions table - stores permission definitions.
 */
export const authPermissions = pgTable(
	'auth_permissions',
	{
		...baseEntityColumns,
		code: varchar('code', { length: 255 }).notNull().unique(),
		subdomain: varchar('subdomain', { length: 50 }).notNull(),
		context: varchar('context', { length: 50 }).notNull(),
		aggregate: varchar('aggregate', { length: 50 }).notNull(),
		action: varchar('action', { length: 50 }).notNull(),
		description: text('description'),
	},
	(table) => [
		index('idx_auth_permissions_code').on(table.code),
		index('idx_auth_permissions_subdomain').on(table.subdomain),
		index('idx_auth_permissions_context').on(table.context),
	],
);

// Type inference
export type AuthRoleRecord = typeof authRoles.$inferSelect;
export type NewAuthRoleRecord = typeof authRoles.$inferInsert;
export type AuthPermissionRecord = typeof authPermissions.$inferSelect;
export type NewAuthPermissionRecord = typeof authPermissions.$inferInsert;
