import { test, describe } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";

import { CreateEventDto } from "../src/outbox/create-event-dto.js";
import { OutboxManager } from "../src/outbox/outbox-manager.js";
import type { OutboxDriver, OutboxMessage } from "../src/outbox/types.js";

// The Go SDK's copy of the shared outbox vector (the TypeScript SDK reads it in place; the
// framework's conformance/outbox/outbox.json is the same file). Its EVENT rows carry the
// deduplicationId the SDK must write: the event's id, which is the row id.
const vectorPath = fileURLToPath(
	new URL("../../../pkg/fcsdk/outbox/testdata/conformance/outbox.json", import.meta.url),
);

interface Ev {
	eventId: string;
	type: string;
	subjectId: string;
	messageGroup?: string;
	fields: Record<string, unknown>;
}
interface Ctx {
	client?: string;
	principal?: string;
	correlation?: string;
	causation?: string;
	execution?: string;
}
const vector = JSON.parse(readFileSync(vectorPath, "utf8")) as {
	cases: Array<{
		name: string;
		context: Ctx;
		events: Ev[];
		rows: Array<{ type: string; payloadText: string }>;
	}>;
};

function build(ev: Ev, ctx: Ctx): CreateEventDto {
	const seg = ev.type.split(":");
	const data: Record<string, unknown> = {};
	for (const [k, v] of Object.entries(ev.fields)) if (v !== null) data[k] = v;
	let dto = CreateEventDto.create(ev.type, data)
		.withSource(`${seg[0]}:${seg[1]}`)
		.withSubject(`${seg[1]}.${seg[2]}.${ev.subjectId}`)
		.withMessageGroup(ev.messageGroup ?? `${seg[1]}:${seg[2]}:${ev.subjectId}`);
	if (ctx.correlation) dto = dto.withCorrelationId(ctx.correlation);
	if (ctx.causation) dto = dto.withCausationId(ctx.causation);
	if (ctx.client) dto = dto.withClientCode(ctx.client);
	const cd: Array<{ key: string; value: string }> = [];
	if (ctx.principal) cd.push({ key: "principalId", value: ctx.principal });
	if (ctx.execution) cd.push({ key: "executionId", value: ctx.execution });
	return cd.length ? dto.withContextData(cd) : dto;
}

class RecordingDriver implements OutboxDriver {
	rows: OutboxMessage[] = [];
	async insert(m: OutboxMessage): Promise<void> {
		this.rows.push(m);
	}
	async insertBatch(ms: OutboxMessage[]): Promise<void> {
		this.rows.push(...ms);
	}
	// biome-ignore lint/suspicious/noExplicitAny: the other driver methods are not used here
	[k: string]: any;
}

describe("outbox.json EVENT rows", () => {
	for (const c of vector.cases) {
		test(c.name, () => {
			const want = c.rows.filter((r) => r.type === "EVENT");
			assert.equal(want.length, c.events.length);
			c.events.forEach((ev, i) => {
				// The row id the vector pins is the event id; the default dedup id is that row id.
				const got = JSON.stringify(build(ev, c.context).toPayload(ev.eventId));
				assert.equal(got, want[i]!.payloadText);
				// An explicit value is written as given.
				const explicit = build(ev, c.context).withDeduplicationId(ev.eventId);
				assert.equal(JSON.stringify(explicit.toPayload("another-row")), want[i]!.payloadText);
			});
		});
	}
});

describe("deduplicationId", () => {
	test("defaults to the row id, so the stored payload is stable across a resend", async () => {
		const driver = new RecordingDriver();
		const outbox = new OutboxManager(driver, "cli_1");
		const id = await outbox.createEvent(CreateEventDto.create("shop:sales:order:placed", { a: 1 }));
		const [id2, id3] = await outbox.createEvents([
			CreateEventDto.create("shop:sales:order:placed", { a: 1 }),
			CreateEventDto.create("shop:sales:order:placed", { a: 1 }),
		]);
		const ids = [id, id2!, id3!];
		driver.rows.forEach((row, i) => {
			assert.equal(row.id, ids[i]);
			// Reading the row again (a resend) yields the same deduplicationId.
			assert.equal(JSON.parse(row.payload).deduplicationId, row.id);
			assert.equal(JSON.parse(row.payload).deduplicationId, JSON.parse(row.payload).deduplicationId);
		});
		assert.equal(new Set(ids).size, 3, "two identical events are two rows, not one");
	});

	test("an explicit value wins; a blank one does not", async () => {
		const driver = new RecordingDriver();
		const outbox = new OutboxManager(driver, "cli_1");
		const base = CreateEventDto.create("shop:sales:order:placed", {});
		await outbox.createEvent(base.withDeduplicationId("mine"));
		await outbox.createEvent(base.withDeduplicationId("  "));
		assert.equal(JSON.parse(driver.rows[0]!.payload).deduplicationId, "mine");
		assert.equal(JSON.parse(driver.rows[1]!.payload).deduplicationId, driver.rows[1]!.id);
	});

	test("toPayload() with no row id omits it when none is set", () => {
		const p = CreateEventDto.create("shop:sales:order:placed", {}).toPayload();
		assert.equal(p.deduplicationId, undefined);
	});
});
