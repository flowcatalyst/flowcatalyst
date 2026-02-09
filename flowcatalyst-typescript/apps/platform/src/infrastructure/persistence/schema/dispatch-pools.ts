/**
 * Dispatch Pool Schema
 *
 * Table definitions for dispatch pool management.
 */

import { pgTable, varchar, integer, index, uniqueIndex } from 'drizzle-orm/pg-core';
import { baseEntityColumns, tsidColumn } from '@flowcatalyst/persistence';

/**
 * Dispatch pools control the rate at which dispatch jobs are processed.
 */
export const dispatchPools = pgTable(
  'dispatch_pools',
  {
    ...baseEntityColumns,
    code: varchar('code', { length: 100 }).notNull(),
    name: varchar('name', { length: 255 }).notNull(),
    description: varchar('description', { length: 500 }),
    rateLimit: integer('rate_limit').notNull().default(100),
    concurrency: integer('concurrency').notNull().default(10),
    clientId: tsidColumn('client_id'),
    clientIdentifier: varchar('client_identifier', { length: 100 }),
    status: varchar('status', { length: 20 }).notNull().default('ACTIVE'),
  },
  (table) => ({
    codeClientIdx: uniqueIndex('idx_dispatch_pools_code_client').on(table.code, table.clientId),
    statusIdx: index('idx_dispatch_pools_status').on(table.status),
    clientIdIdx: index('idx_dispatch_pools_client_id').on(table.clientId),
  }),
);

export type DispatchPoolRecord = typeof dispatchPools.$inferSelect;
export type NewDispatchPoolRecord = typeof dispatchPools.$inferInsert;
