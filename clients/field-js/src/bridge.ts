// The page side of the WebView bridge, protocol version 1
// (docs/field-mobile-libraries-spec.md section 3).

import { b64uDecode, b64uEncode } from "./encoding.js";
import { FieldProviderError, isProviderErrorCode } from "./errors.js";
import { IndexedDbSecureStore, WebCryptoKeyProvider } from "./storage.js";
import type { DeviceKey, KeyProvider, SecureStore } from "./types.js";

/** A request the page may take 60 s over (a biometric prompt can be slow). */
export const BRIDGE_TIMEOUT_MS = 60_000;

interface BridgeHost {
	version?: number;
	/** Installed by the host; never replaced by the page. */
	receive?: (msg: string) => unknown;
	/** Queue of responses that arrived before a listener was set (may be absent). */
	_q?: unknown[];
	/** The page's listener (may be absent). */
	_l?: ((msg: string) => void) | null;
}

/** The slice of `window` the bridge uses (injectable for tests). */
export interface BridgeGlobal {
	ReactNativeWebView?: { postMessage(msg: string): void };
	__fcFieldBridge?: BridgeHost;
}

/** True when the page runs inside a host that installed the bridge. */
export function bridgeAvailable(g: BridgeGlobal = globalThis as BridgeGlobal): boolean {
	return (
		typeof g.__fcFieldBridge === "object" &&
		g.__fcFieldBridge !== null &&
		g.__fcFieldBridge.version === 1 &&
		typeof g.ReactNativeWebView?.postMessage === "function"
	);
}

interface Pending {
	resolve(v: unknown): void;
	reject(e: unknown): void;
	timer: ReturnType<typeof setTimeout>;
}

export interface BridgeClientOptions {
	global?: BridgeGlobal;
	timeoutMs?: number;
	/** Request id prefix (tests); random by default. */
	idPrefix?: string;
}

/**
 * Request/response correlation over `ReactNativeWebView.postMessage` and
 * `__fcFieldBridge`. The page sets `__fcFieldBridge._l` (never replaces
 * `receive`) and drains `_q`; both may be absent until the host first needs
 * them.
 */
export class BridgeClient {
	private readonly g: BridgeGlobal;
	private readonly timeoutMs: number;
	private readonly pending = new Map<string, Pending>();
	private seq = 0;
	private hooked = false;
	private readonly nonce: string;

	constructor(opts: BridgeClientOptions = {}) {
		this.g = opts.global ?? (globalThis as BridgeGlobal);
		this.timeoutMs = opts.timeoutMs ?? BRIDGE_TIMEOUT_MS;
		this.nonce = opts.idPrefix ?? Math.random().toString(36).slice(2, 10);
	}

	/**
	 * Sets the host's `_l` listener (never replaces `receive`) and handles
	 * anything the host queued in `_q` before the listener existed.
	 */
	private hook(): void {
		const host = this.g.__fcFieldBridge;
		if (!host) return;
		if (!this.hooked || host._l == null) {
			host._l = (msg: string) => this.receive(msg);
			this.hooked = true;
		}
		const q = host._q || [];
		host._q = [];
		for (const m of q) this.receive(m);
	}

	/** Handles one host message; ignores anything that is not ours. */
	receive(raw: unknown): void {
		let m: unknown;
		try {
			m = typeof raw === "string" ? JSON.parse(raw) : raw;
		} catch {
			return;
		}
		if (!m || typeof m !== "object") return;
		const r = m as {
			fcField?: unknown;
			id?: unknown;
			ok?: unknown;
			result?: unknown;
			error?: { code?: unknown; message?: unknown };
		};
		if (r.fcField !== 1 || typeof r.id !== "string") return;
		const p = this.pending.get(r.id);
		if (!p) return;
		this.pending.delete(r.id);
		clearTimeout(p.timer);
		if (r.ok === true) {
			p.resolve(r.result ?? {});
			return;
		}
		const code = isProviderErrorCode(r.error?.code) ? r.error.code : "INTERNAL";
		p.reject(
			new FieldProviderError(
				code,
				typeof r.error?.message === "string" ? r.error.message : code,
			),
		);
	}

	call<T>(op: string, args: Record<string, unknown> = {}): Promise<T> {
		const post = this.g.ReactNativeWebView?.postMessage;
		if (!bridgeAvailable(this.g) || !post) {
			return Promise.reject(
				new FieldProviderError("INTERNAL", "the WebView bridge is not installed"),
			);
		}
		const id = `${this.nonce}-${++this.seq}`;
		return new Promise<T>((resolve, reject) => {
			const timer = setTimeout(() => {
				this.pending.delete(id);
				reject(new FieldProviderError("INTERNAL", `bridge request ${op} timed out`));
			}, this.timeoutMs);
			this.pending.set(id, { resolve: resolve as (v: unknown) => void, reject, timer });
			this.hook();
			try {
				this.g.ReactNativeWebView?.postMessage(
					JSON.stringify({ fcField: 1, id, op, args }),
				);
			} catch {
				this.pending.delete(id);
				clearTimeout(timer);
				reject(new FieldProviderError("INTERNAL", "could not post to the host"));
			}
		});
	}
}

let shared: BridgeClient | null = null;
function sharedClient(): BridgeClient {
	shared ??= new BridgeClient();
	return shared;
}

export class BridgeKeyProvider implements KeyProvider {
	private readonly client: BridgeClient;
	constructor(client?: BridgeClient) {
		this.client = client ?? sharedClient();
	}
	getOrCreateDeviceKey(): Promise<DeviceKey> {
		return this.client.call<DeviceKey>("key.getOrCreate");
	}
	async sign(data: Uint8Array): Promise<Uint8Array> {
		const r = await this.client.call<{ signature: string }>("key.sign", {
			data: b64uEncode(data),
		});
		const sig = b64uDecode(r.signature);
		if (sig.length !== 64) {
			throw new FieldProviderError("INTERNAL", "the host returned a malformed signature");
		}
		return sig;
	}
	async deleteDeviceKey(): Promise<void> {
		await this.client.call("key.delete");
	}
}

export class BridgeSecureStore implements SecureStore {
	private readonly client: BridgeClient;
	constructor(client?: BridgeClient) {
		this.client = client ?? sharedClient();
	}
	async get(key: string, opts?: { prompt?: string }): Promise<string | null> {
		const args: Record<string, unknown> = { key };
		if (opts?.prompt !== undefined) args["prompt"] = opts.prompt;
		const r = await this.client.call<{ value: string | null }>("store.get", args);
		return typeof r.value === "string" ? r.value : null;
	}
	async set(
		key: string,
		value: string,
		opts?: { requireBiometric?: boolean },
	): Promise<void> {
		const args: Record<string, unknown> = { key, value };
		if (opts?.requireBiometric) args["requireBiometric"] = true;
		await this.client.call("store.set", args);
	}
	async delete(key: string): Promise<void> {
		await this.client.call("store.delete", { key });
	}
	async biometricAvailable(): Promise<boolean> {
		const r = await this.client.call<{ available: boolean }>("bio.available");
		return r.available === true;
	}
}

/** The bridge providers inside a host, else WebCrypto + IndexedDB. */
export function defaultProviders(): { keys: KeyProvider; store: SecureStore } {
	if (bridgeAvailable()) {
		const c = sharedClient();
		return { keys: new BridgeKeyProvider(c), store: new BridgeSecureStore(c) };
	}
	return { keys: new WebCryptoKeyProvider(), store: new IndexedDbSecureStore() };
}
