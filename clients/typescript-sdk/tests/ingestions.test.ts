import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import {
	FlowCatalystClient,
	type IngestionDefinition,
	type CreateIngestionRequest,
} from "../src/index.js";

interface Seen {
	method?: string;
	url?: string;
	body?: unknown;
}

async function withServer(
	status: number,
	body: unknown,
	fn: (client: FlowCatalystClient, seen: Seen) => Promise<void>,
	contentType = "application/json",
) {
	const seen: Seen = {};
	const server: Server = createServer((req, res) => {
		seen.method = req.method;
		seen.url = req.url;
		const chunks: Buffer[] = [];
		req.on("data", (c: Buffer) => chunks.push(c));
		req.on("end", () => {
			const raw = Buffer.concat(chunks).toString();
			seen.body = raw ? JSON.parse(raw) : undefined;
			res.writeHead(status, { "content-type": contentType });
			res.end(typeof body === "string" ? body : JSON.stringify(body));
		});
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

const definition: IngestionDefinition = {
	name: "Orders feed",
	source: { kind: "sftp", host: "sftp.example.com", passwordSecret: "ORDERS_SFTP_PASSWORD" },
	split: { kind: "csv", header: true },
	target: {
		code: "orders",
		targetUrl: "https://example.com/hook",
		pool: "default",
		connection: "orders-conn",
		mode: "BLOCK_ON_ERROR",
		batchSize: 50,
	},
};

const create: CreateIngestionRequest = {
	...definition,
	application: "integrations",
	client: "acme",
	code: "orders-feed",
};

test("create POSTs the definition and returns the ingestion", async () => {
	await withServer(201, { id: "ing_1", ...definition }, async (client, seen) => {
		const res = await client.ingestions().create(create);
		assert.ok(res.isOk());
		assert.equal(seen.method, "POST");
		assert.equal(seen.url, "/api/ingestions");
		assert.deepEqual(seen.body, create);
	});
});

test("custom source kind with settings round-trips", async () => {
	const custom: CreateIngestionRequest = {
		...create,
		source: { kind: "custom", settings: { yaml: "from: timer" } },
	};
	await withServer(201, { id: "ing_1", ...custom }, async (client, seen) => {
		const res = await client.ingestions().create(custom);
		assert.ok(res.isOk());
		assert.deepEqual((seen.body as CreateIngestionRequest).source, custom.source);
		assert.deepEqual(res._unsafeUnwrap().source, custom.source);
	});
});

test("list sends only the filters given and encodes them", async () => {
	await withServer(200, { ingestions: [], hasMore: false }, async (client, seen) => {
		await client.ingestions().list();
		assert.equal(seen.url, "/api/ingestions");
		await client.ingestions().list({ clientId: "a&b", applicationCode: "integrations", limit: 10 });
		assert.equal(seen.url, "/api/ingestions?clientId=a%26b&applicationCode=integrations&limit=10");
	});
});

test("list hands back the cursor and it is passed as after", async () => {
	await withServer(
		200,
		{ ingestions: [], hasMore: true, nextCursor: "c/1" },
		async (client, seen) => {
			const page = (await client.ingestions().list())._unsafeUnwrap();
			assert.equal(page.hasMore, true);
			await client.ingestions().list({ after: page.nextCursor });
			assert.equal(seen.url, "/api/ingestions?after=c%2F1");
		},
	);
});

test("id-addressed operations encode the id and use the right verb", async () => {
	await withServer(200, {}, async (client, seen) => {
		const r = client.ingestions();
		const cases: Array<[() => Promise<unknown>, string, string]> = [
			[async () => r.get("a/b"), "GET", "/api/ingestions/a%2Fb"],
			[async () => r.pause("a/b"), "POST", "/api/ingestions/a%2Fb/pause"],
			[async () => r.resume("a/b"), "POST", "/api/ingestions/a%2Fb/resume"],
			[async () => r.listVersions("a/b"), "GET", "/api/ingestions/a%2Fb/versions"],
			[async () => r.makeVersionLive("a/b", 3), "POST", "/api/ingestions/a%2Fb/versions/3/live"],
			[async () => r.listHistory("a/b", { limit: 5, offset: 10 }), "GET", "/api/ingestions/a%2Fb/history?limit=5&offset=10"],
			[async () => r.listFailures("a/b", { after: "x" }), "GET", "/api/ingestions/a%2Fb/failures?after=x"],
			[async () => r.update("a/b", { ...definition, expectedVersion: 2 }), "PUT", "/api/ingestions/a%2Fb"],
			[async () => r.saveVersion("a/b", { ...definition, live: true }), "POST", "/api/ingestions/a%2Fb/versions"],
			[async () => r.listSourceKinds(), "GET", "/api/ingestion-source-kinds"],
			[async () => r.getTest("t/1"), "GET", "/api/ingestion-tests/t%2F1"],
		];
		for (const [call, method, url] of cases) {
			await call();
			assert.equal(seen.method, method);
			assert.equal(seen.url, url);
		}
	});
});

test("update carries expectedVersion", async () => {
	await withServer(200, {}, async (client, seen) => {
		await client.ingestions().update("ing_1", { ...definition, expectedVersion: 0 });
		assert.equal((seen.body as { expectedVersion: number }).expectedVersion, 0);
	});
});

test("delete resolves on 204", async () => {
	await withServer(204, "", async (client, seen) => {
		const res = await client.ingestions().delete("ing_1");
		assert.ok(res.isOk());
		assert.equal(seen.method, "DELETE");
	});
});

test("getDefinition returns the YAML text untouched", async () => {
	const yaml = "name: Orders feed\nsource:\n  kind: sftp\n";
	await withServer(
		200,
		yaml,
		async (client, seen) => {
			const res = await client.ingestions().getDefinition("ing_1");
			assert.equal(seen.url, "/api/ingestions/ing_1/definition");
			assert.equal(res._unsafeUnwrap(), yaml);
		},
		"application/yaml",
	);
});

test("testConnection POSTs a saved id or a definition", async () => {
	await withServer(202, { id: "t1", kind: "sftp", status: "PENDING" }, async (client, seen) => {
		const res = await client.ingestions().testConnection({ ingestionId: "ing_1" });
		assert.ok(res.isOk());
		assert.equal(seen.method, "POST");
		assert.equal(seen.url, "/api/ingestion-tests");
		assert.deepEqual(seen.body, { ingestionId: "ing_1" });
		await client.ingestions().testConnection({ definition, application: "integrations", client: "acme" });
		assert.equal((seen.body as { client: string }).client, "acme");
	});
});

test("applySet sends dryRun only when given and encodes the name", async () => {
	await withServer(200, { name: "s", dryRun: true, changes: [] }, async (client, seen) => {
		const items = [{ ...definition, application: "integrations", client: "acme", code: "orders-feed" }];
		await client.ingestions().applySet("my set", { items });
		assert.equal(seen.method, "PUT");
		assert.equal(seen.url, "/api/ingestion-sets/my%20set");
		await client.ingestions().applySet("my set", { items }, true);
		assert.equal(seen.url, "/api/ingestion-sets/my%20set?dryRun=true");
		assert.deepEqual((seen.body as { items: unknown[] }).items, items);
	});
});

test("set reads, reapply and release use their routes", async () => {
	await withServer(200, {}, async (client, seen) => {
		const r = client.ingestions();
		await r.listSets();
		assert.equal(seen.url, "/api/ingestion-sets");
		await r.listSetApplies("s");
		assert.equal(seen.url, "/api/ingestion-sets/s/applies");
		await r.getSetApply("s", "a/1");
		assert.equal(seen.url, "/api/ingestion-sets/s/applies/a%2F1");
		await r.reapplySet("s", "a1");
		assert.equal(seen.method, "POST");
		assert.equal(seen.url, "/api/ingestion-sets/s/applies/a1/reapply");
		await r.reapplySet("s", "a1", false);
		assert.equal(seen.url, "/api/ingestion-sets/s/applies/a1/reapply?dryRun=false");
		await r.releaseSet("s");
		assert.equal(seen.method, "POST");
		assert.equal(seen.url, "/api/ingestion-sets/s/release");
	});
});

test("error statuses map to typed errors", async () => {
	await withServer(400, { error: "VALIDATION", message: "bad pool" }, async (client) => {
		const err = (await client.ingestions().create(create))._unsafeUnwrapErr();
		assert.equal(err.type, "http_error");
		assert.equal((err as { status: number }).status, 400);
		assert.equal(err.message, "bad pool");
	});
	await withServer(403, { error: "FORBIDDEN", message: "no" }, async (client) => {
		const err = (await client.ingestions().get("x"))._unsafeUnwrapErr();
		assert.equal(err.type, "forbidden");
	});
	await withServer(404, { error: "NOT_FOUND", message: "gone" }, async (client) => {
		const err = (await client.ingestions().get("x"))._unsafeUnwrapErr();
		assert.equal(err.type, "not_found");
	});
	await withServer(
		409,
		{ error: "INGESTION_CODE_EXISTS", message: "exists" },
		async (client) => {
			const err = (await client.ingestions().create(create))._unsafeUnwrapErr();
			assert.equal(err.type, "conflict");
			assert.equal((err as { code?: string }).code, "INGESTION_CODE_EXISTS");
		},
	);
});

test("listFailures sends category and undismissed only when given", async () => {
	await withServer(200, { failures: [], hasMore: false }, async (client, seen) => {
		const r = client.ingestions();
		await r.listFailures("ing_1");
		assert.equal(seen.url, "/api/ingestions/ing_1/failures");
		await r.listFailures("ing_1", { category: "rejected", undismissed: true, after: "c", limit: 10 });
		assert.equal(
			seen.url,
			"/api/ingestions/ing_1/failures?category=rejected&after=c&limit=10&undismissed=true",
		);
		await r.listFailures("ing_1", { undismissed: false });
		assert.equal(seen.url, "/api/ingestions/ing_1/failures");
	});
});

test("dismissFailure POSTs the dismiss route and returns the failure", async () => {
	const failure = {
		id: "igf_1",
		kind: "JOBS_REJECTED",
		category: "rejected",
		message: "refused",
		afterRecords: 0,
		occurredAt: "2026-10-25T00:00:00Z",
		createdAt: "2026-10-25T00:00:01Z",
		dismissedAt: "2026-10-25T01:00:00Z",
		dismissedBy: "prn_1",
	};
	await withServer(200, failure, async (client, seen) => {
		const res = await client.ingestions().dismissFailure("ing/1", "igf/1");
		assert.equal(seen.method, "POST");
		assert.equal(seen.url, "/api/ingestions/ing%2F1/failures/igf%2F1/dismiss");
		assert.equal(seen.body, undefined);
		const f = res._unsafeUnwrap();
		assert.equal(f.category, "rejected");
		assert.equal(f.dismissedBy, "prn_1");
	});
});

test("listSecrets GETs names and fetches, never values", async () => {
	const body = {
		secrets: [
			{
				name: "ORDERS_SFTP_PASSWORD",
				bound: true,
				fetches: [
					{
						runtimeId: "rt-1",
						version: 3,
						principalId: "prn_rt",
						lastFetchedAt: "2026-10-25T00:00:00Z",
						fetchCount: 7,
					},
				],
			},
		],
	};
	await withServer(200, body, async (client, seen) => {
		const res = await client.ingestions().listSecrets("ing_1");
		assert.equal(seen.method, "GET");
		assert.equal(seen.url, "/api/ingestions/ing_1/secrets");
		const list = res._unsafeUnwrap();
		assert.equal(list.secrets[0]?.fetches[0]?.fetchCount, 7);
		assert.equal(list.secrets[0]?.bound, true);
	});
});

test("create answers a draft with its save outcome, notices and warnings", async () => {
	const answer = {
		id: "ing_1",
		...definition,
		notices: 2,
		latestNotice: "jobs refused",
		latestNoticeAt: "2026-10-25T00:00:00Z",
		warnings: ["sftp source names no host key"],
		save: { outcome: "DRAFT", version: 1, awaitingPromote: true, message: "saved as a draft" },
	};
	await withServer(201, answer, async (client) => {
		const ing = (await client.ingestions().create({ ...create, live: true }))._unsafeUnwrap();
		assert.equal(ing.save?.outcome, "DRAFT");
		assert.equal(ing.save?.awaitingPromote, true);
		assert.equal(ing.notices, 2);
		assert.deepEqual(ing.warnings, ["sftp source names no host key"]);
	});
});

test("an sftp source carries acceptAnyHostKey", async () => {
	const sftp: CreateIngestionRequest = {
		...create,
		source: {
			kind: "sftp",
			host: "sftp.example.com",
			port: 22,
			directory: "/in",
			username: "u",
			credential: { name: "ORDERS_SFTP_PASSWORD" },
			acceptAnyHostKey: true,
			every: "PT5M",
		},
	};
	await withServer(201, { id: "ing_1", ...sftp }, async (client, seen) => {
		await client.ingestions().create(sftp);
		const sent = seen.body as { source: { acceptAnyHostKey?: boolean } };
		assert.equal(sent.source.acceptAnyHostKey, true);
	});
});
