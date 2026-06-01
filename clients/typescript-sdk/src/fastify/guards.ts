/**
 * Route guards installed on the Fastify instance as `app.fc.requireSession`,
 * `app.fc.requireBearer`, `app.fc.requireAuth`.
 *
 * Each returns a `preHandler` you attach per-route. The guards differ only
 * in what they accept and how they respond to missing/invalid credentials:
 *
 *   requireSession  — cookie only, redirects to /auth/login on miss
 *   requireBearer   — Bearer token only, 401 JSON on miss
 *   requireAuth     — either; redirects iff the request looks like a browser
 *                     navigation (Accept: text/html), else 401 JSON
 */

import type { FastifyReply, FastifyRequest, preHandlerHookHandler } from "fastify";

export type GuardKind = "session" | "bearer" | "any";

export interface GuardContext {
	loginPath: string;
	returnToQueryParam: string;
}

export function makeGuard(
	kind: GuardKind,
	ctx: GuardContext,
): preHandlerHookHandler {
	return async function (req, reply) {
		if (req.principal) return; // already authenticated
		await deny(kind, req, reply, ctx);
	};
}

async function deny(
	kind: GuardKind,
	req: FastifyRequest,
	reply: FastifyReply,
	ctx: GuardContext,
): Promise<void> {
	const wantsHtml = isHtmlNavigation(req);
	const useRedirect =
		kind === "session" || (kind === "any" && wantsHtml);

	if (useRedirect) {
		const returnTo = encodeURIComponent(req.url);
		await reply
			.code(302)
			.redirect(`${ctx.loginPath}?${ctx.returnToQueryParam}=${returnTo}`);
		return;
	}
	await reply
		.code(401)
		.header("WWW-Authenticate", `Bearer realm="flowcatalyst"`)
		.send({ error: "unauthorized" });
}

function isHtmlNavigation(req: FastifyRequest): boolean {
	if (req.method !== "GET" && req.method !== "HEAD") return false;
	const accept = req.headers["accept"];
	return typeof accept === "string" && accept.includes("text/html");
}
