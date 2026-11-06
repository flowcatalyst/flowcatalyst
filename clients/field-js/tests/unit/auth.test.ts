import { strict as assert } from "node:assert";
import { describe, it } from "node:test";
import {
	FieldAuthError,
	FieldProviderError,
	STORE_DEVICE,
	STORE_REFRESH,
	STORE_SESSION,
	parseEnrolmentQr,
	type SecureStore,
} from "../../src/index.js";
import {
	APP,
	FakePlatform,
	GOOD_PIN,
	NEW_PIN,
	PLATFORM,
	enrolledAndLoggedIn,
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

describe("parseEnrolmentQr", () => {
	it("splits the URL (which contains colons) from the code", () => {
		assert.deepEqual(parseEnrolmentQr("fcfield:1:https://fc.example.com:8443:ABCDE12345"), {
			platformUrl: "https://fc.example.com:8443",
			code: "ABCDE12345",
		});
		assert.deepEqual(parseEnrolmentQr("fcfield:1:http://localhost:3000/:CODE123456"), {
			platformUrl: "http://localhost:3000",
			code: "CODE123456",
		});
	});
	it("refuses anything else", async () => {
		for (const bad of ["", "fcfield:2:https://x:CODE", "fcfield:1:nocode", "fcfield:1:ftp://x:CODE", "https://x"]) {
			assert.throws(() => parseEnrolmentQr(bad), (e: unknown) => e instanceof FieldAuthError && e.code === "INVALID_ENROLMENT_CODE", bad);
		}
	});
});

describe("enrolment and proofs", () => {
	it("signs every platform request with a proof an independent ES256 check accepts", async () => {
		const r = rig();
		const dev = await r.auth.enrol("ABCDE12345", { label: "Gate tablet" });
		assert.equal(dev.deviceId, "fdv_1");
		assert.equal(dev.mode, "PERSONAL");
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		const key = await r.keys.getOrCreateDeviceKey();
		const jkt = thumbprint(key.jwk);
		const reqs = r.platform.log.filter((l) => l.url.startsWith(PLATFORM));
		assert.deepEqual(
			reqs.map((l) => new URL(l.url).pathname),
			["/field/auth/nonce", "/field/devices/enrol", "/field/auth/login"],
		);
		const jtis = new Set<string>();
		for (const l of reqs) {
			const p = await verifyProof(l.headers["dpop"] ?? "");
			assert.equal(p.jkt, jkt, "jkt per RFC 7638");
			assert.equal(p.claims["htm"], "POST");
			assert.equal(p.claims["htu"], PLATFORM + new URL(l.url).pathname);
			assert.equal(typeof p.claims["iat"], "number");
			jtis.add(String(p.claims["jti"]));
			assert.equal("nonce" in p.claims, !l.url.endsWith("/nonce"));
		}
		assert.equal(jtis.size, reqs.length, "jti is unique");
		const body = JSON.parse(reqs[1]?.body ?? "{}") as Record<string, unknown>;
		assert.deepEqual(body["publicKeyJwk"], key.jwk);
		assert.equal(body["label"], "Gate tablet");
		assert.equal(body["platform"], "web");
		assert.equal("attestationChain" in body, false);
		const stored = JSON.parse((await r.store.get(STORE_DEVICE)) ?? "null");
		assert.equal(stored.deviceId, "fdv_1");
		assert.equal(stored.platformUrl, PLATFORM);
	});

	it("takes the platform URL from a QR payload", async () => {
		const r = rig({ platformUrl: "" });
		await r.auth.enrol(`fcfield:1:${PLATFORM}:ABCDE12345`);
		assert.equal(r.auth.enrolledDevice?.platformUrl, PLATFORM);
	});

	it("maps an invalid code", async () => {
		const r = rig();
		const e = await errOf(r.auth.enrol("WRONGCODE0"));
		assert.equal(e.code, "INVALID_ENROLMENT_CODE");
		assert.equal(e.status, 401);
	});

	it("restores the device from the store in a new instance", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		const again = rig({ store: r.store, keys: r.keys, platformUrl: "" }, r.platform);
		const s = await again.auth.restore();
		assert.equal(s.device?.deviceId, "fdv_1");
		assert.equal(s.session?.worker.workerCode, "w1");
	});
});

describe("nonce handling", () => {
	it("retries once on use_dpop_nonce with the fresh nonce", async () => {
		const r = rig();
		await r.auth.enrol("ABCDE12345");
		r.platform.staleNonceOnce = true;
		const before = r.platform.log.length;
		await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		const sent = r.platform.log.slice(before);
		assert.equal(sent.length, 2, "one failed attempt and one retry");
		const nonces = await Promise.all(sent.map(async (l) => (await verifyProof(l.headers["dpop"] ?? "")).claims["nonce"]));
		assert.notEqual(nonces[0], nonces[1]);
	});

	it("does not retry forever", async () => {
		const r = rig();
		await r.auth.enrol("ABCDE12345");
		let n = 0;
		const orig = r.platform.fetch;
		r.platform.fetch = async (i, init) => {
			const res = await orig(i, init);
			if (String(i).endsWith("/login")) {
				n++;
				return new Response(JSON.stringify({ error: "use_dpop_nonce", message: "m" }), {
					status: 401,
					headers: { "DPoP-Nonce": "zzz" },
				});
			}
			return res;
		};
		const r2 = rig({ fetch: r.platform.fetch as typeof fetch, store: r.store, keys: r.keys }, r.platform);
		const e = await errOf(r2.auth.login({ workerCode: "w1", pin: GOOD_PIN }));
		assert.equal(e.code, "use_dpop_nonce");
		assert.equal(n, 2);
	});
});

describe("login, errors and events", () => {
	it("returns the session and persists metadata and the refresh token", async () => {
		const r = rig();
		await r.auth.enrol("ABCDE12345");
		const events: string[] = [];
		r.auth.addEventListener("sessionChanged", () => events.push("sessionChanged"));
		const s = await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(s.worker.name, "Worker One");
		assert.equal(s.appCode, "yard");
		assert.equal(s.expiresAt, r.clock.now + 600_000);
		assert.equal(s.pinChangeRequired, false);
		assert.equal(r.auth.session, s);
		assert.ok(JSON.parse((await r.store.get(STORE_SESSION)) ?? "null"));
		assert.match((await r.store.get(STORE_REFRESH)) ?? "", /^fwr_/);
		assert.deepEqual(events, ["sessionChanged"]);
	});

	it("raises pinChangeRequired", async () => {
		const r = rig();
		r.platform.pcr = true;
		await r.auth.enrol("ABCDE12345");
		let got = false;
		r.auth.addEventListener("pinChangeRequired", () => (got = true));
		const s = await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
		assert.equal(s.pinChangeRequired, true);
		assert.ok(got);
	});

	it("maps platform errors and emits locked", async () => {
		const r = rig();
		await r.auth.enrol("ABCDE12345");
		const locks: unknown[] = [];
		r.auth.addEventListener("locked", (e) => locks.push((e as unknown as { detail: unknown }).detail));
		assert.equal((await errOf(r.auth.login({ workerCode: "w1", pin: "000000" }))).code, "INVALID_CREDENTIALS");
		assert.equal((await errOf(r.auth.login({ workerCode: "locked", pin: GOOD_PIN }))).code, "WORKER_LOCKED");
		const slow = await errOf(r.auth.login({ workerCode: "slow", pin: GOOD_PIN }));
		assert.equal(slow.code, "TOO_MANY_ATTEMPTS");
		assert.equal(slow.retryAfter, 42);
		assert.deepEqual(locks, [{ hard: true }, { hard: false, retryAfter: 42 }]);
		assert.equal(r.auth.session, null);
	});

	it("is NOT_ENROLLED when there is neither a platform URL nor an enrolment", async () => {
		const r = rig({ platformUrl: "" });
		assert.equal(
			(await errOf(r.auth.login({ workerCode: "w1", pin: GOOD_PIN, clientIdentifier: "acme" }))).code,
			"NOT_ENROLLED",
		);
	});

	it("maps a dead network to NETWORK", async () => {
		const r = rig();
		await r.auth.enrol("ABCDE12345");
		r.platform.networkDown = true;
		const e = await errOf(r.auth.login({ workerCode: "w1", pin: GOOD_PIN }));
		assert.equal(e.code, "NETWORK");
	});
});

describe("refresh", () => {
	it("is single-flight: concurrent callers share one request", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		r.platform.refreshDelayMs = 20;
		const before = r.platform.refreshCount;
		const all = await Promise.all([r.auth.refresh(), r.auth.refresh(), r.auth.refresh(), r.auth.refresh()]);
		assert.equal(r.platform.refreshCount - before, 1);
		assert.ok(all.every((s) => s === all[0]));
	});

	it("rotates the stored refresh token", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		const t1 = await r.store.get(STORE_REFRESH);
		await r.auth.refresh();
		assert.notEqual(await r.store.get(STORE_REFRESH), t1);
	});

	it("ends the session on INVALID_GRANT and clears storage", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		r.platform.refreshError = { status: 401, error: "INVALID_GRANT" };
		let reason = "";
		r.auth.addEventListener("sessionEnded", (e) => (reason = (e as unknown as { detail: { reason: string } }).detail.reason));
		assert.equal((await errOf(r.auth.refresh())).code, "INVALID_GRANT");
		assert.equal(reason, "INVALID_GRANT");
		assert.equal(r.auth.session, null);
		assert.equal(await r.store.get(STORE_REFRESH), null);
	});

	it("keeps the session on a network error so a retry can succeed", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		r.platform.networkDown = true;
		assert.equal((await errOf(r.auth.refresh())).code, "NETWORK");
		assert.ok(r.auth.session);
		await r.auth.refresh();
	});
});

describe("fetch for application backends", () => {
	it("adds Authorization and a proof bound to the token, htu without query", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		const res = await r.auth.fetch(`${APP}/api/things?x=1#frag`, { method: "post", body: "{}" });
		assert.equal(res.status, 200);
		const l = r.platform.log.filter((x) => x.url.startsWith(APP)).at(-1);
		assert.ok(l);
		assert.match(l.headers["authorization"] ?? "", /^DPoP acc-/);
		const p = await verifyProof(l.headers["dpop"] ?? "");
		assert.equal(p.claims["htm"], "POST");
		assert.equal(p.claims["htu"], `${APP}/api/things`);
	});

	it("refreshes first when the access token expires within 30 s", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		const before = r.platform.refreshCount;
		r.clock.now += 575_000; // 25 s left
		await r.auth.fetch(`${APP}/a`);
		assert.equal(r.platform.refreshCount - before, 1);
		await r.auth.fetch(`${APP}/a`);
		assert.equal(r.platform.refreshCount - before, 1, "fresh token is reused");
	});

	it("restores a session after a restart by refreshing from the store", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		const again = rig({ store: r.store, keys: r.keys }, r.platform);
		const res = await again.auth.fetch(`${APP}/a`);
		assert.equal(res.status, 200);
	});

	it("retries once on an app's use_dpop_nonce and caches the nonce per origin", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		r.platform.appNonceOnce = true;
		const res = await r.auth.fetch(`${APP}/a`);
		assert.equal(res.status, 200);
		assert.equal(((await res.json()) as { nonce: string }).nonce, "app-nonce-1");
		const next = await r.auth.fetch(`${APP}/b`);
		assert.equal(((await next.json()) as { nonce: string }).nonce, "app-nonce-1");
	});

	it("on invalid_token refreshes and retries once", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		r.platform.appInvalidTokens = 1;
		const before = r.platform.refreshCount;
		const res = await r.auth.fetch(`${APP}/a`);
		assert.equal(res.status, 200);
		assert.equal(r.platform.refreshCount - before, 1);
	});

	it("emits sessionEnded when invalid_token persists after a refresh", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		r.platform.appInvalidTokens = 5;
		let reason = "";
		r.auth.addEventListener("sessionEnded", (e) => (reason = (e as unknown as { detail: { reason: string } }).detail.reason));
		const res = await r.auth.fetch(`${APP}/a`);
		assert.equal(res.status, 401);
		assert.equal(reason, "invalid_token");
	});

	it("never sends the refresh token anywhere but /field/auth/refresh|logout on the platform", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		const token = (await r.store.get(STORE_REFRESH)) ?? "";
		await r.auth.fetch(`${APP}/a`);
		r.clock.now += 590_000;
		await r.auth.fetch(`${APP}/a`);
		const latest = (await r.store.get(STORE_REFRESH)) ?? "";
		await r.auth.logout();
		for (const l of r.platform.log) {
			const carries = [token, latest].some((t) => JSON.stringify(l).includes(t));
			if (!carries) continue;
			const u = new URL(l.url);
			assert.equal(u.origin, PLATFORM);
			assert.ok(["/field/auth/refresh", "/field/auth/logout"].includes(u.pathname), u.pathname);
		}
	});
});

describe("changePin and logout", () => {
	it("changes the PIN, refreshes, and the new PIN signs in", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		const refreshes = r.platform.refreshCount;
		await r.auth.changePin({ currentPin: GOOD_PIN, newPin: NEW_PIN });
		assert.equal(r.platform.refreshCount - refreshes, 1, "refresh drops pcr");
		await r.auth.logout();
		await assert.rejects(r.auth.login({ workerCode: "w1", pin: GOOD_PIN }));
		const s = await r.auth.login({ workerCode: "w1", pin: NEW_PIN });
		assert.ok(s);
	});

	it("surfaces PIN_POLICY violations and a wrong current PIN", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		const pol = await errOf(r.auth.changePin({ currentPin: GOOD_PIN, newPin: "111111" }));
		assert.equal(pol.code, "PIN_POLICY");
		assert.deepEqual(pol.violations, ["REPEATED_DIGITS"]);
		assert.equal((await errOf(r.auth.changePin({ currentPin: "000000", newPin: NEW_PIN }))).code, "INVALID_CREDENTIALS");
	});

	it("logout clears local state and emits sessionEnded", async () => {
		const r = rig();
		await enrolledAndLoggedIn(r);
		let reason = "";
		r.auth.addEventListener("sessionEnded", (e) => (reason = (e as unknown as { detail: { reason: string } }).detail.reason));
		await r.auth.logout({ scope: "device-all" });
		const last = r.platform.log.at(-1);
		assert.equal(JSON.parse(last?.body ?? "{}").scope, "device-all");
		assert.equal(reason, "logout");
		assert.equal(r.auth.session, null);
		assert.equal(await r.store.get(STORE_REFRESH), null);
		assert.ok(await r.store.get(STORE_DEVICE), "the device stays enrolled");
	});
});

/** A store that pretends to be biometric. */
class BioStore implements SecureStore {
	data = new Map<string, string>();
	bioKeys = new Set<string>();
	prompts: (string | undefined)[] = [];
	invalidated = false;
	available = true;
	async get(key: string, opts?: { prompt?: string }) {
		if (this.bioKeys.has(key)) {
			this.prompts.push(opts?.prompt);
			if (this.invalidated) throw new FieldProviderError("KEY_INVALIDATED");
		}
		return this.data.get(key) ?? null;
	}
	async set(key: string, value: string, opts?: { requireBiometric?: boolean }) {
		if (opts?.requireBiometric) this.bioKeys.add(key);
		this.data.set(key, value);
	}
	async delete(key: string) {
		this.data.delete(key);
		this.bioKeys.delete(key);
	}
	async biometricAvailable() {
		return this.available;
	}
}

describe("biometric unlock", () => {
	it("stores the refresh token behind biometrics (PERSONAL + opt-in) and unlock() refreshes", async () => {
		const store = new BioStore();
		const r = rig({ store, biometricUnlock: true });
		await enrolledAndLoggedIn(r);
		assert.ok(store.bioKeys.has(STORE_REFRESH));
		assert.ok(!store.bioKeys.has(STORE_SESSION));
		assert.ok(!store.bioKeys.has(STORE_DEVICE));
		const again = rig({ store, keys: r.keys, biometricUnlock: true }, r.platform);
		const s = await again.auth.unlock();
		assert.equal(s.worker.workerCode, "w1");
		assert.equal(store.prompts.length, 1);
	});

	it("without the opt-in the token is plain", async () => {
		const store = new BioStore();
		const r = rig({ store });
		await enrolledAndLoggedIn(r);
		assert.equal(store.bioKeys.size, 0);
	});

	it("is BIOMETRIC_UNAVAILABLE on a store without biometrics or a SHARED device", async () => {
		const plain = rig();
		await enrolledAndLoggedIn(plain);
		assert.equal((await errOf(plain.auth.unlock())).code, "BIOMETRIC_UNAVAILABLE");
		const shared = new FakePlatform();
		shared.mode = "SHARED";
		const r = rig({ store: new BioStore(), biometricUnlock: true }, shared);
		await enrolledAndLoggedIn(r);
		assert.equal((await errOf(r.auth.unlock())).code, "BIOMETRIC_UNAVAILABLE");
	});

	it("KEY_INVALIDATED clears the stored token and raises BIOMETRIC_INVALIDATED", async () => {
		const store = new BioStore();
		const r = rig({ store, biometricUnlock: true });
		await enrolledAndLoggedIn(r);
		store.invalidated = true;
		const e = await errOf(r.auth.unlock());
		assert.equal(e.code, "BIOMETRIC_INVALIDATED");
		assert.equal(store.data.has(STORE_REFRESH), false);
		assert.equal(r.auth.session, null);
	});

	it("passes through BIOMETRIC_CANCELLED", async () => {
		const store = new BioStore();
		const r = rig({ store, biometricUnlock: true });
		await enrolledAndLoggedIn(r);
		store.get = async () => {
			throw new FieldProviderError("BIOMETRIC_CANCELLED");
		};
		assert.equal((await errOf(r.auth.unlock())).code, "BIOMETRIC_CANCELLED");
	});
});

describe("the PIN and refresh token never leak", () => {
	it("not in errors, stored values, or console output during a login", async () => {
		const logged: string[] = [];
		const methods = ["log", "info", "warn", "error", "debug", "trace"] as const;
		const saved = methods.map((m) => console[m]);
		for (const m of methods) console[m] = (...a: unknown[]) => void logged.push(a.map(String).join(" "));
		try {
			const r = rig();
			await r.auth.enrol("ABCDE12345");
			// wrong PIN against a server that echoes it, and a hostile network error
			const pin = "906142";
			const e1 = await errOf(r.auth.login({ workerCode: "echo", pin }));
			r.platform.networkDown = true;
			const e2 = await errOf(r.auth.login({ workerCode: "w1", pin }));
			await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
			await r.auth.changePin({ currentPin: GOOD_PIN, newPin: NEW_PIN });
			const refresh = (await r.store.get(STORE_REFRESH)) ?? "";
			for (const e of [e1, e2]) {
				assert.ok(!e.message.includes(pin), e.message);
				assert.ok(!JSON.stringify({ ...e, m: e.message, s: e.stack }).includes(pin));
			}
			const dump = JSON.stringify(r.store.dump());
			for (const secret of [pin, GOOD_PIN, NEW_PIN]) {
				assert.ok(!dump.includes(secret), "no PIN in storage");
				assert.ok(!logged.join("\n").includes(secret), "no PIN in console output");
			}
			assert.ok(!logged.join("\n").includes(refresh), "no refresh token in console output");
			// the PIN only ever travelled in the body of the platform calls
			for (const l of r.platform.log) {
				const pinsInUrlOrHeaders = JSON.stringify({ u: l.url, h: l.headers });
				for (const secret of [pin, GOOD_PIN, NEW_PIN]) assert.ok(!pinsInUrlOrHeaders.includes(secret));
			}
		} finally {
			methods.forEach((m, i) => (console[m] = saved[i] as never));
		}
	});
});
