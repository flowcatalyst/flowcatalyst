import type { Logger } from "@flowcatalyst/logging";
import type {
	ConfigFetcher,
	MessageRouterConfig,
	PlatformConfigClient,
} from "./platform-config-client.js";

interface ConfigSource {
	url: string;
	client: PlatformConfigClient;
}

interface ConfigResult {
	sourceUrl: string;
	config: MessageRouterConfig | null;
}

/**
 * Merge multiple router configurations using union merge strategy.
 * - Queues are deduped by `queueUri` (first wins); warns on conflicting duplicates.
 * - Pools are deduped by `code` (first wins); warns on conflicting duplicates.
 * - `connections` takes the max across all sources.
 * Returns `null` if all sources failed (config is null).
 */
export function mergeConfigs(
	results: ConfigResult[],
	logger: Logger,
): MessageRouterConfig | null {
	const successful = results.filter(
		(r): r is ConfigResult & { config: MessageRouterConfig } =>
			r.config !== null,
	);

	if (successful.length === 0) {
		return null;
	}

	// Single source — passthrough
	if (successful.length === 1) {
		return successful[0]!.config;
	}

	const seenQueues = new Map<
		string,
		{ source: string; queue: MessageRouterConfig["queues"][number] }
	>();
	const mergedQueues: MessageRouterConfig["queues"] = [];

	const seenPools = new Map<
		string,
		{ source: string; pool: MessageRouterConfig["processingPools"][number] }
	>();
	const mergedPools: MessageRouterConfig["processingPools"] = [];

	let maxConnections = 0;

	for (const { sourceUrl, config } of successful) {
		// Merge queues
		for (const queue of config.queues) {
			const existing = seenQueues.get(queue.queueUri);
			if (existing) {
				// Check if the duplicate has different values
				if (
					existing.queue.queueName !== queue.queueName ||
					existing.queue.connections !== queue.connections
				) {
					logger.warn(
						{
							queueUri: queue.queueUri,
							keptSource: existing.source,
							droppedSource: sourceUrl,
						},
						`Duplicate queue "${queue.queueUri}" with conflicting values — keeping first`,
					);
				}
				// Skip duplicate (first wins)
				continue;
			}
			seenQueues.set(queue.queueUri, { source: sourceUrl, queue });
			mergedQueues.push(queue);
		}

		// Merge pools
		for (const pool of config.processingPools) {
			const existing = seenPools.get(pool.code);
			if (existing) {
				if (
					existing.pool.concurrency !== pool.concurrency ||
					existing.pool.rateLimitPerMinute !== pool.rateLimitPerMinute
				) {
					logger.warn(
						{
							poolCode: pool.code,
							keptSource: existing.source,
							droppedSource: sourceUrl,
						},
						`Duplicate pool "${pool.code}" with conflicting values — keeping first`,
					);
				}
				continue;
			}
			seenPools.set(pool.code, { source: sourceUrl, pool });
			mergedPools.push(pool);
		}

		// Connections — take max
		maxConnections = Math.max(maxConnections, config.connections);
	}

	return {
		queues: mergedQueues,
		processingPools: mergedPools,
		connections: maxConnections,
	};
}

/**
 * Fetches configuration from multiple platform endpoints in parallel
 * and merges the results. Implements the ConfigFetcher interface.
 */
export class MultiConfigFetcher implements ConfigFetcher {
	private readonly sources: ConfigSource[];
	private readonly logger: Logger;

	constructor(sources: ConfigSource[], logger: Logger) {
		this.sources = sources;
		this.logger = logger.child({ component: "MultiConfigFetcher" });
	}

	/**
	 * Fetch config from all sources in parallel (with retry per source).
	 * Returns merged config, or `null` only if ALL sources fail.
	 */
	async fetchConfig(): Promise<MessageRouterConfig | null> {
		const results = await Promise.all(
			this.sources.map(async ({ url, client }) => {
				const config = await client.fetchConfig();
				return { sourceUrl: url, config };
			}),
		);

		return mergeConfigs(results, this.logger);
	}

	/**
	 * Fetch config once (no per-source retry) from all sources in parallel.
	 * Merges successful results. Throws only if ALL sources fail.
	 */
	async fetchConfigOnce(): Promise<MessageRouterConfig> {
		const settled = await Promise.allSettled(
			this.sources.map(async ({ url, client }) => {
				const config = await client.fetchConfigOnce();
				return { sourceUrl: url, config };
			}),
		);

		const results: ConfigResult[] = [];

		for (let i = 0; i < settled.length; i++) {
			const result = settled[i]!;
			const source = this.sources[i]!;

			if (result.status === "fulfilled") {
				results.push(result.value);
			} else {
				this.logger.warn(
					{ sourceUrl: source.url, err: result.reason },
					`Failed to fetch config from ${source.url}`,
				);
				results.push({ sourceUrl: source.url, config: null });
			}
		}

		const merged = mergeConfigs(results, this.logger);
		if (!merged) {
			throw new Error(
				"All config sources failed — cannot fetch configuration",
			);
		}

		return merged;
	}
}
