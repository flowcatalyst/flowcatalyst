/**
 * Auth Plugin
 *
 * Fastify plugin supporting NONE, BASIC, and OIDC authentication modes.
 * Ported from the Hono security middleware.
 */

import type { FastifyPluginAsync, FastifyRequest, FastifyReply } from "fastify";
import fp from "fastify-plugin";
import type { Logger } from "@flowcatalyst/logging";
import * as jose from "jose";

// ── Types ──

export type AuthMode = "NONE" | "BASIC" | "OIDC";

export interface BasicAuthConfig {
	username: string;
	password: string;
}

export interface OidcConfig {
	issuerUrl: string;
	audience?: string | undefined;
	clientId?: string | undefined;
}

export interface AuthConfig {
	enabled: boolean;
	mode: AuthMode;
	basic?: BasicAuthConfig | undefined;
	oidc?: OidcConfig | undefined;
}

export interface AuthUser {
	username?: string | undefined;
	sub?: string | undefined;
	email?: string | undefined;
	name?: string | undefined;
	roles?: string[] | undefined;
	authMode: AuthMode;
}

export interface AuthPluginOptions {
	config: AuthConfig;
	logger: Logger;
}

declare module "fastify" {
	interface FastifyRequest {
		authUser: AuthUser | null;
	}
}

// ── Public Paths ──

const PUBLIC_PATHS = ["/health", "/monitoring/health", "/metrics"];

function isPublicPath(path: string): boolean {
	return PUBLIC_PATHS.some((publicPath) => {
		if (publicPath.endsWith("*")) {
			return path.startsWith(publicPath.slice(0, -1));
		}
		return path === publicPath || path.startsWith(`${publicPath}/`);
	});
}

// ── Basic Auth Helpers ──

function decodeBasicAuth(
	authHeader: string,
): { username: string; password: string } | null {
	if (!authHeader.startsWith("Basic ")) {
		return null;
	}
	const base64 = authHeader.slice(6);
	try {
		const decoded = atob(base64);
		const colonIndex = decoded.indexOf(":");
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

// ── OIDC / JWKS Helpers ──

type JwksGetter = ReturnType<typeof jose.createRemoteJWKSet>;

interface JwksCache {
	jwks: JwksGetter;
	expiresAt: number;
}

const jwksCache = new Map<string, JwksCache>();
const JWKS_CACHE_TTL_MS = 5 * 60 * 1000;

async function getJwks(issuerUrl: string): Promise<JwksGetter> {
	const cached = jwksCache.get(issuerUrl);
	const now = Date.now();
	if (cached && cached.expiresAt > now) {
		return cached.jwks;
	}

	const wellKnownUrl = issuerUrl.endsWith("/")
		? `${issuerUrl}.well-known/openid-configuration`
		: `${issuerUrl}/.well-known/openid-configuration`;

	const discoveryResponse = await fetch(wellKnownUrl);
	if (!discoveryResponse.ok) {
		throw new Error(
			`Failed to fetch OIDC discovery document: ${discoveryResponse.status}`,
		);
	}

	const discovery = (await discoveryResponse.json()) as { jwks_uri: string };
	if (!discovery.jwks_uri) {
		throw new Error("OIDC discovery document missing jwks_uri");
	}

	const jwks = jose.createRemoteJWKSet(new URL(discovery.jwks_uri));
	jwksCache.set(issuerUrl, { jwks, expiresAt: now + JWKS_CACHE_TTL_MS });
	return jwks;
}

// ── Auth Handlers ──

function handleBasicAuth(
	config: BasicAuthConfig,
	childLogger: Logger,
): (request: FastifyRequest, reply: FastifyReply) => Promise<void> {
	return async (request, reply) => {
		const authHeader = request.headers.authorization;

		if (!authHeader) {
			childLogger.debug("Missing Authorization header");
			reply
				.code(401)
				.header("WWW-Authenticate", 'Basic realm="FlowCatalyst Message Router"')
				.header("X-Auth-Mode", "BASIC")
				.send({ error: "Authentication required", mode: "BASIC" });
			return;
		}

		const credentials = decodeBasicAuth(authHeader);
		if (!credentials) {
			childLogger.warn("Invalid Authorization header format");
			reply.code(400).send({ error: "Invalid authorization format" });
			return;
		}

		if (
			credentials.username !== config.username ||
			credentials.password !== config.password
		) {
			childLogger.warn(
				{ username: credentials.username },
				"Invalid credentials",
			);
			reply
				.code(401)
				.header("WWW-Authenticate", 'Basic realm="FlowCatalyst Message Router"')
				.header("X-Auth-Mode", "BASIC")
				.send({ error: "Invalid credentials" });
			return;
		}

		childLogger.debug(
			{ username: credentials.username },
			"Authentication successful",
		);
		request.authUser = { username: credentials.username, authMode: "BASIC" };
	};
}

function handleOidcAuth(
	config: OidcConfig,
	childLogger: Logger,
): (request: FastifyRequest, reply: FastifyReply) => Promise<void> {
	// Pre-warm JWKS cache
	getJwks(config.issuerUrl).catch((err) => {
		childLogger.warn({ err }, "Failed to pre-warm JWKS cache");
	});

	return async (request, reply) => {
		const authHeader = request.headers.authorization;

		if (!authHeader) {
			childLogger.debug("Missing Authorization header");
			reply
				.code(401)
				.header(
					"WWW-Authenticate",
					'Bearer realm="FlowCatalyst Message Router", error="missing_token"',
				)
				.header("X-Auth-Mode", "OIDC")
				.send({ error: "Authentication required", mode: "OIDC" });
			return;
		}

		if (!authHeader.startsWith("Bearer ")) {
			childLogger.debug("Invalid Authorization header format");
			reply
				.code(400)
				.send({ error: "Invalid authorization format, expected Bearer token" });
			return;
		}

		const token = authHeader.slice(7);

		try {
			const jwks = await getJwks(config.issuerUrl);
			const verifyOptions: jose.JWTVerifyOptions = { issuer: config.issuerUrl };
			if (config.audience) {
				verifyOptions.audience = config.audience;
			}

			const { payload } = await jose.jwtVerify(token, jwks, verifyOptions);

			// Additional client ID validation if configured
			if (config.clientId) {
				const azp = payload["azp"] as string | undefined;
				const aud = payload.aud;
				const hasValidClientId =
					azp === config.clientId ||
					aud === config.clientId ||
					(Array.isArray(aud) && aud.includes(config.clientId));

				if (!hasValidClientId) {
					childLogger.warn(
						{ azp, aud, expectedClientId: config.clientId },
						"Token client ID mismatch",
					);
					reply
						.code(401)
						.header(
							"WWW-Authenticate",
							'Bearer realm="FlowCatalyst Message Router", error="invalid_token"',
						)
						.header("X-Auth-Mode", "OIDC")
						.send({ error: "Token not issued for this client" });
					return;
				}
			}

			const user: AuthUser = {
				sub: payload.sub,
				username:
					(payload["preferred_username"] as string) ||
					(payload["email"] as string) ||
					payload.sub,
				email: payload["email"] as string | undefined,
				name: payload["name"] as string | undefined,
				roles: (payload["realm_access"] as { roles?: string[] })?.roles || [],
				authMode: "OIDC",
			};

			childLogger.debug(
				{ username: user.username, sub: user.sub },
				"Authentication successful",
			);
			request.authUser = user;
		} catch (error) {
			if (error instanceof jose.errors.JWTExpired) {
				childLogger.debug("Token expired");
				reply
					.code(401)
					.header(
						"WWW-Authenticate",
						'Bearer realm="FlowCatalyst Message Router", error="invalid_token", error_description="Token expired"',
					)
					.header("X-Auth-Mode", "OIDC")
					.send({ error: "Token expired" });
				return;
			}

			if (error instanceof jose.errors.JWTClaimValidationFailed) {
				childLogger.warn({ err: error }, "Token claim validation failed");
				reply
					.code(401)
					.header(
						"WWW-Authenticate",
						'Bearer realm="FlowCatalyst Message Router", error="invalid_token"',
					)
					.header("X-Auth-Mode", "OIDC")
					.send({ error: "Token validation failed" });
				return;
			}

			childLogger.error({ err: error }, "Token verification failed");
			reply
				.code(401)
				.header(
					"WWW-Authenticate",
					'Bearer realm="FlowCatalyst Message Router", error="invalid_token"',
				)
				.header("X-Auth-Mode", "OIDC")
				.send({ error: "Invalid token" });
		}
	};
}

// ── Plugin ──

const authPluginAsync: FastifyPluginAsync<AuthPluginOptions> = async (
	fastify,
	opts,
) => {
	const { config, logger } = opts;
	const childLogger = logger.child({ component: "Auth" });

	// Decorate request with authUser
	fastify.decorateRequest("authUser", null);

	// If auth disabled, nothing to do — authUser stays null
	if (!config.enabled || config.mode === "NONE") {
		childLogger.info("Authentication disabled");
		return;
	}

	// Build the auth handler
	let authenticate: (
		request: FastifyRequest,
		reply: FastifyReply,
	) => Promise<void>;

	switch (config.mode) {
		case "BASIC": {
			if (!config.basic?.username || !config.basic?.password) {
				throw new Error(
					"BasicAuth requires AUTH_BASIC_USERNAME and AUTH_BASIC_PASSWORD",
				);
			}
			childLogger.info("BasicAuth enabled");
			authenticate = handleBasicAuth(config.basic, childLogger);
			break;
		}
		case "OIDC": {
			if (!config.oidc?.issuerUrl) {
				throw new Error("OIDC requires OIDC_ISSUER_URL");
			}
			childLogger.info({ issuer: config.oidc.issuerUrl }, "OIDC enabled");
			authenticate = handleOidcAuth(config.oidc, childLogger);
			break;
		}
		default: {
			childLogger.warn(
				{ mode: config.mode },
				"Unknown auth mode, disabling authentication",
			);
			return;
		}
	}

	// Apply auth as an onRequest hook, skipping public paths
	fastify.addHook("onRequest", async (request, reply) => {
		if (isPublicPath(request.url)) {
			return;
		}

		// Only protect /api/* and /monitoring/*
		if (
			!request.url.startsWith("/api/") &&
			!request.url.startsWith("/monitoring/")
		) {
			return;
		}

		await authenticate(request, reply);
	});
};

export const authPlugin = fp(authPluginAsync, {
	name: "@flowcatalyst/message-router-auth",
	fastify: "5.x",
});
