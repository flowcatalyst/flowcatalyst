/**
 * Connection Cache
 *
 * In-memory cache of Connection entities for the hot dispatch path.
 * Eliminates per-event DB queries when building dispatch jobs.
 *
 * Strategy:
 * - Full load on start(), then background refresh every N minutes.
 * - Sync get() for pure cache hits (sub-microsecond).
 * - Async resolve() / resolveMany() with DB fallback on miss.
 * - set() / remove() for immediate invalidation after mutations.
 *
 * At 10k events/sec the hot path must never hit the database for
 * connection lookups — only genuinely new connections trigger a
 * single DB read that is then cached.
 */

import type { Connection } from "../../domain/index.js";
import type { ConnectionRepository } from "../persistence/repositories/connection-repository.js";

const DEFAULT_REFRESH_INTERVAL_MS = 5 * 60 * 1000; // 5 minutes

export interface ConnectionCacheLogger {
	info(obj: Record<string, unknown>, msg: string): void;
	error(obj: Record<string, unknown>, msg: string): void;
	debug(obj: Record<string, unknown>, msg: string): void;
}

export interface ConnectionCacheOptions {
	readonly refreshIntervalMs?: number | undefined;
	readonly logger?: ConnectionCacheLogger | undefined;
}

export interface ConnectionCache {
	/** Sync lookup — returns cached value or undefined (no DB). */
	get(id: string): Connection | undefined;

	/** Async lookup with DB fallback — populates cache on miss. */
	resolve(id: string): Promise<Connection | undefined>;

	/** Batch async lookup — serves from cache, falls back to DB for misses only. */
	resolveMany(ids: string[]): Promise<Map<string, Connection>>;

	/** Immediately update a cache entry (call after create/update mutations). */
	set(connection: Connection): void;

	/** Remove from cache (call after delete mutations). */
	remove(id: string): void;

	/** Load all connections and start background refresh. */
	start(): Promise<void>;

	/** Stop background refresh timer. */
	stop(): void;
}

export function createConnectionCache(
	connectionRepository: ConnectionRepository,
	options?: ConnectionCacheOptions,
): ConnectionCache {
	const refreshIntervalMs =
		options?.refreshIntervalMs ?? DEFAULT_REFRESH_INTERVAL_MS;
	const logger = options?.logger;

	// The cache — replaced atomically on each refresh
	let cache = new Map<string, Connection>();
	let refreshTimer: ReturnType<typeof setInterval> | null = null;

	/**
	 * Full refresh: load all connections, swap the map atomically.
	 * Any concurrent get() reads from the old map until swap completes.
	 */
	async function refresh(): Promise<void> {
		const all = await connectionRepository.findAll();
		const next = new Map<string, Connection>();
		for (const conn of all) {
			next.set(conn.id, conn);
		}
		cache = next;
		logger?.debug(
			{ count: next.size },
			"Connection cache refreshed",
		);
	}

	return {
		get(id: string): Connection | undefined {
			return cache.get(id);
		},

		async resolve(id: string): Promise<Connection | undefined> {
			const cached = cache.get(id);
			if (cached) return cached;

			// Cache miss — fall through to DB
			const conn = await connectionRepository.findById(id);
			if (conn) {
				cache.set(conn.id, conn);
			}
			return conn;
		},

		async resolveMany(ids: string[]): Promise<Map<string, Connection>> {
			const result = new Map<string, Connection>();
			const missingIds: string[] = [];

			for (const id of ids) {
				const cached = cache.get(id);
				if (cached) {
					result.set(id, cached);
				} else {
					missingIds.push(id);
				}
			}

			// Only hit DB for genuine cache misses
			if (missingIds.length > 0) {
				const fetched =
					await connectionRepository.findByIds(missingIds);
				for (const conn of fetched) {
					cache.set(conn.id, conn);
					result.set(conn.id, conn);
				}
			}

			return result;
		},

		set(connection: Connection): void {
			cache.set(connection.id, connection);
		},

		remove(id: string): void {
			cache.delete(id);
		},

		async start(): Promise<void> {
			await refresh();
			refreshTimer = setInterval(() => {
				refresh().catch((err) => {
					logger?.error(
						{ err },
						"Connection cache background refresh failed",
					);
				});
			}, refreshIntervalMs);
			logger?.info(
				{ refreshIntervalMs, initialSize: cache.size },
				"Connection cache started",
			);
		},

		stop(): void {
			if (refreshTimer) {
				clearInterval(refreshTimer);
				refreshTimer = null;
			}
			logger?.info({}, "Connection cache stopped");
		},
	};
}
