import { strict as assert } from "node:assert";
import { describe, it } from "node:test";
import { STORE_SESSION, type SecureStore } from "../../src/index.js";
import { CLIENT_IDENTIFIER, FakePlatform, GOOD_CODE, GOOD_PIN, rig } from "./fakes.js";

function jwtPlatform(over: Partial<FakePlatform> = {}): FakePlatform {
	const p = new FakePlatform();
	p.jwtClaims = true;
	return Object.assign(p, over);
}

/** A store that pretends to be biometric. */
class BioStore implements SecureStore {
	data = new Map<string, string>();
	async get(key: string) {
		return this.data.get(key) ?? null;
	}
	async set(key: string, value: string) {
		this.data.set(key, value);
	}
	async delete(key: string) {
		this.data.delete(key);
	}
	async biometricAvailable() {
		return true;
	}
}

describe("the client of the session", () => {
	it("takes the client from the access token at sign-in", async () => {
		const r = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		const s = await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(s.clientId, "clt_1");
		assert.equal(s.clientIdentifier, "acme");
	});

	it("keeps it across a refresh", async () => {
		const r = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		const s = await r.auth.refresh();
		assert.equal(s.clientId, "clt_1");
		assert.equal(s.clientIdentifier, "acme");
	});

	it("restores it after a restart, from the stored session", async () => {
		const r = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		const again = rig({ store: r.store, keys: r.keys }, r.platform);
		const s = (await again.auth.restore()).session;
		assert.equal(s?.clientId, "clt_1");
		assert.equal(s?.clientIdentifier, "acme");
	});

	it("has it after an unlock", async () => {
		const store = new BioStore();
		const r = rig({ store, biometricUnlock: true }, jwtPlatform());
		await r.auth.enrol("ABCDE12345");
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		const again = rig({ store, keys: r.keys, biometricUnlock: true }, r.platform);
		const s = await again.auth.unlock();
		assert.equal(s.clientId, "clt_1");
		assert.equal(s.clientIdentifier, "acme");
	});

	it("has it on an impersonated session", async () => {
		const r = rig({}, jwtPlatform());
		const s = await r.auth.redeemSignInCode(GOOD_CODE);
		assert.notEqual(s.actor, null);
		assert.equal(s.clientId, "clt_1");
		assert.equal(s.clientIdentifier, "acme");
	});

	it("is null for a token without the claims, and for a token that is not a JWT", async () => {
		const old = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform({ clientClaims: false }));
		const s = await old.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(s.clientId, null);
		assert.equal(s.clientIdentifier, null);
		const opaque = rig({ clientIdentifier: CLIENT_IDENTIFIER });
		const o = await opaque.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(o.clientId, null);
		assert.equal(o.clientIdentifier, null);
	});

	it("reads a session stored before the fields existed as null, and the next refresh fills them", async () => {
		const r = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		const stored = JSON.parse((await r.store.get(STORE_SESSION)) ?? "{}") as Record<string, unknown>;
		delete stored["clientId"];
		delete stored["clientIdentifier"];
		await r.store.set(STORE_SESSION, JSON.stringify(stored));
		const again = rig({ store: r.store, keys: r.keys }, r.platform);
		const restored = (await again.auth.restore()).session;
		assert.equal(restored?.clientId, null);
		assert.equal(restored?.clientIdentifier, null);
		const s = await again.auth.refresh();
		assert.equal(s.clientIdentifier, "acme");
	});
});
