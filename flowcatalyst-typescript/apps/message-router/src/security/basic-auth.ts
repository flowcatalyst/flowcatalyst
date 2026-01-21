import type { Context, MiddlewareHandler } from 'hono';
import type { Logger } from '@flowcatalyst/logging';

/**
 * BasicAuth configuration
 */
export interface BasicAuthConfig {
	username: string;
	password: string;
}

/**
 * Decode Basic Auth header
 * @param authHeader - Authorization header value (e.g., "Basic base64encoded")
 * @returns Decoded username and password, or null if invalid
 */
function decodeBasicAuth(authHeader: string): { username: string; password: string } | null {
	if (!authHeader.startsWith('Basic ')) {
		return null;
	}

	const base64 = authHeader.slice(6);
	try {
		const decoded = atob(base64);
		const colonIndex = decoded.indexOf(':');
		if (colonIndex === -1) {
			return null;
		}

		return {
			username: decoded.slice(0, colonIndex),
			password: decoded.slice(colonIndex + 1),
		};
	} catch {
		return null;
	}
}

/**
 * Create BasicAuth middleware for Hono
 */
export function createBasicAuthMiddleware(
	config: BasicAuthConfig,
	logger: Logger,
): MiddlewareHandler {
	const childLogger = logger.child({ component: 'BasicAuth' });

	return async (c: Context, next: () => Promise<void>) => {
		const authHeader = c.req.header('Authorization');

		if (!authHeader) {
			childLogger.debug('Missing Authorization header');
			return c.json(
				{ error: 'Authentication required', mode: 'BASIC' },
				401,
				{
					'WWW-Authenticate': 'Basic realm="FlowCatalyst Message Router"',
					'X-Auth-Mode': 'BASIC',
				},
			);
		}

		const credentials = decodeBasicAuth(authHeader);
		if (!credentials) {
			childLogger.warn('Invalid Authorization header format');
			return c.json(
				{ error: 'Invalid authorization format' },
				400,
			);
		}

		// Validate credentials
		if (credentials.username !== config.username || credentials.password !== config.password) {
			childLogger.warn({ username: credentials.username }, 'Invalid credentials');
			return c.json(
				{ error: 'Invalid credentials' },
				401,
				{
					'WWW-Authenticate': 'Basic realm="FlowCatalyst Message Router"',
					'X-Auth-Mode': 'BASIC',
				},
			);
		}

		childLogger.debug({ username: credentials.username }, 'Authentication successful');

		// Set user info in context
		c.set('user', { username: credentials.username, authMode: 'BASIC' });

		await next();
	};
}
