import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { inspect } from "node:util";
import { FlowCatalystClient } from "../src/index.js";

const SECRET = "s3cr3t-plaintext-value";

async function withServer(
	status: number,
	body: unknown,
	fn: (
		client: FlowCatalystClient,
		seen: { method?: string; url?: string; headers?: Record<string, unknown> },
	) => Promise<void>,
) {
	const seen: { method?: string; url?: string; headers?: Record<string, unknown> } = {};
	const server: Server = createServer((req, res) => {
		seen.method = req.method;
		seen.url = req.url;
		seen.headers = req.headers;
		res.writeHead(status, {
			"content-type": "application/json",
			"cache-control": "no-store",
		});
		res.end(JSON.stringify(body));
	});
	await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
	const port = (server.address() as AddressInfo).port;
	try {
		await fn(
			new FlowCatalystClient({
				baseUrl: `http://127.0.0.1:${port}`,
				accessToken: "test-token",
				retryAttempts: 0,
			}),
			seen,
		);
	} finally {
		await new Promise<void>((r) => server.close(() => r()));
	}
}

const okBody = {
	applicationCode: "my app",
	section: "smtp",
	property: "pass/word",
	clientId: "clt_1",
	value: SECRET,
	updatedAt: "2026-10-23T00:00:00Z",
};

test("revealSecret GETs the secret path, URL-encodes segments, sends clientId only when given", async () => {
	await withServer(200, okBody, async (client, seen) => {
		const res = await client.platformConfig().revealSecret("my app", "smtp", "pass/word", "clt_1");
		assert.ok(res.isOk());
		assert.equal(seen.method, "GET");
		assert.equal(
			seen.url,
			"/api/config/my%20app/smtp/pass%2Fword/secret?clientId=clt_1",
		);
		assert.equal(seen.headers?.["cache-control"], undefined);
		assert.equal(seen.headers?.["pragma"], undefined);
		assert.equal(res._unsafeUnwrap().value, SECRET);
		assert.equal(res._unsafeUnwrap().updatedAt, okBody.updatedAt);
	});
	await withServer(200, { ...okBody, clientId: undefined }, async (client, seen) => {
		await client.platformConfig().revealSecret("app", "sec", "prop");
		assert.equal(seen.url, "/api/config/app/sec/prop/secret");
	});
});

test("set, delete and revealSecret send X-On-Behalf-Of only when given", async () => {
	const prop = { applicationCode: "app", section: "s", property: "p", value: "v", valueType: "PLAIN" };
	await withServer(200, prop, async (client, seen) => {
		const pc = client.platformConfig();
		await pc.set("app", "s", "p", { value: "v" }, { onBehalfOf: "usr:alice@example.com" });
		assert.equal(seen.method, "PUT");
		assert.equal(seen.headers?.["x-on-behalf-of"], "usr:alice@example.com");
		await pc.set("app", "s", "p", { value: "v" });
		assert.equal(seen.headers?.["x-on-behalf-of"], undefined);
		await pc.delete("app", "s", "p", { clientId: "clt_1", onBehalfOf: "bob" });
		assert.equal(seen.method, "DELETE");
		assert.equal(seen.url, "/api/config/app/s/p?clientId=clt_1");
		assert.equal(seen.headers?.["x-on-behalf-of"], "bob");
		await pc.delete("app", "s", "p", { onBehalfOf: "   " });
		assert.equal(seen.headers?.["x-on-behalf-of"], undefined);
	});
	await withServer(200, okBody, async (client, seen) => {
		await client.platformConfig().revealSecret("a", "b", "c", undefined, { onBehalfOf: "carol" });
		assert.equal(seen.url, "/api/config/a/b/c/secret");
		assert.equal(seen.headers?.["x-on-behalf-of"], "carol");
	});
});

test("revealSecret value is redacted in JSON, string and inspect output", async () => {
	await withServer(200, okBody, async (client) => {
		const s = (await client.platformConfig().revealSecret("a", "b", "c"))._unsafeUnwrap();
		for (const out of [JSON.stringify(s), String(s), inspect(s), `${s}`, JSON.stringify({ ...s })]) {
			assert.ok(!out.includes(SECRET), `leaked: ${out}`);
		}
		assert.ok(JSON.stringify(s).includes("[REDACTED]"));
		assert.equal(s.value, SECRET);
	});
});

test("revealSecret maps 403 / 404 / 400 NOT_A_SECRET to SDK errors", async () => {
	await withServer(403, { error: "FORBIDDEN", message: "no" }, async (client) => {
		const e = (await client.platformConfig().revealSecret("a", "b", "c"))._unsafeUnwrapErr();
		assert.equal(e.type, "forbidden");
	});
	await withServer(404, { error: "NOT_FOUND", message: "missing" }, async (client) => {
		const e = (await client.platformConfig().revealSecret("a", "b", "c"))._unsafeUnwrapErr();
		assert.equal(e.type, "not_found");
	});
	await withServer(400, { error: "NOT_A_SECRET", message: "plain" }, async (client) => {
		const e = (await client.platformConfig().revealSecret("a", "b", "c"))._unsafeUnwrapErr();
		assert.equal(e.type, "http_error");
		if (e.type === "http_error") {
			assert.equal(e.status, 400);
			assert.equal((e.body as { error?: string }).error, "NOT_A_SECRET");
		}
	});
});

test("get passes clientId and returns the masked value as-is", async () => {
	await withServer(200, { value: "***", valueType: "SECRET" }, async (client, seen) => {
		const res = await client.platformConfig().get("a", "b", "c", { clientId: "x" });
		assert.equal(seen.url, "/api/config/a/b/c?clientId=x");
		assert.equal(res._unsafeUnwrap().value, "***");
	});
});
