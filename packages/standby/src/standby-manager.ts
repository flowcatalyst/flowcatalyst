import type { Logger } from "@flowcatalyst/logging";
import type { StandbyConfig, StandbyStatus } from "./types.js";
import { LockManager } from "./lock-manager.js";

/** How often to poll for the lock when in standby (ms) */
const POLL_INTERVAL_MS = 10_000;
/** How often to refresh the lock when primary (ms) */
const REFRESH_INTERVAL_MS = 10_000;

/**
 * Simple distributed leader election for the root application.
 *
 * Usage:
 *   const mgr = await createStandbyManager(config, logger);
 *   await mgr.waitUntilPrimary();   // blocks until this instance wins the lock
 *   mgr.startRefreshing();          // keep the lock alive
 *   // ... start services ...
 *   // on shutdown:
 *   await mgr.stop();               // release lock so standby can take over immediately
 */
export class StandbyManager {
	private readonly config: StandbyConfig;
	private readonly lockManager: LockManager;
	private readonly logger: Logger;
	private refreshInterval: ReturnType<typeof setInterval> | null = null;

	constructor(config: StandbyConfig, lockManager: LockManager, logger: Logger) {
		this.config = config;
		this.lockManager = lockManager;
		this.logger = logger.child({ component: "StandbyManager" });
	}

	/**
	 * Block until this instance acquires the primary lock.
	 * Polls every 10 seconds if another instance holds it.
	 */
	async waitUntilPrimary(): Promise<void> {
		this.logger.info(
			{
				instanceId: this.config.instanceId,
				lockKey: this.config.lockKey,
			},
			"Hot standby enabled — competing for primary lock",
		);

		// eslint-disable-next-line no-constant-condition
		while (true) {
			try {
				const acquired = await this.lockManager.acquireLock();
				if (acquired) {
					this.logger.info(
						{ instanceId: this.config.instanceId },
						"Acquired primary lock — starting services",
					);
					return;
				}

				const holder = await this.lockManager.getCurrentLockHolder();
				this.logger.info(
					{ currentHolder: holder, retryMs: POLL_INTERVAL_MS },
					"Standby — waiting for primary lock",
				);
			} catch (err) {
				this.logger.error(
					{ err, retryMs: POLL_INTERVAL_MS },
					"Redis error while competing for lock — retrying",
				);
			}

			await new Promise((resolve) => setTimeout(resolve, POLL_INTERVAL_MS));
		}
	}

	/**
	 * Start periodic lock refresh to maintain primary status.
	 * If the lock is lost unexpectedly, exits the process (same as message-router behaviour).
	 */
	startRefreshing(): void {
		this.refreshInterval = setInterval(async () => {
			try {
				const refreshed = await this.lockManager.refreshLock();
				if (!refreshed) {
					this.logger.error(
						"CRITICAL: Primary lost lock unexpectedly — exiting",
					);
					setTimeout(() => process.exit(1), 1000);
				}
			} catch (err) {
				this.logger.error({ err }, "Redis error during lock refresh");
			}
		}, REFRESH_INTERVAL_MS);
	}

	/**
	 * Release the lock and stop refreshing.
	 * Call this before stopping services so the standby instance can take over immediately.
	 */
	async stop(): Promise<void> {
		if (this.refreshInterval) {
			clearInterval(this.refreshInterval);
			this.refreshInterval = null;
		}
		await this.lockManager.releaseLock();
		this.lockManager.shutdown();
		this.logger.info("Primary lock released");
	}

	async getStatus(): Promise<StandbyStatus> {
		const currentLockHolder = await this.lockManager.getCurrentLockHolder();
		return {
			instanceId: this.config.instanceId,
			isPrimary: this.lockManager.isHoldingLock(),
			redisAvailable: true,
			lastSuccessfulRefresh: null,
			currentLockHolder,
			hasWarning: false,
		};
	}
}

/**
 * Create a StandbyManager, connecting to Redis.
 * Returns null if standby is disabled or Redis URL is missing.
 */
export async function createStandbyManager(
	config: StandbyConfig,
	logger: Logger,
): Promise<StandbyManager | null> {
	if (!config.enabled) return null;

	if (!config.redisUrl) {
		logger.warn(
			"STANDBY_ENABLED=true but no REDIS_URL configured — running as single instance",
		);
		return null;
	}

	const { Redis } = await import("ioredis");
	const redis = new Redis(config.redisUrl, {
		maxRetriesPerRequest: 3,
		retryStrategy: (times: number) => Math.min(times * 200, 5000),
		lazyConnect: true,
	});

	try {
		await redis.connect();
		logger.info({ redisUrl: config.redisUrl }, "Redis connected for standby");
	} catch (err) {
		logger.error({ err }, "Failed to connect to Redis for standby — running as single instance");
		return null;
	}

	const lockManager = new LockManager(redis, config, logger);
	return new StandbyManager(config, lockManager, logger);
}
