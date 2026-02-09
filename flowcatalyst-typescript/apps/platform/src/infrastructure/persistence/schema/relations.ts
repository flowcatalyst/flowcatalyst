/**
 * Drizzle v1 Relational Query Definitions
 *
 * Defines parent ↔ child relationships using defineRelations() for Drizzle's
 * relational query API (db.query). In v1, relations are a separate config
 * passed to drizzle() alongside the schema.
 *
 * These are metadata-only — they don't create FK constraints in the database.
 */

import { defineRelations } from 'drizzle-orm';
import * as schema from './drizzle-schema.js';

export const platformRelations = defineRelations(schema, ({ one, many }) => ({
  // ── OAuth Clients ───────────────────────────────────────────────
  oauthClients: {
    redirectUris: many.oauthClientRedirectUris({
      from: schema.oauthClients.id,
      to: schema.oauthClientRedirectUris.oauthClientId,
    }),
    allowedOrigins: many.oauthClientAllowedOrigins({
      from: schema.oauthClients.id,
      to: schema.oauthClientAllowedOrigins.oauthClientId,
    }),
    grantTypes: many.oauthClientGrantTypes({
      from: schema.oauthClients.id,
      to: schema.oauthClientGrantTypes.oauthClientId,
    }),
    applicationIds: many.oauthClientApplicationIds({
      from: schema.oauthClients.id,
      to: schema.oauthClientApplicationIds.oauthClientId,
    }),
  },
  oauthClientRedirectUris: {
    oauthClient: one.oauthClients({
      from: schema.oauthClientRedirectUris.oauthClientId,
      to: schema.oauthClients.id,
    }),
  },
  oauthClientAllowedOrigins: {
    oauthClient: one.oauthClients({
      from: schema.oauthClientAllowedOrigins.oauthClientId,
      to: schema.oauthClients.id,
    }),
  },
  oauthClientGrantTypes: {
    oauthClient: one.oauthClients({
      from: schema.oauthClientGrantTypes.oauthClientId,
      to: schema.oauthClients.id,
    }),
  },
  oauthClientApplicationIds: {
    oauthClient: one.oauthClients({
      from: schema.oauthClientApplicationIds.oauthClientId,
      to: schema.oauthClients.id,
    }),
  },

  // ── Subscriptions ───────────────────────────────────────────────
  subscriptions: {
    eventTypes: many.subscriptionEventTypes({
      from: schema.subscriptions.id,
      to: schema.subscriptionEventTypes.subscriptionId,
    }),
    customConfigs: many.subscriptionCustomConfigs({
      from: schema.subscriptions.id,
      to: schema.subscriptionCustomConfigs.subscriptionId,
    }),
  },
  subscriptionEventTypes: {
    subscription: one.subscriptions({
      from: schema.subscriptionEventTypes.subscriptionId,
      to: schema.subscriptions.id,
    }),
  },
  subscriptionCustomConfigs: {
    subscription: one.subscriptions({
      from: schema.subscriptionCustomConfigs.subscriptionId,
      to: schema.subscriptions.id,
    }),
  },

  // ── Event Types ─────────────────────────────────────────────────
  eventTypes: {
    specVersions: many.eventTypeSpecVersions({
      from: schema.eventTypes.id,
      to: schema.eventTypeSpecVersions.eventTypeId,
    }),
  },
  eventTypeSpecVersions: {
    eventType: one.eventTypes({
      from: schema.eventTypeSpecVersions.eventTypeId,
      to: schema.eventTypes.id,
    }),
  },

  // ── Identity Providers ──────────────────────────────────────────
  identityProviders: {
    allowedDomains: many.identityProviderAllowedDomains({
      from: schema.identityProviders.id,
      to: schema.identityProviderAllowedDomains.identityProviderId,
    }),
  },
  identityProviderAllowedDomains: {
    identityProvider: one.identityProviders({
      from: schema.identityProviderAllowedDomains.identityProviderId,
      to: schema.identityProviders.id,
    }),
  },

  // ── Email Domain Mappings ───────────────────────────────────────
  emailDomainMappings: {
    additionalClients: many.emailDomainMappingAdditionalClients({
      from: schema.emailDomainMappings.id,
      to: schema.emailDomainMappingAdditionalClients.emailDomainMappingId,
    }),
    grantedClients: many.emailDomainMappingGrantedClients({
      from: schema.emailDomainMappings.id,
      to: schema.emailDomainMappingGrantedClients.emailDomainMappingId,
    }),
    allowedRoles: many.emailDomainMappingAllowedRoles({
      from: schema.emailDomainMappings.id,
      to: schema.emailDomainMappingAllowedRoles.emailDomainMappingId,
    }),
  },
  emailDomainMappingAdditionalClients: {
    emailDomainMapping: one.emailDomainMappings({
      from: schema.emailDomainMappingAdditionalClients.emailDomainMappingId,
      to: schema.emailDomainMappings.id,
    }),
  },
  emailDomainMappingGrantedClients: {
    emailDomainMapping: one.emailDomainMappings({
      from: schema.emailDomainMappingGrantedClients.emailDomainMappingId,
      to: schema.emailDomainMappings.id,
    }),
  },
  emailDomainMappingAllowedRoles: {
    emailDomainMapping: one.emailDomainMappings({
      from: schema.emailDomainMappingAllowedRoles.emailDomainMappingId,
      to: schema.emailDomainMappings.id,
    }),
  },
}));
