/**
 * Principals Database Schema
 *
 * Tables for storing principals (users and service accounts).
 * User identity fields are flattened directly into the principals table.
 * Service account data is stored in the separate service_accounts table.
 * Roles are stored in the separate principal_roles junction table.
 */

import { pgTable, varchar, boolean, index, uniqueIndex } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn, baseEntityColumns } from '@flowcatalyst/persistence';

/**
 * Principals table - stores users and service accounts.
 *
 * User identity fields (email, email_domain, etc.) are stored as flat columns.
 * Service account data is in the service_accounts table, linked via service_account_id.
 *
 * Note: Roles are stored in the principal_roles junction table, not here.
 */
export const principals = pgTable(
  'principals',
  {
    ...baseEntityColumns,
    type: varchar('type', { length: 20 }).notNull(), // 'USER' | 'SERVICE'
    scope: varchar('scope', { length: 20 }), // 'ANCHOR' | 'PARTNER' | 'CLIENT' | null
    clientId: tsidColumn('client_id'),
    applicationId: tsidColumn('application_id'),
    name: varchar('name', { length: 255 }).notNull(),
    active: boolean('active').notNull().default(true),

    // Flattened user identity fields (for USER type)
    email: varchar('email', { length: 255 }),
    emailDomain: varchar('email_domain', { length: 100 }),
    idpType: varchar('idp_type', { length: 50 }), // 'INTERNAL' | 'OIDC'
    externalIdpId: varchar('external_idp_id', { length: 255 }),
    passwordHash: varchar('password_hash', { length: 255 }),
    lastLoginAt: timestampColumn('last_login_at'),

    // FK to service_accounts table (for SERVICE type)
    serviceAccountId: tsidColumn('service_account_id'),
  },
  (table) => [
    index('idx_principals_type').on(table.type),
    index('idx_principals_client_id').on(table.clientId),
    index('idx_principals_active').on(table.active),
    uniqueIndex('idx_principals_email').on(table.email),
    index('idx_principals_email_domain').on(table.emailDomain),
    uniqueIndex('idx_principals_service_account_id').on(table.serviceAccountId),
  ],
);

// Type inference
export type PrincipalRecord = typeof principals.$inferSelect;
export type NewPrincipalRecord = typeof principals.$inferInsert;
