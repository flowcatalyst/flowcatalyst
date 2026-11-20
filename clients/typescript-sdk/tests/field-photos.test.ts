import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type IncomingHttpHeaders, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { FlowCatalystClient } from "../src/index.js";

interface Seen {
	method?: string;
	url?: string;
	headers?: IncomingHttpHeaders;
	body?: Buffer;
	calls: number;
}

type Reply = { status: number; headers?: Record<string, string>; body?: Buffer | string };

async function withServer(
	reply: (seen: Seen) => Reply,
	fn: (client: FlowCatalystClient, seen: Seen) => Promise<void>,
) {
	const seen: Seen = { calls: 0 };
	const server: Server = createServer((req, res) => {
		const chunks: Buffer[] = [];
		req.on("data", (c) => chunks.push(c));
		req.on("end", () => {
			seen.method = req.method;
			seen.url = req.url;
			seen.headers = req.headers;
			seen.body = Buffer.concat(chunks);
			seen.calls++;
			const r = reply(seen);
			res.writeHead(r.status, r.headers ?? {});
			res.end(r.body);
		});
	});
	await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
	const port = (server.address() as AddressInfo).port;
	try {
		await fn(
			new FlowCatalystClient({ baseUrl: `http://127.0.0.1:${port}`, accessToken: "t", retryAttempts: 0 }),
			seen,
		);
	} finally {
		await new Promise<void>((r) => server.close(() => r()));
	}
}

const WORKER = JSON.stringify({ id: "fwu_1", clientId: "clt_1", workerCode: "W1", name: "W", status: "ACTIVE", apps: [], photo: { version: "0123456789abcdef", updatedAt: "2026-11-20T10:00:00Z" } });
const JPEG = Buffer.from([0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46, 0x49, 0x46, 0x00, 0xff, 0xd9]);

test("setPhoto sends the bytes as they are, with their content type", async () => {
	await withServer(
		() => ({ status: 200, headers: { "content-type": "application/json" }, body: WORKER }),
		async (client, seen) => {
			const r = await client.fieldWorkers().setPhoto("fwu_1", new Uint8Array(JPEG), "image/jpeg");
			assert.ok(r.isOk(), JSON.stringify(r));
			assert.equal(seen.method, "PUT");
			assert.equal(seen.url, "/api/field-workers/fwu_1/photo");
			assert.deepEqual(seen.body, JPEG, "not JSON-encoded");
			assert.equal(seen.headers?.["content-type"], "image/jpeg");
			assert.equal(r._unsafeUnwrap().photo?.version, "0123456789abcdef");

			// A Blob, and no content type: the byte stream type.
			const r2 = await client.fieldWorkers().setPhoto("fwu_1", new Blob([JPEG]));
			assert.ok(r2.isOk());
			assert.deepEqual(seen.body, JPEG);
			assert.equal(seen.headers?.["content-type"], "application/octet-stream");
			// PNG and WebP are only another content type of the same call.
			await client.fieldWorkers().setPhoto("fwu_1", JPEG, "image/webp");
			assert.equal(seen.headers?.["content-type"], "image/webp");
		},
	);
});

test("setPhoto refusals carry the platform's code", async () => {
	await withServer(
		() => ({ status: 415, headers: { "content-type": "application/json" }, body: JSON.stringify({ error: "PHOTO_UNSUPPORTED_TYPE", message: "send a JPEG" }) }),
		async (client, seen) => {
			const r = await client.fieldWorkers().setPhoto("fwu_1", JPEG, "image/gif");
			assert.ok(r.isErr());
			assert.match(JSON.stringify(r._unsafeUnwrapErr()), /PHOTO_UNSUPPORTED_TYPE/);
			assert.equal(seen.calls, 1);
		},
	);
});

test("getPhoto reads the image as bytes, with its validator", async () => {
	await withServer(
		() => ({ status: 200, headers: { "content-type": "image/jpeg", etag: '"0123456789abcdef-thumb"' }, body: JPEG }),
		async (client, seen) => {
			const r = await client.fieldWorkers().getPhoto("fwu_1");
			assert.ok(r.isOk(), JSON.stringify(r));
			const img = r._unsafeUnwrap();
			assert.equal(seen.method, "GET");
			assert.equal(seen.url, "/api/field-workers/fwu_1/photo");
			assert.equal(img.notModified, false);
			assert.deepEqual(Buffer.from(img.bytes ?? []), JPEG, "bytes, not text");
			assert.equal(img.contentType, "image/jpeg");
			assert.equal(img.etag, '"0123456789abcdef-thumb"');
			await client.fieldWorkers().getPhoto("fwu_1", { size: "full" });
			assert.equal(seen.url, "/api/field-workers/fwu_1/photo?size=full");
		},
	);
});

test("getPhoto answers a 304 as not modified, not as an error", async () => {
	await withServer(
		() => ({ status: 304, headers: { etag: '"0123456789abcdef-thumb"' } }),
		async (client, seen) => {
			const r = await client.fieldWorkers().getPhoto("fwu_1", { size: "thumb", ifNoneMatch: '"0123456789abcdef-thumb"' });
			assert.ok(r.isOk(), JSON.stringify(r));
			assert.equal(seen.headers?.["if-none-match"], '"0123456789abcdef-thumb"');
			const img = r._unsafeUnwrap();
			assert.equal(img.notModified, true);
			assert.equal(img.bytes, undefined);
			assert.equal(img.etag, '"0123456789abcdef-thumb"');
		},
	);
});

test("getPhoto of a worker with no photo is a not_found error", async () => {
	await withServer(
		() => ({ status: 404, headers: { "content-type": "application/json" }, body: JSON.stringify({ error: "PHOTO_NOT_FOUND", message: "this worker has no photo" }) }),
		async (client) => {
			const r = await client.fieldWorkers().getPhoto("fwu_1");
			assert.ok(r.isErr());
			const e = r._unsafeUnwrapErr();
			assert.equal(e.type, "not_found");
			assert.match(e.message, /no photo/);
		},
	);
});

test("removePhoto and the client-wide delete", async () => {
	await withServer(
		(seen) => ({ status: 200, headers: { "content-type": "application/json" }, body: seen.url?.includes("field-policies") ? '{"removed":3}' : WORKER }),
		async (client, seen) => {
			const r = await client.fieldWorkers().removePhoto("fwu_1");
			assert.ok(r.isOk());
			assert.equal(seen.method, "DELETE");
			assert.equal(seen.url, "/api/field-workers/fwu_1/photo");
			const d = await client.fieldPolicies().deletePhotos("clt_1");
			assert.equal(seen.method, "DELETE");
			assert.equal(seen.url, "/api/field-policies/clt_1/photos");
			assert.deepEqual(d._unsafeUnwrap(), { removed: 3 });
		},
	);
});

test("the policy carries the two photo settings; a PUT sends what it is given", async () => {
	await withServer(
		() => ({ status: 200, headers: { "content-type": "application/json" }, body: JSON.stringify({ clientId: "c", photosEnabled: true, workerMaySetOwnPhoto: false }) }),
		async (client, seen) => {
			await client.fieldPolicies().put("c", { requireAttestation: false });
			assert.deepEqual(JSON.parse(String(seen.body)), { requireAttestation: false }, "a member left out is not sent: the platform defaults it");
			await client.fieldPolicies().put("c", { requireAttestation: false, photosEnabled: false, workerMaySetOwnPhoto: true });
			assert.deepEqual(JSON.parse(String(seen.body)), { requireAttestation: false, photosEnabled: false, workerMaySetOwnPhoto: true });
			const p = (await client.fieldPolicies().get("c"))._unsafeUnwrap();
			assert.equal(p.photosEnabled, true);
			assert.equal(p.workerMaySetOwnPhoto, false);
		},
	);
});
