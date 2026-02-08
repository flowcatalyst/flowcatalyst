/**
 * API Layer
 *
 * REST API endpoints for the platform service.
 */

import type { FastifyInstance } from 'fastify';

import { registerUsersRoutes, type UsersRoutesDeps } from './admin/users.js';
import { registerClientsRoutes, type ClientsRoutesDeps } from './admin/clients.js';
import { registerAnchorDomainsRoutes, type AnchorDomainsRoutesDeps } from './admin/anchor-domains.js';
import { registerApplicationsRoutes, type ApplicationsRoutesDeps } from './admin/applications.js';
import { registerRolesRoutes, type RolesRoutesDeps } from './admin/roles.js';
import { registerAuthConfigsRoutes, type AuthConfigsRoutesDeps } from './admin/auth-configs.js';
import { registerOAuthClientsRoutes, type OAuthClientsRoutesDeps } from './admin/oauth-clients.js';
import { registerAuditLogsRoutes, type AuditLogsRoutesDeps } from './admin/audit-logs.js';
import { registerEventTypesRoutes, type EventTypesRoutesDeps } from './admin/event-types.js';
import { registerDispatchPoolsRoutes, type DispatchPoolsRoutesDeps } from './admin/dispatch-pools.js';
import { registerSubscriptionsRoutes, type SubscriptionsRoutesDeps } from './admin/subscriptions.js';
import { registerEventsRoutes, type EventsRoutesDeps } from './admin/events.js';
import { registerDispatchJobsRoutes, type DispatchJobsRoutesDeps } from './admin/dispatch-jobs.js';
import { registerIdentityProvidersRoutes, type IdentityProvidersRoutesDeps } from './admin/identity-providers.js';
import { registerEmailDomainMappingsRoutes, type EmailDomainMappingsRoutesDeps } from './admin/email-domain-mappings.js';
import { registerServiceAccountsRoutes, type ServiceAccountsRoutesDeps } from './admin/service-accounts.js';
import { registerCorsRoutes, type CorsRoutesDeps } from './admin/cors.js';
import { registerConfigRoutes, type ConfigRoutesDeps } from './admin/config.js';
import { registerConfigAccessRoutes, type ConfigAccessRoutesDeps } from './admin/config-access.js';
import { registerEventTypesBffRoutes, type EventTypesBffDeps } from './bff/event-types.js';
import { registerRolesBffRoutes, type RolesBffDeps } from './bff/roles.js';
import { registerDebugEventsBffRoutes, type DebugEventsBffDeps } from './bff/debug-events.js';
import { registerDebugDispatchJobsBffRoutes, type DebugDispatchJobsBffDeps } from './bff/debug-dispatch-jobs.js';
import { registerSdkClientsRoutes, type SdkClientsDeps } from './sdk/clients.js';
import { registerSdkRolesRoutes, type SdkRolesDeps } from './sdk/roles.js';
import { registerSdkPrincipalsRoutes, type SdkPrincipalsDeps } from './sdk/principals.js';
import { registerMeRoutes, type MeRoutesDeps } from './me.js';
import { registerPublicConfigRoutes, type PublicConfigRoutesDeps } from './public/config.js';

/**
 * Dependencies for admin routes.
 */
export interface AdminRoutesDeps
	extends UsersRoutesDeps,
		ClientsRoutesDeps,
		AnchorDomainsRoutesDeps,
		ApplicationsRoutesDeps,
		RolesRoutesDeps,
		AuthConfigsRoutesDeps,
		OAuthClientsRoutesDeps,
		AuditLogsRoutesDeps,
		EventTypesRoutesDeps,
		DispatchPoolsRoutesDeps,
		SubscriptionsRoutesDeps,
		EventsRoutesDeps,
		DispatchJobsRoutesDeps,
		IdentityProvidersRoutesDeps,
		EmailDomainMappingsRoutesDeps,
		ServiceAccountsRoutesDeps,
		CorsRoutesDeps,
		ConfigRoutesDeps,
		ConfigAccessRoutesDeps {}

/**
 * Register all admin API routes.
 */
export async function registerAdminRoutes(fastify: FastifyInstance, deps: AdminRoutesDeps): Promise<void> {
	await fastify.register(
		async (adminRouter) => {
			await registerUsersRoutes(adminRouter, deps);
			await registerClientsRoutes(adminRouter, deps);
			await registerAnchorDomainsRoutes(adminRouter, deps);
			await registerApplicationsRoutes(adminRouter, deps);
			await registerRolesRoutes(adminRouter, deps);
			await registerAuthConfigsRoutes(adminRouter, deps);
			await registerOAuthClientsRoutes(adminRouter, deps);
			await registerAuditLogsRoutes(adminRouter, deps);
			await registerEventTypesRoutes(adminRouter, deps);
			await registerDispatchPoolsRoutes(adminRouter, deps);
			await registerSubscriptionsRoutes(adminRouter, deps);
			await registerEventsRoutes(adminRouter, deps);
			await registerDispatchJobsRoutes(adminRouter, deps);
			await registerIdentityProvidersRoutes(adminRouter, deps);
			await registerEmailDomainMappingsRoutes(adminRouter, deps);
			await registerServiceAccountsRoutes(adminRouter, deps);
			await registerCorsRoutes(adminRouter, deps);
			await registerConfigRoutes(adminRouter, deps);
			await registerConfigAccessRoutes(adminRouter, deps);
		},
		{ prefix: '/api/admin' },
	);
}

/**
 * Dependencies for BFF routes.
 */
export interface BffRoutesDeps extends EventTypesBffDeps, RolesBffDeps {}

/**
 * Register all BFF routes (frontend-facing).
 */
export async function registerBffRoutes(fastify: FastifyInstance, deps: BffRoutesDeps): Promise<void> {
	await fastify.register(
		async (bffRouter) => {
			await registerEventTypesBffRoutes(bffRouter, deps);
			await registerRolesBffRoutes(bffRouter, deps);
		},
		{ prefix: '/bff' },
	);
}

/**
 * Dependencies for debug BFF routes.
 */
export interface DebugBffRoutesDeps extends DebugEventsBffDeps, DebugDispatchJobsBffDeps {}

/**
 * Register debug BFF routes (admin/debug access).
 */
export async function registerDebugBffRoutes(fastify: FastifyInstance, deps: DebugBffRoutesDeps): Promise<void> {
	await fastify.register(
		async (debugRouter) => {
			await registerDebugEventsBffRoutes(debugRouter, deps);
			await registerDebugDispatchJobsBffRoutes(debugRouter, deps);
		},
		{ prefix: '/api/bff/debug' },
	);
}

/**
 * Dependencies for SDK routes.
 */
export interface SdkRoutesDeps extends SdkClientsDeps, SdkRolesDeps, SdkPrincipalsDeps {}

/**
 * Register all SDK routes (external integrations).
 */
export async function registerSdkRoutes(fastify: FastifyInstance, deps: SdkRoutesDeps): Promise<void> {
	await fastify.register(
		async (sdkRouter) => {
			await registerSdkClientsRoutes(sdkRouter, deps);
			await registerSdkRolesRoutes(sdkRouter, deps);
			await registerSdkPrincipalsRoutes(sdkRouter, deps);
		},
		{ prefix: '/api/sdk' },
	);
}

/**
 * Register user-facing /api/me routes.
 */
export async function registerMeApiRoutes(fastify: FastifyInstance, deps: MeRoutesDeps): Promise<void> {
	await fastify.register(
		async (meRouter) => {
			await registerMeRoutes(meRouter, deps);
		},
		{ prefix: '/api/me' },
	);
}

/**
 * Register public routes (no auth required).
 */
export async function registerPublicApiRoutes(fastify: FastifyInstance, deps: PublicConfigRoutesDeps): Promise<void> {
	await fastify.register(
		async (publicRouter) => {
			await registerPublicConfigRoutes(publicRouter, deps);
		},
		{ prefix: '/api/public' },
	);
}

/**
 * Register platform config routes (no auth required).
 */
export async function registerPlatformConfigApiRoutes(fastify: FastifyInstance, deps: PublicConfigRoutesDeps): Promise<void> {
	await fastify.register(
		async (configRouter) => {
			await registerPublicConfigRoutes(configRouter, deps);
		},
		{ prefix: '/api/config' },
	);
}

export { type MeRoutesDeps } from './me.js';
export { type PublicConfigRoutesDeps } from './public/config.js';
export { type UsersRoutesDeps } from './admin/users.js';
export { type ClientsRoutesDeps } from './admin/clients.js';
export { type AnchorDomainsRoutesDeps } from './admin/anchor-domains.js';
export { type ApplicationsRoutesDeps } from './admin/applications.js';
export { type RolesRoutesDeps } from './admin/roles.js';
export { type AuthConfigsRoutesDeps } from './admin/auth-configs.js';
export { type OAuthClientsRoutesDeps } from './admin/oauth-clients.js';
export { type AuditLogsRoutesDeps } from './admin/audit-logs.js';
export { type EventTypesRoutesDeps } from './admin/event-types.js';
export { type DispatchPoolsRoutesDeps } from './admin/dispatch-pools.js';
export { type SubscriptionsRoutesDeps } from './admin/subscriptions.js';
export { type EventsRoutesDeps } from './admin/events.js';
export { type DispatchJobsRoutesDeps } from './admin/dispatch-jobs.js';
export { type IdentityProvidersRoutesDeps } from './admin/identity-providers.js';
export { type EmailDomainMappingsRoutesDeps } from './admin/email-domain-mappings.js';
export { type ServiceAccountsRoutesDeps } from './admin/service-accounts.js';
export { type CorsRoutesDeps } from './admin/cors.js';
export { type ConfigRoutesDeps } from './admin/config.js';
export { type ConfigAccessRoutesDeps } from './admin/config-access.js';
export { type EventTypesBffDeps } from './bff/event-types.js';
export { type RolesBffDeps } from './bff/roles.js';
export { type DebugEventsBffDeps } from './bff/debug-events.js';
export { type DebugDispatchJobsBffDeps } from './bff/debug-dispatch-jobs.js';
export { type SdkClientsDeps } from './sdk/clients.js';
export { type SdkRolesDeps } from './sdk/roles.js';
export { type SdkPrincipalsDeps } from './sdk/principals.js';
