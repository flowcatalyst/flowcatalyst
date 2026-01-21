/**
 * Database Schema
 *
 * All table definitions for the platform service.
 */

// Re-export common utilities from persistence package
export { tsidColumn, timestampColumn, baseEntityColumns, type BaseEntity, type NewEntity } from '@flowcatalyst/persistence';

// Re-export core tables from persistence package
export { events, auditLogs, type AuditLogRecord, type NewAuditLog } from '@flowcatalyst/persistence';

// Principal tables
export {
	principals,
	type ServiceAccountJson,
	type PrincipalRecord,
	type NewPrincipalRecord,
} from './principals.js';

// Principal roles junction table
export {
	principalRoles,
	type PrincipalRoleRecord,
	type NewPrincipalRoleRecord,
} from './principal-roles.js';

// Client tables
export {
	clients,
	type ClientNoteJson,
	type ClientRecord,
	type NewClientRecord,
} from './clients.js';

// Anchor domain tables
export {
	anchorDomains,
	type AnchorDomainRecord,
	type NewAnchorDomainRecord,
} from './anchor-domains.js';

// Application tables
export {
	applications,
	applicationClientConfigs,
	type ApplicationRecord,
	type NewApplicationRecord,
	type ApplicationClientConfigRecord,
	type NewApplicationClientConfigRecord,
} from './applications.js';

// Role tables
export {
	authRoles,
	authPermissions,
	type AuthRoleRecord,
	type NewAuthRoleRecord,
	type AuthPermissionRecord,
	type NewAuthPermissionRecord,
} from './roles.js';

// Client access grant tables
export {
	clientAccessGrants,
	type ClientAccessGrantRecord,
	type NewClientAccessGrantRecord,
} from './client-access-grants.js';

// Client auth config tables
export {
	clientAuthConfigs,
	type ClientAuthConfigRecord,
	type NewClientAuthConfigRecord,
} from './client-auth-configs.js';

// OAuth client tables
export {
	oauthClients,
	type OAuthClientRecord,
	type NewOAuthClientRecord,
} from './oauth-clients.js';

// OAuth client collection tables
export {
	oauthClientRedirectUris,
	oauthClientAllowedOrigins,
	oauthClientGrantTypes,
	oauthClientApplicationIds,
	type OAuthClientRedirectUriRecord,
	type NewOAuthClientRedirectUriRecord,
	type OAuthClientAllowedOriginRecord,
	type NewOAuthClientAllowedOriginRecord,
	type OAuthClientGrantTypeRecord,
	type NewOAuthClientGrantTypeRecord,
	type OAuthClientApplicationIdRecord,
	type NewOAuthClientApplicationIdRecord,
} from './oauth-client-collections.js';

// OIDC provider payload tables
export {
	oidcPayloads,
	type OidcPayloadData,
	type OidcPayloadRecord,
	type NewOidcPayloadRecord,
} from './oidc-payloads.js';

// Outbox tables (CQRS read model projection)
export {
	dispatchJobOutbox,
	eventOutbox,
	OutboxStatus,
	type OutboxStatusValue,
	type DispatchJobOutboxRecord,
	type NewDispatchJobOutboxRecord,
	type EventOutboxRecord,
	type NewEventOutboxRecord,
} from './outbox.js';
