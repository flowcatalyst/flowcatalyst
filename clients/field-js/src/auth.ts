import { randomId } from "./encoding.js";
import {
	FieldAuthError,
	FieldProviderError,
	isProviderErrorCode,
} from "./errors.js";
import { buildProof } from "./proof.js";
import type { DeviceKey, KeyProvider, SecureStore } from "./types.js";

export const STORE_DEVICE = "fc.field.device";
export const STORE_REFRESH = "fc.field.refresh";
export const STORE_SESSION = "fc.field.session";

/** Refresh first when the access token expires within this many ms. */
export const REFRESH_SKEW_MS = 30_000;

export interface FieldAuthOptions {
	/** Optional until enrolment: an enrolment QR payload carries it. */
	platformUrl?: string;
	appCode: string;
	keys: KeyProvider;
	store: SecureStore;
	fetch?: typeof fetch;
	/** ms since the epoch; for tests. */
	clock?: () => number;
	/**
	 * Keep the refresh token behind a biometric prompt (PERSONAL devices on a
	 * store that supports it) and allow [FieldAuth.unlock].
	 */
	biometricUnlock?: boolean;
	/** The prompt text shown for a biometric read. */
	unlockPrompt?: string;
}

export interface DeviceInfo {
	platform?: string;
	model?: string;
}

export interface EnrolOptions {
	label?: string;
	/** Defaults to `"web"` and `navigator.userAgent`. */
	deviceInfo?: DeviceInfo;
}

export interface DeviceRecord {
	deviceId: string;
	mode: "PERSONAL" | "SHARED";
	clientId: string;
	appCodes: string[];
	platformUrl: string;
}

/** All times are ms since the epoch on the device clock. */
export interface Session {
	worker: { id: string; workerCode: string; name: string };
	appCode: string;
	/** When the access token expires. */
	expiresAt: number;
	idleExpiresAt: number;
	refreshExpiresAt: number;
	pinChangeRequired: boolean;
}

export type SessionEndedReason = "logout" | "invalid_token" | (string & {});

export type FieldAuthEventType =
	| "sessionChanged"
	| "pinChangeRequired"
	| "sessionEnded"
	| "locked";

export class FieldAuthEvent<T = unknown> extends Event {
	readonly detail: T;
	constructor(type: FieldAuthEventType, detail: T) {
		super(type);
		this.detail = detail;
	}
}

export interface LockedDetail {
	/** Seconds until the lock lifts; absent for a hard lock. */
	retryAfter?: number;
	hard: boolean;
}

export interface QrPayload {
	platformUrl: string;
	code: string;
}

const QR_PREFIX = "fcfield:1:";

/** Parses `fcfield:1:<platformBaseUrl>:<code>`; the code is the last segment. */
export function parseEnrolmentQr(payload: string): QrPayload {
	const bad = () =>
		new FieldAuthError("INVALID_ENROLMENT_CODE", "not a FlowCatalyst enrolment QR code");
	const p = payload.trim();
	if (!p.startsWith(QR_PREFIX)) throw bad();
	const rest = p.slice(QR_PREFIX.length);
	const i = rest.lastIndexOf(":");
	if (i <= 0) throw bad();
	const platformUrl = rest.slice(0, i).replace(/\/+$/, "");
	const code = rest.slice(i + 1);
	if (!code || /\s/.test(code)) throw bad();
	let u: URL;
	try {
		u = new URL(platformUrl);
	} catch {
		throw bad();
	}
	if (u.protocol !== "https:" && u.protocol !== "http:") throw bad();
	return { platformUrl, code };
}

interface TokenResponse {
	accessToken: string;
	expiresIn: number;
	refreshToken: string;
	refreshExpiresAt: string;
	idleExpiresAt: string;
	worker: { id: string; workerCode: string; name: string };
	pinChangeRequired: boolean;
}

interface Reply {
	status: number;
	headers: Headers;
	json: Record<string, unknown> | null;
}

const NONCE_PATH = "/field/auth/nonce";

const SESSION_ENDING = new Set([
	"INVALID_GRANT",
	"SESSION_EXPIRED",
	"DEVICE_REVOKED",
	"DEVICE_SUSPENDED",
	"UNKNOWN_DEVICE",
	"NO_APP_ACCESS",
]);

function wwwAuth(res: Response): string {
	return res.headers.get("WWW-Authenticate") ?? "";
}

export class FieldAuth extends EventTarget {
	private readonly opts: FieldAuthOptions;
	private readonly fetchImpl: typeof fetch;
	private readonly clock: () => number;
	private platformUrl: string | null;
	private device: DeviceRecord | null = null;
	private sessionState: Session | null = null;
	private access: { token: string; expiresAt: number } | null = null;
	private refreshToken: string | null = null;
	private jwk: DeviceKey["jwk"] | null = null;
	private loaded: Promise<void> | null = null;
	private refreshing: Promise<Session> | null = null;
	private queue: Promise<unknown> = Promise.resolve();
	private readonly nonces = new Map<string, string>();

	constructor(opts: FieldAuthOptions) {
		super();
		this.opts = opts;
		this.fetchImpl = opts.fetch ?? ((...a) => globalThis.fetch(...a));
		this.clock = opts.clock ?? (() => Date.now());
		this.platformUrl = opts.platformUrl ? opts.platformUrl.replace(/\/+$/, "") : null;
	}

	static parseEnrolmentQr(payload: string): QrPayload {
		return parseEnrolmentQr(payload);
	}

	parseEnrolmentQr(payload: string): QrPayload {
		return parseEnrolmentQr(payload);
	}

	/** The signed-in session (metadata restored from the store counts), or null. */
	get session(): Session | null {
		return this.sessionState;
	}

	/** The enrolled device, once [restore] or [enrol] has run. */
	get enrolledDevice(): DeviceRecord | null {
		return this.device;
	}

	private emit<T>(type: FieldAuthEventType, detail: T): void {
		this.dispatchEvent(new FieldAuthEvent(type, detail));
	}

	// ── state ────────────────────────────────────────────────────────────

	/** Loads the enrolled device and session metadata from the store (once). */
	restore(): Promise<{ device: DeviceRecord | null; session: Session | null }> {
		this.loaded ??= this.load();
		return this.loaded.then(() => ({ device: this.device, session: this.sessionState }));
	}

	private async load(): Promise<void> {
		const dev = await this.store("get", STORE_DEVICE);
		if (dev) {
			try {
				this.device = JSON.parse(dev) as DeviceRecord;
				this.platformUrl ??= this.device.platformUrl.replace(/\/+$/, "");
			} catch {
				this.device = null;
			}
		}
		const ses = await this.store("get", STORE_SESSION);
		if (ses) {
			try {
				this.sessionState = JSON.parse(ses) as Session;
			} catch {
				this.sessionState = null;
			}
		}
	}

	/** Maps a provider failure to a [FieldAuthError] that carries only its code. */
	private async store(op: "get", key: string, prompt?: string): Promise<string | null>;
	private async store(op: "set", key: string, value: string, bio?: boolean): Promise<void>;
	private async store(op: "delete", key: string): Promise<void>;
	private async store(
		op: "get" | "set" | "delete",
		key: string,
		a?: string,
		bio?: boolean,
	): Promise<string | null | void> {
		try {
			if (op === "get") {
				return await this.opts.store.get(key, a ? { prompt: a } : undefined);
			}
			if (op === "set") {
				return await this.opts.store.set(key, a ?? "", bio ? { requireBiometric: true } : undefined);
			}
			return await this.opts.store.delete(key);
		} catch (e) {
			throw providerToAuthError(e);
		}
	}

	private requirePlatform(): string {
		if (!this.platformUrl) {
			throw new FieldAuthError("NOT_ENROLLED", "no platform URL: enrol this device first");
		}
		return this.platformUrl;
	}

	private async requireDevice(): Promise<DeviceRecord> {
		await this.restore();
		if (!this.device) throw new FieldAuthError("NOT_ENROLLED", "this device is not enrolled");
		return this.device;
	}

	private async publicKey(): Promise<{ jwk: DeviceKey["jwk"]; key: DeviceKey }> {
		const key = await this.getKey();
		this.jwk = key.jwk;
		return { jwk: key.jwk, key };
	}

	private async getKey(): Promise<DeviceKey> {
		try {
			return await this.opts.keys.getOrCreateDeviceKey();
		} catch (e) {
			throw providerToAuthError(e);
		}
	}

	private async proof(
		htm: string,
		htu: string,
		nonce: string | undefined,
		accessToken?: string,
	): Promise<string> {
		const jwk = this.jwk ?? (await this.publicKey()).jwk;
		try {
			return await buildProof(this.opts.keys, jwk, {
				htm,
				htu,
				nonce,
				accessToken,
				iat: Math.floor(this.clock() / 1000),
			});
		} catch (e) {
			throw providerToAuthError(e);
		}
	}

	// ── platform calls ───────────────────────────────────────────────────

	/** Serialises platform calls so a single-use nonce is never shared. */
	private enqueue<T>(fn: () => Promise<T>): Promise<T> {
		const run = this.queue.then(fn, fn);
		this.queue = run.catch(() => undefined);
		return run;
	}

	private async send(
		url: string,
		body: unknown,
		proof: string,
		accessToken?: string,
	): Promise<Reply> {
		const headers: Record<string, string> = {
			"Content-Type": "application/json",
			Accept: "application/json",
			DPoP: proof,
		};
		if (accessToken) headers["Authorization"] = `DPoP ${accessToken}`;
		let res: Response;
		try {
			res = await this.fetchImpl(url, {
				method: "POST",
				headers,
				body: JSON.stringify(body ?? {}),
			});
		} catch {
			// The original error is dropped on purpose: it may quote the request.
			throw new FieldAuthError("NETWORK", "the platform could not be reached");
		}
		const nonce = res.headers.get("DPoP-Nonce");
		if (nonce) this.nonces.set(new URL(url).origin, nonce);
		let json: Record<string, unknown> | null = null;
		try {
			const text = await res.text();
			if (text) json = JSON.parse(text) as Record<string, unknown>;
		} catch {
			json = null;
		}
		return { status: res.status, headers: res.headers, json };
	}

	private async primeNonce(base: string): Promise<void> {
		const url = base + NONCE_PATH;
		const reply = await this.send(url, {}, await this.proof("POST", url, undefined));
		if (reply.status >= 400) throw replyError(reply, []);
	}

	/** One platform POST with nonce handling (prime, one retry on use_dpop_nonce). */
	private platformPost(
		path: string,
		body: unknown,
		opts: { bearer?: string; secrets?: string[] } = {},
	): Promise<Reply> {
		return this.enqueue(async () => {
			const base = this.requirePlatform();
			const url = base + path;
			const origin = new URL(url).origin;
			if (!this.nonces.get(origin)) await this.primeNonce(base);
			for (let attempt = 0; ; attempt++) {
				const proof = await this.proof("POST", url, this.nonces.get(origin), opts.bearer);
				const reply = await this.send(url, body, proof, opts.bearer);
				if (
					reply.status === 401 &&
					reply.json?.["error"] === "use_dpop_nonce" &&
					attempt === 0
				) {
					continue;
				}
				if (reply.status >= 400) throw replyError(reply, opts.secrets ?? []);
				return reply;
			}
		});
	}

	// ── enrolment ────────────────────────────────────────────────────────

	/**
	 * Enrols this device. `codeOrQr` is the 10-character code or a whole
	 * `fcfield:1:` QR payload (which also sets the platform URL).
	 */
	async enrol(codeOrQr: string, opts: EnrolOptions = {}): Promise<DeviceRecord> {
		await this.restore();
		let code = codeOrQr.trim();
		if (code.startsWith(QR_PREFIX)) {
			const q = parseEnrolmentQr(code);
			this.platformUrl = q.platformUrl;
			code = q.code;
		}
		const base = this.requirePlatform();
		const { key } = await this.publicKey();
		const info = opts.deviceInfo ?? {};
		const nav = (globalThis as { navigator?: { userAgent?: string } }).navigator;
		const body: Record<string, unknown> = {
			code,
			publicKeyJwk: key.jwk,
			platform: (info.platform ?? "web").slice(0, 40),
			model: (info.model ?? nav?.userAgent ?? "unknown").slice(0, 120),
		};
		if (key.attestation.chain?.length) body["attestationChain"] = key.attestation.chain;
		if (opts.label) body["label"] = opts.label;
		const reply = await this.platformPost("/field/devices/enrol", body);
		const j = reply.json ?? {};
		const record: DeviceRecord = {
			deviceId: String(j["deviceId"] ?? ""),
			mode: j["mode"] === "SHARED" ? "SHARED" : "PERSONAL",
			clientId: String(j["clientId"] ?? ""),
			appCodes: Array.isArray(j["appCodes"]) ? (j["appCodes"] as string[]) : [],
			platformUrl: base,
		};
		this.device = record;
		await this.store("set", STORE_DEVICE, JSON.stringify(record));
		return record;
	}

	// ── sign in ──────────────────────────────────────────────────────────

	/** Signs a worker in. The PIN is passed through: never stored, never logged. */
	async login(creds: { workerCode: string; pin: string }): Promise<Session> {
		await this.requireDevice();
		try {
			const reply = await this.platformPost(
				"/field/auth/login",
				{ appCode: this.opts.appCode, workerCode: creds.workerCode, pin: creds.pin },
				{ secrets: [creds.pin] },
			);
			return await this.apply(reply.json as unknown as TokenResponse);
		} catch (e) {
			throw this.noteFailure(e);
		}
	}

	private noteFailure(e: unknown): unknown {
		if (e instanceof FieldAuthError) {
			if (e.code === "WORKER_LOCKED") {
				this.emit<LockedDetail>("locked", { hard: true });
			} else if (e.code === "WORKER_LOCKED_TEMPORARILY" || e.code === "TOO_MANY_ATTEMPTS") {
				const d: LockedDetail = { hard: false };
				if (e.retryAfter !== undefined) d.retryAfter = e.retryAfter;
				this.emit("locked", d);
			}
		}
		return e;
	}

	private async useBiometricStore(): Promise<boolean> {
		if (!this.opts.biometricUnlock || this.device?.mode !== "PERSONAL") return false;
		try {
			return await this.opts.store.biometricAvailable();
		} catch {
			return false;
		}
	}

	/** Stores a token response and publishes the new session. */
	private async apply(t: TokenResponse): Promise<Session> {
		const now = this.clock();
		const prevApp = this.sessionState?.appCode;
		const session: Session = {
			worker: { id: t.worker.id, workerCode: t.worker.workerCode, name: t.worker.name },
			appCode: prevApp ?? this.opts.appCode,
			expiresAt: now + t.expiresIn * 1000,
			idleExpiresAt: Date.parse(t.idleExpiresAt),
			refreshExpiresAt: Date.parse(t.refreshExpiresAt),
			pinChangeRequired: t.pinChangeRequired === true,
		};
		this.access = { token: t.accessToken, expiresAt: session.expiresAt };
		this.refreshToken = t.refreshToken;
		this.sessionState = session;
		await this.store("set", STORE_REFRESH, t.refreshToken, await this.useBiometricStore());
		await this.store("set", STORE_SESSION, JSON.stringify(session));
		this.emit("sessionChanged", session);
		if (session.pinChangeRequired) this.emit("pinChangeRequired", session);
		return session;
	}

	/** Clears local session state and tells listeners. */
	private async endSession(reason: SessionEndedReason): Promise<void> {
		this.access = null;
		this.refreshToken = null;
		this.sessionState = null;
		try {
			await this.opts.store.delete(STORE_REFRESH);
			await this.opts.store.delete(STORE_SESSION);
		} catch {
			// best effort: the in-memory state is already gone
		}
		this.emit("sessionEnded", { reason });
		this.emit("sessionChanged", null);
	}

	// ── refresh ──────────────────────────────────────────────────────────

	/** Rotates the refresh token. Concurrent callers share one request. */
	refresh(): Promise<Session> {
		this.refreshing ??= this.doRefresh().finally(() => {
			this.refreshing = null;
		});
		return this.refreshing;
	}

	private async doRefresh(): Promise<Session> {
		await this.requireDevice();
		let token = this.refreshToken;
		if (!token) {
			try {
				token = await this.store("get", STORE_REFRESH, this.opts.unlockPrompt);
			} catch (e) {
				if (e instanceof FieldAuthError && e.code === "KEY_INVALIDATED") {
					await this.endSession("BIOMETRIC_INVALIDATED");
					throw new FieldAuthError("BIOMETRIC_INVALIDATED", "the biometric key was invalidated; sign in with the PIN");
				}
				throw e;
			}
		}
		if (!token) {
			await this.endSession("SESSION_EXPIRED");
			throw new FieldAuthError("SESSION_EXPIRED", "there is no session to refresh");
		}
		try {
			const reply = await this.platformPost("/field/auth/refresh", { refreshToken: token }, { secrets: [token] });
			return await this.apply(reply.json as unknown as TokenResponse);
		} catch (e) {
			if (e instanceof FieldAuthError && SESSION_ENDING.has(String(e.code))) {
				await this.endSession(String(e.code));
			}
			throw e;
		}
	}

	private async ensureAccess(): Promise<string> {
		if (!this.access || this.access.expiresAt - this.clock() < REFRESH_SKEW_MS) {
			await this.refresh();
		}
		const a = this.access;
		if (!a) throw new FieldAuthError("SESSION_EXPIRED", "there is no session");
		return a.token;
	}

	// ── biometric unlock ─────────────────────────────────────────────────

	/**
	 * Local biometric unlock: reads the stored refresh token (the host shows
	 * the prompt), then refreshes. PERSONAL devices on a biometric store only.
	 */
	async unlock(): Promise<Session> {
		const dev = await this.requireDevice();
		let available = false;
		try {
			available = await this.opts.store.biometricAvailable();
		} catch {
			available = false;
		}
		if (dev.mode !== "PERSONAL" || !available) {
			throw new FieldAuthError("BIOMETRIC_UNAVAILABLE", "biometric unlock is not available on this device");
		}
		let token: string | null;
		try {
			token = await this.opts.store.get(STORE_REFRESH, {
				prompt: this.opts.unlockPrompt ?? "Unlock",
			});
		} catch (e) {
			if (e instanceof FieldProviderError && e.code === "KEY_INVALIDATED") {
				await this.endSession("BIOMETRIC_INVALIDATED");
				throw new FieldAuthError("BIOMETRIC_INVALIDATED", "the biometric key was invalidated; sign in with the PIN");
			}
			throw providerToAuthError(e);
		}
		if (!token) {
			throw new FieldAuthError("SESSION_EXPIRED", "there is no stored session to unlock");
		}
		this.refreshToken = token;
		return this.refresh();
	}

	// ── PIN change, logout ───────────────────────────────────────────────

	/** Changes the PIN, then refreshes (which drops a `pcr` token). */
	async changePin(pins: { currentPin: string; newPin: string }): Promise<Session> {
		await this.requireDevice();
		const call = async (): Promise<void> => {
			const bearer = await this.ensureAccess();
			await this.platformPost(
				"/field/auth/pin",
				{ currentPin: pins.currentPin, newPin: pins.newPin },
				{ bearer, secrets: [pins.currentPin, pins.newPin] },
			);
		};
		try {
			try {
				await call();
			} catch (e) {
				if (!(e instanceof FieldAuthError && e.code === "invalid_token")) throw e;
				this.access = null;
				await call();
			}
		} catch (e) {
			throw this.noteFailure(e);
		}
		return this.refresh();
	}

	/** Ends the session (or every session on this device) and clears local state. */
	async logout(opts: { scope?: "session" | "device-all" } = {}): Promise<void> {
		await this.requireDevice();
		const scope = opts.scope ?? "session";
		let token = this.refreshToken;
		if (!token) {
			try {
				token = await this.store("get", STORE_REFRESH, this.opts.unlockPrompt);
			} catch {
				token = null;
			}
		}
		let failure: unknown = null;
		if (token) {
			try {
				await this.platformPost("/field/auth/logout", { refreshToken: token, scope }, { secrets: [token] });
			} catch (e) {
				failure = e;
			}
		}
		await this.endSession("logout");
		if (failure) throw failure;
	}

	// ── authenticated fetch for application backends ─────────────────────

	/**
	 * `fetch` with `Authorization: DPoP <access>` and a DPoP proof. Refreshes
	 * first when the access token expires within 30 s; retries once on
	 * `use_dpop_nonce`; on `invalid_token` refreshes and retries once, and if
	 * the answer is still `invalid_token` emits `sessionEnded`.
	 */
	async fetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
		await this.requireDevice();
		let refreshed = false;
		if (!this.access || this.access.expiresAt - this.clock() < REFRESH_SKEW_MS) {
			await this.refresh();
			refreshed = true;
		}
		const isReq = typeof Request !== "undefined" && input instanceof Request;
		const raw = isReq ? (input as Request).url : String(input);
		const loc = (globalThis as { location?: { href?: string } }).location;
		const target = new URL(raw, loc?.href);
		const htu = target.origin + target.pathname;
		const htm = (init?.method ?? (isReq ? (input as Request).method : "GET")).toUpperCase();

		let nonceRetried = false;
		let refreshRetried = false;
		for (;;) {
			const token = this.access?.token;
			if (!token) throw new FieldAuthError("SESSION_EXPIRED", "there is no session");
			const proof = await this.proof(htm, htu, this.nonces.get(target.origin), token);
			const headers = new Headers(isReq ? (input as Request).headers : undefined);
			new Headers(init?.headers).forEach((v, k) => headers.set(k, v));
			headers.set("Authorization", `DPoP ${token}`);
			headers.set("DPoP", proof);
			let res: Response;
			try {
				res = await this.fetchImpl(isReq ? (input as Request).clone() : (input as string | URL), {
					...init,
					headers,
				});
			} catch (e) {
				if (e instanceof FieldAuthError) throw e;
				throw new FieldAuthError("NETWORK", "the request could not be sent");
			}
			const n = res.headers.get("DPoP-Nonce");
			if (n) this.nonces.set(target.origin, n);
			if (res.status === 401) {
				const www = wwwAuth(res);
				if (/use_dpop_nonce/.test(www) && !nonceRetried) {
					nonceRetried = true;
					await drain(res);
					continue;
				}
				if (/invalid_token/.test(www)) {
					if (!refreshed && !refreshRetried) {
						refreshRetried = true;
						await drain(res);
						await this.refresh();
						refreshed = true;
						continue;
					}
					this.emit("sessionEnded", { reason: "invalid_token" });
				}
			}
			return res;
		}
	}

	// ── helpers exposed for the app ──────────────────────────────────────

	/** A fresh random id, e.g. for idempotency keys. */
	static newId(): string {
		return randomId(12);
	}
}

async function drain(res: Response): Promise<void> {
	try {
		await res.arrayBuffer();
	} catch {
		// ignore
	}
}

function redact(s: string, secrets: string[]): string {
	let out = s;
	for (const x of secrets) if (x && x.length >= 4) out = out.split(x).join("***");
	return out;
}

function replyError(r: Reply, secrets: string[]): FieldAuthError {
	const j = r.json ?? {};
	const code = typeof j["error"] === "string" ? j["error"] : `HTTP_${r.status}`;
	const message = redact(typeof j["message"] === "string" ? j["message"] : code, secrets);
	const extra: { status: number; retryAfter?: number; violations?: string[] } = { status: r.status };
	const ra = Number.parseInt(r.headers.get("Retry-After") ?? "", 10);
	if (Number.isFinite(ra)) extra.retryAfter = ra;
	if (Array.isArray(j["violations"])) extra.violations = j["violations"].map(String);
	return new FieldAuthError(code, message, extra);
}

function providerToAuthError(e: unknown): FieldAuthError {
	if (e instanceof FieldAuthError) return e;
	const code = (e as { code?: unknown } | null)?.code;
	if (isProviderErrorCode(code)) return new FieldAuthError(code, `secure storage failed: ${code}`);
	return new FieldAuthError("INTERNAL", "secure storage failed");
}
