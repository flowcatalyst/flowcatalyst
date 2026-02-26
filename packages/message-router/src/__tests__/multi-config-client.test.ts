import { describe, it, expect, vi, beforeEach } from "vitest";
import type { Logger } from "@flowcatalyst/logging";
import type { MessageRouterConfig } from "../clients/platform-config-client.js";
import { mergeConfigs, MultiConfigFetcher } from "../clients/multi-config-client.js";

function makeLogger(): Logger {
	return {
		info: vi.fn(),
		warn: vi.fn(),
		error: vi.fn(),
		debug: vi.fn(),
		fatal: vi.fn(),
		trace: vi.fn(),
		child: () => makeLogger(),
	} as unknown as Logger;
}

function makeConfig(
	overrides: Partial<MessageRouterConfig> = {},
): MessageRouterConfig {
	return {
		queues: [],
		processingPools: [],
		connections: 1,
		...overrides,
	};
}

describe("mergeConfigs", () => {
	let logger: Logger;

	beforeEach(() => {
		logger = makeLogger();
	});

	it("returns null when all sources are null", () => {
		const result = mergeConfigs(
			[
				{ sourceUrl: "http://a", config: null },
				{ sourceUrl: "http://b", config: null },
			],
			logger,
		);
		expect(result).toBeNull();
	});

	it("passes through a single source unchanged", () => {
		const config = makeConfig({
			queues: [{ queueUri: "q1", queueName: "Queue1", connections: 2 }],
			processingPools: [
				{ code: "p1", concurrency: 5, rateLimitPerMinute: null },
			],
			connections: 3,
		});

		const result = mergeConfigs(
			[{ sourceUrl: "http://a", config }],
			logger,
		);
		expect(result).toBe(config);
	});

	it("unions queues and pools from two non-overlapping sources", () => {
		const configA = makeConfig({
			queues: [{ queueUri: "q1", queueName: "Queue1", connections: 1 }],
			processingPools: [
				{ code: "p1", concurrency: 5, rateLimitPerMinute: null },
			],
			connections: 2,
		});
		const configB = makeConfig({
			queues: [{ queueUri: "q2", queueName: "Queue2", connections: 1 }],
			processingPools: [
				{ code: "p2", concurrency: 10, rateLimitPerMinute: 100 },
			],
			connections: 3,
		});

		const result = mergeConfigs(
			[
				{ sourceUrl: "http://a", config: configA },
				{ sourceUrl: "http://b", config: configB },
			],
			logger,
		);

		expect(result).not.toBeNull();
		expect(result!.queues).toHaveLength(2);
		expect(result!.queues.map((q) => q.queueUri)).toEqual(["q1", "q2"]);
		expect(result!.processingPools).toHaveLength(2);
		expect(result!.processingPools.map((p) => p.code)).toEqual([
			"p1",
			"p2",
		]);
		expect(result!.connections).toBe(3);
		expect(logger.warn).not.toHaveBeenCalled();
	});

	it("dedupes overlapping queueUri — first wins and warns on conflict", () => {
		const configA = makeConfig({
			queues: [{ queueUri: "q1", queueName: "Queue1", connections: 1 }],
		});
		const configB = makeConfig({
			queues: [{ queueUri: "q1", queueName: "DifferentName", connections: 5 }],
		});

		const result = mergeConfigs(
			[
				{ sourceUrl: "http://a", config: configA },
				{ sourceUrl: "http://b", config: configB },
			],
			logger,
		);

		expect(result!.queues).toHaveLength(1);
		expect(result!.queues[0]!.queueName).toBe("Queue1");
		expect(logger.warn).toHaveBeenCalledWith(
			expect.objectContaining({
				queueUri: "q1",
				keptSource: "http://a",
				droppedSource: "http://b",
			}),
			expect.stringContaining("Duplicate queue"),
		);
	});

	it("dedupes overlapping pool code — first wins and warns on conflict", () => {
		const configA = makeConfig({
			processingPools: [
				{ code: "p1", concurrency: 5, rateLimitPerMinute: null },
			],
		});
		const configB = makeConfig({
			processingPools: [
				{ code: "p1", concurrency: 10, rateLimitPerMinute: 200 },
			],
		});

		const result = mergeConfigs(
			[
				{ sourceUrl: "http://a", config: configA },
				{ sourceUrl: "http://b", config: configB },
			],
			logger,
		);

		expect(result!.processingPools).toHaveLength(1);
		expect(result!.processingPools[0]!.concurrency).toBe(5);
		expect(logger.warn).toHaveBeenCalledWith(
			expect.objectContaining({
				poolCode: "p1",
				keptSource: "http://a",
				droppedSource: "http://b",
			}),
			expect.stringContaining("Duplicate pool"),
		);
	});

	it("dedupes identical duplicates without warning", () => {
		const queue = { queueUri: "q1", queueName: "Queue1", connections: 2 };
		const pool = { code: "p1", concurrency: 5, rateLimitPerMinute: null };

		const configA = makeConfig({
			queues: [queue],
			processingPools: [pool],
		});
		const configB = makeConfig({
			queues: [{ ...queue }],
			processingPools: [{ ...pool }],
		});

		const result = mergeConfigs(
			[
				{ sourceUrl: "http://a", config: configA },
				{ sourceUrl: "http://b", config: configB },
			],
			logger,
		);

		expect(result!.queues).toHaveLength(1);
		expect(result!.processingPools).toHaveLength(1);
		expect(logger.warn).not.toHaveBeenCalled();
	});

	it("takes max connections across sources", () => {
		const configA = makeConfig({ connections: 2 });
		const configB = makeConfig({ connections: 8 });
		const configC = makeConfig({ connections: 5 });

		const result = mergeConfigs(
			[
				{ sourceUrl: "http://a", config: configA },
				{ sourceUrl: "http://b", config: configB },
				{ sourceUrl: "http://c", config: configC },
			],
			logger,
		);

		expect(result!.connections).toBe(8);
	});

	it("merges only successful sources when some are null (partial failure)", () => {
		const config = makeConfig({
			queues: [{ queueUri: "q1", queueName: "Queue1", connections: 1 }],
			connections: 4,
		});

		const result = mergeConfigs(
			[
				{ sourceUrl: "http://a", config },
				{ sourceUrl: "http://b", config: null },
			],
			logger,
		);

		expect(result).not.toBeNull();
		expect(result!.queues).toHaveLength(1);
		expect(result!.connections).toBe(4);
	});
});

describe("MultiConfigFetcher", () => {
	function makeMockClient(config: MessageRouterConfig | null) {
		return {
			fetchConfig: vi.fn().mockResolvedValue(config),
			fetchConfigOnce: config
				? vi.fn().mockResolvedValue(config)
				: vi.fn().mockRejectedValue(new Error("fetch failed")),
			healthCheck: vi.fn().mockResolvedValue(true),
		} as any;
	}

	it("fetchConfig merges results from multiple sources", async () => {
		const logger = makeLogger();
		const configA = makeConfig({
			queues: [{ queueUri: "q1", queueName: "Q1", connections: 1 }],
			connections: 2,
		});
		const configB = makeConfig({
			queues: [{ queueUri: "q2", queueName: "Q2", connections: 1 }],
			connections: 5,
		});

		const fetcher = new MultiConfigFetcher(
			[
				{ url: "http://a", client: makeMockClient(configA) },
				{ url: "http://b", client: makeMockClient(configB) },
			],
			logger,
		);

		const result = await fetcher.fetchConfig();
		expect(result).not.toBeNull();
		expect(result!.queues).toHaveLength(2);
		expect(result!.connections).toBe(5);
	});

	it("fetchConfig returns null when all sources fail", async () => {
		const logger = makeLogger();
		const fetcher = new MultiConfigFetcher(
			[
				{ url: "http://a", client: makeMockClient(null) },
				{ url: "http://b", client: makeMockClient(null) },
			],
			logger,
		);

		const result = await fetcher.fetchConfig();
		expect(result).toBeNull();
	});

	it("fetchConfigOnce throws when all sources fail", async () => {
		const logger = makeLogger();
		const fetcher = new MultiConfigFetcher(
			[
				{ url: "http://a", client: makeMockClient(null) },
				{ url: "http://b", client: makeMockClient(null) },
			],
			logger,
		);

		await expect(fetcher.fetchConfigOnce()).rejects.toThrow(
			"All config sources failed",
		);
	});

	it("fetchConfigOnce merges partial successes", async () => {
		const logger = makeLogger();
		const config = makeConfig({
			queues: [{ queueUri: "q1", queueName: "Q1", connections: 1 }],
			connections: 3,
		});

		const fetcher = new MultiConfigFetcher(
			[
				{ url: "http://a", client: makeMockClient(config) },
				{ url: "http://b", client: makeMockClient(null) },
			],
			logger,
		);

		const result = await fetcher.fetchConfigOnce();
		expect(result.queues).toHaveLength(1);
		expect(result.connections).toBe(3);
	});
});
