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

import type { FastifyInstance } from "fastify";
import type { PrincipalRepository } from "../persistence/repositories/principal-repository.js";
import type { EmailDomainMappingRepository } from "../persistence/repositories/email-domain-mapping-repository.js";
import type { IdentityProviderRepository } from "../persistence/repositories/identity-provider-repository.js";
import type { ClientRepository } from "../persistence/repositories/client-repository.js";
import type { PasswordService } from "@flowcatalyst/platform-crypto";
import { getMappingAccessibleClientIds } from "../../domain/email-domain-mapping/email-domain-mapping.js";
import { getEffectiveIssuerPattern } from "../../domain/identity-provider/identity-provider.js";

/**
 * Session cookie configuration.
 */
export interface SessionCookieConfig {
	/** Cookie name (default: fc_session) */
	name: string;
	/** Whether to set Secure flag (default: true in production) */
	secure: boolean;
	/** SameSite attribute (default: lax) */
	sameSite: "strict" | "lax" | "none";
	/** Max age in seconds (default: 86400 = 24 hours) */
	maxAge: number;
}

/**
 * Dependencies for auth routes.
 */
export interface AuthRoutesDeps {
	principalRepository: PrincipalRepository;
	emailDomainMappingRepository: EmailDomainMappingRepository;
	identityProviderRepository: IdentityProviderRepository;
	clientRepository: ClientRepository;
	passwordService: PasswordService;
	issueSessionToken: (
		principalId: string,
		email: string,
		roles: string[],
		clients: string[],
	) => Promise<string>;
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
 * Login response (used for POST /auth/login).
 */
interface LoginResponse {
	principalId: string;
	name: string;
	email: string;
	roles: string[];
	clientId: string | null;
}

/**
 * Session user response (used for GET /auth/me).
 */
interface SessionUserResponse {
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
	fastify.post<{ Body: LoginRequest }>(
		"/auth/login",
		async (request, reply) => {
			const { email, password } = request.body ?? {};

			if (!email || !password) {
				return reply
					.status(400)
					.send({ error: "Email and password are required" });
			}

			// Find user by email
			const principal = await principalRepository.findByEmail(
				email.toLowerCase(),
			);

			if (!principal) {
				fastify.log.info({ email }, "Login failed: user not found");
				return reply.status(401).send({ error: "Invalid email or password" });
			}

			// Verify it's a user (not service account)
			if (principal.type !== "USER") {
				fastify.log.warn({ email }, "Login attempt for non-user principal");
				return reply.status(401).send({ error: "Invalid email or password" });
			}

			// Verify user is active
			if (!principal.active) {
				fastify.log.info({ email }, "Login failed: user is inactive");
				return reply.status(401).send({ error: "Account is disabled" });
			}

			// Verify password
			if (!principal.userIdentity?.passwordHash) {
				fastify.log.warn({ email }, "Login failed: no password set");
				return reply.status(401).send({ error: "Invalid email or password" });
			}

			const isValid = await passwordService.verify(
				password,
				principal.userIdentity.passwordHash,
			);
			if (!isValid) {
				fastify.log.info({ email }, "Login failed: invalid password");
				return reply.status(401).send({ error: "Invalid email or password" });
			}

			// Load roles
			const roles = principal.roles.map((r) => r.roleName);

			// Determine accessible clients (using email domain mapping for richer client access)
			const clients = await determineAccessibleClients(principal, deps);

			// Issue session token
			const token = await issueSessionToken(
				principal.id,
				principal.userIdentity.email,
				roles,
				clients,
			);

			// Set session cookie
			reply.setCookie(cookieConfig.name, token, {
				path: "/",
				maxAge: cookieConfig.maxAge,
				httpOnly: true,
				secure: cookieConfig.secure,
				sameSite: cookieConfig.sameSite,
			});

			fastify.log.info(
				{ email, principalId: principal.id },
				"Login successful",
			);

			const response: LoginResponse = {
				principalId: principal.id,
				name: principal.name,
				email: principal.userIdentity.email,
				roles,
				clientId: principal.clientId,
			};

			return reply.send(response);
		},
	);

	/**
	 * POST /auth/logout
	 * Logout and clear session cookie.
	 */
	fastify.post("/auth/logout", async (_request, reply) => {
		// Clear session cookie by setting expired cookie
		reply.setCookie(cookieConfig.name, "", {
			path: "/",
			maxAge: 0,
			httpOnly: true,
			secure: cookieConfig.secure,
			sameSite: cookieConfig.sameSite,
		});

		return reply.send({ message: "Logged out successfully" });
	});

	/**
	 * GET /auth/me
	 * Get current authenticated user from session cookie.
	 */
	fastify.get("/auth/me", async (request, reply) => {
		const sessionToken = request.cookies[cookieConfig.name];

		if (!sessionToken) {
			return reply.status(401).send({ error: "Not authenticated" });
		}

		// Validate session token
		const principalId = await validateSessionToken(sessionToken);
		if (!principalId) {
			return reply.status(401).send({ error: "Invalid session" });
		}

		// Load principal
		const principal = await principalRepository.findById(principalId);
		if (!principal) {
			return reply.status(401).send({ error: "User not found" });
		}

		if (!principal.active) {
			return reply.status(401).send({ error: "Account is disabled" });
		}

		const roles = principal.roles.map((r) => r.roleName);

		const response: SessionUserResponse = {
			principalId: principal.id,
			name: principal.name,
			email: principal.userIdentity?.email ?? "",
			roles,
			clientId: principal.clientId,
		};

		return reply.send(response);
	});

	/**
	 * POST /auth/check-domain
	 * Determine authentication method for an email domain.
	 * Returns 'internal' for password auth, 'external' with IDP URL for SSO.
	 */
	fastify.post<{ Body: { email?: string } }>(
		"/auth/check-domain",
		async (request, reply) => {
			const email = request.body?.email;

			if (!email || typeof email !== "string" || email.trim() === "") {
				return reply.status(400).send({ error: "Email is required" });
			}

			const normalised = email.toLowerCase().trim();
			const atIndex = normalised.indexOf("@");
			if (atIndex < 0) {
				return reply.status(400).send({ error: "Invalid email format" });
			}

			const domain = normalised.substring(atIndex + 1);

			// Look up email domain mapping -> identity provider
			const mapping =
				await deps.emailDomainMappingRepository.findByEmailDomain(domain);
			if (!mapping) {
				return reply.send({
					authMethod: "internal",
					loginUrl: null,
					idpIssuer: null,
				});
			}

			const idp = await deps.identityProviderRepository.findById(
				mapping.identityProviderId,
			);
			if (!idp) {
				return reply.send({
					authMethod: "internal",
					loginUrl: null,
					idpIssuer: null,
				});
			}

			// Check if OIDC is configured (supports multi-tenant IDPs)
			const isOidcConfigured =
				idp.type === "OIDC" &&
				(idp.oidcIssuerUrl !== null ||
					(idp.oidcMultiTenant && getEffectiveIssuerPattern(idp) !== null));

			if (isOidcConfigured) {
				const loginUrl = `/auth/oidc/login?domain=${domain}`;
				const issuerInfo = idp.oidcIssuerUrl ?? getEffectiveIssuerPattern(idp);
				return reply.send({
					authMethod: "external",
					loginUrl,
					idpIssuer: issuerInfo,
				});
			}

			return reply.send({
				authMethod: "internal",
				loginUrl: null,
				idpIssuer: null,
			});
		},
	);

	fastify.log.info(
		"Auth routes registered (/auth/login, /auth/logout, /auth/me, /auth/check-domain)",
	);
}

/**
 * Determine which clients the user can access based on their scope and email domain mapping.
 * Uses EmailDomainMapping for richer client access (additionalClientIds, grantedClientIds).
 */
async function determineAccessibleClients(
	principal: {
		scope: string | null;
		clientId: string | null;
		roles: readonly { roleName: string }[];
		userIdentity: { emailDomain: string } | null;
	},
	deps: AuthRoutesDeps,
): Promise<string[]> {
	// Check explicit scope
	if (principal.scope) {
		switch (principal.scope) {
			case "ANCHOR":
				return ["*"];
			case "CLIENT":
			case "PARTNER": {
				// Try to use EmailDomainMapping for richer client access
				if (principal.userIdentity?.emailDomain) {
					const mapping =
						await deps.emailDomainMappingRepository.findByEmailDomain(
							principal.userIdentity.emailDomain,
						);
					if (mapping) {
						const clientIds = getMappingAccessibleClientIds(mapping);
						return formatClientEntries(clientIds, deps.clientRepository);
					}
				}
				// Fallback to just the home client
				if (principal.clientId) {
					return [principal.clientId];
				}
				return [];
			}
		}
	}

	// Fallback: check roles for platform admins
	const hasAdminRole = principal.roles.some(
		(r) =>
			r.roleName.includes("platform:admin") ||
			r.roleName.includes("super-admin"),
	);
	if (hasAdminRole) {
		return ["*"];
	}

	// User is bound to a specific client
	if (principal.clientId) {
		return [principal.clientId];
	}

	return [];
}

/**
 * Format client IDs as "id:identifier" entries for the clients claim.
 */
async function formatClientEntries(
	clientIds: string[],
	clientRepository: ClientRepository,
): Promise<string[]> {
	if (clientIds.length === 0) return [];

	const entries: string[] = [];
	for (const id of clientIds) {
		const client = await clientRepository.findById(id);
		if (client && "identifier" in client && client.identifier) {
			entries.push(`${id}:${client.identifier}`);
		} else {
			entries.push(id);
		}
	}
	return entries;
}
