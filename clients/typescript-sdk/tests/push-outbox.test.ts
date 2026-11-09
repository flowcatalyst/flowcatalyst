import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { existsSync, readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { OutboxManager } from "../src/outbox/outbox-manager.js";
import { CreatePushDto, PushError } from "../src/outbox/create-push-dto.js";
import {
	PgOutboxDriver,
	type PgQueryable,
} from "../src/outbox/drivers/pg-outbox-driver.js";
import type { OutboxDriver, OutboxMessage } from "../src/outbox/types.js";

// The shared vector (pinned copy of the framework's conformance/outbox/outbox-push.json, which
// the Go SDK pins as well): each case is the context, the clock, the pushes and the rows read
// back; invalidPushes are builder inputs and the refusal code.
const vectorPath = fileURLToPath(new URL("./fixtures/push/outbox-push.json", import.meta.url));
const vector = JSON.parse(readFileSync(vectorPath, "utf8")) as {
	cases: Array<{
		name: string;
		now: string;
		context: { client: string };
		pushes: PushInput[];
		rows: Array<{
			id: string;
			type: string;
			message_group: string | null;
			status: number;
			client_id: string;
			payload_size: number;
			headers: unknown;
			created_at: string;
			payloadText: string;
			payload: unknown;
		}>;
	}>;
	invalidPushes: Array<PushInput & { code: string }>;
};

interface PushInput {
	id?: string;
	topic: string;
	type: string;
	data?: unknown;
	dataText?: string;
	padBytes?: number;
	clientCode?: string;
	ungrouped?: boolean;
	messageGroup?: string;
}

function build(v: PushInput): CreatePushDto {
	let data: unknown = v.data;
	if (v.dataText !== undefined) data = v.dataText;
	else if (v.padBytes) data = { blob: "x".repeat(v.padBytes) };
	let p = CreatePushDto.create(v.topic, v.type, data);
	if (v.id) p = p.withId(v.id);
	if (v.clientCode) p = p.withClientCode(v.clientCode);
	if (v.ungrouped) p = p.withUngrouped();
	if (v.messageGroup) p = p.withMessageGroup(v.messageGroup);
	return p;
}

class RecordingDriver implements OutboxDriver {
	rows: OutboxMessage[] = [];
	async insert(m: OutboxMessage): Promise<void> {
		this.rows.push(m);
	}
	async insertBatch(ms: OutboxMessage[]): Promise<void> {
		this.rows.push(...ms);
	}
}

describe("outbox-push.json conformance", () => {
	for (const c of vector.cases) {
		test(c.name, async (t) => {
			t.mock.timers.enable({ apis: ["Date"], now: new Date(c.now).getTime() });
			const driver = new RecordingDriver();
			// The writer's client: row client_id and the default clientCode (`_platform` leaves it out).
			const outbox = new OutboxManager(driver, c.context.client, {
				clientCode: c.context.client,
			});
			for (const p of c.pushes) await outbox.createPush(build(p));

			const want = c.rows.filter((r) => r.type === "PUSH");
			assert.equal(driver.rows.length, want.length);
			driver.rows.forEach((got, i) => {
				const w = want[i]!;
				assert.equal(got.id, w.id);
				assert.equal(got.type, "PUSH");
				assert.equal(got.message_group, w.message_group);
				assert.equal(got.status, w.status);
				assert.equal(got.client_id, w.client_id);
				assert.equal(got.headers, w.headers);
				assert.equal(got.payload, w.payloadText, "payload bytes, members in order");
				assert.equal(got.payload_size, w.payload_size);
				assert.deepEqual(JSON.parse(got.payload), w.payload);
				assert.equal(Date.parse(got.created_at), Date.parse(w.created_at));
				assert.equal(got.updated_at, got.created_at);
			});
		});
	}

	for (const [i, bad] of vector.invalidPushes.entries()) {
		test(`invalidPushes[${i}] ${bad.code}`, async () => {
			const outbox = new OutboxManager(new RecordingDriver(), "cli_1");
			await assert.rejects(
				() => outbox.createPush(build(bad)),
				(e: unknown) => e instanceof PushError && e.code === bad.code,
			);
		});
	}

	test("the pinned vector is the Go SDK's copy while it sits in this repository", (t) => {
		const goCopy = fileURLToPath(
			new URL(
				"../../../pkg/fcsdk/outbox/testdata/conformance/outbox-push.json",
				import.meta.url,
			),
		);
		if (!existsSync(goCopy)) return t.skip("SDK mirrored without the Go SDK");
		assert.equal(readFileSync(vectorPath, "utf8"), readFileSync(goCopy, "utf8"));
	});
});

describe("push rows", () => {
	test("a generated id is a 13-character TSID and the pushId", async () => {
		const driver = new RecordingDriver();
		const outbox = new OutboxManager(driver, "cli_1");
		const id = await outbox.createPush(CreatePushDto.create("shop:board", "refresh", {}));
		assert.equal(id.length, 13);
		assert.equal(JSON.parse(driver.rows[0]!.payload).pushId, id);
		assert.equal(driver.rows[0]!.id, id);
	});

	test("data may be JSON text; whitespace is dropped and numbers are kept as written", async () => {
		const driver = new RecordingDriver();
		const outbox = new OutboxManager(driver, "cli_1");
		await outbox.createPush(
			CreatePushDto.create("shop:board", "refresh", ' { "n" : 1.0 , "s" : "a b" } ').withId("p1"),
		);
		assert.equal(
			driver.rows[0]!.payload,
			'{"topic":"shop:board","type":"refresh","data":{"n":1.0,"s":"a b"},"pushId":"p1"}',
		);
	});

	test("an application's own data limit is honoured, capped at 64 KiB", async () => {
		const outbox = new OutboxManager(new RecordingDriver(), "cli_1");
		const mid = CreatePushDto.create("shop:board", "refresh", { blob: "x".repeat(20000) });
		await assert.rejects(
			() => outbox.createPush(mid),
			(e: unknown) => e instanceof PushError && e.code === "DATA_TOO_LARGE",
		);
		await outbox.createPush(mid.withMaxDataBytes(32768));
		const huge = CreatePushDto.create("shop:board", "refresh", { blob: "x".repeat(70000) });
		await assert.rejects(
			() => outbox.createPush(huge.withMaxDataBytes(1_000_000)),
			(e: unknown) => e instanceof PushError && e.code === "DATA_TOO_LARGE",
		);
	});

	test("createPushes validates every push before writing any", async () => {
		const driver = new RecordingDriver();
		const outbox = new OutboxManager(driver, "cli_1");
		await assert.rejects(() =>
			outbox.createPushes([
				CreatePushDto.create("shop:a", "refresh", {}),
				CreatePushDto.create("Shop:a", "refresh", {}),
			]),
		);
		assert.equal(driver.rows.length, 0);
	});

	test("a push client code wins over the writer's", async () => {
		const driver = new RecordingDriver();
		const outbox = new OutboxManager(driver, "cli_1", { clientCode: "acme" });
		await outbox.createPush(
			CreatePushDto.create("shop:a", "refresh", {}).withClientCode("globex"),
		);
		assert.equal(JSON.parse(driver.rows[0]!.payload).clientCode, "globex");
	});
});

describe("PgOutboxDriver writes PUSH rows with the wake-up", () => {
	function recorder() {
		const calls: Array<{ text: string; params: ReadonlyArray<unknown> }> = [];
		const q: PgQueryable = {
			async query(text, params) {
				calls.push({ text, params: params ?? [] });
				return {};
			},
		};
		return { q, calls };
	}

	test("a PUSH insert is one statement with pg_notify on fc_outbox_push", async () => {
		const { q, calls } = recorder();
		const outbox = new OutboxManager(new PgOutboxDriver(q), "cli_1");
		await outbox.createPush(CreatePushDto.create("shop:a", "refresh", {}));
		await outbox.createEvent(
			(await import("../src/outbox/create-event-dto.js")).CreateEventDto.create("a:b:c:d", {}),
		);
		assert.match(calls[0]!.text, /WITH woken AS \(SELECT pg_notify\('fc_outbox_push', ''\)\)/);
		assert.match(calls[0]!.text, /INSERT INTO outbox_messages/);
		assert.match(calls[0]!.text, /FROM woken/);
		assert.equal(calls[0]!.params.length, 10);
		assert.equal(calls[0]!.params[1], "PUSH");
		assert.doesNotMatch(calls[1]!.text, /pg_notify/, "other rows are the plain insert");
	});

	test("the same statement as the Go SDK's PushInsertStatement (columns and casts)", async () => {
		const { q, calls } = recorder();
		await new PgOutboxDriver(q).insert({
			id: "i",
			type: "PUSH",
			message_group: null,
			payload: "{}",
			status: 0,
			created_at: "2026-01-01T00:00:00.000Z",
			updated_at: "2026-01-01T00:00:00.000Z",
			client_id: "c",
			payload_size: 2,
			headers: null,
		});
		const norm = (s: string) => s.replace(/\s+/g, " ").trim();
		assert.equal(
			norm(calls[0]!.text),
			norm(`WITH woken AS (SELECT pg_notify('fc_outbox_push', ''))
INSERT INTO outbox_messages (
  id, type, message_group, payload, status, created_at, updated_at,
  client_id, payload_size, headers
)
SELECT $1::text, $2::text, $3::text, $4::text, $5::smallint, $6::timestamptz, $7::timestamptz,
  $8::text, $9::integer, $10::jsonb
FROM woken`),
		);
	});
});
