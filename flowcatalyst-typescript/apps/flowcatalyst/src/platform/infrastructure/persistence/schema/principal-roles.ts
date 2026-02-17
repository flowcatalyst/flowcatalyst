/**
 * Principal Roles Database Schema
 *
 * Junction table for principal role assignments.
 * Replaces the JSONB roles array on principals table for better
 * querying and referential integrity.
 */

import { pgTable, varchar, primaryKey, index } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn } from '@flowcatalyst/persistence';
import { principals } from './principals.js';

/**
 * Principal roles junction table.
 *
 * Stores role assignments for principals (users and service accounts).
 * Each row represents one role assignment to one principal.
 */
export const principalRoles = pgTable(
  'iam_principal_roles',
  {
    principalId: tsidColumn('principal_id')
      .notNull()
      .references(() => principals.id, { onDelete: 'cascade' }),
    roleName: varchar('role_name', { length: 100 }).notNull(),
    assignmentSource: varchar('assignment_source', { length: 50 }), // 'MANUAL' | 'AUTO' | 'IDP_SYNC'
    assignedAt: timestampColumn('assigned_at').notNull().defaultNow(),
  },
  (table) => [
    primaryKey({ columns: [table.principalId, table.roleName] }),
    index('idx_iam_principal_roles_role_name').on(table.roleName),
    index('idx_iam_principal_roles_assigned_at').on(table.assignedAt),
  ],
);

// Type inference
export type PrincipalRoleRecord = typeof principalRoles.$inferSelect;
export type NewPrincipalRoleRecord = typeof principalRoles.$inferInsert;
