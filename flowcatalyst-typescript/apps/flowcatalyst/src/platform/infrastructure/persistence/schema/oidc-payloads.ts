/**
 * OIDC Payloads Database Schema
 *
 * Storage for oidc-provider artifacts (authorization codes, tokens, sessions, etc.)
 * Uses a flexible JSONB payload structure as recommended by oidc-provider.
 */

import { pgTable, varchar, jsonb, index, timestamp } from 'drizzle-orm/pg-core';
import { tsidColumn, timestampColumn } from '@flowcatalyst/persistence';

/**
 * OIDC payloads table - stores all oidc-provider artifacts.
 *
 * Model types stored:
 * - Session: User sessions
 * - AccessToken: OAuth 2.0 access tokens
 * - AuthorizationCode: Authorization codes for code flow
 * - RefreshToken: Refresh tokens
 * - DeviceCode: Device authorization flow codes
 * - ClientCredentials: Client credentials grants
 * - Client: Dynamic client registration (if enabled)
 * - InitialAccessToken: Dynamic registration tokens
 * - RegistrationAccessToken: Client management tokens
 * - Interaction: Consent/login interactions
 * - ReplayDetection: Replay attack prevention
 * - PushedAuthorizationRequest: PAR requests
 * - Grant: User consent grants
 * - BackchannelAuthenticationRequest: CIBA requests
 */
export const oidcPayloads = pgTable(
  'oauth_oidc_payloads',
  {
    // Primary identifier (model-specific ID from oidc-provider)
    id: varchar('id', { length: 128 }).primaryKey(),

    // Model type discriminator
    type: varchar('type', { length: 64 }).notNull(),

    // The actual payload data (oidc-provider manages structure)
    payload: jsonb('payload').$type<OidcPayloadData>().notNull(),

    // Grant ID - links related tokens (access, refresh) to same grant
    grantId: varchar('grant_id', { length: 128 }),

    // User code - for device authorization flow
    userCode: varchar('user_code', { length: 128 }),

    // UID - unique identifier for certain model types
    uid: varchar('uid', { length: 128 }),

    // Expiration timestamp
    expiresAt: timestamp('expires_at', { withTimezone: true }),

    // Consumption timestamp (for single-use artifacts like auth codes)
    consumedAt: timestamp('consumed_at', { withTimezone: true }),

    // Creation timestamp
    createdAt: timestampColumn('created_at').notNull().defaultNow(),
  },
  (table) => [
    // Index for grant-based lookups (revocation)
    index('oauth_oidc_payloads_grant_id_idx').on(table.grantId),

    // Index for user code lookups (device flow)
    index('oauth_oidc_payloads_user_code_idx').on(table.userCode),

    // Index for UID lookups
    index('oauth_oidc_payloads_uid_idx').on(table.uid),

    // Index for type-based queries
    index('oauth_oidc_payloads_type_idx').on(table.type),

    // Index for expiration cleanup
    index('oauth_oidc_payloads_expires_at_idx').on(table.expiresAt),
  ],
);

/**
 * Payload data structure - flexible JSON managed by oidc-provider.
 * We don't strictly type this as oidc-provider owns the structure.
 */
export interface OidcPayloadData {
  [key: string]: unknown;
}

// Type inference
export type OidcPayloadRecord = typeof oidcPayloads.$inferSelect;
export type NewOidcPayloadRecord = typeof oidcPayloads.$inferInsert;
