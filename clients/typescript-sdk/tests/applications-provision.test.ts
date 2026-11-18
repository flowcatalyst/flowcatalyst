import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { FlowCatalystClient } from "../src/index.js";

// The platform's answer to POST /api/applications/{id}/provision-service-account
// (docs/wire-contract.md, "Application service-account provisioning").
const provisioned = {
	message: "Service account provisioned",
	serviceAccount: {
		principalId: "prn_1",
		name: "Orders Service Account",
		oauthClient: { id: "oac_1", clientId: "cid", clientSecret: "csecret" },
		webhook: { authToken: "fc_token", signingSecret: "sign-me" },
	},
};

test("provisionServiceAccount POSTs the provisioning path and returns the platform's shape", async () => {
	const seen: { method?: string; url?: string } = {};
	const server: Server = createServer((req, res) => {
		seen.method = req.method;
		seen.url = req.url;
		res.writeHead(201, { "content-type": "application/json" });
		res.end(JSON.stringify(provisioned));
	});
	await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
	const port = (server.address() as AddressInfo).port;
	try {
		const client = new FlowCatalystClient({
			baseUrl: `http://127.0.0.1:${port}`,
			accessToken: "test-token",
			retryAttempts: 0,
		});
		const res = await client.applications().provisionServiceAccount("app_1");
		assert.ok(res.isOk());
		assert.equal(seen.method, "POST");
		assert.equal(seen.url, "/api/applications/app_1/provision-service-account");
		const sa = res._unsafeUnwrap().serviceAccount;
		assert.equal(sa.principalId, "prn_1");
		assert.equal(sa.oauthClient.clientId, "cid");
		assert.equal(sa.oauthClient.clientSecret, "csecret");
		assert.equal(sa.webhook?.signingSecret, "sign-me");
		assert.equal(sa.webhook?.authToken, "fc_token");
	} finally {
		await new Promise<void>((r) => server.close(() => r()));
	}
});
