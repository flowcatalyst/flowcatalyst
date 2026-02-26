/**
 * OIDC Interaction Routes
 *
 * Custom interaction handlers that bridge oidc-provider's interaction model
 * with FlowCatalyst's fc_session-based authentication.
 *
 * Registered BEFORE the oidc-provider wildcard mount so Fastify's parametric
 * routes take priority.
 */

import type { FastifyInstance, FastifyRequest, FastifyReply } from "fastify";
import type Provider from "oidc-provider";
import type { PrincipalRepository } from "../persistence/repositories/principal-repository.js";
import type { OAuthClientRepository } from "../persistence/repositories/oauth-client-repository.js";

export interface InteractionRoutesDeps {
	provider: Provider;
	validateSessionToken: (token: string) => Promise<string | null>;
	principalRepository: PrincipalRepository;
	oauthClientRepository: OAuthClientRepository;
	cookieName: string;
	loginPageUrl: string;
}

export async function registerInteractionRoutes(
	fastify: FastifyInstance,
	deps: InteractionRoutesDeps,
): Promise<void> {
	const {
		provider,
		validateSessionToken,
		principalRepository,
		oauthClientRepository,
		cookieName,
		loginPageUrl,
	} = deps;

	/**
	 * Complete the interaction by validating the session, creating a grant,
	 * and finishing the interaction with oidc-provider.
	 *
	 * Returns true if the interaction was completed, false if the user needs to log in.
	 */
	async function tryCompleteInteraction(
		request: FastifyRequest,
		reply: FastifyReply,
		uid: string,
	): Promise<boolean> {
		const req = request.raw;
		const res = reply.raw;

		// Strip /oidc prefix — provider expects paths relative to its mount
		const originalUrl = req.url;
		req.url = `/interaction/${uid}`;

		try {
			// Load interaction details
			const interactionDetails = await provider.interactionDetails(req, res);
			const { prompt, params } = interactionDetails;

			if (prompt.name !== "login" && prompt.name !== "consent") {
				fastify.log.warn(
					{ prompt: prompt.name, uid },
					"Unexpected interaction prompt",
				);
			}

			// Check fc_session cookie
			const sessionToken = request.cookies[cookieName];
			if (!sessionToken) {
				return false;
			}

			const principalId = await validateSessionToken(sessionToken);
			if (!principalId) {
				return false;
			}

			// Verify principal exists and is active
			const principal = await principalRepository.findById(principalId);
			if (!principal || !principal.active) {
				return false;
			}

			// Create a grant for all requested scopes (auto-consent)
			const Grant = provider.Grant;
			const clientId = params["client_id"] as string;
			const grant = new Grant({
				accountId: principalId,
				clientId,
			});

			// Grant all requested scopes and claims.
			// For public clients, automatically include offline_access so they
			// receive refresh tokens (SPAs can't securely store long-lived tokens,
			// but short-lived access tokens + refresh rotation is the recommended pattern).
			let requestedScope = (params["scope"] as string) || "openid";
			const oauthClient = await oauthClientRepository.findByClientId(clientId);
			if (
				oauthClient?.clientType === "PUBLIC" &&
				!requestedScope.includes("offline_access")
			) {
				requestedScope = `${requestedScope} offline_access`;
			}
			grant.addOIDCScope(requestedScope);

			// Also grant resource scopes for the default resource indicator (the issuer).
			// Without this, oidc-provider's consent policy (rs_scopes_missing check) finds
			// that resource scopes were never encountered, creates a new consent interaction,
			// and the handler auto-completes it with the same insufficient grant → redirect loop.
			const resource = (params["resource"] as string) || provider.issuer;
			grant.addResourceScope(resource, requestedScope);

			// If there are specific claims requested, grant them
			const claimsRaw = params["claims"];
			if (claimsRaw) {
				const claimsParam =
					typeof claimsRaw === "string" ? JSON.parse(claimsRaw) : claimsRaw;
				grant.addOIDCClaims(Object.keys(claimsParam.id_token || {}));
				grant.addOIDCClaims(Object.keys(claimsParam.userinfo || {}));
			}

			const grantId = await grant.save();

			// Finish the interaction
			const result = {
				login: {
					accountId: principalId,
				},
				consent: {
					grantId,
				},
			};

			await provider.interactionFinished(req, res, result);
			reply.hijack();

			fastify.log.info(
				{ uid, principalId, clientId },
				"OIDC interaction completed",
			);

			return true;
		} finally {
			// Restore original URL
			req.url = originalUrl!;
		}
	}

	// GET /oidc/interaction/:uid — Interaction entry point
	fastify.get<{ Params: { uid: string } }>(
		"/oidc/interaction/:uid",
		async (request, reply) => {
			const { uid } = request.params;

			try {
				fastify.log.info(
					{ uid, cookies: Object.keys(request.cookies) },
					"Interaction entry point hit",
				);
				const completed = await tryCompleteInteraction(request, reply, uid);
				if (!completed) {
					fastify.log.info({ uid }, "No valid session, redirecting to login");
					return reply.redirect(
						`${loginPageUrl}?interaction=${encodeURIComponent(uid)}`,
					);
				}
				// Response already sent by interactionFinished + hijack
			} catch (err) {
				fastify.log.error({ err, uid }, "Failed to handle interaction");
				return reply.redirect(
					`${loginPageUrl}?error=${encodeURIComponent("Login required")}`,
				);
			}
		},
	);

	// GET /oidc/interaction/:uid/login — Post-login completion
	fastify.get<{ Params: { uid: string } }>(
		"/oidc/interaction/:uid/login",
		async (request, reply) => {
			const { uid } = request.params;

			try {
				fastify.log.info(
					{ uid, cookies: Object.keys(request.cookies) },
					"Post-login interaction hit",
				);
				const completed = await tryCompleteInteraction(request, reply, uid);
				if (!completed) {
					fastify.log.info(
						{ uid },
						"No valid session after login, redirecting back",
					);
					return reply.redirect(
						`${loginPageUrl}?interaction=${encodeURIComponent(uid)}`,
					);
				}
				fastify.log.info(
					{ uid },
					"Post-login interaction completed successfully",
				);
			} catch (err) {
				fastify.log.error(
					{ err, uid },
					"Failed to complete interaction after login",
				);
				return reply.redirect(
					`${loginPageUrl}?error=${encodeURIComponent("Login failed. Please try again.")}`,
				);
			}
		},
	);

	fastify.log.info("OIDC interaction routes registered");
}
