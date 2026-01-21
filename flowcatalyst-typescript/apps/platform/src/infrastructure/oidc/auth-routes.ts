/**
 * Authentication Routes for oidc-provider
 *
 * Implements the /auth/* endpoints to match the Java API:
 * - POST /auth/login - Login with email/password
 * - POST /auth/logout - Logout and clear session
 * - GET /auth/me - Get current authenticated user
 *
 * These routes work alongside oidc-provider to provide a complete
 * authentication solution that matches the Java platform API.
 */

import type { FastifyInstance, FastifyRequest, FastifyReply } from 'fastify';
import type { PrincipalRepository } from '../persistence/repositories/principal-repository.js';
import type { PasswordService } from '@flowcatalyst/platform-crypto';

/**
 * Session cookie configuration.
 */
export interface SessionCookieConfig {
	/** Cookie name (default: fc_session) */
	name: string;
	/** Whether to set Secure flag (default: true in production) */
	secure: boolean;
	/** SameSite attribute (default: lax) */
	sameSite: 'strict' | 'lax' | 'none';
	/** Max age in seconds (default: 86400 = 24 hours) */
	maxAge: number;
}

/**
 * Dependencies for auth routes.
 */
export interface AuthRoutesDeps {
	principalRepository: PrincipalRepository;
	passwordService: PasswordService;
	issueSessionToken: (principalId: string, email: string, roles: string[], clients: string[]) => string;
	validateSessionToken: (token: string) => Promise<string | null>;
	cookieConfig: SessionCookieConfig;
}

/**
 * Login request body.
 */
interface LoginRequest {
	email: string;
	password: string;
}

/**
 * Login response.
 */
interface LoginResponse {
	principalId: string;
	name: string;
	email: string;
	roles: string[];
	clientId: string | null;
}

/**
 * Register authentication routes on Fastify.
 */
export async function registerAuthRoutes(
	fastify: FastifyInstance,
	deps: AuthRoutesDeps,
): Promise<void> {
	const {
		principalRepository,
		passwordService,
		issueSessionToken,
		validateSessionToken,
		cookieConfig,
	} = deps;

	/**
	 * POST /auth/login
	 * Login with email and password, returns session cookie.
	 */
	fastify.post<{ Body: LoginRequest }>('/auth/login', async (request, reply) => {
		const { email, password } = request.body ?? {};

		if (!email || !password) {
			return reply.status(400).send({ error: 'Email and password are required' });
		}

		// Find user by email
		const principal = await principalRepository.findByEmail(email.toLowerCase());

		if (!principal) {
			fastify.log.info({ email }, 'Login failed: user not found');
			return reply.status(401).send({ error: 'Invalid email or password' });
		}

		// Verify it's a user (not service account)
		if (principal.type !== 'USER') {
			fastify.log.warn({ email }, 'Login attempt for non-user principal');
			return reply.status(401).send({ error: 'Invalid email or password' });
		}

		// Verify user is active
		if (!principal.active) {
			fastify.log.info({ email }, 'Login failed: user is inactive');
			return reply.status(401).send({ error: 'Account is disabled' });
		}

		// Verify password
		if (!principal.userIdentity?.passwordHash) {
			fastify.log.warn({ email }, 'Login failed: no password set');
			return reply.status(401).send({ error: 'Invalid email or password' });
		}

		const isValid = await passwordService.verify(password, principal.userIdentity.passwordHash);
		if (!isValid) {
			fastify.log.info({ email }, 'Login failed: invalid password');
			return reply.status(401).send({ error: 'Invalid email or password' });
		}

		// Load roles
		const roles = principal.roles.map((r) => r.roleName);

		// Determine accessible clients
		const clients = determineAccessibleClients(principal);

		// Issue session token
		const token = issueSessionToken(
			principal.id,
			principal.userIdentity.email,
			roles,
			clients,
		);

		// Set session cookie
		reply.setCookie(cookieConfig.name, token, {
			path: '/',
			maxAge: cookieConfig.maxAge,
			httpOnly: true,
			secure: cookieConfig.secure,
			sameSite: cookieConfig.sameSite,
		});

		fastify.log.info({ email, principalId: principal.id }, 'Login successful');

		const response: LoginResponse = {
			principalId: principal.id,
			name: principal.name,
			email: principal.userIdentity.email,
			roles,
			clientId: principal.clientId,
		};

		return reply.send(response);
	});

	/**
	 * POST /auth/logout
	 * Logout and clear session cookie.
	 */
	fastify.post('/auth/logout', async (request, reply) => {
		// Clear session cookie by setting expired cookie
		reply.setCookie(cookieConfig.name, '', {
			path: '/',
			maxAge: 0,
			httpOnly: true,
			secure: cookieConfig.secure,
			sameSite: cookieConfig.sameSite,
		});

		return reply.send({ message: 'Logged out successfully' });
	});

	/**
	 * GET /auth/me
	 * Get current authenticated user from session cookie.
	 */
	fastify.get('/auth/me', async (request, reply) => {
		const sessionToken = request.cookies[cookieConfig.name];

		if (!sessionToken) {
			return reply.status(401).send({ error: 'Not authenticated' });
		}

		// Validate session token
		const principalId = await validateSessionToken(sessionToken);
		if (!principalId) {
			return reply.status(401).send({ error: 'Invalid session' });
		}

		// Load principal
		const principal = await principalRepository.findById(principalId);
		if (!principal) {
			return reply.status(401).send({ error: 'User not found' });
		}

		if (!principal.active) {
			return reply.status(401).send({ error: 'Account is disabled' });
		}

		const roles = principal.roles.map((r) => r.roleName);

		const response: LoginResponse = {
			principalId: principal.id,
			name: principal.name,
			email: principal.userIdentity?.email ?? '',
			roles,
			clientId: principal.clientId,
		};

		return reply.send(response);
	});

	fastify.log.info('Auth routes registered (/auth/login, /auth/logout, /auth/me)');
}

/**
 * Determine which clients the user can access based on their scope.
 */
function determineAccessibleClients(principal: {
	scope: string | null;
	clientId: string | null;
	roles: readonly { roleName: string }[];
}): string[] {
	// Check explicit scope
	if (principal.scope) {
		switch (principal.scope) {
			case 'ANCHOR':
				return ['*'];
			case 'CLIENT':
			case 'PARTNER':
				if (principal.clientId) {
					return [principal.clientId];
				}
				return [];
		}
	}

	// Fallback: check roles for platform admins
	const hasAdminRole = principal.roles.some(
		(r) => r.roleName.includes('platform:admin') || r.roleName.includes('super-admin'),
	);
	if (hasAdminRole) {
		return ['*'];
	}

	// User is bound to a specific client
	if (principal.clientId) {
		return [principal.clientId];
	}

	return [];
}
