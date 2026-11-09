import { test } from "node:test";
import assert from "node:assert/strict";
import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";

import { FlowCatalystClient, usecase, CreatePushDto, TicketMinter } from "../src/index.js";
import { defineApplication } from "../src/sync/definitions.js";
import type { OutboxDriver, OutboxMessage } from "../src/outbox/index.js";

// ─── Plan.andPush ────────────────────────────────────────────────────────────
// Same in-memory harness as usecase-envelope.test.ts: a transaction is a buffer of deferred
// writes, flushed on success and discarded on throw or a failed Result.

type Tx = { ops: Array<() => void> };

class MemoryDriver implements OutboxDriver {
	committed: OutboxMessage[] = [];
	async insert(message: OutboxMessage, tx?: unknown): Promise<void> {
		if (tx) (tx as Tx).ops.push(() => this.committed.push(message));
		else this.committed.push(message);
	}
	async insertBatch(messages: OutboxMessage[], tx?: unknown): Promise<void> {
		for (const m of messages) await this.insert(m, tx);
	}
	async withTransaction<T>(callback: (tx: unknown) => Promise<T>): Promise<T> {
		const tx: Tx = { ops: [] };
		const result = await callback(tx);
		for (const op of tx.ops) op();
		return result;
	}
}

interface Order extends usecase.Aggregate {
	readonly id: string;
}

class MemoryOrderRepo implements usecase.Repo<Order> {
	saved: Order[] = [];
	async persist(order: Order, tx: unknown): Promise<void> {
		(tx as Tx).ops.push(() => this.saved.push(order));
	}
	async delete(): Promise<void> {}
}

class OrderPlaced extends usecase.BaseDomainEvent<{ orderId: string }> {
	constructor(ctx: usecase.ExecutionContext, data: { orderId: string }) {
		super(
			{
				eventType: "shop:orders:order:placed",
				specVersion: "1.0",
				source: "shop:orders",
				subject: usecase.DomainEvent.subject("orders", "order", data.orderId),
				messageGroup: usecase.DomainEvent.messageGroup("orders", "order", data.orderId),
			},
			ctx,
			data,
		);
	}
}

const ctx = () => usecase.ExecutionContext.create("prn_test");

function op(
	repo: MemoryOrderRepo,
	pushes: (e: OrderPlaced) => Array<CreatePushDto | ((e: OrderPlaced) => CreatePushDto)>,
): usecase.Operation<usecase.Command, OrderPlaced> {
	return {
		authorize: usecase.publicAuthorize,
		async execute(_cmd, c) {
			const order: Order = { id: "ord_1" };
			const event = new OrderPlaced(c, { orderId: order.id });
			return usecase.Plan.andPush(usecase.Plan.save(order, repo, event), ...pushes(event));
		},
	};
}

test("Plan.andPush writes the PUSH rows in the plan's transaction, after the event", async () => {
	const driver = new MemoryDriver();
	const repo = new MemoryOrderRepo();
	const uow = usecase.OutboxUnitOfWork.fromDriver(driver, "cli_1", { clientCode: "acme" });
	const result = await usecase.run(
		uow,
		op(repo, () => [
			CreatePushDto.create("shop:orders:ord_1", "order.updated", { status: "placed" }),
			(e) => CreatePushDto.create("shop:board", "refresh", { eventId: e.eventId }).withUngrouped(),
		]),
		{},
		ctx(),
	);
	assert.ok(usecase.Result.isSuccess(result));
	assert.deepEqual(driver.committed.map((m) => m.type), ["EVENT", "PUSH", "PUSH"]);
	assert.equal(repo.saved.length, 1);
	const [p1, p2] = [driver.committed[1]!, driver.committed[2]!];
	assert.equal(p1.message_group, "shop:orders:ord_1");
	assert.equal(JSON.parse(p1.payload).clientCode, "acme", "the unit of work's client code");
	assert.equal(p2.message_group, null);
	assert.equal(JSON.parse(p2.payload).data.eventId, usecase.Result.unwrap(result).eventId);
});

test("an invalid push fails the plan: validation error, nothing written, aggregate rolled back", async () => {
	const driver = new MemoryDriver();
	const repo = new MemoryOrderRepo();
	const uow = usecase.OutboxUnitOfWork.fromDriver(driver, "cli_1");
	const result = await usecase.run(
		uow,
		op(repo, () => [CreatePushDto.create("shop:orders:ord_1", "fw.reserved", {})]),
		{},
		ctx(),
	);
	assert.ok(usecase.Result.isFailure(result));
	if (usecase.Result.isFailure(result)) {
		assert.equal(result.error.type, "validation");
		assert.equal(result.error.code, "TYPE_RESERVED");
	}
	assert.equal(driver.committed.length, 0, "no event, no push");
	assert.equal(repo.saved.length, 0, "aggregate not persisted");
});

test("a plan without pushes is unchanged", async () => {
	const driver = new MemoryDriver();
	const result = await usecase.run(
		usecase.OutboxUnitOfWork.fromDriver(driver, "cli_1"),
		op(new MemoryOrderRepo(), () => []),
		{},
		ctx(),
	);
	assert.ok(usecase.Result.isSuccess(result));
	assert.deepEqual(driver.committed.map((m) => m.type), ["EVENT"]);
});

// ─── Push sync ───────────────────────────────────────────────────────────────

const SEED = "5BgJptHbkz4uIeRqgLz3Ws_38Ovwi1uCpSdyW45yMdc";

async function withServer(
	handler: (url: string, body: unknown) => unknown,
	fn: (client: FlowCatalystClient, seen: Array<{ url: string; body: any }>) => Promise<void>,
) {
	const seen: Array<{ url: string; body: any }> = [];
	const server: Server = createServer((req, res) => {
		let raw = "";
		req.on("data", (c) => (raw += c));
		req.on("end", () => {
			const body = raw ? JSON.parse(raw) : undefined;
			seen.push({ url: req.url ?? "", body });
			res.writeHead(200, { "content-type": "application/json" });
			res.end(JSON.stringify(handler(req.url ?? "", body)));
		});
	});
	await new Promise<void>((r) => server.listen(0, "127.0.0.1", r));
	const port = (server.address() as AddressInfo).port;
	try {
		await fn(new FlowCatalystClient({ baseUrl: `http://127.0.0.1:${port}`, accessToken: "t" }), seen);
	} finally {
		await new Promise<void>((r) => server.close(() => r()));
	}
}

const pushResponse = {
	applicationCode: "shop",
	rules: { created: 2, updated: 0, deleted: 0 },
	keys: { created: 1, updated: 0, deleted: 0 },
	syncedPatterns: ["shop:orders:{id}", "shop:board"],
	syncedKids: ["k1"],
};

test("syncPush posts {rules, keys: JWKS} to /push/sync", async () => {
	const minter = TicketMinter.fromConfig("shop", `k1:${SEED}`);
	await withServer(
		() => pushResponse,
		async (client, seen) => {
			const r = await client.definitions().syncPush("shop", {
				rules: [
					{ pattern: "shop:orders:{id}", requires: ["shop:orders:order:view"] },
					{ pattern: "shop:board", public: true },
				],
				keys: minter,
			});
			assert.ok(r.isOk());
			assert.deepEqual(r._unsafeUnwrap(), pushResponse);
			assert.equal(seen.length, 1);
			assert.equal(seen[0]!.url, "/api/applications/shop/push/sync");
			assert.deepEqual(seen[0]!.body, {
				rules: [
					{ pattern: "shop:orders:{id}", requires: ["shop:orders:order:view"] },
					{ pattern: "shop:board", public: true },
				],
				keys: minter.jwks(),
			});
		},
	);
});

test("DefinitionSet.push rides sync(): a push step with the gateway's counts, skippable", async () => {
	const minter = TicketMinter.fromConfig("shop", `k1:${SEED}`);
	const set = defineApplication("shop")
		.withPush({ rules: [{ pattern: "shop:board", public: true }], keys: minter.jwks() })
		.build();
	await withServer(
		() => pushResponse,
		async (client, seen) => {
			const r = await client.definitions().sync(set);
			assert.ok(r.isOk(), JSON.stringify(r));
			const v = r._unsafeUnwrap();
			assert.deepEqual(v.push, pushResponse);
			assert.deepEqual(seen.map((s) => s.url), ["/api/applications/shop/push/sync"]);

			const skipped = await client.definitions().sync(set, { skipPush: true });
			assert.ok(skipped.isOk());
			assert.deepEqual(skipped._unsafeUnwrap().push, { skipped: true });
			assert.equal(seen.length, 1, "no second call");
		},
	);
	// a set with no push declaration leaves the gateway alone
	await withServer(
		() => ({}),
		async (client, seen) => {
			const r = await client.definitions().sync(defineApplication("shop").build());
			assert.ok(r.isOk());
			assert.deepEqual(r._unsafeUnwrap().push, { skipped: true });
			assert.equal(seen.length, 0);
		},
	);
});

test("syncGrouped merges push declarations of one application: rules concatenated, keys de-duplicated by kid", async () => {
	const a = TicketMinter.fromConfig("shop", `k1:${SEED}`);
	const s1 = defineApplication("shop").withPush({ rules: [{ pattern: "shop:a", public: true }], keys: a }).build();
	const s2 = defineApplication("shop")
		.forClient("acme")
		.withPush({ rules: [{ pattern: "shop:b", public: true }], keys: a.jwks() })
		.build();
	await withServer(
		() => pushResponse,
		async (client, seen) => {
			await client.definitions().syncGrouped([s1, s2]);
			const pushCalls = seen.filter((s) => s.url.endsWith("/push/sync"));
			assert.equal(pushCalls.length, 1);
			assert.deepEqual(pushCalls[0]!.body.rules.map((r: { pattern: string }) => r.pattern), ["shop:a", "shop:b"]);
			assert.equal(pushCalls[0]!.body.keys.keys.length, 1);
		},
	);
});
