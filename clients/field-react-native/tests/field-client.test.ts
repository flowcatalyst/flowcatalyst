import assert from "node:assert/strict";
import { describe, it } from "node:test";
import {
	FieldAuthError,
	FieldClient,
	isPinResetRequired,
	mapNativeError,
	type NativeEventSource,
	type Session,
} from "../src/FieldClient";
import type { FcFieldNative } from "../src/native";

const SESSION: Session = {
	worker: { id: "w1", workerCode: "W001", name: "Ann", photoVersion: null, canSetOwnPhoto: false },
	actor: null,
	test: false,
	deviceMode: "PERSONAL",
	deviceId: "d1",
	appCode: "fulfil",
	expiresAt: 1000,
	idleExpiresAt: 2000,
	refreshExpiresAt: 3000,
	pinChangeRequired: false,
	clientId: "clt_1",
	clientIdentifier: "acme",
};

const CONFIG = { platformUrl: "https://p.example.com", appCode: "fulfil" };

type Call = [string, ...unknown[]];

function mockNative(over: Record<string, unknown> = {}): { native: FcFieldNative; calls: Call[] } {
	const calls: Call[] = [];
	const rec =
		(name: string, result: unknown) =>
		async (...args: unknown[]) => {
			calls.push([name, ...args]);
			return typeof result === "function" ? (result as (...a: unknown[]) => unknown)(...args) : result;
		};
	const native = {
		configure: rec("configure", undefined),
		restore: rec("restore", { state: "SignedIn", session: SESSION, device: null }),
		getState: rec("getState", {}),
		enrol: rec("enrol", { deviceId: "d1" }),
		login: rec("login", SESSION),
		redeemSignInCode: rec("redeemSignInCode", SESSION),
		actAs: rec("actAs", SESSION),
		endImpersonation: rec("endImpersonation", { session: null }),
		changePin: rec("changePin", undefined),
		getPhoto: rec("getPhoto", { photo: null }),
		setPhoto: rec("setPhoto", SESSION),
		removePhoto: rec("removePhoto", SESSION),
		logout: rec("logout", undefined),
		unlock: rec("unlock", SESSION),
		refresh: rec("refresh", SESSION),
		headersFor: rec("headersFor", { Authorization: "DPoP t1", DPoP: "p1" }),
		headersForRetry: rec("headersForRetry", { Authorization: "DPoP t2", DPoP: "p2" }),
		noteNonce: (...a: unknown[]) => void calls.push(["noteNonce", ...a]),
		keepAlive: (...a: unknown[]) => void calls.push(["keepAlive", ...a]),
		addListener: () => {},
		removeListeners: () => {},
		...Object.fromEntries(Object.entries(over).map(([k, v]) => [k, typeof v === "function" ? rec(k, v) : v])),
	} as unknown as FcFieldNative;
	return { native, calls };
}

function mockEvents(): NativeEventSource & { emit(name: string, p: unknown): void; removed: string[] } {
	const subs = new Map<string, ((p: unknown) => void)[]>();
	const removed: string[] = [];
	return {
		addListener(name, cb) {
			subs.set(name, [...(subs.get(name) ?? []), cb]);
			return { remove: () => void removed.push(name) };
		},
		emit(name, p) {
			for (const cb of subs.get(name) ?? []) cb(p);
		},
		removed,
	};
}

function res(status: number, headers: Record<string, string> = {}): Response {
	return new Response("{}", { status, headers });
}

function client(over: Record<string, unknown> = {}, opts: Record<string, unknown> = {}) {
	const m = mockNative(over);
	const events = mockEvents();
	const c = new FieldClient(CONFIG, { native: m.native, events, ...opts });
	return { c, events, ...m };
}

describe("FieldClient photo methods", () => {
	const WITH_PHOTO: Session = { ...SESSION, worker: { ...SESSION.worker, photoVersion: "0123456789abcdef", canSetOwnPhoto: true } };

	it("getPhoto asks for thumb by default and decodes the base64url bytes", async () => {
		const { c, calls } = client({
			getPhoto: async () => ({ photo: { bytes: "_9j_4A", contentType: "image/jpeg", version: "0123456789abcdef", notModified: false } }),
		});
		const p = await c.getPhoto();
		assert.deepEqual(calls.slice(1), [["getPhoto", "thumb", null]]);
		assert.deepEqual([...(p?.bytes ?? [])], [0xff, 0xd8, 0xff, 0xe0]);
		assert.equal(p?.contentType, "image/jpeg");
		assert.equal(p?.version, "0123456789abcdef");
		assert.equal(p?.notModified, false);
	});

	it("getPhoto passes size and haveVersion, and a not-modified answer carries no bytes", async () => {
		const { c, calls } = client({ getPhoto: async () => ({ photo: { bytes: null, contentType: null, version: "v1", notModified: true } }) });
		const p = await c.getPhoto({ size: "full", haveVersion: "v1" });
		assert.deepEqual(calls.slice(1), [["getPhoto", "full", "v1"]]);
		assert.equal(p?.bytes, null);
		assert.equal(p?.notModified, true);
	});

	it("getPhoto is null when there is no photo", async () => {
		const { c } = client();
		assert.equal(await c.getPhoto(), null);
	});

	it("setPhoto sends the bytes as base64url with the content type and tracks the new session", async () => {
		const { c, calls } = client({ setPhoto: async () => WITH_PHOTO });
		const s = await c.setPhoto({ bytes: new Uint8Array([0xff, 0xd8, 0xff, 0xe0]), contentType: "image/jpeg" });
		assert.deepEqual(calls.slice(1), [["setPhoto", "_9j_4A", "image/jpeg"]]);
		assert.equal(s.worker.photoVersion, "0123456789abcdef");
		assert.equal(c.session?.worker.photoVersion, "0123456789abcdef");
		await c.setPhoto({ bytes: new Uint8Array([1]) });
		assert.deepEqual(calls.at(-1), ["setPhoto", "AQ", null]);
	});

	it("removePhoto resolves the session", async () => {
		const { c, calls } = client({ removePhoto: async () => SESSION });
		const s = await c.removePhoto();
		assert.equal(s.worker.photoVersion, null);
		assert.deepEqual(calls.at(-1), ["removePhoto"]);
	});

	it("the platform's refusals arrive by code, with status and retry time", async () => {
		for (const code of ["OWN_PHOTO_NOT_ALLOWED", "PHOTOS_DISABLED", "PHOTOS_DISABLED_FOR_CLIENT", "IMPERSONATING", "PHOTO_TOO_LARGE", "PHOTO_UNSUPPORTED_TYPE", "PHOTO_TYPE_MISMATCH", "PHOTO_DIMENSIONS_TOO_LARGE", "PHOTO_UNREADABLE", "PHOTO_EMPTY", "PHOTO_BUSY"]) {
			const { c } = client({ setPhoto: async () => { throw { code, message: code, userInfo: { status: 400 } }; } });
			await assert.rejects(c.setPhoto({ bytes: new Uint8Array([1]) }), (e: unknown) => e instanceof FieldAuthError && e.code === code && e.status === 400);
		}
		const { c } = client({ getPhoto: async () => { throw { code: "TOO_MANY_REQUESTS", message: "slow down", userInfo: { status: 429, retryAfterSeconds: 90 } }; } });
		await assert.rejects(c.getPhoto(), (e: unknown) => e instanceof FieldAuthError && e.code === "TOO_MANY_REQUESTS" && e.retryAfterSeconds === 90);
	});
});

describe("FieldClient session methods", () => {
	it("configures the native owner once, before the first call", async () => {
		const { c, calls } = client();
		await c.login({ workerCode: "W001", pin: "1234" });
		await c.refresh();
		assert.deepEqual(calls.map((x) => x[0]), ["configure", "login", "refresh"]);
		assert.deepEqual(calls[0]?.[1], CONFIG);
		assert.deepEqual(calls[1]?.[1], { workerCode: "W001", pin: "1234" });
		assert.equal(c.session?.worker.workerCode, "W001");
	});

	it("restore tracks state and session", async () => {
		const { c } = client({ restore: async () => ({ state: "Locked", session: SESSION, device: null }) });
		const r = await c.restore();
		assert.equal(r.state, "Locked");
		assert.equal(c.state, "Locked");
		assert.equal(c.session, SESSION);
	});

	it("endImpersonation unwraps { session }; logout clears", async () => {
		const { c } = client();
		assert.equal(await c.endImpersonation(), null);
		await c.login({ badgeToken: "B", pin: "1" });
		await c.logout({ scope: "device-all" });
		assert.equal(c.session, null);
		assert.equal(c.state, "SignedOut");
	});

	it("changePin changes then refreshes, returning the session", async () => {
		const { c, calls } = client();
		const s = await c.changePin({ currentPin: "1111", newPin: "2468" });
		assert.equal(s, SESSION);
		assert.deepEqual(calls.slice(1).map((x) => x[0]), ["changePin", "refresh"]);
		assert.deepEqual(calls[1], ["changePin", "1111", "2468"]);
	});

	it("keepAlive and logout pass their arguments", async () => {
		const { c, calls } = client();
		await c.keepAlive(true);
		await c.logout();
		assert.deepEqual(calls[1], ["keepAlive", true]);
		assert.deepEqual(calls[2], ["logout", "session"]);
	});

	it("unlock moves Locked to SignedIn", async () => {
		const { c } = client({ restore: async () => ({ state: "Locked", session: SESSION, device: null }) });
		await c.restore();
		await c.unlock("Unlock please");
		assert.equal(c.state, "SignedIn");
	});
});

describe("error mapping", () => {
	it("passes native codes through with status/retryAfter from userInfo", async () => {
		const { c } = client({
			login: async () => {
				throw Object.assign(new Error("bad pin"), {
					code: "INVALID_CREDENTIALS",
					userInfo: { status: 401, retryAfterSeconds: 30, violations: ["short", 7] },
				});
			},
		});
		await assert.rejects(c.login({ workerCode: "W", pin: "0" }), (e: unknown) => {
			assert.ok(e instanceof FieldAuthError);
			assert.equal(e.code, "INVALID_CREDENTIALS");
			assert.equal(e.status, 401);
			assert.equal(e.retryAfterSeconds, 30);
			assert.deepEqual(e.violations, ["short"]);
			return true;
		});
	});

	it("PIN_RESET_REQUIRED from login and actAs is a typed outcome: one native call, no session change", async () => {
		const n = { login: 0, actAs: 0 };
		const stale = () => {
			throw Object.assign(new Error("pin reset required"), { code: "PIN_RESET_REQUIRED", userInfo: { status: 403 } });
		};
		const { c, calls } = client({ login: async () => (n.login++, stale()), actAs: async () => (n.actAs++, stale()) });
		for (const run of [() => c.login({ workerCode: "W", pin: "1234" }), () => c.actAs({ workerCode: "W2", pin: "1234" })]) {
			await assert.rejects(run(), (e: unknown) => {
				assert.ok(e instanceof FieldAuthError);
				assert.equal(e.code, "PIN_RESET_REQUIRED");
				assert.equal(e.status, 403);
				assert.equal(isPinResetRequired(e), true);
				return true;
			});
		}
		assert.deepEqual(n, { login: 1, actAs: 1 });
		assert.equal(calls.filter(([name]) => name === "logout").length, 0);
		assert.equal(c.session, null);
		assert.equal(isPinResetRequired(new FieldAuthError("INVALID_CREDENTIALS")), false);
	});

	it("an error without a code is INTERNAL", () => {
		assert.equal(mapNativeError(new Error("boom")).code, "INTERNAL");
		assert.equal(mapNativeError(null).code, "INTERNAL");
		assert.equal(mapNativeError({ code: "" }).code, "INTERNAL");
	});

	it("keeps lower-case DPoP and key codes", () => {
		assert.equal(mapNativeError({ code: "use_dpop_nonce" }).code, "use_dpop_nonce");
		assert.equal(mapNativeError({ code: "KEY_INVALIDATED", message: "m" }).code, "KEY_INVALIDATED");
	});

	it("a configure failure rejects every call and is retried next time", async () => {
		let n = 0;
		const { c } = client({
			configure: async () => {
				if (n++ === 0) throw Object.assign(new Error("different config"), { code: "CONFIG_MISMATCH" });
			},
		});
		await assert.rejects(c.refresh(), (e: unknown) => e instanceof FieldAuthError && e.code === "CONFIG_MISMATCH");
		assert.equal((await c.refresh()).worker.id, "w1");
	});

	it("session methods reach the native module on every platform (no iOS gate)", async () => {
		const { c, calls } = client();
		await c.login({ workerCode: "W", pin: "1" });
		await c.headersFor("get", "https://a/b");
		assert.deepEqual(calls.map((x) => x[0]), ["configure", "login", "headersFor"]);
	});

	it("maps a rejection the way React Native iOS delivers it (code, message, userInfo from the NSError)", async () => {
		const iosReject = Object.assign(new Error("wrong PIN"), {
			code: "INVALID_CREDENTIALS",
			domain: "io.flowcatalyst.field",
			userInfo: { status: 401, retryAfterSeconds: 30, violations: ["too short", 7] },
			nativeStackIOS: [],
		});
		const { c } = client({ login: async () => { throw iosReject; } });
		await assert.rejects(c.login({ workerCode: "W", pin: "1" }), (e: unknown) =>
			e instanceof FieldAuthError && e.code === "INVALID_CREDENTIALS" && e.status === 401 &&
			e.retryAfterSeconds === 30 && e.violations.length === 1 && e.violations[0] === "too short");
	});
});

describe("events", () => {
	it("delivers to subscribers, updates session, and unsubscribes", async () => {
		const { c, events } = client();
		const got: unknown[] = [];
		const off = c.on("SessionChanged", (p) => got.push(p));
		const ended: string[] = [];
		c.on("SessionEnded", (p) => ended.push(p.reason));
		await c.login({ workerCode: "W", pin: "1" }); // forces configure + subscription
		events.emit("SessionChanged", { session: SESSION });
		assert.deepEqual(got, [{ session: SESSION }]);
		off();
		events.emit("SessionChanged", { session: null });
		assert.equal(got.length, 1);
		events.emit("SessionEnded", { reason: "DEVICE_REVOKED" });
		assert.deepEqual(ended, ["DEVICE_REVOKED"]);
		assert.equal(c.session, null);
		assert.equal(c.state, "SignedOut");
	});

	it("a throwing listener does not block the others", async () => {
		const { c, events } = client();
		const seen: number[] = [];
		c.on("ExpiryWarning", () => {
			throw new Error("bad listener");
		});
		c.on("ExpiryWarning", (p) => seen.push(p.remainingMs));
		await c.refresh();
		events.emit("ExpiryWarning", { remainingMs: 600000 });
		assert.deepEqual(seen, [600000]);
	});

	it("Locked event sets state; dispose removes native subscriptions", async () => {
		const { c, events } = client();
		c.on("Locked", () => {});
		await c.refresh();
		events.emit("Locked", { session: SESSION });
		assert.equal(c.state, "Locked");
		c.dispose();
		assert.equal(events.removed.length, 6);
	});
});

describe("fetch", () => {
	it("signs with headersFor and keeps the caller's other headers", async () => {
		const seen: { url: string; headers: Record<string, string> }[] = [];
		const { c, calls } = client({}, {
			fetch: async (url: string, init?: RequestInit) => {
				seen.push({ url, headers: init?.headers as Record<string, string> });
				return res(200);
			},
		});
		const r = await c.fetch("https://app.example.com/x?y=1", {
			method: "post",
			headers: { "Content-Type": "application/json", authorization: "stale" },
		});
		assert.equal(r.status, 200);
		assert.deepEqual(calls.find((x) => x[0] === "headersFor"), ["headersFor", "POST", "https://app.example.com/x?y=1"]);
		assert.deepEqual(seen[0]?.headers, { "Content-Type": "application/json", Authorization: "DPoP t1", DPoP: "p1" });
	});

	it("retries once on 401 use_dpop_nonce with the response nonce", async () => {
		let n = 0;
		const sent: Record<string, string>[] = [];
		const { c, calls } = client({}, {
			fetch: async (_u: string, init?: RequestInit) => {
				sent.push(init?.headers as Record<string, string>);
				return n++ === 0
					? res(401, { "WWW-Authenticate": 'DPoP error="use_dpop_nonce"', "DPoP-Nonce": "N1" })
					: res(200, { "DPoP-Nonce": "N2" });
			},
		});
		const r = await c.fetch("https://app.example.com/x");
		assert.equal(r.status, 200);
		assert.equal(sent.length, 2);
		assert.equal(sent[0]?.["DPoP"], "p1");
		assert.equal(sent[1]?.["DPoP"], "p2");
		assert.deepEqual(calls.find((x) => x[0] === "headersForRetry"), ["headersForRetry", "GET", "https://app.example.com/x", "N1"]);
		assert.deepEqual(calls.filter((x) => x[0] === "noteNonce").map((x) => x[2]), ["N1", "N2"]);
	});

	it("retries only once even if the nonce challenge repeats", async () => {
		let n = 0;
		const { c } = client({}, {
			fetch: async () => {
				n++;
				return res(401, { "WWW-Authenticate": 'DPoP error="use_dpop_nonce"', "DPoP-Nonce": "N" + n });
			},
		});
		const r = await c.fetch("https://app.example.com/x");
		assert.equal(r.status, 401);
		assert.equal(n, 2);
	});

	it("does not retry a 401 without a nonce or other 401 reasons, but still notes nonces", async () => {
		let n = 0;
		const { c, calls } = client({}, {
			fetch: async () => {
				n++;
				return res(401, { "WWW-Authenticate": 'DPoP error="invalid_token"', "DPoP-Nonce": "N" });
			},
		});
		assert.equal((await c.fetch("https://a.example.com/x")).status, 401);
		assert.equal(n, 1);
		assert.equal(calls.filter((x) => x[0] === "noteNonce").length, 1);
	});

	it("a transport failure is NETWORK", async () => {
		const { c } = client({}, { fetch: async () => { throw new TypeError("Network request failed"); } });
		await assert.rejects(c.fetch("https://a.example.com/x"), (e: unknown) => e instanceof FieldAuthError && e.code === "NETWORK");
	});

	it("a signing failure rejects with the native code", async () => {
		const { c } = client({
			headersFor: async () => { throw Object.assign(new Error("gone"), { code: "SESSION_EXPIRED" }); },
		}, { fetch: async () => res(200) });
		await assert.rejects(c.fetch("https://a.example.com/x"), (e: unknown) => e instanceof FieldAuthError && e.code === "SESSION_EXPIRED");
	});
});

describe("the client of the session", () => {
	const NO_CLIENT: Session = { ...SESSION, clientId: null, clientIdentifier: null };

	it("every session method and the events hand over clientId and clientIdentifier as the native owner gave them", async () => {
		const { c, events } = client({ restore: async () => ({ state: "SignedIn", session: SESSION, device: null }) });
		assert.equal((await c.restore()).session?.clientIdentifier, "acme");
		for (const s of [
			await c.login({ workerCode: "W001", pin: "1234" }),
			await c.redeemSignInCode("ABCD-1234"),
			await c.actAs({ workerCode: "W002", pin: "1234" }),
			await c.refresh(),
			await c.unlock(),
		]) {
			assert.equal(s.clientId, "clt_1");
			assert.equal(s.clientIdentifier, "acme");
		}
		events.emit("SessionChanged", { session: NO_CLIENT });
		assert.equal(c.session?.clientId, null);
		assert.equal(c.session?.clientIdentifier, null);
	});

	it("a session without the claims stays null, not undefined", async () => {
		const { c } = client({ login: async () => NO_CLIENT });
		const s = await c.login({ workerCode: "W001", pin: "1234" });
		assert.strictEqual(s.clientId, null);
		assert.strictEqual(s.clientIdentifier, null);
	});
});
