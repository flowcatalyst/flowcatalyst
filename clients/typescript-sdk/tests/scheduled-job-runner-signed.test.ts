/**
 * ScheduledJobRunner signature verification: processSigned / signingSecret /
 * requireSignature, and the one-time unverified warning.
 */
import { afterEach, beforeEach, describe, it, mock } from "node:test";
import assert from "node:assert/strict";
import { createHmac } from "node:crypto";

import { ScheduledJobRunner } from "../src/runner/scheduled-job-runner.js";

const SECRET = "runner-secret";
const BODY = JSON.stringify({
	jobId: "sjb_1",
	jobCode: "nightly",
	instanceId: "sji_1",
	firedAt: "2026-08-07T00:00:00Z",
	triggerKind: "CRON",
	tracksCompletion: false,
	concurrent: true,
});

function sign(ts: string, body: string, secret = SECRET): string {
	return createHmac("sha256", secret).update(ts).update(body).digest("hex");
}

const resource = {} as never;
let calls = 0;
let warn: ReturnType<typeof mock.method>;

function runner(opts: ConstructorParameters<typeof ScheduledJobRunner>[2] = {}) {
	return new ScheduledJobRunner(null, resource, opts).handler("nightly", async () => {
		calls++;
	});
}
const tick = () => new Promise((r) => setTimeout(r, 10));
const unverifiedWarnings = () =>
	warn.mock.calls.filter((c) => String(c.arguments[0]).includes("NOT being signature-verified"))
		.length;

beforeEach(() => {
	calls = 0;
	warn = mock.method(console, "warn", () => {});
});
afterEach(() => mock.restoreAll());

describe("processSigned with a secret", () => {
	it("valid signature runs the handler and returns 202", async () => {
		const ts = new Date().toISOString();
		const r = await runner({ signingSecret: SECRET }).processSigned(BODY, {
			signature: sign(ts, BODY),
			timestamp: ts,
		});
		await tick();
		assert.equal(r.status, 202);
		assert.equal(calls, 1);
		assert.equal(unverifiedWarnings(), 0);
	});

	it("accepts raw bytes", async () => {
		const ts = new Date().toISOString();
		const r = await runner({ signingSecret: SECRET }).processSigned(new TextEncoder().encode(BODY), {
			signature: sign(ts, BODY),
			timestamp: ts,
		});
		assert.equal(r.status, 202);
	});

	const ts = () => new Date().toISOString();
	const bad: Array<[string, () => { body: string; h: { signature?: string; timestamp?: string } }]> = [
		["tampered body", () => { const t = ts(); return { body: BODY.replace("nightly", "evil"), h: { signature: sign(t, BODY), timestamp: t } }; }],
		["bad signature", () => { const t = ts(); return { body: BODY, h: { signature: sign(t, BODY, "other"), timestamp: t } }; }],
		["missing headers", () => ({ body: BODY, h: {} })],
		["stale timestamp", () => { const t = new Date(Date.now() - 3_600_000).toISOString(); return { body: BODY, h: { signature: sign(t, BODY), timestamp: t } }; }],
	];
	for (const [name, mk] of bad) {
		it(`${name} -> 401, handler not called`, async () => {
			const { body, h } = mk();
			const r = await runner({ signingSecret: SECRET }).processSigned(body, h);
			await tick();
			assert.equal(r.status, 401);
			assert.equal(r.ok, false);
			assert.equal(calls, 0);
		});
	}

	it("process() returns 401 when the option was set", async () => {
		const r = await runner({ signingSecret: SECRET }).process(JSON.parse(BODY));
		await tick();
		assert.equal(r.status, 401);
		assert.match((r.bodyJson as { error: string }).error, /processSigned/);
		assert.equal(calls, 0);
	});

	it("a bad signature takes no lock", async () => {
		let acquired = 0;
		const r = runner({
			signingSecret: SECRET,
			enforceLock: true,
			lockProvider: { acquire: async () => { acquired++; return null; } } as never,
		});
		await r.processSigned(BODY, {});
		await tick();
		assert.equal(acquired, 0);
	});
});

describe("without a secret", () => {
	it("process() and processSigned() run the handler; one warning across calls", async () => {
		const r = runner();
		assert.equal((await r.process(JSON.parse(BODY))).status, 202);
		assert.equal((await r.processSigned(BODY, {})).status, 202);
		assert.equal((await r.process(JSON.parse(BODY))).status, 202);
		await tick();
		assert.equal(calls, 3);
		assert.equal(unverifiedWarnings(), 1);
	});

	it("requireSignature: true -> 401 from both methods", async () => {
		const r = runner({ requireSignature: true });
		assert.equal((await r.process(JSON.parse(BODY))).status, 401);
		assert.equal((await r.processSigned(BODY, {})).status, 401);
		await tick();
		assert.equal(calls, 0);
	});
});
