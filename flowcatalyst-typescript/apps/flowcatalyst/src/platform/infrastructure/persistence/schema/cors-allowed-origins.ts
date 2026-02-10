/**
 * CORS Allowed Origins Database Schema
 */

import { pgTable, varchar, text, index } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn } from '@flowcatalyst/persistence';

/**
 * CORS allowed origins table.
 */
export const corsAllowedOrigins = pgTable(
  'cors_allowed_origins',
  {
    id: tsidColumn('id').primaryKey(),
    origin: varchar('origin', { length: 500 }).notNull().unique(),
    description: text('description'),
    createdBy: varchar('created_by', { length: 17 }),
    createdAt: timestampColumn('created_at').notNull().defaultNow(),
    updatedAt: timestampColumn('updated_at').notNull().defaultNow(),
  },
  (table) => [index('cors_allowed_origins_origin_idx').on(table.origin)],
);

export type CorsAllowedOriginRecord = typeof corsAllowedOrigins.$inferSelect;
export type NewCorsAllowedOriginRecord = typeof corsAllowedOrigins.$inferInsert;
