/**
 * OIDC Infrastructure Module
 *
 * Exports all OIDC-related infrastructure components.
 */

// Drizzle adapter for oidc-provider
export {
	createDrizzleAdapterFactory,
	cleanupExpiredPayloads,
	getPayloadStats,
} from './drizzle-adapter.js';

// Account adapter (Principal integration)
export {
	createFindAccount,
	createAccountAdapter,
	type AccountAdapter,
} from './account-adapter.js';

// Client adapter (OAuthClient integration)
export {
	createClientLoader,
	createClientValidator,
	getClientAllowedOrigins,
	isOriginAllowedForAnyClient,
} from './client-adapter.js';

// OIDC provider configuration and setup
export {
	createOidcProvider,
	getProviderCallback,
	type OidcProviderConfig,
	type OidcProvider,
} from './provider.js';

// Fastify integration
export {
	mountOidcProvider,
	registerWellKnownRedirects,
	registerOAuthCompatibilityRoutes,
} from './fastify-adapter.js';

// Auth routes (login, logout, me)
export {
	registerAuthRoutes,
	type AuthRoutesDeps,
	type SessionCookieConfig,
} from './auth-routes.js';
