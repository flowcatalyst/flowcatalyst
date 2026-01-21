/**
 * Anchor Domains Database Schema
 *
 * Tables for storing anchor (platform admin) email domains.
 */

import { pgTable, varchar, index } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn } from '@flowcatalyst/persistence';

/**
 * Anchor domains table - email domains that grant ANCHOR scope.
 */
export const anchorDomains = pgTable(
	'anchor_domains',
	{
		id: tsidColumn('id').primaryKey(),
		domain: varchar('domain', { length: 255 }).notNull().unique(),
		createdAt: timestampColumn('created_at').notNull().defaultNow(),
		updatedAt: timestampColumn('updated_at').notNull().defaultNow(),
	},
	(table) => [index('anchor_domains_domain_idx').on(table.domain)],
);

// Type inference
export type AnchorDomainRecord = typeof anchorDomains.$inferSelect;
export type NewAnchorDomainRecord = typeof anchorDomains.$inferInsert;
