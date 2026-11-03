/**
 * Fastify glue for field-worker tokens.
 *
 * Register {@link fieldAuth} once, then opt routes in:
 *
 *   await app.register(fieldAuth, {
 *     issuer: "https://auth.example.com",
 *     appCode: "my-app",
 *     publicBaseUrl: "https://api.my-app.example.com",
 *   });
 *   app.post("/orders", { preHandler: app.fcField.require() }, (req) => {
 *     req.fieldPrincipal!.workerId; // verified worker + device
 *   });
 *
 * Deliberately NOT a global hook: it adds no onRequest/preHandler of its own,
 * only runs on routes that use `app.fcField.require()`, and never touches
 * `request.principal` (the OIDC/bearer plane's principal). It is safe next to
 * (and inside the same app as) `flowcatalystAuth`, including a root-context
 * (fp()-wrapped) registration of it.
 *
 * The signed URL is `publicBaseUrl` + the request path. The Host header is
 * never consulted.
 */

import type {
	FastifyPluginAsync,
	FastifyReply,
	FastifyRequest,
	preHandlerAsyncHookHandler,
} from "fastify";
import fp from "fastify-plugin";
import {
	FieldError,
	FieldReason,
	FieldTokenVerifier,
	FieldVerifierUnavailableError,
	type FieldPrincipal,
	type FieldTokenVerifierConfig,
} from "../field/verifier.js";

export interface FieldAuthOptions extends FieldTokenVerifierConfig {
	/**
	 * This service's public base URL (scheme + host, plus any proxy path
	 * prefix). The request URL used for the proof's `htu` check is
	 * `publicBaseUrl` + the request path.
	 */
	publicBaseUrl: string;
}

export interface FieldAuthDecorator {
	/** The underlying verifier. */
	readonly verifier: FieldTokenVerifier;
	/** A preHandler that requires a valid field credential and sets `request.fieldPrincipal`. */
	require(): preHandlerAsyncHookHandler;
}

declare module "fastify" {
	interface FastifyRequest {
		fieldPrincipal?: FieldPrincipal;
	}
	interface FastifyInstance {
		fcField: FieldAuthDecorator;
	}
}

const fieldAuthImpl: FastifyPluginAsync<FieldAuthOptions> = async (
	fastify,
	opts,
) => {
	const { publicBaseUrl, ...verifierConfig } = opts;
	const verifier = new FieldTokenVerifier(verifierConfig);
	const base = publicBaseUrl.replace(/\/+$/, "");

	fastify.decorateRequest("fieldPrincipal", undefined);

	const handler: preHandlerAsyncHookHandler = async (
		req: FastifyRequest,
		reply: FastifyReply,
	) => {
		const rawUrl = req.raw.url ?? req.url;
		const q = rawUrl.indexOf("?");
		const path = q === -1 ? rawUrl : rawUrl.slice(0, q);
		const dpop = req.headers["dpop"];
		try {
			req.fieldPrincipal = await verifier.verify(
				req.headers.authorization,
				Array.isArray(dpop) ? dpop[0] : dpop,
				req.method,
				base + path,
			);
		} catch (e) {
			if (e instanceof FieldError) {
				if (e.reason === FieldReason.PCR) {
					return reply.code(403).send({ error: "PIN_CHANGE_REQUIRED" });
				}
				return reply
					.code(401)
					.header("WWW-Authenticate", 'DPoP error="invalid_token"')
					.send({ error: e.reason });
			}
			if (e instanceof FieldVerifierUnavailableError) {
				return reply.code(503).send({ error: "unavailable" });
			}
			throw e;
		}
	};

	fastify.decorate("fcField", { verifier, require: () => handler });
};

export const fieldAuth = fp(fieldAuthImpl, {
	fastify: "5.x",
	name: "@flowcatalyst/sdk/fastify-field",
});
