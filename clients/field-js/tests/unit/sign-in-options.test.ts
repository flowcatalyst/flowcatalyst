import { strict as assert } from "node:assert";
import { describe, it } from "node:test";
import {
	FieldAuthError,
	FieldProviderError,
	STORE_REFRESH,
	STORE_SESSION,
	STORE_SUPERVISOR,
	STORE_SUPERVISOR_REFRESH,
	normaliseBadgeToken,
	type DeviceKey,
	type KeyProvider,
	type SecureStore,
	WebCryptoKeyProvider,
	MemoryDb,
} from "../../src/index.js";
import {
	APP,
	CLIENT_IDENTIFIER,
	FakePlatform,
	GOOD_BADGE,
	GOOD_CODE,
	GOOD_PIN,
	PLATFORM,
	rig,
	thumbprint,
	verifyProof,
} from "./fakes.js";

async function errOf(p: Promise<unknown>): Promise<FieldAuthError> {
	try {
		await p;
	} catch (e) {
		assert.ok(e instanceof FieldAuthError, `expected FieldAuthError, got ${String(e)}`);
		return e;
	}
	throw new Error("expected a rejection");
}

/** A store that pretends to be biometric. */
class BioStore implements SecureStore {
	data = new Map<string, string>();
	bioKeys = new Set<string>();
	async get(key: string) {
		return this.data.get(key) ?? null;
	}
	async set(key: string, value: string, opts?: { requireBiometric?: boolean }) {
		if (opts?.requireBiometric) this.bioKeys.add(key);
		else this.bioKeys.delete(key);
		this.data.set(key, value);
	}
	async delete(key: string) {
		this.data.delete(key);
		this.bioKeys.delete(key);
	}
	async biometricAvailable() {
		return true;
	}
}

/** Counts key creations so a test can see the unenrolled key being made. */
class SpyKeys implements KeyProvider {
	created = 0;
	private readonly inner = new WebCryptoKeyProvider({ db: new MemoryDb() });
	async getOrCreateDeviceKey(): Promise<DeviceKey> {
		this.created++;
		return this.inner.getOrCreateDeviceKey();
	}
	sign(d: Uint8Array) {
		return this.inner.sign(d);
	}
	deleteDeviceKey() {
		return this.inner.deleteDeviceKey();
	}
}

function jwtPlatform(over: Partial<FakePlatform> = {}): FakePlatform {
	const p = new FakePlatform();
	p.jwtClaims = true;
	return Object.assign(p, over);
}

const platformCalls = (p: FakePlatform) =>
	p.log.filter((l) => l.url.startsWith(PLATFORM)).map((l) => new URL(l.url).pathname);

const lastBody = (p: FakePlatform, path: string): Record<string, unknown> => {
	const l = [...p.log].reverse().find((x) => new URL(x.url).pathname === path);
	return JSON.parse(l?.body ?? "{}") as Record<string, unknown>;
};

async function supervisorSignedIn(over: Parameters<typeof rig>[0] = {}, platform = jwtPlatform()) {
	const r = rig({ clientIdentifier: CLIENT_IDENTIFIER, ...over }, platform);
	await r.auth.enrol("ABCDE12345");
	await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
	return r;
}

describe("unenrolled sign-in", () => {
	it("generates a key through the KeyProvider and signs proofs with it, with no enrolment call", async () => {
		const keys = new SpyKeys();
		const r = rig({ keys, clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		assert.equal(keys.created, 0);
		const s = await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.ok(keys.created >= 1, "the key provider made the key");
		assert.equal(r.auth.enrolledDevice, null);
		assert.deepEqual(platformCalls(r.platform), ["/field/auth/nonce", "/field/auth/login"]);
		const key = await keys.getOrCreateDeviceKey();
		const login = r.platform.log.at(-1);
		assert.equal((await verifyProof(login?.headers["dpop"] ?? "")).jkt, thumbprint(key.jwk));
		assert.deepEqual(lastBody(r.platform, "/field/auth/login"), {
			appCode: "yard",
			workerCode: "w1",
			pin: GOOD_PIN,
			clientIdentifier: CLIENT_IDENTIFIER,
		});
		assert.equal(s.deviceMode, "UNENROLLED");
		assert.equal(s.deviceId, null);
		assert.equal(s.actor, null);
		assert.equal(s.test, false);
	});

	it("needs a clientIdentifier for a worker code, from the options or the call, and says so locally", async () => {
		const r = rig({}, jwtPlatform());
		const e = await errOf(r.auth.login({ workerCode: "w1", pin: GOOD_PIN }));
		assert.equal(e.code, "INVALID_REQUEST");
		assert.equal(r.platform.log.length, 0, "nothing was sent");
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN, clientIdentifier: "per-call" });
		assert.equal(lastBody(r.platform, "/field/auth/login")["clientIdentifier"], "per-call");
		const o = rig({ clientIdentifier: "from-options" }, jwtPlatform());
		await o.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(lastBody(o.platform, "/field/auth/login")["clientIdentifier"], "from-options");
		await o.auth.login({ workerCode: "w1", pin: GOOD_PIN, clientIdentifier: "override" });
		assert.equal(lastBody(o.platform, "/field/auth/login")["clientIdentifier"], "override");
	});

	it("refreshes, restores after a restart, changes the PIN and calls an app backend", async () => {
		const r = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		const s2 = await r.auth.refresh();
		assert.equal(s2.deviceMode, "UNENROLLED");
		const res = await r.auth.fetch(`${APP}/things`);
		assert.equal(res.status, 200);
		await r.auth.changePin({ currentPin: GOOD_PIN, newPin: "739162" });
		const again = rig({ store: r.store, keys: r.keys }, r.platform);
		const restored = await again.auth.restore();
		assert.equal(restored.device, null);
		assert.equal(restored.session?.deviceMode, "UNENROLLED");
		assert.equal((await again.auth.refresh()).worker.workerCode, "w1");
	});

	it("falls back to UNENROLLED / no device id when the access token is opaque", async () => {
		const r = rig({ clientIdentifier: CLIENT_IDENTIFIER });
		const s = await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(s.deviceMode, "UNENROLLED");
		assert.equal(s.deviceId, null);
	});

	it("an enrolled device is unchanged: no clientIdentifier unless given, PERSONAL with its id", async () => {
		const r = rig({}, jwtPlatform());
		await r.auth.enrol("ABCDE12345");
		const s = await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.deepEqual(lastBody(r.platform, "/field/auth/login"), {
			appCode: "yard",
			workerCode: "w1",
			pin: GOOD_PIN,
		});
		assert.equal(s.deviceMode, "PERSONAL");
		assert.equal(s.deviceId, "fdv_1");
	});

	it("maps CLIENT_MISMATCH and UNKNOWN_DEVICE (requireEnrolledDevice)", async () => {
		const r = rig({}, jwtPlatform());
		await r.auth.enrol("ABCDE12345");
		const m = await errOf(r.auth.login({ workerCode: "w1", pin: GOOD_PIN, clientIdentifier: "other" }));
		assert.equal(m.code, "CLIENT_MISMATCH");
		assert.equal(m.status, 400);
		const u = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform({ requireEnrolled: true }));
		const e = await errOf(u.auth.login({ workerCode: "w1", pin: GOOD_PIN }));
		assert.equal(e.code, "UNKNOWN_DEVICE");
		assert.equal(u.auth.session, null);
	});

	it("refuses logout device-all locally; a plain logout works", async () => {
		const r = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		const before = r.platform.log.length;
		const e = await errOf(r.auth.logout({ scope: "device-all" }));
		assert.equal(e.code, "INVALID_REQUEST");
		assert.match(e.message, /enrolled device/);
		assert.equal(r.platform.log.length, before, "nothing was sent");
		assert.ok(r.auth.session, "the session is untouched");
		await r.auth.logout();
		assert.equal(r.auth.session, null);
		assert.equal(lastBody(r.platform, "/field/auth/logout")["scope"], "session");
	});

	it("exposes the test flag from the token", async () => {
		const r = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform({ testIdentity: true }));
		const s = await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(s.test, true);
	});

	it("fills the new session fields for a session stored by an older version", async () => {
		const r = rig();
		await r.auth.enrol("ABCDE12345");
		await r.store.set(
			STORE_SESSION,
			JSON.stringify({
				worker: { id: "fwu_1", workerCode: "w1", name: "W" },
				appCode: "yard",
				expiresAt: 1,
				idleExpiresAt: 2,
				refreshExpiresAt: 3,
				pinChangeRequired: false,
			}),
		);
		const again = rig({ store: r.store, keys: r.keys }, r.platform);
		const s = (await again.auth.restore()).session;
		assert.equal(s?.actor, null);
		assert.equal(s?.test, false);
		assert.equal(s?.deviceMode, "PERSONAL");
		assert.equal(s?.deviceId, "fdv_1");
	});
});

describe("badge sign-in", () => {
	it("normalises a scanned or keyboard-wedge value before sending", () => {
		assert.equal(normaliseBadgeToken("  fwb1abc\r\n"), "FWB1ABC");
		assert.equal(normaliseBadgeToken("fwb1\tabc def\n"), "FWB1ABCDEF");
		assert.equal(normaliseBadgeToken(GOOD_BADGE.toLowerCase()), GOOD_BADGE);
	});

	it("sends badgeToken (normalised) with the PIN and no workerCode", async () => {
		const r = rig({}, jwtPlatform());
		await r.auth.enrol("ABCDE12345");
		await r.auth.login({ badgeToken: ` ${GOOD_BADGE.toLowerCase()}\r\n`, pin: GOOD_PIN });
		assert.deepEqual(lastBody(r.platform, "/field/auth/login"), {
			appCode: "yard",
			badgeToken: GOOD_BADGE,
			pin: GOOD_PIN,
		});
	});

	it("does not need a clientIdentifier unenrolled (the badge carries its client)", async () => {
		const r = rig({}, jwtPlatform());
		const s = await r.auth.login({ badgeToken: GOOD_BADGE, pin: GOOD_PIN });
		assert.equal(s.deviceMode, "UNENROLLED");
		assert.equal("clientIdentifier" in lastBody(r.platform, "/field/auth/login"), false);
	});

	it("maps PIN_REQUIRED; badge-only works only where the platform allows it", async () => {
		const r = rig({}, jwtPlatform());
		await r.auth.enrol("ABCDE12345");
		assert.equal((await errOf(r.auth.login({ badgeToken: GOOD_BADGE }))).code, "PIN_REQUIRED");
		r.platform.badgeOnly = true;
		const s = await r.auth.login({ badgeToken: GOOD_BADGE });
		assert.equal("pin" in lastBody(r.platform, "/field/auth/login"), false);
		assert.equal(s.deviceId, "fdv_1");
		const u = rig({}, jwtPlatform({ badgeOnly: true }));
		assert.equal((await errOf(u.auth.login({ badgeToken: GOOD_BADGE }))).code, "PIN_REQUIRED");
	});

	it("needs exactly one of workerCode / badgeToken", async () => {
		const r = rig({ clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		for (const bad of [
			{ pin: GOOD_PIN } as never,
			{ workerCode: "w1", badgeToken: GOOD_BADGE, pin: GOOD_PIN } as never,
			{ badgeToken: "  \r\n" },
		]) {
			assert.equal((await errOf(r.auth.login(bad))).code, "INVALID_REQUEST");
		}
		assert.equal(r.platform.log.length, 0);
	});

	it("never leaks the badge token in an error message", async () => {
		const r = rig({}, jwtPlatform());
		const orig = r.platform.fetch;
		r.platform.fetch = async (i, init) => {
			const res = await orig(i, init);
			if (String(i).endsWith("/login")) {
				return new Response(JSON.stringify({ error: "INVALID_CREDENTIALS", message: `no ${GOOD_BADGE}` }), {
					status: 401,
					headers: { "DPoP-Nonce": "n" },
				});
			}
			return res;
		};
		const r2 = rig({ fetch: r.platform.fetch as typeof fetch, store: r.store, keys: r.keys }, r.platform);
		const e = await errOf(r2.auth.login({ badgeToken: GOOD_BADGE, pin: GOOD_PIN }));
		assert.ok(!e.message.includes(GOOD_BADGE));
	});
});

describe("sign-in code", () => {
	it("redeems a code (normalised, proof only) into an impersonated session with the admin as actor", async () => {
		const r = rig({}, jwtPlatform());
		const s = await r.auth.redeemSignInCode(` ${GOOD_CODE.toLowerCase()}\n`);
		assert.deepEqual(lastBody(r.platform, "/field/auth/sign-in-code"), { code: GOOD_CODE });
		assert.equal(r.platform.log.at(-1)?.headers["authorization"], undefined, "no bearer");
		assert.equal(s.worker.workerCode, "w3");
		assert.deepEqual(s.actor, { id: "prn_1", type: "principal", name: "Admin Ann" });
		assert.equal(s.deviceMode, "UNENROLLED");
		assert.equal(r.auth.impersonating, true);
		assert.equal((await errOf(r.auth.redeemSignInCode(GOOD_CODE))).code, "INVALID_CODE");
	});

	it("rejects an empty code locally", async () => {
		const r = rig({}, jwtPlatform());
		assert.equal((await errOf(r.auth.redeemSignInCode("  "))).code, "INVALID_REQUEST");
		assert.equal(r.platform.log.length, 0);
	});

	it("endImpersonation on a code session just ends it", async () => {
		const r = rig({}, jwtPlatform());
		await r.auth.redeemSignInCode(GOOD_CODE);
		assert.equal(await r.auth.endImpersonation(), null);
		assert.equal(r.auth.session, null);
		assert.equal(lastBody(r.platform, "/field/auth/logout")["scope"], "session");
	});

	it("is never stored behind biometrics, even for an enrolled PERSONAL device", async () => {
		const store = new BioStore();
		// Opaque tokens: only the library's own knowledge that this is impersonated protects the store.
		const r = rig({ store, biometricUnlock: true });
		await r.auth.enrol("ABCDE12345");
		await r.auth.redeemSignInCode(GOOD_CODE);
		assert.equal(store.bioKeys.has(STORE_REFRESH), false);
		assert.ok(store.data.get(STORE_REFRESH));
	});
});

describe("actAs", () => {
	it("keeps the supervisor aside and makes the worker's session active", async () => {
		const r = await supervisorSignedIn();
		const sup = r.auth.session;
		const supRefresh = await r.store.get(STORE_REFRESH);
		const s = await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		assert.deepEqual(lastBody(r.platform, "/field/auth/act-as"), { pin: GOOD_PIN, workerCode: "w2" });
		assert.match(r.platform.log.at(-1)?.headers["authorization"] ?? "", /^DPoP /);
		assert.equal(s.worker.workerCode, "w2");
		assert.deepEqual(s.actor, { id: "fwu_1", type: "field_worker", name: "Worker One" });
		assert.equal(r.auth.session, s);
		assert.equal(r.auth.impersonating, true);
		assert.equal(JSON.parse((await r.store.get(STORE_SUPERVISOR)) ?? "null").worker.workerCode, "w1");
		assert.equal(await r.store.get(STORE_SUPERVISOR_REFRESH), supRefresh);
		assert.notEqual(await r.store.get(STORE_REFRESH), supRefresh);
		assert.equal(JSON.parse((await r.store.get(STORE_SESSION)) ?? "null").worker.workerCode, "w2");
		assert.notEqual(r.auth.session, sup);
	});

	it("accepts a workerId", async () => {
		const r = await supervisorSignedIn();
		await r.auth.actAs({ workerId: "fwu_9", pin: GOOD_PIN });
		assert.deepEqual(lastBody(r.platform, "/field/auth/act-as"), { pin: GOOD_PIN, workerId: "fwu_9" });
	});

	it("needs exactly one of workerCode / workerId", async () => {
		const r = await supervisorSignedIn();
		const before = r.platform.log.length;
		for (const bad of [{ pin: GOOD_PIN } as never, { workerCode: "a", workerId: "b", pin: GOOD_PIN } as never]) {
			assert.equal((await errOf(r.auth.actAs(bad))).code, "INVALID_REQUEST");
		}
		assert.equal(r.platform.log.length, before);
	});

	it("rejects locally with IMPERSONATING while impersonating (no nesting)", async () => {
		const r = await supervisorSignedIn();
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		const before = r.platform.log.length;
		assert.equal((await errOf(r.auth.actAs({ workerCode: "w4", pin: GOOD_PIN }))).code, "IMPERSONATING");
		assert.equal(r.platform.log.length, before, "nothing was sent");
		assert.equal(r.auth.session?.worker.workerCode, "w2");
	});

	it("also refuses nesting after a restart (supervisor restored from the store)", async () => {
		const r = await supervisorSignedIn();
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		const again = rig({ store: r.store, keys: r.keys, clientIdentifier: CLIENT_IDENTIFIER }, r.platform);
		await again.auth.restore();
		assert.equal(again.auth.impersonating, true);
		assert.equal((await errOf(again.auth.actAs({ workerCode: "w4", pin: GOOD_PIN }))).code, "IMPERSONATING");
	});

	it("maps platform refusals and leaves the supervisor session intact", async () => {
		const r = await supervisorSignedIn();
		const sup = r.auth.session;
		assert.equal((await errOf(r.auth.actAs({ workerCode: "sup2", pin: GOOD_PIN }))).code, "NO_APP_ACCESS");
		assert.equal((await errOf(r.auth.actAs({ workerCode: "w2", pin: "000000" }))).code, "INVALID_CREDENTIALS");
		r.platform.canImpersonate = false;
		assert.equal((await errOf(r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN }))).code, "IMPERSONATION_FORBIDDEN");
		assert.equal(r.auth.session, sup);
		assert.equal(r.auth.impersonating, false);
		assert.equal(await r.store.get(STORE_SUPERVISOR), null);
	});

	it("never puts the supervisor's PIN in an error", async () => {
		const r = await supervisorSignedIn();
		const orig = r.platform.fetch;
		r.platform.fetch = async (i, init) => {
			const res = await orig(i, init);
			if (String(i).endsWith("/act-as")) {
				return new Response(JSON.stringify({ error: "INVALID_CREDENTIALS", message: `bad ${GOOD_PIN}` }), {
					status: 401,
					headers: { "DPoP-Nonce": "n" },
				});
			}
			return res;
		};
		const r2 = rig({ fetch: r.platform.fetch as typeof fetch, store: r.store, keys: r.keys }, r.platform);
		await r2.auth.restore();
		await r2.auth.refresh();
		const e = await errOf(r2.auth.actAs({ workerCode: "w2", pin: GOOD_PIN }));
		assert.ok(!e.message.includes(GOOD_PIN));
	});

	it("changePin is refused locally while impersonating", async () => {
		const r = await supervisorSignedIn();
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		const before = r.platform.log.length;
		const e = await errOf(r.auth.changePin({ currentPin: GOOD_PIN, newPin: "739162" }));
		assert.equal(e.code, "IMPERSONATING");
		assert.equal(r.platform.log.length, before);
	});
});

describe("endImpersonation", () => {
	it("logs out the impersonated family and restores the supervisor session", async () => {
		const r = await supervisorSignedIn();
		const supRefresh = (await r.store.get(STORE_REFRESH)) as string;
		const supSession = r.auth.session;
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		const impRefresh = (await r.store.get(STORE_REFRESH)) as string;
		const events: unknown[] = [];
		r.auth.addEventListener("sessionChanged", (e) => events.push((e as unknown as { detail: unknown }).detail));
		const back = await r.auth.endImpersonation();
		const out = lastBody(r.platform, "/field/auth/logout");
		assert.deepEqual(out, { refreshToken: impRefresh, scope: "session" });
		assert.equal(back?.worker.workerCode, "w1");
		assert.equal(back?.actor, null);
		assert.equal(r.auth.session, back);
		assert.deepEqual(back, supSession);
		assert.equal(r.auth.impersonating, false);
		assert.equal(await r.store.get(STORE_REFRESH), supRefresh);
		assert.equal(JSON.parse((await r.store.get(STORE_SESSION)) ?? "null").worker.workerCode, "w1");
		assert.equal(await r.store.get(STORE_SUPERVISOR), null);
		assert.equal(await r.store.get(STORE_SUPERVISOR_REFRESH), null);
		assert.equal(events.at(-1), back);
		// the supervisor's token still works: no refresh needed, the app call goes out as the supervisor
		const refreshes = r.platform.refreshCount;
		const res = await r.auth.fetch(`${APP}/x`);
		assert.equal(res.status, 200);
		assert.equal(r.platform.refreshCount, refreshes);
	});

	it("refreshes the supervisor when its access token has lapsed", async () => {
		const r = await supervisorSignedIn();
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		r.clock.now += 3_600_000;
		const refreshes = r.platform.refreshCount;
		const back = await r.auth.endImpersonation();
		assert.equal(r.platform.refreshCount - refreshes, 1);
		assert.equal(back?.worker.workerCode, "w1");
		assert.equal(back?.expiresAt, r.clock.now + 600_000);
	});

	it("works after a restart mid-impersonation (supervisor read back from the store)", async () => {
		const r = await supervisorSignedIn();
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		const again = rig({ store: r.store, keys: r.keys, clientIdentifier: CLIENT_IDENTIFIER }, r.platform);
		await again.auth.restore();
		const back = await again.auth.endImpersonation();
		assert.equal(back?.worker.workerCode, "w1");
		assert.equal(again.auth.impersonating, false);
	});

	it("is NOT_IMPERSONATING when no one is", async () => {
		const r = await supervisorSignedIn();
		assert.equal((await errOf(r.auth.endImpersonation())).code, "NOT_IMPERSONATING");
	});

	it("still restores the supervisor when the logout call fails, then reports it", async () => {
		const r = await supervisorSignedIn();
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		const orig = r.platform.fetch;
		r.platform.fetch = async (i, init) => {
			if (String(i).endsWith("/logout")) throw new TypeError("down");
			return orig(i, init);
		};
		const r2 = rig({ fetch: r.platform.fetch as typeof fetch, store: r.store, keys: r.keys }, r.platform);
		await r2.auth.restore();
		assert.equal((await errOf(r2.auth.endImpersonation())).code, "NETWORK");
		assert.equal(r2.auth.session?.worker.workerCode, "w1");
		assert.equal(r2.auth.impersonating, false);
	});

	it("falls back to the supervisor when the impersonated session expires on its own", async () => {
		const r = await supervisorSignedIn();
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		r.platform.refreshError = { status: 401, error: "SESSION_EXPIRED" };
		let reason = "";
		r.auth.addEventListener("sessionEnded", (e) => (reason = (e as unknown as { detail: { reason: string } }).detail.reason));
		assert.equal((await errOf(r.auth.refresh())).code, "SESSION_EXPIRED");
		assert.equal(reason, "SESSION_EXPIRED");
		assert.equal(r.auth.session?.worker.workerCode, "w1");
		assert.equal(r.auth.impersonating, false);
		assert.equal(await r.store.get(STORE_SUPERVISOR), null);
	});

	it("logout while impersonating signs out both families and clears the supervisor", async () => {
		const r = await supervisorSignedIn();
		const supRefresh = (await r.store.get(STORE_REFRESH)) as string;
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		await r.auth.logout();
		const logouts = r.platform.log.filter((l) => l.url.endsWith("/logout")).map((l) => JSON.parse(l.body).refreshToken);
		assert.equal(logouts.length, 2);
		assert.ok(logouts.includes(supRefresh));
		assert.equal(r.auth.session, null);
		assert.equal(r.auth.impersonating, false);
		assert.equal(await r.store.get(STORE_SUPERVISOR), null);
		assert.equal(await r.store.get(STORE_SUPERVISOR_REFRESH), null);
	});
});

describe("biometric unlock stays enrolled-PERSONAL only", () => {
	it("an unenrolled session is never stored behind biometrics and unlock() is unavailable", async () => {
		const store = new BioStore();
		const r = rig({ store, biometricUnlock: true, clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(store.bioKeys.size, 0);
		assert.equal((await errOf(r.auth.unlock())).code, "BIOMETRIC_UNAVAILABLE");
	});

	it("a session that started unenrolled stays plain after the device enrols mid-session", async () => {
		const store = new BioStore();
		const r = rig({ store, biometricUnlock: true, clientIdentifier: CLIENT_IDENTIFIER }, jwtPlatform());
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		await r.auth.enrol("ABCDE12345");
		const s = await r.auth.refresh();
		assert.equal(s.deviceMode, "UNENROLLED");
		assert.equal(store.bioKeys.has(STORE_REFRESH), false);
		assert.equal((await errOf(r.auth.unlock())).code, "BIOMETRIC_UNAVAILABLE");
		// the next login is enrolled PERSONAL and biometric again
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(r.auth.session?.deviceMode, "PERSONAL");
		assert.ok(store.bioKeys.has(STORE_REFRESH));
	});

	it("an impersonated session is never behind biometrics; the supervisor's token keeps its protection", async () => {
		const store = new BioStore();
		const r = await supervisorSignedIn({ store, biometricUnlock: true });
		assert.ok(store.bioKeys.has(STORE_REFRESH), "supervisor session is biometric before");
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		assert.equal(store.bioKeys.has(STORE_REFRESH), false, "impersonated refresh token is plain");
		assert.ok(store.bioKeys.has(STORE_SUPERVISOR_REFRESH), "supervisor refresh token stays biometric");
		assert.equal(store.bioKeys.has(STORE_SUPERVISOR), false);
		await r.auth.refresh();
		assert.equal(store.bioKeys.has(STORE_REFRESH), false, "still plain after a rotation");
		assert.equal((await errOf(r.auth.unlock())).code, "BIOMETRIC_UNAVAILABLE");
		await r.auth.endImpersonation();
		assert.ok(store.bioKeys.has(STORE_REFRESH), "restored supervisor is biometric again");
		assert.equal(store.bioKeys.has(STORE_SUPERVISOR_REFRESH), false);
	});

	it("an impersonated session stays plain even when the token carries no act claim", async () => {
		const store = new BioStore();
		const r = await supervisorSignedIn({ store, biometricUnlock: true }, new FakePlatform());
		await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		assert.equal(store.bioKeys.has(STORE_REFRESH), false);
		await r.auth.refresh();
		assert.equal(store.bioKeys.has(STORE_REFRESH), false);
	});
});

describe("key providers errors are unchanged", () => {
	it("a provider failure on first unenrolled use maps to an auth error carrying only the code", async () => {
		const keys: KeyProvider = {
			async getOrCreateDeviceKey() {
				throw new FieldProviderError("INTERNAL", "secret detail");
			},
			async sign() {
				throw new FieldProviderError("INTERNAL");
			},
			async deleteDeviceKey() {},
		};
		const r = rig({ keys, clientIdentifier: CLIENT_IDENTIFIER });
		const e = await errOf(r.auth.login({ workerCode: "w1", pin: GOOD_PIN }));
		assert.equal(e.code, "INTERNAL");
		assert.ok(!e.message.includes("secret detail"));
	});
});
