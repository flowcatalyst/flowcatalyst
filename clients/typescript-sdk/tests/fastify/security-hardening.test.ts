/**
 * Security hardening of the Fastify plugin:
 *   1. the OAuth state cookie / a sibling registration's cookie must not
 *      authenticate as a session (purpose + cookie-name binding),
 *   2. returnTo / logout redirectTo cannot leave the origin,
 *   3. an OIDC ID token is not accepted as an API bearer.
 */

import { strict as assert } from "node:assert";
import { after, before, describe, it } from "node:test";
import Fastify, { type FastifyInstance } from "fastify";
import {
	flowcatalystAuth,
	generateSessionSecret,
} from "../../src/fastify/index.js";
import { createSessionCrypto } from "../../src/fastify/crypto.js";
import {
	isSafeLocalPath,
	resolveLogoutRedirect,
	sanitizeReturnTo,
} from "../../src/fastify/plugin.js";
import { startMockIssuer, type MockIssuer } from "./mock-issuer.js";

let issuer: MockIssuer;
let app: FastifyInstance;
let base: string;
const secret = generateSessionSecret();
const crypto = createSessionCrypto(secret);

const principal = (id: string) => ({
	id,
	type: "USER" as const,
	scope: "client",
	name: "x",
	clients: [],
	clientIds: [],
	clientCodes: [],
	roles: [],
	applications: [],
	applicationCodes: [],
	allApplications: false,
});

async function listen(a: FastifyInstance): Promise<string> {
	await a.listen({ port: 0, host: "127.0.0.1" });
	const addr = a.server.address();
	if (!addr || typeof addr === "string") throw new Error("bind");
	return `http://127.0.0.1:${addr.port}`;
}

function pickCookie(res: Response, name: string): string | null {
	for (const c of res.headers.getSetCookie?.() ?? []) {
		if (c.startsWith(`${name}=`)) return c.split(";")[0]!.slice(name.length + 1);
	}
	return null;
}

before(async () => {
	issuer = await startMockIssuer();
	app = Fastify({ logger: false });

	// Plane A (encapsulated) and B (root) share ONE secret, different cookies.
	await app.register(async (a) => {
		await a.register(flowcatalystAuth, {
			baseUrl: issuer.baseUrl,
			clientId: "clt_a",
			clientSecret: "s",
			cookie: { name: "sess_a", secret, secure: false },
			routes: { login: "/a/login", callback: "/a/callback", logout: "/a/logout" },
		});
		a.get("/a/me", { preHandler: a.fc.requireSession() }, async (req) => ({
			id: req.principal!.id,
		}));
	});
	await app.register(flowcatalystAuth, {
		baseUrl: issuer.baseUrl,
		clientId: "clt_b",
		clientSecret: "s",
		cookie: { name: "sess_b", secret, secure: false },
		routes: { login: "/b/login", callback: "/b/callback", logout: "/b/logout" },
		acceptIdTokenAsBearer: false,
		allowedLogoutRedirectOrigins: ["https://ok.example"],
	});
	app.get("/b/me", { preHandler: app.fc.requireSession() }, async (req) => ({
		id: req.principal!.id,
	}));
	app.get("/b/api", { preHandler: app.fc.requireBearer() }, async (req) => ({
		id: req.principal!.id,
	}));
	base = await listen(app);
});

after(async () => {
	await app.close();
	await issuer.stop();
});

async function login(loginPath: string, cookieName: string, sub: string, qs = "") {
	issuer.setNextTokens({
		accessToken: await issuer.signAccessToken({ sub, name: sub }),
		refreshToken: `rt_${sub}`,
		expiresIn: 600,
	});
	const l = await fetch(`${base}${loginPath}${qs}`, { redirect: "manual" });
	assert.equal(l.status, 302);
	const state = pickCookie(l, "fc_oauth_state");
	assert.ok(state);
	const a = await fetch(l.headers.get("location")!, { redirect: "manual" });
	const cb = await fetch(a.headers.get("location")!, {
		headers: { cookie: `fc_oauth_state=${state}` },
		redirect: "manual",
	});
	return { state: state!, cb, session: pickCookie(cb, cookieName) };
}

const me = (path: string, cookie: string) =>
	fetch(`${base}${path}`, { headers: { cookie, Accept: "application/json" } });

describe("fix 1: cookie purpose binding", () => {
	it("(a) the fc_oauth_state value replayed as the session cookie is unauthenticated", async () => {
		const l = await fetch(`${base}/b/login`, { redirect: "manual" });
		const state = pickCookie(l, "fc_oauth_state")!;
		assert.equal((await me("/b/me", `sess_b=${state}`)).status, 401);
	});

	it("(b) a real session still works, (f) and the login round trip completes", async () => {
		const { cb, session } = await login("/b/login", "sess_b", "usr_1");
		assert.equal(cb.status, 302);
		assert.ok(session);
		const r = await me("/b/me", `sess_b=${session}`);
		assert.equal(r.status, 200);
		assert.equal(((await r.json()) as { id: string }).id, "usr_1");
	});

	it("(c) a legacy session payload (no purpose) still authenticates", async () => {
		const legacy = await crypto.encrypt(
			JSON.stringify({
				principal: principal("usr_legacy"),
				tokens: { accessToken: "at", accessTokenExpiresAt: Date.now() + 60_000 },
				sessionData: {},
				expiresAt: Date.now() + 60_000,
			}),
		);
		const r = await me("/b/me", `sess_b=${legacy}`);
		assert.equal(r.status, 200);
	});

	it("(d) a legacy state payload replayed as a session is rejected", async () => {
		const legacyState = await crypto.encrypt(
			JSON.stringify({
				principal: principal("_oauth_state"),
				tokens: { accessToken: "", accessTokenExpiresAt: 0 },
				sessionData: { bag: {} },
				expiresAt: Date.now() + 60_000,
			}),
		);
		assert.equal((await me("/b/me", `sess_b=${legacyState}`)).status, 401);
	});

	it("(d2) a legacy state payload still completes a login callback", async () => {
		// Written by an old SDK instance mid-rollout: no purpose/cookie fields.
		const l = await fetch(`${base}/b/login`, { redirect: "manual" });
		const st = new URL(l.headers.get("location")!).searchParams.get("state")!;
		const legacyState = await crypto.encrypt(
			JSON.stringify({
				principal: principal("_oauth_state"),
				tokens: { accessToken: "", accessTokenExpiresAt: 0 },
				sessionData: { bag: { state: st, codeVerifier: "v", returnTo: "/x" } },
				expiresAt: Date.now() + 60_000,
			}),
		);
		const cb = await fetch(`${base}/b/callback?code=c&state=${st}`, {
			headers: { cookie: `fc_oauth_state=${legacyState}` },
			redirect: "manual",
		});
		assert.equal(cb.status, 302);
		assert.equal(cb.headers.get("location"), "/x");
	});

	it("a session cookie replayed as the state cookie is rejected", async () => {
		const { session } = await login("/b/login", "sess_b", "usr_2");
		const cb = await fetch(`${base}/b/callback?code=c&state=s`, {
			headers: { cookie: `fc_oauth_state=${session}` },
			redirect: "manual",
		});
		assert.equal(cb.status, 400);
	});

	it("(e) two registrations sharing one secret do not accept each other's cookies", async () => {
		const a = await login("/a/login", "sess_a", "ptu_a");
		const b = await login("/b/login", "sess_b", "usr_b");
		assert.equal((await me("/a/me", `sess_a=${a.session}`)).status, 200);
		// B's payload presented under A's cookie name, and vice versa.
		assert.equal((await me("/a/me", `sess_a=${b.session}`)).status, 401);
		assert.equal((await me("/b/me", `sess_b=${a.session}`)).status, 401);
	});
});

describe("fix 2: open redirect", () => {
	const cases: Array<[string, string]> = [
		["/ok", "/ok"],
		["/a?b=c#d", "/a?b=c#d"],
		["//evil", "/"],
		["/\\evil.example", "/"],
		["/%5Cevil.example", "/"],
		["/%09/evil.example", "/"],
		["https://evil.example", "/"],
		["javascript:alert(1)", "/"],
	];
	for (const [input, want] of cases) {
		it(`sanitizeReturnTo(${JSON.stringify(input)}) -> ${want}`, () => {
			assert.equal(sanitizeReturnTo(input), want);
		});
	}
	it("rejects control characters and raw tabs", () => {
		assert.equal(isSafeLocalPath("/\t/evil.example"), false);
		assert.equal(isSafeLocalPath("/a\u007f"), false);
	});

	it("login -> callback never redirects off-origin", async () => {
		const { cb } = await login("/b/login", "sess_b", "usr_3", `?returnTo=${encodeURIComponent("/\\evil.example")}`);
		assert.equal(cb.headers.get("location"), "/");
		const ok = await login("/b/login", "sess_b", "usr_3", `?returnTo=${encodeURIComponent("/ok?x=1")}`);
		assert.equal(ok.cb.headers.get("location"), "/ok?x=1");
	});

	it("logout: absolute accepted with no allow-list", () => {
		assert.equal(resolveLogoutRedirect("https://any.example/x"), "https://any.example/x");
		assert.equal(resolveLogoutRedirect("/bye"), "/bye");
		assert.equal(resolveLogoutRedirect("/\\evil.example"), "/");
		assert.equal(resolveLogoutRedirect("//evil.example"), "/");
	});
	it("logout: allow-list honours listed origin only", () => {
		const list = ["https://ok.example"];
		assert.equal(resolveLogoutRedirect("https://ok.example/bye", list), "https://ok.example/bye");
		assert.equal(resolveLogoutRedirect("https://evil.example/bye", list), "/");
		assert.equal(resolveLogoutRedirect("/bye", list), "/bye");
	});
	it("logout route applies the allow-list", async () => {
		const post = (redirectTo: string) =>
			fetch(`${base}/b/logout`, {
				method: "POST",
				headers: { "content-type": "application/json" },
				body: JSON.stringify({ redirectTo }),
				redirect: "manual",
			});
		assert.equal((await post("https://ok.example/bye")).headers.get("location"), "https://ok.example/bye");
		assert.equal((await post("https://evil.example/")).headers.get("location"), "/");
	});
});

describe("fix 3: ID token as bearer", () => {
	const bearer = (t: string) =>
		fetch(`${base}/b/api`, { headers: { authorization: `Bearer ${t}` } });
	const idToken = () =>
		issuer.signIdToken({ sub: "usr_id", aud: "clt_other", nonce: "n1" } as never);

	it("an ID-token-shaped JWT (nonce, no token_use) is rejected by default", async () => {
		assert.equal((await bearer(await idToken())).status, 401);
	});
	it("azp equal to the single aud is also ID-token-shaped", async () => {
		const t = await issuer.signIdToken({ sub: "u", aud: "clt_o", azp: "clt_o" } as never);
		assert.equal((await bearer(t)).status, 401);
	});
	it("an api access token is still accepted", async () => {
		const t = await issuer.signAccessToken({ sub: "usr_api", name: "n", token_use: "api" } as never);
		assert.equal((await bearer(t)).status, 200);
	});
	it("a token_use=identity token keeps today's behaviour (accepted)", async () => {
		const t = await issuer.signAccessToken({
			sub: "usr_idn",
			name: "n",
			token_use: "identity",
			nonce: "n",
		} as never);
		assert.equal((await bearer(t)).status, 200);
	});
	it("accepted with acceptIdTokenAsBearer: true", async () => {
		const a = Fastify({ logger: false });
		await a.register(flowcatalystAuth, {
			baseUrl: issuer.baseUrl,
			clientId: "clt_c",
			clientSecret: "s",
			cookie: { secret, secure: false },
			acceptIdTokenAsBearer: true,
		});
		a.get("/api", { preHandler: a.fc.requireBearer() }, async () => ({ ok: true }));
		const u = await listen(a);
		try {
			const r = await fetch(`${u}/api`, { headers: { authorization: `Bearer ${await idToken()}` } });
			assert.equal(r.status, 200);
		} finally {
			await a.close();
		}
	});
});
