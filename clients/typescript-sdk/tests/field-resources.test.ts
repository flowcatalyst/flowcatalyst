import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { FlowCatalystClient } from "../src/index.js";

interface Seen {
	method?: string;
	url?: string;
	body?: unknown;
}

async function withServer(
	fn: (client: FlowCatalystClient, seen: Seen) => Promise<void>,
	responseBody: unknown = {},
) {
	const seen: Seen = {};
	const server: Server = createServer((req, res) => {
		const chunks: Buffer[] = [];
		req.on("data", (c) => chunks.push(c));
		req.on("end", () => {
			seen.method = req.method;
			seen.url = req.url;
			const raw = Buffer.concat(chunks).toString();
			seen.body = raw ? JSON.parse(raw) : undefined;
			res.writeHead(200, { "content-type": "application/json" });
			res.end(JSON.stringify(responseBody));
		});
	});
	await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
	const port = (server.address() as AddressInfo).port;
	try {
		await fn(
			new FlowCatalystClient({
				baseUrl: `http://127.0.0.1:${port}`,
				accessToken: "t",
				retryAttempts: 0,
			}),
			seen,
		);
	} finally {
		await new Promise<void>((r) => server.close(() => r()));
	}
}

const cases: [string, (c: FlowCatalystClient) => unknown, string, string, unknown?][] = [
	["workers.ensure", (c) => c.fieldWorkers().ensure({ clientId: "c1", workerCode: "W1", name: "N", appCodes: ["a"] }), "POST", "/api/field-workers", { clientId: "c1", workerCode: "W1", name: "N", appCodes: ["a"] }],
	["workers.list", (c) => c.fieldWorkers().list("c1", { q: "x", pageSize: 5 }), "GET", "/api/field-workers?q=x&pageSize=5&clientId=c1"],
	["workers.get", (c) => c.fieldWorkers().get("w 1"), "GET", "/api/field-workers/w%201"],
	["workers.update", (c) => c.fieldWorkers().update("w1", { name: "M" }), "PUT", "/api/field-workers/w1", { name: "M" }],
	["workers.delete", (c) => c.fieldWorkers().delete("w1"), "DELETE", "/api/field-workers/w1"],
	["workers.resetPin", (c) => c.fieldWorkers().resetPin("w1"), "POST", "/api/field-workers/w1/pin-reset", {}],
	["workers.unlock", (c) => c.fieldWorkers().unlock("w1"), "POST", "/api/field-workers/w1/unlock"],
	["workers.suspend", (c) => c.fieldWorkers().suspend("w1"), "POST", "/api/field-workers/w1/suspend"],
	["workers.activate", (c) => c.fieldWorkers().activate("w1"), "POST", "/api/field-workers/w1/activate"],
	["workers.grantApp", (c) => c.fieldWorkers().grantApp("w1", "app"), "POST", "/api/field-workers/w1/apps", { appCode: "app" }],
	["workers.revokeApp", (c) => c.fieldWorkers().revokeApp("w1", "app"), "DELETE", "/api/field-workers/w1/apps/app"],
	["workers.signIns", (c) => c.fieldWorkers().signIns("w1", { page: 2 }), "GET", "/api/field-workers/w1/sign-ins?page=2"],
	["workers.ensure (flags)", (c) => c.fieldWorkers().ensure({ clientId: "c1", workerCode: "W1", name: "N", canImpersonate: true, isTest: true }), "POST", "/api/field-workers", { clientId: "c1", workerCode: "W1", name: "N", canImpersonate: true, isTest: true }],
	["workers.update (flags)", (c) => c.fieldWorkers().update("w1", { name: "M", isTest: false, canImpersonate: false }), "PUT", "/api/field-workers/w1", { name: "M", isTest: false, canImpersonate: false }],
	["workers.list (isTest)", (c) => c.fieldWorkers().list("c1", { isTest: true }), "GET", "/api/field-workers?isTest=true&clientId=c1"],
	["workers.issueBadge", (c) => c.fieldWorkers().issueBadge("w1", { label: "L" }), "POST", "/api/field-workers/w1/badges", { label: "L" }],
	["workers.listBadges", (c) => c.fieldWorkers().listBadges("w1"), "GET", "/api/field-workers/w1/badges"],
	["workers.revokeBadge", (c) => c.fieldWorkers().revokeBadge("w1", "b1"), "POST", "/api/field-workers/w1/badges/b1/revoke"],
	["workers.bulkIssueBadges", (c) => c.fieldWorkers().bulkIssueBadges(["w1", "w2"]), "POST", "/api/field-workers/badges", { workerIds: ["w1", "w2"] }],
	["workers.createSignInCode", (c) => c.fieldWorkers().createSignInCode("w1", "orders"), "POST", "/api/field-workers/w1/sign-in-codes", { appCode: "orders" }],
	["policies.put (sign-in options)", (c) => c.fieldPolicies().put("c1", { requireAttestation: false, requireEnrolledDevice: true, unenrolledIdleSecs: 600, badgeOnlyLogin: true, impersonationMaxSecs: 3600 }), "PUT", "/api/field-policies/c1", { requireAttestation: false, requireEnrolledDevice: true, unenrolledIdleSecs: 600, badgeOnlyLogin: true, impersonationMaxSecs: 3600 }],
	["devices.createEnrolmentCode", (c) => c.fieldDevices().createEnrolmentCode({ clientId: "c1", mode: "SHARED", appCodes: ["a"] }), "POST", "/api/field-devices/enrolment-codes", { clientId: "c1", mode: "SHARED", appCodes: ["a"] }],
	["devices.list", (c) => c.fieldDevices().list("c1", { status: "ACTIVE" }), "GET", "/api/field-devices?status=ACTIVE&clientId=c1"],
	["devices.get", (c) => c.fieldDevices().get("d1"), "GET", "/api/field-devices/d1"],
	["devices.suspend", (c) => c.fieldDevices().suspend("d1"), "POST", "/api/field-devices/d1/suspend"],
	["devices.activate", (c) => c.fieldDevices().activate("d1"), "POST", "/api/field-devices/d1/activate"],
	["devices.revoke", (c) => c.fieldDevices().revoke("d1"), "POST", "/api/field-devices/d1/revoke"],
	["policies.get", (c) => c.fieldPolicies().get("c1"), "GET", "/api/field-policies/c1"],
	["policies.put", (c) => c.fieldPolicies().put("c1", { requireAttestation: true, pinMinLength: 6 }), "PUT", "/api/field-policies/c1", { requireAttestation: true, pinMinLength: 6 }],
	["introspectFieldToken", (c) => c.introspectFieldToken("tok"), "POST", "/api/field-tokens/introspect", { token: "tok" }],
];

for (const [name, call, method, url, body] of cases) {
	test(`field resource ${name} -> ${method} ${url.split("?")[0]}`, async () => {
		await withServer(async (client, seen) => {
			const res = (await call(client)) as { isOk(): boolean };
			assert.ok(res.isOk());
			assert.equal(seen.method, method);
			const [wantPath, wantQuery] = url.split("?");
			const [gotPath, gotQuery] = (seen.url ?? "").split("?");
			assert.equal(gotPath, wantPath);
			if (wantQuery) {
				assert.deepEqual(
					[...new URLSearchParams(gotQuery).entries()].sort(),
					[...new URLSearchParams(wantQuery).entries()].sort(),
				);
			}
			assert.deepEqual(seen.body, body);
		});
	});
}
