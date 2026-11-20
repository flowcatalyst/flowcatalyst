import { strict as assert } from "node:assert";
import { describe, it } from "node:test";
import { FieldAuthError, STORE_SESSION } from "../../src/index.js";
import {
	FakePlatform,
	GOOD_CODE,
	GOOD_PIN,
	JPEG,
	PHOTO_VERSION,
	rig,
	verifyProof,
	PLATFORM,
	type Rig,
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

async function signedIn(over: Partial<FakePlatform> = {}): Promise<Rig> {
	const p = new FakePlatform();
	Object.assign(p, over);
	const r = rig({}, p);
	await r.auth.enrol("ABCDE12345");
	await r.auth.login({ workerCode: "w1", pin: GOOD_PIN });
	return r;
}

describe("the session's worker", () => {
	it("carries photoVersion and canSetOwnPhoto from the sign-in answer", async () => {
		const r = await signedIn({ photoVersion: PHOTO_VERSION, ownPhoto: true });
		assert.equal(r.auth.session?.worker.photoVersion, PHOTO_VERSION);
		assert.equal(r.auth.session?.worker.canSetOwnPhoto, true);
	});

	it("is null and false with no photo, photos off for the client, or off on the platform", async () => {
		let r = await signedIn();
		assert.deepEqual([r.auth.session?.worker.photoVersion, r.auth.session?.worker.canSetOwnPhoto], [null, false]);
		r = await signedIn({ photoVersion: PHOTO_VERSION, ownPhoto: true, clientPhotosOff: true });
		assert.deepEqual([r.auth.session?.worker.photoVersion, r.auth.session?.worker.canSetOwnPhoto], [null, false]);
		r = await signedIn({ photoVersion: PHOTO_VERSION, ownPhoto: true, photosOff: true });
		assert.deepEqual([r.auth.session?.worker.photoVersion, r.auth.session?.worker.canSetOwnPhoto], [null, false]);
	});

	it("is kept across a restart, and filled in for a session stored by an older version", async () => {
		const r = await signedIn({ photoVersion: PHOTO_VERSION, ownPhoto: true });
		const again = rig({ store: r.store, keys: r.keys }, r.platform);
		const s = (await again.auth.restore()).session;
		assert.equal(s?.worker.photoVersion, PHOTO_VERSION);
		assert.equal(s?.worker.canSetOwnPhoto, true);

		await r.store.set(
			STORE_SESSION,
			JSON.stringify({ worker: { id: "fwu_1", workerCode: "w1", name: "W" }, appCode: "yard", expiresAt: 1, idleExpiresAt: 2, refreshExpiresAt: 3, pinChangeRequired: false }),
		);
		const old = rig({ store: r.store, keys: r.keys }, r.platform);
		const o = (await old.auth.restore()).session;
		assert.equal(o?.worker.photoVersion, null);
		assert.equal(o?.worker.canSetOwnPhoto, false);
	});

	it("learns of a photo that changed elsewhere at the next refresh, by sessionChanged", async () => {
		const r = await signedIn({ photoVersion: PHOTO_VERSION, ownPhoto: true });
		const seen: Array<string | null | undefined> = [];
		r.auth.addEventListener("sessionChanged", (e) => seen.push((e as unknown as { detail: { worker: { photoVersion: string | null } } | null }).detail?.worker.photoVersion));
		r.platform.photoVersion = "fedcba9876543210"; // changed by an administrator
		await r.auth.refresh();
		r.platform.photoVersion = null; // removed
		await r.auth.refresh();
		assert.deepEqual(seen, ["fedcba9876543210", null]);
	});

	it("an act-as session cannot set its own photo", async () => {
		const r = await signedIn({ ownPhoto: true });
		const s = await r.auth.actAs({ workerCode: "w2", pin: GOOD_PIN });
		assert.equal(s.worker.canSetOwnPhoto, false);
		const e = await errOf(r.auth.setPhoto({ bytes: JPEG, contentType: "image/jpeg" }));
		assert.equal(e.code, "IMPERSONATING");
		assert.equal((await errOf(r.auth.removePhoto())).code, "IMPERSONATING");
		assert.equal(r.platform.log.filter((l) => l.url.includes("/photo")).length, 0, "refused before anything was sent");
	});
});

describe("getPhoto", () => {
	it("reads the bytes, signed as the library signs its other calls", async () => {
		const r = await signedIn({ photoVersion: PHOTO_VERSION });
		const p = await r.auth.getPhoto();
		assert.deepEqual(p?.bytes ? [...p.bytes] : null, [...JPEG]);
		assert.equal(p?.contentType, "image/jpeg");
		assert.equal(p?.version, PHOTO_VERSION);
		assert.equal(p?.notModified, false);
		const call = r.platform.log.at(-1)!;
		assert.equal(call.url, `${PLATFORM}/field/worker/photo`);
		assert.deepEqual(JSON.parse(call.body), { size: "thumb" });
		const proof = await verifyProof(call.headers["dpop"]!);
		assert.equal(proof.claims["htu"], `${PLATFORM}/field/worker/photo`);
		assert.ok(proof.claims["ath"], "bound to the access token");
		assert.match(call.headers["authorization"]!, /^DPoP /);
	});

	it("asks for the full size, and answers a held version as not modified", async () => {
		const r = await signedIn({ photoVersion: PHOTO_VERSION });
		const full = await r.auth.getPhoto({ size: "full" });
		assert.deepEqual(JSON.parse(r.platform.log.at(-1)!.body), { size: "full" });
		assert.equal(full?.version, PHOTO_VERSION);
		const again = await r.auth.getPhoto({ size: "full", haveVersion: PHOTO_VERSION });
		assert.deepEqual(JSON.parse(r.platform.log.at(-1)!.body), { size: "full", version: PHOTO_VERSION });
		assert.equal(again?.notModified, true);
		assert.equal(again?.bytes, null);
		assert.equal(again?.version, PHOTO_VERSION);
		const other = await r.auth.getPhoto({ haveVersion: "0000000000000000" });
		assert.equal(other?.notModified, false);
	});

	it("is null when there is no photo, and the platform's refusals come by their codes", async () => {
		let r = await signedIn();
		assert.equal(await r.auth.getPhoto(), null);
		r = await signedIn({ photoVersion: PHOTO_VERSION, photosOff: true });
		assert.equal((await errOf(r.auth.getPhoto())).code, "PHOTOS_DISABLED");
		r = await signedIn({ photoVersion: PHOTO_VERSION });
		r.platform.photoError = { status: 429, error: "TOO_MANY_REQUESTS", retryAfter: 90 };
		const e = await errOf(r.auth.getPhoto());
		assert.equal(e.code, "TOO_MANY_REQUESTS");
		assert.equal(e.retryAfter, 90);
		assert.equal(e.status, 429);
	});
});

describe("setPhoto and removePhoto", () => {
	it("sends the bytes as they are, with their content type, and updates the session", async () => {
		const r = await signedIn({ ownPhoto: true });
		const events: unknown[] = [];
		r.auth.addEventListener("sessionChanged", (e) => events.push((e as unknown as { detail: unknown }).detail));
		const s = await r.auth.setPhoto({ bytes: JPEG, contentType: "image/jpeg" });
		const call = r.platform.log.at(-1)!;
		assert.equal(call.url, `${PLATFORM}/field/worker/photo/set`);
		assert.equal(call.headers["content-type"], "image/jpeg");
		assert.deepEqual([...(call.bytes ?? [])], [...JPEG]);
		assert.equal(s.worker.photoVersion, r.platform.photoVersion);
		assert.notEqual(s.worker.photoVersion, null);
		assert.equal(r.auth.session?.worker.photoVersion, s.worker.photoVersion);
		assert.equal(events.length, 1, "sessionChanged");
		// stored: a restart sees it
		const again = rig({ store: r.store, keys: r.keys }, r.platform);
		assert.equal((await again.auth.restore()).session?.worker.photoVersion, s.worker.photoVersion);
		const proof = await verifyProof(call.headers["dpop"]!);
		assert.equal(proof.claims["htu"], `${PLATFORM}/field/worker/photo/set`);
	});

	it("takes a Blob, and sends a byte stream when no type is given", async () => {
		const r = await signedIn({ ownPhoto: true });
		await r.auth.setPhoto({ bytes: new Blob([JPEG as BlobPart], { type: "image/png" }) });
		assert.equal(r.platform.log.at(-1)!.headers["content-type"], "application/octet-stream");
		assert.deepEqual([...(r.platform.uploaded?.bytes ?? [])], [...JPEG]);
	});

	it("removePhoto clears the version and tells listeners", async () => {
		const r = await signedIn({ ownPhoto: true, photoVersion: PHOTO_VERSION });
		const s = await r.auth.removePhoto();
		assert.equal(s.worker.photoVersion, null);
		assert.equal(r.platform.photoVersion, null);
		assert.equal(r.platform.log.at(-1)!.url, `${PLATFORM}/field/worker/photo/remove`);
	});

	it("surfaces the platform's refusals by their codes", async () => {
		const jpg = { bytes: JPEG, contentType: "image/jpeg" };
		let r = await signedIn(); // opt-in off
		assert.equal((await errOf(r.auth.setPhoto(jpg))).code, "OWN_PHOTO_NOT_ALLOWED");
		assert.equal((await errOf(r.auth.removePhoto())).code, "OWN_PHOTO_NOT_ALLOWED");
		r = await signedIn({ ownPhoto: true, clientPhotosOff: true });
		assert.equal((await errOf(r.auth.setPhoto(jpg))).code, "PHOTOS_DISABLED_FOR_CLIENT");
		r = await signedIn({ ownPhoto: true, photosOff: true });
		assert.equal((await errOf(r.auth.setPhoto(jpg))).code, "PHOTOS_DISABLED");
		r = await signedIn({ ownPhoto: true });
		for (const [status, code] of [[413, "PHOTO_TOO_LARGE"], [415, "PHOTO_UNSUPPORTED_TYPE"], [400, "PHOTO_TYPE_MISMATCH"], [400, "PHOTO_DIMENSIONS_TOO_LARGE"], [400, "PHOTO_UNREADABLE"], [503, "PHOTO_BUSY"]] as const) {
			r.platform.photoError = { status, error: code };
			const e = await errOf(r.auth.setPhoto(jpg));
			assert.equal(e.code, code);
			assert.equal(e.status, status);
		}
		r.platform.photoError = { status: 429, error: "TOO_MANY_REQUESTS", retryAfter: 30 };
		const e = await errOf(r.auth.setPhoto(jpg));
		assert.equal(e.retryAfter, 30);
		assert.equal(r.auth.session?.worker.photoVersion, null, "a refused upload changed nothing");
		assert.equal((await errOf(rig().auth.setPhoto(jpg))).code, "SESSION_EXPIRED", "no session, nothing sent");
		void GOOD_CODE;
	});
});
