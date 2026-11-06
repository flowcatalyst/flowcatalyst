// FieldClient: the React Native face of the Android FieldSession (field-js FieldAuth method names, Session shape).
// No EventTarget, no DOM, no crypto.subtle: it runs on Hermes and only talks to the native module.
import { getNativeModule, type FcFieldNative } from "./native";

// ── shapes (mirror clients/field-js/src/auth.ts) ─────────────────────────

export type DeviceMode = "PERSONAL" | "SHARED" | "UNENROLLED";

export interface Actor {
	id: string;
	type: "field_worker" | "principal" | (string & {});
	name: string;
}

/** All times are ms since the epoch on the device clock. */
export interface Session {
	worker: { id: string; workerCode: string; name: string };
	actor: Actor | null;
	test: boolean;
	deviceMode: DeviceMode;
	deviceId: string | null;
	appCode: string;
	expiresAt: number;
	idleExpiresAt: number;
	refreshExpiresAt: number;
	pinChangeRequired: boolean;
}

export interface DeviceRecord {
	deviceId: string;
	mode: "PERSONAL" | "SHARED";
	clientId: string;
	appCodes: string[];
	platformUrl: string;
}

export interface DeviceInfo {
	platform?: string;
	model?: string;
}

export interface EnrolOptions {
	label?: string;
	deviceInfo?: DeviceInfo;
}

export type LoginCredentials =
	| { workerCode: string; pin: string; badgeToken?: never; clientIdentifier?: string }
	| { badgeToken: string; pin?: string; workerCode?: never; clientIdentifier?: string };

export type ActAsTarget =
	| { workerCode: string; workerId?: never; pin: string }
	| { workerId: string; workerCode?: never; pin: string };

export interface UnlockPolicy {
	/** Lock a restored session after a process start. */
	afterColdStart?: boolean;
	/** Lock after the app has been in the background this many ms. */
	afterBackgroundMs?: number;
}

export interface FieldClientConfig {
	platformUrl: string;
	appCode: string;
	clientIdentifier?: string;
	requireUnlock?: UnlockPolicy;
	/** Default [30 min, 10 min]. */
	expiryWarningsMs?: number[];
	/** keepAlive refreshes this long before the idle deadline. Default 2 min. */
	keepAliveLeadMs?: number;
}

export type FieldState = "SignedOut" | "SignedIn" | "Locked";

export interface RestoreResult {
	state: FieldState;
	session: Session | null;
	device: DeviceRecord | null;
}

export interface FieldEvents {
	/** `session` is null on sign-out. */
	SessionChanged: { session: Session | null };
	/** `reason`: "logout", "invalid_token" or a platform code (INVALID_GRANT, SESSION_EXPIRED, DEVICE_REVOKED, ...). */
	SessionEnded: { reason: string };
	PinChangeRequired: { session: Session };
	ExpiryWarning: { remainingMs: number };
	Locked: { session: Session };
	/** `retryAfterSeconds` is absent for a hard lock. */
	WorkerLocked: { hard: boolean; retryAfterSeconds?: number };
}
export type FieldEventName = keyof FieldEvents;

export const FIELD_EVENT_NAMES: readonly FieldEventName[] = [
	"SessionChanged",
	"SessionEnded",
	"PinChangeRequired",
	"ExpiryWarning",
	"Locked",
	"WorkerLocked",
];

// ── errors ───────────────────────────────────────────────────────────────

/**
 * Every failure of a session method. `code` is the field-js code: a platform code (INVALID_CREDENTIALS,
 * INVALID_GRANT, SESSION_EXPIRED, DEVICE_REVOKED, ...), a lower-case DPoP code (invalid_token, use_dpop_nonce),
 * a key/biometric code (KEY_INVALIDATED, BIOMETRIC_UNAVAILABLE, BIOMETRIC_CANCELLED, ...), or the library's own
 * (NETWORK, NOT_ENROLLED, IMPERSONATING, NOT_CONFIGURED, NOT_SUPPORTED_ON_IOS_YET, INTERNAL). Never carries a PIN.
 */
export class FieldAuthError extends Error {
	readonly code: string;
	readonly status?: number;
	readonly retryAfterSeconds?: number;
	readonly violations: string[];
	constructor(
		code: string,
		message?: string,
		extra: { status?: number; retryAfterSeconds?: number; violations?: string[] } = {},
	) {
		super(message ?? code);
		this.name = "FieldAuthError";
		this.code = code;
		if (extra.status !== undefined) this.status = extra.status;
		if (extra.retryAfterSeconds !== undefined) this.retryAfterSeconds = extra.retryAfterSeconds;
		this.violations = extra.violations ?? [];
	}
}

/** Native rejection -> FieldAuthError. The native `code` passes through; anything without one is INTERNAL. */
export function mapNativeError(e: unknown): FieldAuthError {
	if (e instanceof FieldAuthError) return e;
	const err = e as { code?: unknown; message?: unknown; userInfo?: unknown } | null;
	const code = typeof err?.code === "string" && err.code !== "" ? err.code : "INTERNAL";
	const message = typeof err?.message === "string" ? err.message : "native error";
	const ui = (err?.userInfo ?? {}) as { status?: unknown; retryAfterSeconds?: unknown; violations?: unknown };
	const extra: { status?: number; retryAfterSeconds?: number; violations?: string[] } = {};
	if (typeof ui.status === "number") extra.status = ui.status;
	if (typeof ui.retryAfterSeconds === "number") extra.retryAfterSeconds = ui.retryAfterSeconds;
	if (Array.isArray(ui.violations)) extra.violations = ui.violations.filter((v): v is string => typeof v === "string");
	return new FieldAuthError(code, message, extra);
}

// ── client ───────────────────────────────────────────────────────────────

export interface NativeEventSource {
	addListener(eventName: string, listener: (payload: unknown) => void): { remove(): void };
}

export interface FieldClientOptions {
	/** Defaults to the linked TurboModule. */
	native?: FcFieldNative;
	/** Defaults to `new NativeEventEmitter(native)`. */
	events?: NativeEventSource;
	/** Defaults to react-native's Platform.OS. */
	platform?: string;
	/** Defaults to globalThis.fetch. */
	fetch?: (url: string, init?: RequestInit) => Promise<Response>;
}

export type FieldHeaders = { Authorization: string; DPoP: string } & Record<string, string>;

declare const require: (id: string) => unknown;

function defaultPlatform(): string {
	const rn = require("react-native") as { Platform: { OS: string } };
	return rn.Platform.OS;
}

function defaultEvents(native: FcFieldNative): NativeEventSource {
	const rn = require("react-native") as {
		NativeEventEmitter: new (m: unknown) => NativeEventSource;
	};
	return new rn.NativeEventEmitter(native);
}

function headersToRecord(h: RequestInit["headers"] | undefined): Record<string, string> {
	const out: Record<string, string> = {};
	if (!h) return out;
	if (Array.isArray(h)) {
		for (const [k, v] of h) if (k !== undefined && v !== undefined) out[k] = v;
	} else if (typeof (h as { forEach?: unknown }).forEach === "function") {
		(h as { forEach(cb: (v: string, k: string) => void): void }).forEach((v, k) => {
			out[k] = v;
		});
	} else {
		Object.assign(out, h);
	}
	return out;
}

function dropKey(h: Record<string, string>, name: string): void {
	const lower = name.toLowerCase();
	for (const k of Object.keys(h)) if (k.toLowerCase() === lower) delete h[k];
}

export class FieldClient {
	private readonly config: FieldClientConfig;
	private readonly opts: FieldClientOptions;
	private configured: Promise<FcFieldNative> | null = null;
	private current: Session | null = null;
	private currentState: FieldState = "SignedOut";
	private readonly listeners = new Map<FieldEventName, Set<(payload: never) => void>>();
	private subscriptions: { remove(): void }[] = [];

	constructor(config: FieldClientConfig, opts: FieldClientOptions = {}) {
		this.config = config;
		this.opts = opts;
	}

	/** The last session this client saw (null when signed out or before [restore]). */
	get session(): Session | null {
		return this.current;
	}

	get state(): FieldState {
		return this.currentState;
	}

	// ── lifecycle ────────────────────────────────────────────────────────

	/** Loads the stored device and session; also configures the native owner on first use. */
	async restore(): Promise<RestoreResult> {
		const n = await this.native();
		return this.call(async () => this.track((await n.restore()) as RestoreResult));
	}

	async enrol(codeOrQr: string, opts: EnrolOptions = {}): Promise<DeviceRecord> {
		const n = await this.native();
		const info: Record<string, unknown> = { ...opts.deviceInfo };
		if (opts.label !== undefined) info["label"] = opts.label;
		return this.call(async () => (await n.enrol(codeOrQr, info)) as DeviceRecord);
	}

	async login(creds: LoginCredentials): Promise<Session> {
		const n = await this.native();
		return this.call(async () => this.took((await n.login({ ...creds })) as Session));
	}

	async redeemSignInCode(code: string): Promise<Session> {
		const n = await this.native();
		return this.call(async () => this.took((await n.redeemSignInCode(code)) as Session));
	}

	async actAs(target: ActAsTarget): Promise<Session> {
		const n = await this.native();
		return this.call(async () => this.took((await n.actAs({ ...target })) as Session));
	}

	async endImpersonation(): Promise<Session | null> {
		const n = await this.native();
		return this.call(async () => {
			const r = (await n.endImpersonation()) as { session?: Session | null };
			const s = r.session ?? null;
			this.current = s;
			return s;
		});
	}

	/** Changes the PIN, then refreshes (which drops a pinChangeRequired token). */
	async changePin(pins: { currentPin: string; newPin: string }): Promise<Session> {
		const n = await this.native();
		return this.call(async () => {
			await n.changePin(pins.currentPin, pins.newPin);
			return this.took((await n.refresh()) as Session);
		});
	}

	async logout(opts: { scope?: "session" | "device-all" } = {}): Promise<void> {
		const n = await this.native();
		return this.call(async () => {
			await n.logout(opts.scope ?? "session");
			this.current = null;
			this.currentState = "SignedOut";
		});
	}

	/** App unlock (biometric / device credential). Only when the state is "Locked". */
	async unlock(prompt = "Unlock"): Promise<Session> {
		const n = await this.native();
		return this.call(async () => {
			const s = this.took((await n.unlock(prompt)) as Session);
			this.currentState = "SignedIn";
			return s;
		});
	}

	async refresh(): Promise<Session> {
		const n = await this.native();
		return this.call(async () => this.took((await n.refresh()) as Session));
	}

	/** Declare the worker busy (a trip / tracking is active): the owner refreshes before the idle deadline. */
	async keepAlive(on: boolean): Promise<void> {
		const n = await this.native();
		return this.call(async () => n.keepAlive(on));
	}

	// ── proofs for other code ────────────────────────────────────────────

	/** Authorization + DPoP for one request; refreshes first if needed. Sign when sending, never ahead of time. */
	async headersFor(method: string, url: string): Promise<FieldHeaders> {
		const n = await this.native();
		return this.call(async () => (await n.headersFor(method.toUpperCase(), url)) as FieldHeaders);
	}

	/** Sign again after a `401 use_dpop_nonce` (pass the response's `DPoP-Nonce`). */
	async headersForRetry(method: string, url: string, nonce: string): Promise<FieldHeaders> {
		const n = await this.native();
		return this.call(async () => (await n.headersForRetry(method.toUpperCase(), url, nonce)) as FieldHeaders);
	}

	/** Feed back a `DPoP-Nonce` seen on any response for this origin. */
	async noteNonce(url: string, nonce: string): Promise<void> {
		const n = await this.native();
		return this.call(async () => n.noteNonce(url, nonce));
	}

	/**
	 * `fetch` with `Authorization: DPoP` and a proof. Retries once on `401 use_dpop_nonce` with the response's
	 * `DPoP-Nonce`, and remembers every `DPoP-Nonce` it sees. Network failures reject with code NETWORK.
	 */
	async fetch(url: string | URL, init: RequestInit = {}): Promise<Response> {
		const target = String(url);
		const method = (init.method ?? "GET").toUpperCase();
		const doFetch = this.opts.fetch ?? ((u: string, i?: RequestInit) => globalThis.fetch(u, i));
		const send = async (signed: Record<string, string>): Promise<Response> => {
			const headers = headersToRecord(init.headers);
			dropKey(headers, "Authorization");
			dropKey(headers, "DPoP");
			Object.assign(headers, signed);
			try {
				return await doFetch(target, { ...init, headers });
			} catch {
				throw new FieldAuthError("NETWORK", "the request could not be sent");
			}
		};
		let res = await send(await this.headersFor(method, target));
		const nonce = res.headers.get("DPoP-Nonce");
		if (nonce) await this.noteNonce(target, nonce);
		if (res.status === 401 && nonce && /use_dpop_nonce/.test(res.headers.get("WWW-Authenticate") ?? "")) {
			res = await send(await this.headersForRetry(method, target, nonce));
			const n2 = res.headers.get("DPoP-Nonce");
			if (n2) await this.noteNonce(target, n2);
		}
		return res;
	}

	// ── events ───────────────────────────────────────────────────────────

	/** Subscribe to a native event. Returns the unsubscribe function. */
	on<E extends FieldEventName>(event: E, cb: (payload: FieldEvents[E]) => void): () => void {
		let set = this.listeners.get(event);
		if (!set) {
			set = new Set();
			this.listeners.set(event, set);
		}
		const entry = cb as (payload: never) => void;
		set.add(entry);
		// Configures the native owner (and subscribes to its events); failures surface on the next method call.
		this.native().then((n) => this.subscribe(n), () => {});
		return () => {
			set.delete(entry);
		};
	}

	/** Drops all native subscriptions and listeners. */
	dispose(): void {
		for (const s of this.subscriptions) s.remove();
		this.subscriptions = [];
		this.listeners.clear();
	}

	// ── internals ────────────────────────────────────────────────────────

	private subscribe(n: FcFieldNative): void {
		if (this.subscriptions.length > 0) return;
		const src = this.opts.events ?? defaultEvents(n);
		for (const name of FIELD_EVENT_NAMES) {
			this.subscriptions.push(src.addListener(name, (p) => this.dispatch(name, p)));
		}
	}

	private dispatch(name: FieldEventName, payload: unknown): void {
		const p = (payload ?? {}) as Record<string, unknown>;
		if (name === "SessionChanged") this.current = (p["session"] as Session | null | undefined) ?? null;
		if (name === "SessionEnded") {
			this.current = null;
			this.currentState = "SignedOut";
		}
		if (name === "Locked") this.currentState = "Locked";
		const set = this.listeners.get(name);
		if (!set) return;
		for (const cb of [...set]) {
			try {
				(cb as (x: unknown) => void)(payload);
			} catch {
				// a listener must not break delivery to the others
			}
		}
	}

	private native(): Promise<FcFieldNative> {
		if (!this.configured) {
			this.configured = (async () => {
				const platform = this.opts.platform ?? defaultPlatform();
				if (platform === "ios") {
					throw new FieldAuthError(
						"NOT_SUPPORTED_ON_IOS_YET",
						"FieldClient sessions are Android-only for now; the Swift session owner is not built yet",
					);
				}
				const n = this.opts.native ?? getNativeModule();
				await n.configure({ ...this.config });
				this.subscribe(n);
				return n;
			})();
			this.configured.catch(() => {
				this.configured = null; // retry on the next call
			});
		}
		return this.configured.catch((e) => {
			throw mapNativeError(e);
		});
	}

	private async call<T>(fn: () => Promise<T>): Promise<T> {
		try {
			return await fn();
		} catch (e) {
			throw mapNativeError(e);
		}
	}

	private took(s: Session): Session {
		this.current = s;
		if (this.currentState === "SignedOut") this.currentState = "SignedIn";
		return s;
	}

	private track(r: RestoreResult): RestoreResult {
		this.current = r.session ?? null;
		this.currentState = r.state;
		return r;
	}
}
