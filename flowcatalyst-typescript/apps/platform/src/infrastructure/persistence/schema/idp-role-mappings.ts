/**
 * IDP Role Mapping Schema
 *
 * Maps external IDP role names to internal platform role names.
 */

import { pgTable, varchar, uniqueIndex } from 'drizzle-orm/pg-core';
import { baseEntityColumns } from '@flowcatalyst/persistence';

/**
 * IDP role mappings table.
 */
export const idpRoleMappings = pgTable(
  'idp_role_mappings',
  {
    ...baseEntityColumns,
    idpRoleName: varchar('idp_role_name', { length: 200 }).notNull(),
    internalRoleName: varchar('internal_role_name', { length: 200 }).notNull(),
  },
  (table) => [uniqueIndex('idx_idp_role_mappings_idp_role_name').on(table.idpRoleName)],
);

export type IdpRoleMappingRecord = typeof idpRoleMappings.$inferSelect;
export type NewIdpRoleMappingRecord = typeof idpRoleMappings.$inferInsert;
