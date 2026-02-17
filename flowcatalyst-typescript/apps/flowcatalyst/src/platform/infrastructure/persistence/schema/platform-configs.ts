/**
 * Platform Config Database Schema
 */

import { pgTable, varchar, text, boolean, index, unique } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn } from '@flowcatalyst/persistence';

/**
 * Platform configs table.
 */
export const platformConfigs = pgTable(
  'app_platform_configs',
  {
    id: tsidColumn('id').primaryKey(),
    applicationCode: varchar('application_code', { length: 100 }).notNull(),
    section: varchar('section', { length: 100 }).notNull(),
    property: varchar('property', { length: 100 }).notNull(),
    scope: varchar('scope', { length: 20 }).notNull(), // 'GLOBAL' | 'CLIENT'
    clientId: varchar('client_id', { length: 17 }),
    valueType: varchar('value_type', { length: 20 }).notNull(), // 'PLAIN' | 'SECRET'
    value: text('value').notNull(),
    description: text('description'),
    createdAt: timestampColumn('created_at').notNull().defaultNow(),
    updatedAt: timestampColumn('updated_at').notNull().defaultNow(),
  },
  (table) => [
    unique('uq_app_platform_config_key').on(
      table.applicationCode,
      table.section,
      table.property,
      table.scope,
      table.clientId,
    ),
    index('idx_app_platform_configs_lookup').on(
      table.applicationCode,
      table.section,
      table.scope,
      table.clientId,
    ),
    index('idx_app_platform_configs_app_section').on(table.applicationCode, table.section),
  ],
);

export type PlatformConfigRecord = typeof platformConfigs.$inferSelect;
export type NewPlatformConfigRecord = typeof platformConfigs.$inferInsert;

/**
 * Platform config access table.
 */
export const platformConfigAccess = pgTable(
  'app_platform_config_access',
  {
    id: tsidColumn('id').primaryKey(),
    applicationCode: varchar('application_code', { length: 100 }).notNull(),
    roleCode: varchar('role_code', { length: 200 }).notNull(),
    canRead: boolean('can_read').notNull().default(true),
    canWrite: boolean('can_write').notNull().default(false),
    createdAt: timestampColumn('created_at').notNull().defaultNow(),
  },
  (table) => [
    unique('uq_app_config_access_role').on(table.applicationCode, table.roleCode),
    index('idx_app_config_access_app').on(table.applicationCode),
    index('idx_app_config_access_role').on(table.roleCode),
  ],
);

export type PlatformConfigAccessRecord = typeof platformConfigAccess.$inferSelect;
export type NewPlatformConfigAccessRecord = typeof platformConfigAccess.$inferInsert;
