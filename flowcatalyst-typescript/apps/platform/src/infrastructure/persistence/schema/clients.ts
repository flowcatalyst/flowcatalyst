/**
 * Clients Database Schema
 *
 * Tables for storing client organizations (tenants).
 */

import { pgTable, varchar, jsonb, index } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn, baseEntityColumns } from '@flowcatalyst/persistence';

/**
 * Client note structure stored in JSONB.
 */
export interface ClientNoteJson {
	category: string;
	text: string;
	addedBy: string;
	addedAt: string; // ISO date string
}

/**
 * Clients table - stores client organizations.
 * Status uses VARCHAR to match Java schema (not enum).
 */
export const clients = pgTable(
	'clients',
	{
		...baseEntityColumns,
		name: varchar('name', { length: 255 }).notNull(),
		identifier: varchar('identifier', { length: 100 }).notNull().unique(),
		status: varchar('status', { length: 50 }).notNull().default('ACTIVE'), // 'ACTIVE' | 'INACTIVE' | 'SUSPENDED'
		statusReason: varchar('status_reason', { length: 255 }),
		statusChangedAt: timestampColumn('status_changed_at'),
		notes: jsonb('notes').$type<ClientNoteJson[]>().default([]),
	},
	(table) => [
		index('idx_clients_identifier').on(table.identifier),
		index('idx_clients_status').on(table.status),
	],
);

// Type inference
export type ClientRecord = typeof clients.$inferSelect;
export type NewClientRecord = typeof clients.$inferInsert;
