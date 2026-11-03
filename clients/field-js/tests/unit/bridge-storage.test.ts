import { strict as assert } from "node:assert";
import { describe, it } from "node:test";
import "fake-indexeddb/auto";
import { webcrypto } from "node:crypto";
import {
	BridgeClient,
	BridgeKeyProvider,
	BridgeSecureStore,
	FieldProviderError,
	IdbDb,
	IndexedDbSecureStore,
	MemoryDb,
	MemorySecureStore,
	WebCryptoKeyProvider,
	bridgeAvailable,
	type BridgeGlobal,
	type KeyProvider,
	type SecureStore,
} from "../../src/index.js";
import { b64uDecode, b64uEncode } from "../../src/encoding.js";

/** The RN host's stub, exactly: receive queues until the page sets _l. */
function hostStub(lazy = false): NonNullable<BridgeGlobal["__fcFieldBridge"]> {
	const b: Record<string, unknown> = { version: 1 };
	if (!lazy) {
		b["_q"] = [];
		b["_l"] = null;
	}
	b["receive"] = function (this: Record<string, unknown>, m: string) {
		if (this["_l"]) (this["_l"] as (m: string) => void)(m);
		else (this["_q"] ??= [] as unknown[]), (this["_q"] as unknown[]).push(m);
	};
	return b as NonNullable<BridgeGlobal["__fcFieldBridge"]>;
}

/** A host that implements section 3 using a real key provider and store. */
function fakeHost(keys: KeyProvider, store: SecureStore, opts: { silent?: boolean } = {}) {
	const g: BridgeGlobal = {
		__fcFieldBridge: hostStub(),
		ReactNativeWebView: {
			postMessage(raw: string) {
				const m = JSON.parse(raw) as { fcField: number; id: string; op: string; args: Record<string, string> };
				if (opts.silent) return;
				void (async () => {
					const reply = async (): Promise<unknown> => {
						const a = m.args;
						switch (m.op) {
							case "key.getOrCreate":
								return keys.getOrCreateDeviceKey();
							case "key.sign":
								return { signature: b64uEncode(await keys.sign(b64uDecode(a["data"] ?? ""))) };
							case "key.delete":
								await keys.deleteDeviceKey();
								return {};
							case "store.get":
								return { value: await store.get(a["key"] ?? "", a["prompt"] ? { prompt: a["prompt"] } : undefined) };
							case "store.set":
								await store.set(a["key"] ?? "", a["value"] ?? "", a["requireBiometric"] ? { requireBiometric: true } : undefined);
								return {};
							case "store.delete":
								await store.delete(a["key"] ?? "");
								return {};
							case "bio.available":
								return { available: await store.biometricAvailable() };
							default:
								throw new FieldProviderError("INTERNAL", "unknown op");
						}
					};
					let out: unknown;
					try {
						out = { fcField: 1, id: m.id, ok: true, result: await reply() };
					} catch (e) {
						const code = (e as { code?: string }).code ?? "INTERNAL";
						out = { fcField: 1, id: m.id, ok: false, error: { code, message: String(code) } };
					}
					queueMicrotask(() => g.__fcFieldBridge?.receive?.call(g.__fcFieldBridge, JSON.stringify(out)));
				})();
			},
		},
	};
	return g;
}

describe("bridge", () => {
	it("bridgeAvailable needs version 1 and postMessage", () => {
		assert.equal(bridgeAvailable({}), false);
		assert.equal(bridgeAvailable({ __fcFieldBridge: { version: 1 } }), false);
		assert.equal(bridgeAvailable(fakeHost(new WebCryptoKeyProvider({ db: new MemoryDb() }), new MemorySecureStore())), true);
	});

	it("round-trips every op against a host", async () => {
		const hostKeys = new WebCryptoKeyProvider({ db: new MemoryDb() });
		const hostStore = new MemorySecureStore();
		const g = fakeHost(hostKeys, hostStore);
		const client = new BridgeClient({ global: g });
		const keys = new BridgeKeyProvider(client);
		const store = new BridgeSecureStore(client);

		const k = await keys.getOrCreateDeviceKey();
		assert.equal(k.jwk.crv, "P-256");
		const sig = await keys.sign(new TextEncoder().encode("hello"));
		assert.equal(sig.length, 64);
		const pub = await webcrypto.subtle.importKey("jwk", k.jwk, { name: "ECDSA", namedCurve: "P-256" }, false, ["verify"]);
		assert.ok(await webcrypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, pub, sig as BufferSource, new TextEncoder().encode("hello")));

		await store.set("a", "va");
		assert.equal(await store.get("a", { prompt: "p" }), "va");
		assert.equal(await store.get("missing"), null);
		await store.delete("a");
		assert.equal(await store.get("a"), null);
		assert.equal(await store.biometricAvailable(), false);
		await keys.deleteDeviceKey();
	});

	it("maps host errors to codes and unknown codes to INTERNAL", async () => {
		const g = fakeHost(new WebCryptoKeyProvider({ db: new MemoryDb() }), new MemorySecureStore());
		const client = new BridgeClient({ global: g });
		await assert.rejects(new BridgeSecureStore(client).set("k", "v", { requireBiometric: true }), (e: unknown) => e instanceof FieldProviderError && e.code === "BIOMETRIC_UNAVAILABLE");
		await assert.rejects(client.call("nope"), (e: unknown) => e instanceof FieldProviderError && e.code === "INTERNAL");
	});

	it("times out when the host never answers", async () => {
		const g = fakeHost(new WebCryptoKeyProvider({ db: new MemoryDb() }), new MemorySecureStore(), { silent: true });
		const client = new BridgeClient({ global: g, timeoutMs: 20 });
		await assert.rejects(new BridgeKeyProvider(client).getOrCreateDeviceKey(), /timed out/);
	});

	it("ignores messages that are not ours and answers for a different id", async () => {
		const g = fakeHost(new WebCryptoKeyProvider({ db: new MemoryDb() }), new MemorySecureStore(), { silent: true });
		const client = new BridgeClient({ global: g, timeoutMs: 30 });
		const p = new BridgeSecureStore(client).get("x");
		g.__fcFieldBridge?.receive?.(JSON.stringify({ hello: "app message" }));
		g.__fcFieldBridge?.receive?.("not json");
		g.__fcFieldBridge?.receive?.(JSON.stringify({ fcField: 1, id: "other", ok: true, result: { value: "v" } }));
		await assert.rejects(p, /timed out/);
	});

	it("rejects when no bridge is installed", async () => {
		await assert.rejects(new BridgeClient({ global: {} }).call("key.delete"), /not installed/);
	});

	for (const lazy of [false, true]) {
		it(`sets _l, never replaces receive, and drains _q (${lazy ? "lazy" : "eager"} stub)`, async () => {
			const g = fakeHost(new WebCryptoKeyProvider({ db: new MemoryDb() }), new MemorySecureStore(), { silent: true });
			const stub = hostStub(lazy);
			g.__fcFieldBridge = stub;
			const receive = stub.receive;
			// a response that arrived before the page listened
			stub.receive?.call(stub, JSON.stringify({ fcField: 1, id: "pre-1", ok: true, result: { value: "queued" } }));
			assert.equal((stub._q ?? []).length, 1);
			const client = new BridgeClient({ global: g, idPrefix: "pre" });
			const got = await new BridgeSecureStore(client).get("k");
			assert.equal(got, "queued");
			assert.equal(stub.receive, receive, "receive is untouched");
			assert.equal(typeof stub._l, "function");
			assert.deepEqual(stub._q, []);
			// later responses arrive through _l
			const g2 = g.ReactNativeWebView;
			if (g2) g2.postMessage = (raw: string) => {
				const m = JSON.parse(raw) as { id: string };
				stub.receive?.call(stub, JSON.stringify({ fcField: 1, id: m.id, ok: true, result: { value: "live" } }));
			};
			assert.equal(await new BridgeSecureStore(client).get("k"), "live");
			// unknown ids are ignored
			stub.receive?.call(stub, JSON.stringify({ fcField: 1, id: "nobody", ok: true, result: {} }));
		});
	}
});

describe("WebCryptoKeyProvider and stores", () => {
	it("makes a non-extractable key that survives IndexedDB", async () => {
		const db = new IdbDb("t-keys", "keys");
		const p1 = new WebCryptoKeyProvider({ db });
		const k1 = await p1.getOrCreateDeviceKey();
		assert.equal(k1.attestation.level, "SOFTWARE");
		const p2 = new WebCryptoKeyProvider({ db });
		const k2 = await p2.getOrCreateDeviceKey();
		assert.deepEqual(k2.jwk, k1.jwk);
		const sig = await p2.sign(new Uint8Array([1, 2, 3]));
		assert.equal(sig.length, 64);
		const pub = await webcrypto.subtle.importKey("jwk", k1.jwk, { name: "ECDSA", namedCurve: "P-256" }, false, ["verify"]);
		assert.ok(await webcrypto.subtle.verify({ name: "ECDSA", hash: "SHA-256" }, pub, sig as BufferSource, new Uint8Array([1, 2, 3])));
		const raw = (await db.get("device-key")) as { privateKey: CryptoKey };
		assert.equal(raw.privateKey.extractable, false);
		await assert.rejects(webcrypto.subtle.exportKey("jwk", raw.privateKey as never));
		await p2.deleteDeviceKey();
		const k3 = await new WebCryptoKeyProvider({ db }).getOrCreateDeviceKey();
		assert.notDeepEqual(k3.jwk, k1.jwk);
	});

	it("IndexedDbSecureStore stores strings and refuses biometrics", async () => {
		const s = new IndexedDbSecureStore({ db: new IdbDb("t-store", "values") });
		await s.set("a", "1");
		assert.equal(await s.get("a"), "1");
		await s.delete("a");
		assert.equal(await s.get("a"), null);
		assert.equal(await s.biometricAvailable(), false);
		await assert.rejects(s.set("a", "1", { requireBiometric: true }), (e: unknown) => e instanceof FieldProviderError && e.code === "BIOMETRIC_UNAVAILABLE");
		await assert.rejects(new MemorySecureStore().set("a", "1", { requireBiometric: true }), (e: unknown) => e instanceof FieldProviderError && e.code === "BIOMETRIC_UNAVAILABLE");
	});
});
