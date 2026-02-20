/**
 * Identity Provider Schema
 */

import {
	pgTable,
	varchar,
	boolean,
	serial,
	index,
	uniqueIndex,
} from "drizzle-orm/pg-core";
import { baseEntityColumns } from "@flowcatalyst/persistence";

/**
 * Identity providers table.
 */
export const identityProviders = pgTable(
	"oauth_identity_providers",
	{
		...baseEntityColumns,
		code: varchar("code", { length: 50 }).notNull(),
		name: varchar("name", { length: 200 }).notNull(),
		type: varchar("type", { length: 20 }).notNull(),
		oidcIssuerUrl: varchar("oidc_issuer_url", { length: 500 }),
		oidcClientId: varchar("oidc_client_id", { length: 200 }),
		oidcClientSecretRef: varchar("oidc_client_secret_ref", { length: 500 }),
		oidcMultiTenant: boolean("oidc_multi_tenant").notNull().default(false),
		oidcIssuerPattern: varchar("oidc_issuer_pattern", { length: 500 }),
	},
	(table) => [uniqueIndex("idx_oauth_identity_providers_code").on(table.code)],
);

export type IdentityProviderRecord = typeof identityProviders.$inferSelect;
export type NewIdentityProviderRecord = typeof identityProviders.$inferInsert;

/**
 * Identity provider allowed email domains junction table.
 */
export const identityProviderAllowedDomains = pgTable(
	"oauth_identity_provider_allowed_domains",
	{
		id: serial("id").primaryKey(),
		identityProviderId: varchar("identity_provider_id", {
			length: 17,
		}).notNull(),
		emailDomain: varchar("email_domain", { length: 255 }).notNull(),
	},
	(table) => [
		index("idx_oauth_idp_allowed_domains_idp").on(table.identityProviderId),
	],
);

export type IdentityProviderAllowedDomainRecord =
	typeof identityProviderAllowedDomains.$inferSelect;
export type NewIdentityProviderAllowedDomainRecord =
	typeof identityProviderAllowedDomains.$inferInsert;
