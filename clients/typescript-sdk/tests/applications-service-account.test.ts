import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { FlowCatalystClient } from "../src/index.js";

// GET /api/applications/{id}/service-account answers the account as
// GET /api/service-accounts/{id} does (docs/wire-contract.md, "Reading an
// application's service account").
const account = {
	id: "sac_1",
	code: "app:orders",
	name: "Orders Service Account",
	active: true,
	clientIds: [],
	applicationId: "app_1",
	authType: "BEARER_TOKEN",
	roles: ["platform:application-service"],
	principalId: "prn_1",
	oauthClientId: "orders-client",
	createdAt: "2026-11-18T10:00:00Z",
	updatedAt: "2026-11-18T10:00:00Z",
};

async function withPlatform(
	handler: (method: string, url: string) => { status: number; body?: unknown },
	run: (client: FlowCatalystClient, seen: string[]) => Promise<void>,
): Promise<void> {
	const seen: string[] = [];
	const server: Server = createServer((req, res) => {
		seen.push(`${req.method} ${req.url}`);
		const { status, body } = handler(req.method ?? "", req.url ?? "");
		res.writeHead(status, { "content-type": "application/json" });
		res.end(body === undefined ? "" : JSON.stringify(body));
	});
	await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
	const port = (server.address() as AddressInfo).port;
	try {
		await run(
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

test("getServiceAccount reads GET /api/applications/{id}/service-account", async () => {
	await withPlatform(
		() => ({ status: 200, body: account }),
		async (client, seen) => {
			const res = await client.applications().getServiceAccount("app_1");
			assert.ok(res.isOk(), JSON.stringify(res));
			assert.deepEqual(seen, ["GET /api/applications/app_1/service-account"]);
			const sa = res._unsafeUnwrap();
			assert.equal(sa.id, "sac_1");
			assert.equal(sa.code, "app:orders");
			assert.equal(sa.name, "Orders Service Account");
			assert.equal(sa.principalId, "prn_1");
			assert.equal(sa.oauthClientId, "orders-client");
		},
	);
});

test("getServiceAccount of an application without one is not_found", async () => {
	await withPlatform(
		() => ({
			status: 404,
			body: { title: "Not Found", status: 404, detail: "Application app_2 has no service account" },
		}),
		async (client, seen) => {
			const res = await client.applications().getServiceAccount("app_2");
			assert.ok(res.isErr());
			assert.equal(res._unsafeUnwrapErr().type, "not_found");
			assert.equal(seen.length, 1);
		},
	);
});
