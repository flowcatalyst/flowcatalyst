import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { FlowCatalystClient, type SdkError } from "../src/index.js";

// The retry rule (client.ts, `request`): a request is sent again only when
// that is safe. GET/HEAD/OPTIONS/PUT/DELETE and the POSTs a wrapper marks
// repeatable (the declarative syncs, read-only POSTs) are retried on
// 502/503/504 and on a connection failure or timeout after sending; any other
// POST is not, and ends with an `outcome_unknown` error. 408/429 and a
// connection that was never made are retried for every request.

type Step =
	| { status: number; body?: unknown }
	| "hangup" // read the request, then drop the connection without answering
	| "stall"; // read the request and never answer

type Platform = {
	url: string;
	seen: string[];
	close: () => Promise<void>;
};

async function platform(steps: Step[], port = 0): Promise<Platform> {
	const seen: string[] = [];
	const server: Server = createServer((req, res) => {
		seen.push(`${req.method} ${req.url}`);
		const step = steps.shift() ?? { status: 599, body: { message: "unscripted request" } };
		req.resume();
		req.on("end", () => {
			if (step === "hangup") {
				req.socket.destroy();
				return;
			}
			if (step === "stall") return;
			res.writeHead(step.status, { "content-type": "application/json" });
			res.end(step.body === undefined ? "" : JSON.stringify(step.body));
		});
	});
	await new Promise<void>((r) => server.listen(port, "127.0.0.1", r));
	return {
		url: `http://127.0.0.1:${(server.address() as AddressInfo).port}`,
		seen,
		close: () =>
			new Promise<void>((r) => {
				server.closeAllConnections();
				server.close(() => r());
			}),
	};
}

function client(baseUrl: string, opts: { retryAttempts?: number; timeout?: number } = {}) {
	return new FlowCatalystClient({
		baseUrl,
		accessToken: "test-token",
		retryAttempts: opts.retryAttempts ?? 3,
		retryDelay: 1,
		timeout: opts.timeout ?? 2000,
	});
}

async function withPlatform(
	steps: Step[],
	run: (p: Platform) => Promise<void>,
): Promise<void> {
	const p = await platform(steps);
	try {
		await run(p);
	} finally {
		await p.close();
	}
}

const APP = {
	id: "app_1",
	code: "orders",
	name: "Orders",
	type: "APPLICATION",
	active: true,
	hasLoginClient: false,
	createdAt: "2026-11-18T10:00:00Z",
	updatedAt: "2026-11-18T10:00:00Z",
};
const gateway = (status: number): Step => ({ status, body: { message: `gateway ${status}` } });

function outcomeUnknown(e: SdkError) {
	assert.equal(e.type, "outcome_unknown", JSON.stringify(e));
	if (e.type !== "outcome_unknown") throw new Error("unreachable");
	assert.match(e.message, /may have been applied and was not repeated/);
	return e;
}

test("a GET answered 502 is retried", async () => {
	await withPlatform([gateway(502), { status: 200, body: APP }], async (p) => {
		const res = await client(p.url).applications().get("app_1");
		assert.ok(res.isOk(), JSON.stringify(res));
		assert.equal(p.seen.length, 2);
	});
});

test("a PUT answered 503 is retried", async () => {
	await withPlatform([gateway(503), { status: 204 }], async (p) => {
		const res = await client(p.url).applications().update("app_1", { name: "Orders" });
		assert.ok(res.isOk(), JSON.stringify(res));
		assert.deepEqual(p.seen, ["PUT /api/applications/app_1", "PUT /api/applications/app_1"]);
	});
});

test("a DELETE answered 502 then 404 has succeeded", async () => {
	await withPlatform([gateway(502), { status: 404, body: { message: "not found" } }], async (p) => {
		const res = await client(p.url).applications().delete("app_1");
		assert.ok(res.isOk(), JSON.stringify(res));
		assert.equal(p.seen.length, 2);
	});
});

test("a DELETE cut off after sending then 404 has succeeded", async () => {
	await withPlatform(["hangup", { status: 404, body: { message: "not found" } }], async (p) => {
		const res = await client(p.url).applications().delete("app_1");
		assert.ok(res.isOk(), JSON.stringify(res));
		assert.equal(p.seen.length, 2);
	});
});

test("a DELETE answered 404 first is not found", async () => {
	await withPlatform([{ status: 404, body: { message: "not found" } }], async (p) => {
		const res = await client(p.url).applications().delete("app_1");
		assert.ok(res.isErr());
		assert.equal(res._unsafeUnwrapErr().type, "not_found");
		assert.equal(p.seen.length, 1);
	});
});

test("a DELETE answered 429 then 404 is not found (429 says nothing was applied)", async () => {
	await withPlatform([{ status: 429 }, { status: 404, body: { message: "not found" } }], async (p) => {
		const res = await client(p.url).applications().delete("app_1");
		assert.ok(res.isErr());
		assert.equal(res._unsafeUnwrapErr().type, "not_found");
		assert.equal(p.seen.length, 2);
	});
});

for (const status of [502, 503, 504]) {
	test(`a POST answered ${status} is not repeated and its outcome is unknown`, async () => {
		await withPlatform([gateway(status), { status: 201, body: { id: "app_2" } }], async (p) => {
			const res = await client(p.url).applications().create({ code: "x", name: "X", type: "APPLICATION" });
			assert.ok(res.isErr());
			const e = outcomeUnknown(res._unsafeUnwrapErr());
			assert.equal(e.status, status);
			assert.equal(e.method, "POST");
			assert.equal(e.url, `${p.url}/api/applications`);
			assert.deepEqual(e.body, { message: `gateway ${status}` });
			assert.equal(p.seen.length, 1);
		});
	});
}

test("provisioning answered 502 is sent once; the scripted 409 is never requested", async () => {
	// As reported: the platform committed the provisioning and the gateway lost
	// its answer. A second POST would be refused 409 and the secrets lost.
	await withPlatform(
		[gateway(502), { status: 409, body: { error: "ALREADY_PROVISIONED", message: "already provisioned" } }],
		async (p) => {
			const c = new FlowCatalystClient({ baseUrl: p.url, accessToken: "test-token" }); // default retries
			const res = await c.applications().provisionServiceAccount("app_1");
			assert.ok(res.isErr());
			const e = outcomeUnknown(res._unsafeUnwrapErr());
			assert.equal(e.status, 502);
			assert.deepEqual(p.seen, ["POST /api/applications/app_1/provision-service-account"]);
		},
	);
});

test("a POST answered 500 is the ordinary error, not repeated", async () => {
	await withPlatform([{ status: 500, body: { message: "boom" } }], async (p) => {
		const res = await client(p.url).applications().activate("app_1");
		assert.ok(res.isErr());
		const e = res._unsafeUnwrapErr();
		assert.equal(e.type, "http_error");
		assert.equal(p.seen.length, 1);
	});
});

test("a POST to a declarative sync answered 502 is retried", async () => {
	const result = { applicationCode: "orders", created: 1, updated: 0, deleted: 0, syncedCodes: ["orders:clerk"] };
	await withPlatform([gateway(502), { status: 200, body: result }], async (p) => {
		const res = await client(p.url).roles().sync("orders", [{ name: "clerk" }]);
		assert.ok(res.isOk(), JSON.stringify(res));
		assert.equal(p.seen.length, 2);
		assert.match(p.seen[1]!, /^POST \/api\/applications\/orders\/roles\/sync/);
	});
});

test("a field worker created without a code is retried only with an idempotency key", async () => {
	const worker = { id: "fwu_1", clientId: "clt_1", workerCode: "KTM482", name: "Kim", status: "ACTIVE", apps: [], created: false };
	await withPlatform([gateway(502), { status: 200, body: worker }], async (p) => {
		const res = await client(p.url).fieldWorkers().ensure({ clientId: "clt_1", name: "Kim", idempotencyKey: "hr-1001" });
		assert.ok(res.isOk(), JSON.stringify(res));
		assert.equal(res._unsafeUnwrap().workerCode, "KTM482");
		assert.equal(p.seen.length, 2);
	});
	await withPlatform([gateway(502)], async (p) => {
		const res = await client(p.url).fieldWorkers().ensure({ clientId: "clt_1", name: "Kim" });
		outcomeUnknown(res._unsafeUnwrapErr());
		assert.equal(p.seen.length, 1, "no code and no key: not sent again");
	});
	await withPlatform([gateway(502)], async (p) => {
		const res = await client(p.url).fieldWorkers().ensure({ clientId: "clt_1", workerCode: "W1", name: "Kim" });
		outcomeUnknown(res._unsafeUnwrapErr());
		assert.equal(p.seen.length, 1, "a typed code is sent as before");
	});
});

for (const status of [408, 429]) {
	test(`a POST refused with ${status} is retried`, async () => {
		await withPlatform([{ status }, { status: 201, body: { id: "app_2" } }], async (p) => {
			const res = await client(p.url).applications().create({ code: "x", name: "X", type: "APPLICATION" });
			assert.ok(res.isOk(), JSON.stringify(res));
			assert.equal(p.seen.length, 2);
		});
	});
}

test("a POST that could not connect is retried", async () => {
	// Take a free port, close it, and only listen on it again after the first
	// attempt was refused.
	const probe = await platform([]);
	const port = Number(new URL(probe.url).port);
	await probe.close();
	const c = new FlowCatalystClient({
		baseUrl: `http://127.0.0.1:${port}`,
		accessToken: "test-token",
		retryAttempts: 3,
		retryDelay: 150,
	});
	let p: Platform | undefined;
	const pending = c.applications().create({ code: "x", name: "X", type: "APPLICATION" });
	await new Promise((r) => setTimeout(r, 50));
	p = await platform([{ status: 201, body: { id: "app_2" } }], port);
	try {
		const res = await pending;
		assert.ok(res.isOk(), JSON.stringify(res));
		assert.equal(p.seen.length, 1);
	} finally {
		await p.close();
	}
});

test("a POST that can never connect ends as a network error, not an unknown outcome", async () => {
	const probe = await platform([]);
	const url = probe.url;
	await probe.close();
	const res = await client(url, { retryAttempts: 1 }).applications().create({ code: "x", name: "X", type: "APPLICATION" });
	assert.ok(res.isErr());
	assert.equal(res._unsafeUnwrapErr().type, "network");
});

test("a POST cut off after sending is not repeated and its outcome is unknown", async () => {
	await withPlatform(["hangup", { status: 201, body: { id: "app_2" } }], async (p) => {
		const res = await client(p.url).applications().create({ code: "x", name: "X", type: "APPLICATION" });
		assert.ok(res.isErr());
		const e = outcomeUnknown(res._unsafeUnwrapErr());
		assert.equal(e.status, undefined);
		assert.ok(e.cause instanceof Error);
		assert.equal(p.seen.length, 1);
	});
});

test("a POST that timed out after sending is not repeated and its outcome is unknown", async () => {
	await withPlatform(["stall", { status: 201, body: { id: "app_2" } }], async (p) => {
		const res = await client(p.url, { timeout: 150 }).applications().create({ code: "x", name: "X", type: "APPLICATION" });
		assert.ok(res.isErr());
		outcomeUnknown(res._unsafeUnwrapErr());
		assert.equal(p.seen.length, 1);
	});
});

test("a GET that timed out is retried", async () => {
	await withPlatform(["stall", { status: 200, body: APP }], async (p) => {
		const res = await client(p.url, { timeout: 150 }).applications().get("app_1");
		assert.ok(res.isOk(), JSON.stringify(res));
		assert.equal(p.seen.length, 2);
	});
});

test("with retries off a POST answered 502 is still an unknown outcome, a GET the ordinary error", async () => {
	await withPlatform([gateway(502), gateway(502)], async (p) => {
		const c = client(p.url, { retryAttempts: 0 });
		const post = await c.applications().activate("app_1");
		assert.ok(post.isErr());
		outcomeUnknown(post._unsafeUnwrapErr());
		const get = await c.applications().get("app_1");
		assert.ok(get.isErr());
		const e = get._unsafeUnwrapErr();
		assert.equal(e.type, "http_error");
		assert.equal(e.type === "http_error" ? e.status : 0, 502);
		assert.equal(p.seen.length, 2);
	});
});

test("a wrapper that calls fetch itself follows the rule by its method", async () => {
	// scheduledJobs() and ingestions() do their own fetch and pass the method.
	await withPlatform([gateway(502), gateway(502), { status: 200, body: { id: "sj_1" } }], async (p) => {
		const c = client(p.url);
		const create = await c.scheduledJobs().create({ code: "nightly" } as never);
		assert.ok(create.isErr());
		const e = outcomeUnknown(create._unsafeUnwrapErr());
		assert.equal(e.method, "POST");
		assert.equal(e.url, "/api/scheduled-jobs");
		const get = await c.scheduledJobs().get("sj_1");
		assert.ok(get.isOk(), JSON.stringify(get));
		assert.deepEqual(p.seen, ["POST /api/scheduled-jobs", "GET /api/scheduled-jobs/sj_1", "GET /api/scheduled-jobs/sj_1"]);
	});
});

test("request() takes a per-call repeatable mark for a custom POST", async () => {
	await withPlatform([gateway(503), { status: 200, body: { ok: true } }], async (p) => {
		const c = client(p.url);
		const res = await c.request<{ ok: boolean }>(
			(http, headers) => http.post({ url: "/api/principals/match-emails", headers, body: {} }),
			{ repeatable: true },
		);
		assert.ok(res.isOk(), JSON.stringify(res));
		assert.equal(p.seen.length, 2);
	});
});
