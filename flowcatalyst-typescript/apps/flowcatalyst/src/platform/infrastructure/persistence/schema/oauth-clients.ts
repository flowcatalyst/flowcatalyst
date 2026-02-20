/**
 * OAuth Clients Database Schema
 *
 * Tables for storing OAuth client applications.
 * Note: Array fields (redirectUris, allowedOrigins, grantTypes, applicationIds)
 * are stored in separate collection tables - see oauth-client-collections.ts.
 */

import { pgTable, varchar, boolean, index } from "drizzle-orm/pg-core";
import { tsidColumn, timestampColumn } from "@flowcatalyst/persistence";

/**
 * OAuth clients table - OAuth 2.0 client applications.
 *
 * Note: The following fields are stored in separate collection tables:
 * - redirectUris -> oauth_client_redirect_uris
 * - allowedOrigins -> oauth_client_allowed_origins
 * - grantTypes -> oauth_client_grant_types
 * - applicationIds -> oauth_client_application_ids
 */
export const oauthClients = pgTable(
	"oauth_clients",
	{
		id: tsidColumn("id").primaryKey(),
		clientId: varchar("client_id", { length: 100 }).notNull().unique(),
		clientName: varchar("client_name", { length: 255 }).notNull(),
		clientType: varchar("client_type", { length: 20 })
			.notNull()
			.default("PUBLIC"),
		clientSecretRef: varchar("client_secret_ref", { length: 500 }),
		defaultScopes: varchar("default_scopes", { length: 500 }),
		pkceRequired: boolean("pkce_required").notNull().default(true),
		serviceAccountPrincipalId: tsidColumn("service_account_principal_id"),
		active: boolean("active").notNull().default(true),
		createdAt: timestampColumn("created_at").notNull().defaultNow(),
		updatedAt: timestampColumn("updated_at").notNull().defaultNow(),
	},
	(table) => [
		index("oauth_clients_client_id_idx").on(table.clientId),
		index("oauth_clients_active_idx").on(table.active),
	],
);

// Type inference
export type OAuthClientRecord = typeof oauthClients.$inferSelect;
export type NewOAuthClientRecord = typeof oauthClients.$inferInsert;
