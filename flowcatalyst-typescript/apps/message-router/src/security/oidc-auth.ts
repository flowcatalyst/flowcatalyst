import type { Context, MiddlewareHandler } from 'hono';
import type { Logger } from '@flowcatalyst/logging';
import * as jose from 'jose';

/**
 * OIDC configuration
 */
export interface OidcConfig {
	/** OIDC issuer URL (e.g., https://keycloak.example.com/realms/myrealm) */
	issuerUrl: string;
	/** Expected audience (usually client ID) */
	audience?: string | undefined;
	/** Client ID for additional validation */
	clientId?: string | undefined;
}

/** JWKS getter function type */
type JwksGetter = ReturnType<typeof jose.createRemoteJWKSet>;

/**
 * JWKS cache entry
 */
interface JwksCache {
	jwks: JwksGetter;
	expiresAt: number;
}

// Global JWKS cache (5 minute TTL)
const jwksCache = new Map<string, JwksCache>();
const JWKS_CACHE_TTL_MS = 5 * 60 * 1000;

/**
 * Get or create JWKS for issuer
 */
async function getJwks(issuerUrl: string): Promise<JwksGetter> {
	const cached = jwksCache.get(issuerUrl);
	const now = Date.now();

	if (cached && cached.expiresAt > now) {
		return cached.jwks;
	}

	// Discover OIDC configuration
	const wellKnownUrl = issuerUrl.endsWith('/')
		? `${issuerUrl}.well-known/openid-configuration`
		: `${issuerUrl}/.well-known/openid-configuration`;

	const discoveryResponse = await fetch(wellKnownUrl);
	if (!discoveryResponse.ok) {
		throw new Error(`Failed to fetch OIDC discovery document: ${discoveryResponse.status}`);
	}

	const discovery = (await discoveryResponse.json()) as { jwks_uri: string };
	if (!discovery.jwks_uri) {
		throw new Error('OIDC discovery document missing jwks_uri');
	}

	// Create JWKS from remote endpoint
	const jwks = jose.createRemoteJWKSet(new URL(discovery.jwks_uri));

	// Cache it
	jwksCache.set(issuerUrl, {
		jwks,
		expiresAt: now + JWKS_CACHE_TTL_MS,
	});

	return jwks;
}

/**
 * Extract Bearer token from Authorization header
 */
function extractBearerToken(authHeader: string): string | null {
	if (!authHeader.startsWith('Bearer ')) {
		return null;
	}
	return authHeader.slice(7);
}

/**
 * Create OIDC/JWT middleware for Hono
 */
export function createOidcMiddleware(
	config: OidcConfig,
	logger: Logger,
): MiddlewareHandler {
	const childLogger = logger.child({ component: 'OidcAuth' });

	// Pre-warm JWKS cache
	getJwks(config.issuerUrl).catch((err) => {
		childLogger.warn({ err }, 'Failed to pre-warm JWKS cache');
	});

	return async (c: Context, next: () => Promise<void>) => {
		const authHeader = c.req.header('Authorization');

		if (!authHeader) {
			childLogger.debug('Missing Authorization header');
			return c.json(
				{ error: 'Authentication required', mode: 'OIDC' },
				401,
				{
					'WWW-Authenticate': `Bearer realm="FlowCatalyst Message Router", error="missing_token"`,
					'X-Auth-Mode': 'OIDC',
				},
			);
		}

		const token = extractBearerToken(authHeader);
		if (!token) {
			childLogger.debug('Invalid Authorization header format');
			return c.json(
				{ error: 'Invalid authorization format, expected Bearer token' },
				400,
			);
		}

		try {
			// Get JWKS for validation
			const jwks = await getJwks(config.issuerUrl);

			// Verify token
			const verifyOptions: jose.JWTVerifyOptions = {
				issuer: config.issuerUrl,
			};

			if (config.audience) {
				verifyOptions.audience = config.audience;
			}

			const { payload } = await jose.jwtVerify(token, jwks, verifyOptions);

			// Additional client ID validation if configured
			if (config.clientId) {
				const azp = payload['azp'] as string | undefined;
				const aud = payload.aud;
				const hasValidClientId =
					azp === config.clientId ||
					aud === config.clientId ||
					(Array.isArray(aud) && aud.includes(config.clientId));

				if (!hasValidClientId) {
					childLogger.warn(
						{ azp, aud, expectedClientId: config.clientId },
						'Token client ID mismatch',
					);
					return c.json(
						{ error: 'Token not issued for this client' },
						401,
						{
							'WWW-Authenticate': `Bearer realm="FlowCatalyst Message Router", error="invalid_token"`,
							'X-Auth-Mode': 'OIDC',
						},
					);
				}
			}

			// Extract user info from token
			const user = {
				sub: payload.sub,
				username:
					(payload['preferred_username'] as string) || (payload['email'] as string) || payload.sub,
				email: payload['email'] as string | undefined,
				name: payload['name'] as string | undefined,
				roles: (payload['realm_access'] as { roles?: string[] })?.roles || [],
				authMode: 'OIDC' as const,
			};

			childLogger.debug({ username: user.username, sub: user.sub }, 'Authentication successful');

			// Set user info in context
			c.set('user', user);

			await next();
		} catch (error) {
			if (error instanceof jose.errors.JWTExpired) {
				childLogger.debug('Token expired');
				return c.json(
					{ error: 'Token expired' },
					401,
					{
						'WWW-Authenticate': `Bearer realm="FlowCatalyst Message Router", error="invalid_token", error_description="Token expired"`,
						'X-Auth-Mode': 'OIDC',
					},
				);
			}

			if (error instanceof jose.errors.JWTClaimValidationFailed) {
				childLogger.warn({ err: error }, 'Token claim validation failed');
				return c.json(
					{ error: 'Token validation failed' },
					401,
					{
						'WWW-Authenticate': `Bearer realm="FlowCatalyst Message Router", error="invalid_token"`,
						'X-Auth-Mode': 'OIDC',
					},
				);
			}

			childLogger.error({ err: error }, 'Token verification failed');
			return c.json(
				{ error: 'Invalid token' },
				401,
				{
					'WWW-Authenticate': `Bearer realm="FlowCatalyst Message Router", error="invalid_token"`,
					'X-Auth-Mode': 'OIDC',
				},
			);
		}
	};
}
