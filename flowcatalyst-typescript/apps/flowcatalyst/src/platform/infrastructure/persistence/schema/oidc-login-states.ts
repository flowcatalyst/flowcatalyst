/**
 * OIDC Login State Schema
 *
 * Stores state for in-progress OIDC login flows.
 * Each state is single-use and expires after 10 minutes.
 */

import { pgTable, varchar, timestamp, index } from 'drizzle-orm/pg-core';

/**
 * OIDC login states table.
 */
export const oidcLoginStates = pgTable(
  'oidc_login_states',
  {
    state: varchar('state', { length: 200 }).primaryKey(),
    emailDomain: varchar('email_domain', { length: 255 }).notNull(),
    identityProviderId: varchar('identity_provider_id', { length: 17 }).notNull(),
    emailDomainMappingId: varchar('email_domain_mapping_id', { length: 17 }).notNull(),
    nonce: varchar('nonce', { length: 200 }).notNull(),
    codeVerifier: varchar('code_verifier', { length: 200 }).notNull(),
    returnUrl: varchar('return_url', { length: 2000 }),
    // OAuth passthrough fields (when login was triggered from /oauth/authorize)
    oauthClientId: varchar('oauth_client_id', { length: 200 }),
    oauthRedirectUri: varchar('oauth_redirect_uri', { length: 2000 }),
    oauthScope: varchar('oauth_scope', { length: 500 }),
    oauthState: varchar('oauth_state', { length: 500 }),
    oauthCodeChallenge: varchar('oauth_code_challenge', { length: 500 }),
    oauthCodeChallengeMethod: varchar('oauth_code_challenge_method', { length: 20 }),
    oauthNonce: varchar('oauth_nonce', { length: 500 }),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
    expiresAt: timestamp('expires_at', { withTimezone: true }).notNull(),
  },
  (table) => [index('idx_oidc_login_states_expires').on(table.expiresAt)],
);

export type OidcLoginStateRecord = typeof oidcLoginStates.$inferSelect;
export type NewOidcLoginStateRecord = typeof oidcLoginStates.$inferInsert;
