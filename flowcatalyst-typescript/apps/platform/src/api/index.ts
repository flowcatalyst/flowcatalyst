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
		AuditLogsRoutesDeps {}

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
		},
		{ prefix: '/api/admin' },
	);
}

export { type UsersRoutesDeps } from './admin/users.js';
export { type ClientsRoutesDeps } from './admin/clients.js';
export { type AnchorDomainsRoutesDeps } from './admin/anchor-domains.js';
export { type ApplicationsRoutesDeps } from './admin/applications.js';
export { type RolesRoutesDeps } from './admin/roles.js';
export { type AuthConfigsRoutesDeps } from './admin/auth-configs.js';
export { type OAuthClientsRoutesDeps } from './admin/oauth-clients.js';
export { type AuditLogsRoutesDeps } from './admin/audit-logs.js';
