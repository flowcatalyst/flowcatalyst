/**
 * Database Schema
 *
 * All table definitions for the platform service.
 */

// Re-export common utilities from persistence package
export {
	tsidColumn,
	timestampColumn,
	baseEntityColumns,
	type BaseEntity,
	type NewEntity,
} from "@flowcatalyst/persistence";

// Re-export core tables from persistence package
export {
	events,
	auditLogs,
	type AuditLogRecord,
	type NewAuditLog,
} from "@flowcatalyst/persistence";

// Re-export dispatch jobs write table from persistence package
export {
	dispatchJobs,
	type DispatchJobRecord,
	type NewDispatchJobRecord,
} from "@flowcatalyst/persistence";

// Re-export read model tables from persistence package
export {
	eventsRead,
	type EventReadRecord,
	type NewEventReadRecord,
} from "@flowcatalyst/persistence";
export {
	dispatchJobsRead,
	type DispatchJobReadRecord,
	type NewDispatchJobReadRecord,
} from "@flowcatalyst/persistence";
export {
	dispatchJobAttempts,
	type DispatchJobAttemptRecord,
	type NewDispatchJobAttemptRecord,
} from "@flowcatalyst/persistence";

// Principal tables
export {
	principals,
	type PrincipalRecord,
	type NewPrincipalRecord,
} from "./principals.js";

// Service account tables
export {
	serviceAccounts,
	type ServiceAccountRecord,
	type NewServiceAccountRecord,
} from "./service-accounts.js";

// Principal roles junction table
export {
	principalRoles,
	type PrincipalRoleRecord,
	type NewPrincipalRoleRecord,
} from "./principal-roles.js";

// Client tables
export {
	clients,
	type ClientNoteJson,
	type ClientRecord,
	type NewClientRecord,
} from "./clients.js";

// Anchor domain tables
export {
	anchorDomains,
	type AnchorDomainRecord,
	type NewAnchorDomainRecord,
} from "./anchor-domains.js";

// Application tables
export {
	applications,
	applicationClientConfigs,
	type ApplicationRecord,
	type NewApplicationRecord,
	type ApplicationClientConfigRecord,
	type NewApplicationClientConfigRecord,
} from "./applications.js";

// Role tables
export {
	authRoles,
	authPermissions,
	rolePermissions,
	type AuthRoleRecord,
	type NewAuthRoleRecord,
	type AuthPermissionRecord,
	type NewAuthPermissionRecord,
	type RolePermissionRecord,
	type NewRolePermissionRecord,
} from "./roles.js";

// Client access grant tables
export {
	clientAccessGrants,
	type ClientAccessGrantRecord,
	type NewClientAccessGrantRecord,
} from "./client-access-grants.js";

// Client auth config tables
export {
	clientAuthConfigs,
	type ClientAuthConfigRecord,
	type NewClientAuthConfigRecord,
} from "./client-auth-configs.js";

// OAuth client tables
export {
	oauthClients,
	type OAuthClientRecord,
	type NewOAuthClientRecord,
} from "./oauth-clients.js";

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
} from "./oauth-client-collections.js";

// OIDC provider payload tables
export {
	oidcPayloads,
	type OidcPayloadData,
	type OidcPayloadRecord,
	type NewOidcPayloadRecord,
} from "./oidc-payloads.js";

// Event type tables
export {
	eventTypes,
	eventTypeSpecVersions,
	type EventTypeRecord,
	type NewEventTypeRecord,
	type EventTypeSpecVersionRecord,
	type NewEventTypeSpecVersionRecord,
} from "./event-types.js";

// Dispatch pool tables
export {
	dispatchPools,
	type DispatchPoolRecord,
	type NewDispatchPoolRecord,
} from "./dispatch-pools.js";

// Subscription tables
export {
	subscriptions,
	subscriptionEventTypes,
	subscriptionCustomConfigs,
	type SubscriptionRecord,
	type NewSubscriptionRecord,
	type SubscriptionEventTypeRecord,
	type NewSubscriptionEventTypeRecord,
	type SubscriptionCustomConfigRecord,
	type NewSubscriptionCustomConfigRecord,
} from "./subscriptions.js";

// Identity provider tables
export {
	identityProviders,
	identityProviderAllowedDomains,
	type IdentityProviderRecord,
	type NewIdentityProviderRecord,
	type IdentityProviderAllowedDomainRecord,
	type NewIdentityProviderAllowedDomainRecord,
} from "./identity-providers.js";

// Email domain mapping tables
export {
	emailDomainMappings,
	emailDomainMappingAdditionalClients,
	emailDomainMappingGrantedClients,
	emailDomainMappingAllowedRoles,
	type EmailDomainMappingRecord,
	type NewEmailDomainMappingRecord,
	type EmailDomainMappingAdditionalClientRecord,
	type EmailDomainMappingGrantedClientRecord,
	type EmailDomainMappingAllowedRoleRecord,
} from "./email-domain-mappings.js";

// IDP role mapping tables
export {
	idpRoleMappings,
	type IdpRoleMappingRecord,
	type NewIdpRoleMappingRecord,
} from "./idp-role-mappings.js";

// OIDC login state tables
export {
	oidcLoginStates,
	type OidcLoginStateRecord,
	type NewOidcLoginStateRecord,
} from "./oidc-login-states.js";

// Principal application access junction table
export {
	principalApplicationAccess,
	type PrincipalApplicationAccessRecord,
	type NewPrincipalApplicationAccessRecord,
} from "./principal-application-access.js";

// Platform config tables
export {
	platformConfigs,
	platformConfigAccess,
	type PlatformConfigRecord,
	type NewPlatformConfigRecord,
	type PlatformConfigAccessRecord,
	type NewPlatformConfigAccessRecord,
} from "./platform-configs.js";

// CORS allowed origin tables
export {
	corsAllowedOrigins,
	type CorsAllowedOriginRecord,
	type NewCorsAllowedOriginRecord,
} from "./cors-allowed-origins.js";

// Projection feed tables (CQRS read model projection)
export {
	dispatchJobProjectionFeed,
	eventProjectionFeed,
	ProjectionFeedStatus,
	type ProjectionFeedStatusValue,
	type DispatchJobProjectionFeedRecord,
	type NewDispatchJobProjectionFeedRecord,
	type EventProjectionFeedRecord,
	type NewEventProjectionFeedRecord,
} from "./outbox.js";
