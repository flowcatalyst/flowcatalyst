import { createHash, webcrypto } from "node:crypto";
import { b64uDecode, b64uEncode } from "../../src/encoding.js";
import {
	WebCryptoKeyProvider,
	MemoryDb,
	MemorySecureStore,
	FieldAuth,
	type FieldAuthOptions,
} from "../../src/index.js";

export const PLATFORM = "https://fc.test";
export const APP = "https://app.test";
export const GOOD_PIN = "482915";
export const NEW_PIN = "739162";

export const GOOD_BADGE = "FWB1ABCDEFGHJKMNPQRSTVWXYZ";
export const GOOD_CODE = "FWS1ABCDEFGHJKMNPQRSTVWXYZ";
export const CLIENT_IDENTIFIER = "acme";

interface Ctx {
	jkt: string;
	/** The session started on a key the platform had not enrolled; it stays so until the next login. */
	unenrolled: boolean;
	worker: { id: string; workerCode: string; name: string };
	actor: { sub: string; typ: string; name: string } | null;
	test: boolean;
}

function fakeJwt(claims: Record<string, unknown>): string {
	const enc = (o: unknown) => Buffer.from(JSON.stringify(o)).toString("base64url");
	return `${enc({ typ: "fc-field+jwt", alg: "ES256" })}.${enc(claims)}.c2ln`;
}

export interface Logged {
	method: string;
	url: string;
	headers: Record<string, string>;
	body: string;
}

interface Parsed {
	header: { typ: string; alg: string; jwk: { kty: string; crv: string; x: string; y: string } };
	claims: Record<string, unknown>;
	jkt: string;
}

/** Independent RFC 7638 thumbprint (node:crypto, not the library's code). */
export function thumbprint(jwk: { crv: string; kty: string; x: string; y: string }): string {
	const canonical = JSON.stringify({ crv: jwk.crv, kty: jwk.kty, x: jwk.x, y: jwk.y });
	return createHash("sha256").update(canonical).digest("base64url");
}

/** Independent ES256 check of a compact proof. Throws when it does not verify. */
export async function verifyProof(compact: string): Promise<Parsed> {
	const [h, c, s] = compact.split(".");
	if (!h || !c || !s) throw new Error("not a compact JWS");
	const header = JSON.parse(Buffer.from(h, "base64url").toString()) as Parsed["header"];
	if (header.typ !== "dpop+jwt" || header.alg !== "ES256") throw new Error("bad typ/alg");
	if ("d" in header.jwk) throw new Error("private member");
	const key = await webcrypto.subtle.importKey(
		"jwk",
		header.jwk,
		{ name: "ECDSA", namedCurve: "P-256" },
		false,
		["verify"],
	);
	const sig = b64uDecode(s);
	if (sig.length !== 64) throw new Error("sig length");
	const ok = await webcrypto.subtle.verify(
		{ name: "ECDSA", hash: "SHA-256" },
		key,
		sig as BufferSource,
		Buffer.from(`${h}.${c}`),
	);
	if (!ok) throw new Error("signature does not verify");
	return {
		header,
		claims: JSON.parse(Buffer.from(c, "base64url").toString()) as Record<string, unknown>,
		jkt: thumbprint(header.jwk),
	};
}

function json(status: number, body: unknown, headers: Record<string, string> = {}): Response {
	return new Response(status === 204 ? null : JSON.stringify(body), {
		status,
		headers: { "Content-Type": "application/json", ...headers },
	});
}

export class FakePlatform {
	log: Logged[] = [];
	private seq = 0;
	private nonces = new Set<string>();
	private deviceJkt: string | null = null;
	private refreshTokens = new Map<string, { family: string } & Ctx>(); // token -> session
	private liveAccess = new Map<string, Ctx>();
	/** Issue real-looking JWT access tokens carrying device/act/test claims. */
	jwtClaims = false;
	/** The next logins/refreshes mark the worker as a test identity. */
	testIdentity = false;
	/** The policy refuses unknown keys on login. */
	requireEnrolled = false;
	/** The policy lets an enrolled device sign in by badge alone. */
	badgeOnly = false;
	canImpersonate = true;
	private codeUsed = false;
	pin = GOOD_PIN;
	expiresIn = 600;
	mode: "PERSONAL" | "SHARED" = "PERSONAL";
	pcr = false;
	/** Reject the next nonce-bearing request with use_dpop_nonce, once. */
	staleNonceOnce = false;
	/** Make the app answer 401 invalid_token this many times. */
	appInvalidTokens = 0;
	/** Make the app demand a nonce once. */
	appNonceOnce = false;
	/** Answer the next refresh with this error. */
	refreshError: { status: number; error: string } | null = null;
	refreshCount = 0;
	appHits = 0;
	/** Delay refresh responses (ms) so callers overlap. */
	refreshDelayMs = 0;
	/** Throw from fetch on the next call. */
	networkDown = false;

	private newNonce(): string {
		const n = `n-${++this.seq}`;
		this.nonces.add(n);
		return n;
	}

	fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
		const req = new Request(input as RequestInfo, init);
		const body = init?.body ? String(init.body) : await req.clone().text();
		const headers: Record<string, string> = {};
		req.headers.forEach((v, k) => (headers[k.toLowerCase()] = v));
		this.log.push({ method: req.method, url: req.url, headers, body });
		if (this.networkDown) {
			this.networkDown = false;
			throw new TypeError(`fetch failed: ${body}`); // hostile: quotes the body
		}
		const url = new URL(req.url);
		if (url.origin === APP) return this.app(req, url, headers);
		return this.platform(req, url, headers, body);
	};

	private async platform(
		req: Request,
		url: URL,
		headers: Record<string, string>,
		bodyText: string,
	): Promise<Response> {
		const fresh = this.newNonce();
		const h = { "DPoP-Nonce": fresh };
		const fail = (status: number, error: string, extra: Record<string, unknown> = {}, more = {}) =>
			json(status, { error, message: `${error} message`, ...extra }, { ...h, ...more });
		let proof: Parsed;
		try {
			proof = await verifyProof(headers["dpop"] ?? "");
		} catch (e) {
			return fail(401, "invalid_dpop_proof", { message: String(e) });
		}
		const c = proof.claims;
		if (c["htm"] !== req.method || c["htu"] !== PLATFORM + url.pathname) {
			return fail(401, "invalid_dpop_proof", { message: "htm/htu" });
		}
		if (typeof c["jti"] !== "string" || typeof c["iat"] !== "number") {
			return fail(401, "invalid_dpop_proof", { message: "jti/iat" });
		}
		const path = url.pathname;
		if (path !== "/field/auth/nonce") {
			const n = c["nonce"];
			const stale = this.staleNonceOnce;
			this.staleNonceOnce = false;
			if (typeof n !== "string" || stale || !this.nonces.delete(n)) {
				return fail(401, "use_dpop_nonce", {}, { "WWW-Authenticate": 'DPoP error="use_dpop_nonce"' });
			}
		}
		const body = bodyText ? (JSON.parse(bodyText) as Record<string, unknown>) : {};
		if (path === "/field/auth/nonce") return json(200, { nonce: fresh, expiresAt: "x" }, h);
		if (path === "/field/devices/enrol") {
			if (body["code"] !== "ABCDE12345") return fail(401, "INVALID_ENROLMENT_CODE");
			this.deviceJkt = proof.jkt;
			return json(201, { deviceId: "fdv_1", mode: this.mode, clientId: "clt_1", appCodes: ["yard"] }, h);
		}
		const known = proof.jkt === this.deviceJkt;
		if (!known && this.requireEnrolled) return fail(401, "UNKNOWN_DEVICE");
		const ctx = (worker = { id: "fwu_1", workerCode: "w1", name: "Worker One" }): Ctx => ({
			jkt: proof.jkt,
			unenrolled: !known,
			worker,
			actor: null,
			test: this.testIdentity,
		});
		if (path === "/field/auth/login") {
			const hasBadge = body["badgeToken"] !== undefined;
			if (hasBadge === (body["workerCode"] !== undefined)) return fail(400, "INVALID_REQUEST");
			if (known && body["clientIdentifier"] !== undefined && body["clientIdentifier"] !== CLIENT_IDENTIFIER) {
				return fail(400, "CLIENT_MISMATCH");
			}
			if (!known && !hasBadge && body["clientIdentifier"] === undefined) return fail(400, "INVALID_REQUEST");
			if (hasBadge) {
				if (body["badgeToken"] !== GOOD_BADGE) return fail(401, "INVALID_CREDENTIALS");
				if (body["pin"] === undefined) {
					if (!(this.badgeOnly && known)) return fail(400, "PIN_REQUIRED");
					return json(200, this.issue("fam-1", ctx()), h);
				}
			}
			if (body["workerCode"] === "locked") return fail(423, "WORKER_LOCKED");
			if (body["workerCode"] === "slow") {
				return fail(429, "TOO_MANY_ATTEMPTS", {}, { "Retry-After": "42" });
			}
			if (body["workerCode"] === "echo") {
				return fail(401, "INVALID_CREDENTIALS", { message: `bad pin ${String(body["pin"])}` });
			}
			if (body["pin"] !== this.pin) return fail(401, "INVALID_CREDENTIALS");
			return json(200, this.issue("fam-1", ctx()), h);
		}
		if (path === "/field/auth/sign-in-code") {
			if (body["code"] !== GOOD_CODE || this.codeUsed) return fail(401, "INVALID_CODE");
			this.codeUsed = true;
			const c = ctx({ id: "fwu_3", workerCode: "w3", name: "Worker Three" });
			c.actor = { sub: "prn_1", typ: "principal", name: "Admin Ann" };
			return json(200, this.issue("fam-code", c), h);
		}
		if (path === "/field/auth/act-as") {
			const tok = (headers["authorization"] ?? "").replace(/^DPoP /, "");
			const sup = this.liveAccess.get(tok);
			if (!sup) return fail(401, "invalid_token", {}, { "WWW-Authenticate": 'DPoP error="invalid_token"' });
			if (sup.actor) return fail(403, "IMPERSONATING");
			if (!this.canImpersonate) return fail(403, "IMPERSONATION_FORBIDDEN");
			if (body["pin"] !== this.pin) return fail(401, "INVALID_CREDENTIALS");
			if (body["workerCode"] === "sup2") return fail(403, "NO_APP_ACCESS");
			const c = ctx({ id: "fwu_2", workerCode: String(body["workerCode"] ?? "w2"), name: "Worker Two" });
			c.actor = { sub: sup.worker.id, typ: "field_worker", name: sup.worker.name };
			return json(200, this.issue(`fam-imp-${++this.seq}`, c), h);
		}
		if (path === "/field/auth/refresh") {
			this.refreshCount++;
			if (this.refreshDelayMs) await new Promise((r) => setTimeout(r, this.refreshDelayMs));
			if (this.refreshError) return fail(this.refreshError.status, this.refreshError.error);
			const rec = this.refreshTokens.get(String(body["refreshToken"]));
			if (!rec || rec.jkt !== proof.jkt) return fail(401, "INVALID_GRANT");
			this.refreshTokens.delete(String(body["refreshToken"]));
			this.pcr = false;
			return json(200, this.issue(rec.family, rec), h);
		}
		if (path === "/field/auth/pin") {
			const auth = headers["authorization"] ?? "";
			const tok = auth.replace(/^DPoP /, "");
			const cur = this.liveAccess.get(tok);
			if (!cur) {
				return fail(401, "invalid_token", {}, { "WWW-Authenticate": 'DPoP error="invalid_token"' });
			}
			if (cur.actor) return fail(403, "IMPERSONATING");
			const ath = createHash("sha256").update(tok).digest("base64url");
			if (c["ath"] !== ath) return fail(401, "invalid_dpop_proof");
			if (body["currentPin"] !== this.pin) return fail(401, "INVALID_CREDENTIALS");
			if (body["newPin"] === "111111") {
				return fail(422, "PIN_POLICY", { violations: ["REPEATED_DIGITS"] });
			}
			this.pin = String(body["newPin"]);
			return json(204, null, h);
		}
		if (path === "/field/auth/logout") {
			if (body["scope"] === "device-all" && !known) return fail(400, "INVALID_REQUEST");
			this.refreshTokens.delete(String(body["refreshToken"]));
			return json(204, null, h);
		}
		return fail(404, "NOT_FOUND");
	}

	private issue(family: string, ctx: Ctx): Record<string, unknown> {
		const n = ++this.seq;
		const known = !ctx.unenrolled;
		const access = this.jwtClaims
			? fakeJwt({
					sub: ctx.worker.id,
					jti: `jti-${n}`,
					...(known ? { device_id: "fdv_1", device_mode: this.mode } : { device_mode: "UNENROLLED" }),
					...(ctx.actor ? { act: ctx.actor } : {}),
					...(ctx.test ? { test: true } : {}),
				})
			: `acc-${n}`;
		const refresh = `fwr_${++this.seq}secretrefresh`;
		this.liveAccess.set(access, ctx);
		this.refreshTokens.set(refresh, { family, ...ctx });
		return {
			accessToken: access,
			tokenType: "DPoP",
			expiresIn: this.expiresIn,
			refreshToken: refresh,
			refreshExpiresAt: "2030-01-01T00:00:00Z",
			idleExpiresAt: "2029-01-01T00:00:00Z",
			worker: ctx.worker,
			pinChangeRequired: this.pcr,
		};
	}

	private async app(req: Request, url: URL, headers: Record<string, string>): Promise<Response> {
		this.appHits++;
		const h: Record<string, string> = {};
		if (this.appNonceOnce) {
			this.appNonceOnce = false;
			const proof0 = headers["dpop"] ? await verifyProof(headers["dpop"]) : null;
			if (!proof0?.claims["nonce"]) {
				return json(401, { error: "use_dpop_nonce" }, {
					"DPoP-Nonce": "app-nonce-1",
					"WWW-Authenticate": 'DPoP error="use_dpop_nonce"',
				});
			}
		}
		const tok = (headers["authorization"] ?? "").replace(/^DPoP /, "");
		if (this.appInvalidTokens > 0 || !this.liveAccess.has(tok)) {
			this.appInvalidTokens = Math.max(0, this.appInvalidTokens - 1);
			return json(401, { error: "invalid_token" }, { "WWW-Authenticate": 'DPoP error="invalid_token"' });
		}
		const proof = await verifyProof(headers["dpop"] ?? "");
		const ok =
			proof.jkt === this.liveAccess.get(tok)?.jkt &&
			proof.claims["htm"] === req.method &&
			proof.claims["htu"] === APP + url.pathname &&
			proof.claims["ath"] === createHash("sha256").update(tok).digest("base64url");
		if (!ok) return json(401, { error: "invalid_proof" }, { "WWW-Authenticate": 'DPoP error="invalid_token"' });
		return json(200, { ok: true, nonce: proof.claims["nonce"] ?? null }, h);
	}
}

export interface Rig {
	platform: FakePlatform;
	auth: FieldAuth;
	store: MemorySecureStore;
	keys: WebCryptoKeyProvider;
	clock: { now: number };
}

export function rig(over: Partial<FieldAuthOptions> = {}, platform = new FakePlatform()): Rig {
	const store = new MemorySecureStore();
	const keys = new WebCryptoKeyProvider({ db: new MemoryDb() });
	const clock = { now: 1_800_000_000_000 };
	const auth = new FieldAuth({
		platformUrl: PLATFORM,
		appCode: "yard",
		keys,
		store,
		fetch: platform.fetch as typeof fetch,
		clock: () => clock.now,
		...over,
	});
	return { platform, auth, store, keys, clock };
}

export async function enrolledAndLoggedIn(r: Rig): Promise<void> {
	await r.auth.enrol("ABCDE12345");
	await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
}

export { b64uEncode };
