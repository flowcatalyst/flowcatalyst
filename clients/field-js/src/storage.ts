import { FieldProviderError } from "./errors.js";
import type {
	DeviceKey,
	KeyProvider,
	PublicEcJwk,
	SecureStore,
} from "./types.js";

/** The little async key/value surface both IndexedDB-backed classes use. */
export interface KeyValueDb {
	get(key: string): Promise<unknown>;
	put(key: string, value: unknown): Promise<void>;
	delete(key: string): Promise<void>;
}

export class MemoryDb implements KeyValueDb {
	private readonly m = new Map<string, unknown>();
	async get(key: string): Promise<unknown> {
		return this.m.get(key);
	}
	async put(key: string, value: unknown): Promise<void> {
		this.m.set(key, value);
	}
	async delete(key: string): Promise<void> {
		this.m.delete(key);
	}
}

/** One object store in one IndexedDB database. */
export class IdbDb implements KeyValueDb {
	private dbp: Promise<IDBDatabase> | null = null;
	private readonly dbName: string;
	private readonly storeName: string;
	private readonly factory: IDBFactory | undefined;
	constructor(dbName: string, storeName: string, factory?: IDBFactory) {
		this.dbName = dbName;
		this.storeName = storeName;
		this.factory = factory;
	}

	private open(): Promise<IDBDatabase> {
		this.dbp ??= new Promise((resolve, reject) => {
			const f = this.factory ?? globalThis.indexedDB;
			if (!f) {
				reject(new FieldProviderError("INTERNAL", "indexedDB is unavailable"));
				return;
			}
			const req = f.open(this.dbName, 1);
			req.onupgradeneeded = () => {
				req.result.createObjectStore(this.storeName);
			};
			req.onsuccess = () => resolve(req.result);
			req.onerror = () =>
				reject(new FieldProviderError("INTERNAL", "could not open indexedDB"));
		});
		return this.dbp;
	}

	private async run<T>(
		mode: IDBTransactionMode,
		fn: (s: IDBObjectStore) => IDBRequest<T>,
	): Promise<T> {
		const db = await this.open();
		return new Promise<T>((resolve, reject) => {
			const tx = db.transaction(this.storeName, mode);
			const req = fn(tx.objectStore(this.storeName));
			tx.oncomplete = () => resolve(req.result);
			tx.onerror = tx.onabort = () =>
				reject(new FieldProviderError("INTERNAL", "indexedDB transaction failed"));
		});
	}

	get(key: string): Promise<unknown> {
		return this.run("readonly", (s) => s.get(key));
	}
	async put(key: string, value: unknown): Promise<void> {
		await this.run("readwrite", (s) => s.put(value, key));
	}
	async delete(key: string): Promise<void> {
		await this.run("readwrite", (s) => s.delete(key));
	}
}

const DEVICE_KEY = "device-key";

interface StoredKey {
	privateKey: CryptoKey;
	jwk: PublicEcJwk;
}

export interface WebCryptoKeyProviderOptions {
	/** Defaults to IndexedDB database `fc-field-keys`. */
	db?: KeyValueDb;
}

/**
 * A non-extractable ECDSA P-256 key kept in IndexedDB (a CryptoKey survives
 * structured clone). Attestation level SOFTWARE.
 */
export class WebCryptoKeyProvider implements KeyProvider {
	private readonly db: KeyValueDb;
	private pending: Promise<StoredKey> | null = null;

	constructor(opts: WebCryptoKeyProviderOptions = {}) {
		this.db = opts.db ?? new IdbDb("fc-field-keys", "keys");
	}

	private load(): Promise<StoredKey> {
		this.pending ??= (async () => {
			const found = (await this.db.get(DEVICE_KEY)) as StoredKey | undefined;
			if (found?.privateKey && found.jwk) return found;
			const pair = await crypto.subtle.generateKey(
				{ name: "ECDSA", namedCurve: "P-256" },
				false,
				["sign", "verify"],
			);
			const raw = (await crypto.subtle.exportKey(
				"jwk",
				pair.publicKey,
			)) as JsonWebKey;
			const jwk: PublicEcJwk = {
				kty: "EC",
				crv: "P-256",
				x: raw.x ?? "",
				y: raw.y ?? "",
			};
			const stored: StoredKey = { privateKey: pair.privateKey, jwk };
			await this.db.put(DEVICE_KEY, stored);
			return stored;
		})().catch((e) => {
			this.pending = null;
			throw e instanceof FieldProviderError
				? e
				: new FieldProviderError("INTERNAL", "could not load the device key");
		});
		return this.pending;
	}

	async getOrCreateDeviceKey(): Promise<DeviceKey> {
		const k = await this.load();
		return { jwk: { ...k.jwk }, attestation: { level: "SOFTWARE" } };
	}

	async sign(data: Uint8Array): Promise<Uint8Array> {
		const k = await this.load();
		const sig = await crypto.subtle.sign(
			{ name: "ECDSA", hash: "SHA-256" },
			k.privateKey,
			data as BufferSource,
		);
		return new Uint8Array(sig); // WebCrypto already returns raw r‖s
	}

	async deleteDeviceKey(): Promise<void> {
		this.pending = null;
		await this.db.delete(DEVICE_KEY);
	}
}

export interface IndexedDbSecureStoreOptions {
	db?: KeyValueDb;
}

/**
 * Plain key/value storage in IndexedDB. No biometric support:
 * `set(..., {requireBiometric: true})` throws BIOMETRIC_UNAVAILABLE.
 */
export class IndexedDbSecureStore implements SecureStore {
	private readonly db: KeyValueDb;
	constructor(opts: IndexedDbSecureStoreOptions = {}) {
		this.db = opts.db ?? new IdbDb("fc-field-store", "values");
	}
	async get(key: string): Promise<string | null> {
		const v = await this.db.get(key);
		return typeof v === "string" ? v : null;
	}
	async set(
		key: string,
		value: string,
		opts?: { requireBiometric?: boolean },
	): Promise<void> {
		if (opts?.requireBiometric) throw new FieldProviderError("BIOMETRIC_UNAVAILABLE");
		await this.db.put(key, value);
	}
	async delete(key: string): Promise<void> {
		await this.db.delete(key);
	}
	async biometricAvailable(): Promise<boolean> {
		return false;
	}
}

/** For tests. Same behaviour as [IndexedDbSecureStore], in memory. */
export class MemorySecureStore implements SecureStore {
	private readonly m = new Map<string, string>();
	async get(key: string): Promise<string | null> {
		return this.m.get(key) ?? null;
	}
	async set(
		key: string,
		value: string,
		opts?: { requireBiometric?: boolean },
	): Promise<void> {
		if (opts?.requireBiometric) throw new FieldProviderError("BIOMETRIC_UNAVAILABLE");
		this.m.set(key, value);
	}
	async delete(key: string): Promise<void> {
		this.m.delete(key);
	}
	async biometricAvailable(): Promise<boolean> {
		return false;
	}
	/** Test hook: everything stored, to assert nothing secret leaks. */
	dump(): Record<string, string> {
		return Object.fromEntries(this.m);
	}
}
