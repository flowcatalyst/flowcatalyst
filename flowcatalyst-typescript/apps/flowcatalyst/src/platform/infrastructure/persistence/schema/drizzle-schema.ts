/**
 * Unified Drizzle Schema
 *
 * Re-exports all table definitions and relation definitions as a single schema bundle.
 * Pass this to drizzle() constructor to enable the relational query API (db.query.*).
 *
 * Usage:
 *   import * as schema from './drizzle-schema.js';
 *   const db = drizzle({ client, schema });
 *   // Now db.query.oauthClients.findFirst({ with: { redirectUris: true } }) works
 */

// ── Tables ──────────────────────────────────────────────────────

// OAuth clients
export { oauthClients } from "./oauth-clients.js";
export {
	oauthClientRedirectUris,
	oauthClientAllowedOrigins,
	oauthClientGrantTypes,
	oauthClientApplicationIds,
} from "./oauth-client-collections.js";

// Subscriptions
export {
	subscriptions,
	subscriptionEventTypes,
	subscriptionCustomConfigs,
} from "./subscriptions.js";

// Event types
export { eventTypes, eventTypeSpecVersions } from "./event-types.js";

// Identity providers
export {
	identityProviders,
	identityProviderAllowedDomains,
} from "./identity-providers.js";

// Email domain mappings
export {
	emailDomainMappings,
	emailDomainMappingAdditionalClients,
	emailDomainMappingGrantedClients,
	emailDomainMappingAllowedRoles,
} from "./email-domain-mappings.js";

// Other platform tables (needed for schema completeness)
export { principals } from "./principals.js";
export { serviceAccounts } from "./service-accounts.js";
export { principalRoles } from "./principal-roles.js";
export { clients } from "./clients.js";
export { anchorDomains } from "./anchor-domains.js";
export { applications, applicationClientConfigs } from "./applications.js";
export { authRoles, authPermissions, rolePermissions } from "./roles.js";
export { clientAccessGrants } from "./client-access-grants.js";
export { clientAuthConfigs } from "./client-auth-configs.js";
export { oidcPayloads } from "./oidc-payloads.js";
export { dispatchPools } from "./dispatch-pools.js";
export { idpRoleMappings } from "./idp-role-mappings.js";
export { oidcLoginStates } from "./oidc-login-states.js";
export { principalApplicationAccess } from "./principal-application-access.js";
export { platformConfigs, platformConfigAccess } from "./platform-configs.js";
export { corsAllowedOrigins } from "./cors-allowed-origins.js";
export { dispatchJobProjectionFeed, eventProjectionFeed } from "./outbox.js";

// Note: Core tables from @flowcatalyst/persistence (events, auditLogs, dispatchJobs, etc.)
// are NOT included here. Those repositories use standard db.select() and don't need
// relational query support. Including them would cause type issues since the persistence
// package may not have declaration files.
