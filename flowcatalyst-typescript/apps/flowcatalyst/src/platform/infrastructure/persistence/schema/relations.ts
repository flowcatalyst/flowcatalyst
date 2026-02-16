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

export const platformRelations = defineRelations(schema, (r) => ({
  // ── OAuth Clients ───────────────────────────────────────────────
  oauthClients: {
    redirectUris: r.many.oauthClientRedirectUris({
      from: r.oauthClients.id,
      to: r.oauthClientRedirectUris.oauthClientId,
    }),
    allowedOrigins: r.many.oauthClientAllowedOrigins({
      from: r.oauthClients.id,
      to: r.oauthClientAllowedOrigins.oauthClientId,
    }),
    grantTypes: r.many.oauthClientGrantTypes({
      from: r.oauthClients.id,
      to: r.oauthClientGrantTypes.oauthClientId,
    }),
    applicationIds: r.many.oauthClientApplicationIds({
      from: r.oauthClients.id,
      to: r.oauthClientApplicationIds.oauthClientId,
    }),
  },
  oauthClientRedirectUris: {
    oauthClient: r.one.oauthClients({
      from: r.oauthClientRedirectUris.oauthClientId,
      to: r.oauthClients.id,
    }),
  },
  oauthClientAllowedOrigins: {
    oauthClient: r.one.oauthClients({
      from: r.oauthClientAllowedOrigins.oauthClientId,
      to: r.oauthClients.id,
    }),
  },
  oauthClientGrantTypes: {
    oauthClient: r.one.oauthClients({
      from: r.oauthClientGrantTypes.oauthClientId,
      to: r.oauthClients.id,
    }),
  },
  oauthClientApplicationIds: {
    oauthClient: r.one.oauthClients({
      from: r.oauthClientApplicationIds.oauthClientId,
      to: r.oauthClients.id,
    }),
  },

  // ── Subscriptions ───────────────────────────────────────────────
  subscriptions: {
    eventTypes: r.many.subscriptionEventTypes({
      from: r.subscriptions.id,
      to: r.subscriptionEventTypes.subscriptionId,
    }),
    customConfigs: r.many.subscriptionCustomConfigs({
      from: r.subscriptions.id,
      to: r.subscriptionCustomConfigs.subscriptionId,
    }),
  },
  subscriptionEventTypes: {
    subscription: r.one.subscriptions({
      from: r.subscriptionEventTypes.subscriptionId,
      to: r.subscriptions.id,
    }),
  },
  subscriptionCustomConfigs: {
    subscription: r.one.subscriptions({
      from: r.subscriptionCustomConfigs.subscriptionId,
      to: r.subscriptions.id,
    }),
  },

  // ── Event Types ─────────────────────────────────────────────────
  eventTypes: {
    specVersions: r.many.eventTypeSpecVersions({
      from: r.eventTypes.id,
      to: r.eventTypeSpecVersions.eventTypeId,
    }),
  },
  eventTypeSpecVersions: {
    eventType: r.one.eventTypes({
      from: r.eventTypeSpecVersions.eventTypeId,
      to: r.eventTypes.id,
    }),
  },

  // ── Identity Providers ──────────────────────────────────────────
  identityProviders: {
    allowedDomains: r.many.identityProviderAllowedDomains({
      from: r.identityProviders.id,
      to: r.identityProviderAllowedDomains.identityProviderId,
    }),
  },
  identityProviderAllowedDomains: {
    identityProvider: r.one.identityProviders({
      from: r.identityProviderAllowedDomains.identityProviderId,
      to: r.identityProviders.id,
    }),
  },

  // ── Email Domain Mappings ───────────────────────────────────────
  emailDomainMappings: {
    additionalClients: r.many.emailDomainMappingAdditionalClients({
      from: r.emailDomainMappings.id,
      to: r.emailDomainMappingAdditionalClients.emailDomainMappingId,
    }),
    grantedClients: r.many.emailDomainMappingGrantedClients({
      from: r.emailDomainMappings.id,
      to: r.emailDomainMappingGrantedClients.emailDomainMappingId,
    }),
    allowedRoles: r.many.emailDomainMappingAllowedRoles({
      from: r.emailDomainMappings.id,
      to: r.emailDomainMappingAllowedRoles.emailDomainMappingId,
    }),
  },
  emailDomainMappingAdditionalClients: {
    emailDomainMapping: r.one.emailDomainMappings({
      from: r.emailDomainMappingAdditionalClients.emailDomainMappingId,
      to: r.emailDomainMappings.id,
    }),
  },
  emailDomainMappingGrantedClients: {
    emailDomainMapping: r.one.emailDomainMappings({
      from: r.emailDomainMappingGrantedClients.emailDomainMappingId,
      to: r.emailDomainMappings.id,
    }),
  },
  emailDomainMappingAllowedRoles: {
    emailDomainMapping: r.one.emailDomainMappings({
      from: r.emailDomainMappingAllowedRoles.emailDomainMappingId,
      to: r.emailDomainMappings.id,
    }),
  },

  // ── Auth Roles ────────────────────────────────────────────────────
  authRoles: {
    permissions: r.many.rolePermissions({
      from: r.authRoles.id,
      to: r.rolePermissions.roleId,
    }),
  },
  rolePermissions: {
    role: r.one.authRoles({
      from: r.rolePermissions.roleId,
      to: r.authRoles.id,
    }),
  },
}));
