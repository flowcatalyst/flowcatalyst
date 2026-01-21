import type { MiddlewareHandler, Context } from 'hono';
import type { Logger } from '@flowcatalyst/logging';
import { createBasicAuthMiddleware, type BasicAuthConfig } from './basic-auth.js';
import { createOidcMiddleware, type OidcConfig } from './oidc-auth.js';

/**
 * Authentication mode
 */
export type AuthMode = 'NONE' | 'BASIC' | 'OIDC';

/**
 * Authentication configuration
 */
export interface AuthConfig {
	/** Whether authentication is enabled */
	enabled: boolean;
	/** Authentication mode */
	mode: AuthMode;
	/** BasicAuth configuration */
	basic?: BasicAuthConfig | undefined;
	/** OIDC configuration */
	oidc?: OidcConfig | undefined;
}

/**
 * Paths that are always public (no authentication required)
 */
const PUBLIC_PATHS = [
	'/health',
	'/monitoring/health',
	'/metrics',
];

/**
 * Check if a path is public
 */
function isPublicPath(path: string): boolean {
	return PUBLIC_PATHS.some((publicPath) => {
		if (publicPath.endsWith('*')) {
			return path.startsWith(publicPath.slice(0, -1));
		}
		return path === publicPath || path.startsWith(`${publicPath}/`);
	});
}

/**
 * No-op middleware that allows all requests
 */
const noAuthMiddleware: MiddlewareHandler = async (_c, next) => {
	await next();
};

/**
 * Create authentication middleware based on configuration
 */
export function createAuthMiddleware(
	config: AuthConfig,
	logger: Logger,
): MiddlewareHandler {
	const childLogger = logger.child({ component: 'Auth' });

	// If authentication is disabled, return no-op middleware
	if (!config.enabled || config.mode === 'NONE') {
		childLogger.info('Authentication disabled');
		return noAuthMiddleware;
	}

	// Create the appropriate middleware based on mode
	let authMiddleware: MiddlewareHandler;

	switch (config.mode) {
		case 'BASIC':
			if (!config.basic?.username || !config.basic?.password) {
				childLogger.error('BasicAuth enabled but credentials not configured');
				throw new Error('BasicAuth requires AUTH_BASIC_USERNAME and AUTH_BASIC_PASSWORD');
			}
			childLogger.info('BasicAuth enabled');
			authMiddleware = createBasicAuthMiddleware(config.basic, logger);
			break;

		case 'OIDC':
			if (!config.oidc?.issuerUrl) {
				childLogger.error('OIDC enabled but issuer URL not configured');
				throw new Error('OIDC requires OIDC_ISSUER_URL');
			}
			childLogger.info({ issuer: config.oidc.issuerUrl }, 'OIDC enabled');
			authMiddleware = createOidcMiddleware(config.oidc, logger);
			break;

		default:
			childLogger.warn({ mode: config.mode }, 'Unknown auth mode, disabling authentication');
			return noAuthMiddleware;
	}

	// Wrap with public path check
	return async (c: Context, next: () => Promise<void>) => {
		const path = c.req.path;

		// Skip authentication for public paths
		if (isPublicPath(path)) {
			await next();
			return;
		}

		// Apply authentication
		await authMiddleware(c, next);
	};
}

/**
 * User info set by authentication middleware
 */
export interface AuthUser {
	username?: string;
	sub?: string;
	email?: string;
	name?: string;
	roles?: string[];
	authMode: AuthMode;
}

/**
 * Get authenticated user from context
 */
export function getAuthUser(c: Context): AuthUser | undefined {
	return c.get('user') as AuthUser | undefined;
}

/**
 * Check if user has a specific role (for OIDC)
 */
export function hasRole(c: Context, role: string): boolean {
	const user = getAuthUser(c);
	return user?.roles?.includes(role) ?? false;
}
