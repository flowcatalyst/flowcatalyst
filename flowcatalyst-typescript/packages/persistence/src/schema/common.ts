/**
 * Common Schema Definitions
 *
 * Shared column definitions and types used across all database tables.
 */

import { varchar, timestamp } from 'drizzle-orm/pg-core';

/**
 * Typed ID column - 17-character prefixed TSID.
 * Format: "{prefix}_{tsid}" (e.g., "clt_0HZXEQ5Y8JY5Z")
 * - 3-character prefix
 * - 1 underscore separator
 * - 13-character Crockford Base32 TSID
 *
 * Used for most primary keys and foreign keys.
 */
export const tsidColumn = (name: string) => varchar(name, { length: 17 });

/**
 * Raw TSID column - 13-character unprefixed TSID.
 * Format: Crockford Base32 TSID only (e.g., "0HZXEQ5Y8JY5Z")
 *
 * Used for high-volume tables (events, dispatch_jobs) where the 4-byte
 * prefix overhead adds up significantly. The entity type is clear from
 * the table context.
 */
export const rawTsidColumn = (name: string) => varchar(name, { length: 13 });

/**
 * Standard timestamp column with timezone.
 */
export const timestampColumn = (name: string) => timestamp(name, { withTimezone: true, mode: 'date' });

/**
 * Base entity fields that all entities should have.
 * - id: TSID primary key
 * - createdAt: When the entity was created
 * - updatedAt: When the entity was last modified
 */
export const baseEntityColumns = {
	id: tsidColumn('id').primaryKey(),
	createdAt: timestampColumn('created_at').notNull().defaultNow(),
	updatedAt: timestampColumn('updated_at').notNull().defaultNow(),
};

/**
 * Base entity type with common fields.
 */
export interface BaseEntity {
	readonly id: string;
	readonly createdAt: Date;
	readonly updatedAt: Date;
}

/**
 * Input type for creating a new entity.
 * createdAt and updatedAt are auto-populated.
 */
export type NewEntity<T extends BaseEntity> = Omit<T, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};
