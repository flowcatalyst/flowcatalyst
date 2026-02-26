/**
 * OAuth Client Collection Tables Database Schema
 *
 * Junction tables for OAuth client array fields (redirect URIs, allowed origins, etc.).
 * Replaces the JSONB/array columns on oauth_clients for better querying.
 */

import { pgTable, varchar, primaryKey, index } from "drizzle-orm/pg-core";
import { tsidColumn } from "@flowcatalyst/persistence";
import { oauthClients } from "./oauth-clients.js";

/**
 * OAuth client redirect URIs collection table.
 */
export const oauthClientRedirectUris = pgTable(
	"oauth_client_redirect_uris",
	{
		oauthClientId: tsidColumn("oauth_client_id")
			.notNull()
			.references(() => oauthClients.id, { onDelete: "cascade" }),
		redirectUri: varchar("redirect_uri", { length: 500 }).notNull(),
	},
	(table) => [
		primaryKey({ columns: [table.oauthClientId, table.redirectUri] }),
		index("idx_oauth_client_redirect_uris_client").on(table.oauthClientId),
	],
);

export type OAuthClientRedirectUriRecord =
	typeof oauthClientRedirectUris.$inferSelect;
export type NewOAuthClientRedirectUriRecord =
	typeof oauthClientRedirectUris.$inferInsert;

/**
 * OAuth client allowed origins collection table (for CORS).
 */
export const oauthClientAllowedOrigins = pgTable(
	"oauth_client_allowed_origins",
	{
		oauthClientId: tsidColumn("oauth_client_id")
			.notNull()
			.references(() => oauthClients.id, { onDelete: "cascade" }),
		allowedOrigin: varchar("allowed_origin", { length: 200 }).notNull(),
	},
	(table) => [
		primaryKey({ columns: [table.oauthClientId, table.allowedOrigin] }),
		index("idx_oauth_client_allowed_origins_client").on(table.oauthClientId),
		index("idx_oauth_client_allowed_origins_origin").on(table.allowedOrigin),
	],
);

export type OAuthClientAllowedOriginRecord =
	typeof oauthClientAllowedOrigins.$inferSelect;
export type NewOAuthClientAllowedOriginRecord =
	typeof oauthClientAllowedOrigins.$inferInsert;

/**
 * OAuth client grant types collection table.
 */
export const oauthClientGrantTypes = pgTable(
	"oauth_client_grant_types",
	{
		oauthClientId: tsidColumn("oauth_client_id")
			.notNull()
			.references(() => oauthClients.id, { onDelete: "cascade" }),
		grantType: varchar("grant_type", { length: 50 }).notNull(),
	},
	(table) => [
		primaryKey({ columns: [table.oauthClientId, table.grantType] }),
		index("idx_oauth_client_grant_types_client").on(table.oauthClientId),
	],
);

export type OAuthClientGrantTypeRecord =
	typeof oauthClientGrantTypes.$inferSelect;
export type NewOAuthClientGrantTypeRecord =
	typeof oauthClientGrantTypes.$inferInsert;

/**
 * OAuth client application IDs collection table.
 */
export const oauthClientApplicationIds = pgTable(
	"oauth_client_application_ids",
	{
		oauthClientId: tsidColumn("oauth_client_id")
			.notNull()
			.references(() => oauthClients.id, { onDelete: "cascade" }),
		applicationId: tsidColumn("application_id").notNull(),
	},
	(table) => [
		primaryKey({ columns: [table.oauthClientId, table.applicationId] }),
		index("idx_oauth_client_application_ids_client").on(table.oauthClientId),
	],
);

export type OAuthClientApplicationIdRecord =
	typeof oauthClientApplicationIds.$inferSelect;
export type NewOAuthClientApplicationIdRecord =
	typeof oauthClientApplicationIds.$inferInsert;
