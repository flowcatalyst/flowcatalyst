/**
 * Email Domain Mapping Schema
 */

import { pgTable, varchar, boolean, serial, index, uniqueIndex } from 'drizzle-orm/pg-core';
import { baseEntityColumns } from '@flowcatalyst/persistence';

/**
 * Email domain mappings table.
 * Maps email domains to identity providers with access scope configuration.
 */
export const emailDomainMappings = pgTable(
	'email_domain_mappings',
	{
		...baseEntityColumns,
		emailDomain: varchar('email_domain', { length: 255 }).notNull(),
		identityProviderId: varchar('identity_provider_id', { length: 17 }).notNull(),
		scopeType: varchar('scope_type', { length: 20 }).notNull(),
		primaryClientId: varchar('primary_client_id', { length: 17 }),
		requiredOidcTenantId: varchar('required_oidc_tenant_id', { length: 100 }),
		syncRolesFromIdp: boolean('sync_roles_from_idp').notNull().default(false),
	},
	(table) => [
		uniqueIndex('idx_email_domain_mappings_domain').on(table.emailDomain),
		index('idx_email_domain_mappings_idp').on(table.identityProviderId),
		index('idx_email_domain_mappings_scope').on(table.scopeType),
	],
);

export type EmailDomainMappingRecord = typeof emailDomainMappings.$inferSelect;
export type NewEmailDomainMappingRecord = typeof emailDomainMappings.$inferInsert;

/**
 * Additional clients junction table.
 * For CLIENT scope: extra clients beyond the primary.
 */
export const emailDomainMappingAdditionalClients = pgTable(
	'email_domain_mapping_additional_clients',
	{
		id: serial('id').primaryKey(),
		emailDomainMappingId: varchar('email_domain_mapping_id', { length: 17 }).notNull(),
		clientId: varchar('client_id', { length: 17 }).notNull(),
	},
	(table) => [
		index('idx_edm_additional_clients_mapping').on(table.emailDomainMappingId),
	],
);

export type EmailDomainMappingAdditionalClientRecord = typeof emailDomainMappingAdditionalClients.$inferSelect;

/**
 * Granted clients junction table.
 * For PARTNER scope: clients explicitly granted access.
 */
export const emailDomainMappingGrantedClients = pgTable(
	'email_domain_mapping_granted_clients',
	{
		id: serial('id').primaryKey(),
		emailDomainMappingId: varchar('email_domain_mapping_id', { length: 17 }).notNull(),
		clientId: varchar('client_id', { length: 17 }).notNull(),
	},
	(table) => [
		index('idx_edm_granted_clients_mapping').on(table.emailDomainMappingId),
	],
);

export type EmailDomainMappingGrantedClientRecord = typeof emailDomainMappingGrantedClients.$inferSelect;

/**
 * Allowed roles junction table.
 * Restricts which roles users from this domain can be assigned.
 */
export const emailDomainMappingAllowedRoles = pgTable(
	'email_domain_mapping_allowed_roles',
	{
		id: serial('id').primaryKey(),
		emailDomainMappingId: varchar('email_domain_mapping_id', { length: 17 }).notNull(),
		roleId: varchar('role_id', { length: 17 }).notNull(),
	},
	(table) => [
		index('idx_edm_allowed_roles_mapping').on(table.emailDomainMappingId),
	],
);

export type EmailDomainMappingAllowedRoleRecord = typeof emailDomainMappingAllowedRoles.$inferSelect;
