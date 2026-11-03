import assert from "node:assert/strict";
import { describe, it } from "node:test";
import { NativeKeyProvider } from "../src/NativeKeyProvider";
import { NativeSecureStore } from "../src/NativeSecureStore";
import type { FcFieldNative } from "../src/native";
import { FieldError } from "../src/types";

const base = (over: Partial<FcFieldNative>): FcFieldNative => ({
	getOrCreateDeviceKey: async () => ({ jwk: { kty: "EC", crv: "P-256", x: "x", y: "y" }, attestation: { level: "SOFTWARE" } }),
	sign: async () => "AAEC",
	deleteDeviceKey: async () => {},
	storeGet: async () => null,
	storeSet: async () => {},
	storeDelete: async () => {},
	biometricAvailable: async () => false,
	...over,
});

describe("NativeKeyProvider", () => {
	it("encodes data, decodes signature", async () => {
		let seen = "";
		const p = new NativeKeyProvider(base({ sign: async (d) => ((seen = d), "AAEC") }));
		const sig = await p.sign(new Uint8Array([1, 2, 3]));
		assert.equal(seen, "AQID");
		assert.deepEqual([...sig], [0, 1, 2]);
		assert.equal((await p.getOrCreateDeviceKey()).attestation.level, "SOFTWARE");
		await p.deleteDeviceKey();
	});
	it("maps rejection codes", async () => {
		const p = new NativeKeyProvider(base({ sign: async () => { throw Object.assign(new Error("x"), { code: "KEY_INVALIDATED" }); } }));
		await assert.rejects(p.sign(new Uint8Array([1])), (e: unknown) => e instanceof FieldError && e.code === "KEY_INVALIDATED");
		const q = new NativeKeyProvider(base({ deleteDeviceKey: async () => { throw new Error("odd"); } }));
		await assert.rejects(q.deleteDeviceKey(), (e: unknown) => e instanceof FieldError && e.code === "INTERNAL");
	});
});

describe("NativeSecureStore", () => {
	it("forwards args with defaults", async () => {
		const calls: unknown[][] = [];
		const s = new NativeSecureStore(base({
			storeGet: async (...a) => (calls.push(["get", ...a]), "v"),
			storeSet: async (...a) => void calls.push(["set", ...a]),
			storeDelete: async (...a) => void calls.push(["del", ...a]),
			biometricAvailable: async () => true,
		}));
		assert.equal(await s.get("k"), "v");
		assert.equal(await s.get("k", { prompt: "Unlock" }), "v");
		await s.set("k", "v");
		await s.set("k", "v", { requireBiometric: true });
		await s.delete("k");
		assert.equal(await s.biometricAvailable(), true);
		assert.deepEqual(calls, [["get", "k", null], ["get", "k", "Unlock"], ["set", "k", "v", false], ["set", "k", "v", true], ["del", "k"]]);
	});
	it("maps rejection codes", async () => {
		const s = new NativeSecureStore(base({ storeGet: async () => { throw { code: "NO_SECURE_LOCK_SCREEN", message: "m" }; } }));
		await assert.rejects(s.get("k"), (e: unknown) => e instanceof FieldError && e.code === "NO_SECURE_LOCK_SCREEN");
	});
});
