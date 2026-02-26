import type { Redis } from "ioredis";
import type { Logger } from "@flowcatalyst/logging";
import type { StandbyConfig } from "./types.js";

/**
 * Lua script: Atomically refresh lock TTL only if we own it.
 * Returns 1 if refreshed, 0 if we don't own the lock.
 *
 * KEYS[1] = lock key
 * ARGV[1] = our instance ID
 * ARGV[2] = TTL in seconds
 */
const REFRESH_SCRIPT = `
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("expire", KEYS[1], ARGV[2])
else
    return 0
end
`;

/**
 * Lua script: Atomically release lock only if we own it.
 * Returns 1 if deleted, 0 if we don't own the lock.
 *
 * KEYS[1] = lock key
 * ARGV[1] = our instance ID
 */
const RELEASE_SCRIPT = `
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end
`;

/** Refresh interval as fraction of TTL (1/3 = refresh 3x before expiry) */
const REFRESH_DIVISOR = 3;

/**
 * Manages Redis-based distributed lock for primary/standby election.
 *
 * Direct port of Java LockManager:
 * - SET NX EX pattern for atomic lock acquisition
 * - Lua scripts for atomic refresh and release operations
 * - Automatic lock renewal (watchdog) while lock is held
 */
export class LockManager {
	private readonly redis: Redis;
	private readonly config: StandbyConfig;
	private readonly logger: Logger;

	private lockHeldByThisInstance = false;
	private watchdogInterval: ReturnType<typeof setInterval> | null = null;

	constructor(redis: Redis, config: StandbyConfig, logger: Logger) {
		this.redis = redis;
		this.config = config;
		this.logger = logger.child({ component: "LockManager" });
	}

	/**
	 * Attempt to acquire the distributed lock using SET NX EX pattern.
	 * If acquired, starts the automatic renewal watchdog.
	 *
	 * @throws Error if Redis is unavailable
	 */
	async acquireLock(): Promise<boolean> {
		const { lockKey, instanceId, lockTtlSeconds } = this.config;

		try {
			// Atomic SET NX EX: only set if key doesn't exist, with TTL
			const result = await this.redis.set(
				lockKey,
				instanceId,
				"EX",
				lockTtlSeconds,
				"NX",
			);

			if (result === "OK") {
				this.lockHeldByThisInstance = true;
				this.startWatchdog();
				this.logger.info({ instanceId }, "Lock acquired");
				return true;
			}

			// Lock exists - check if we already own it (e.g., after restart with same instanceId)
			const currentOwner = await this.redis.get(lockKey);
			if (currentOwner === instanceId) {
				await this.redis.expire(lockKey, lockTtlSeconds);
				this.lockHeldByThisInstance = true;
				this.startWatchdog();
				this.logger.info("Lock already owned by this instance, refreshed TTL");
				return true;
			}

			this.logger.debug({ currentOwner }, "Lock held by another instance");
			return false;
		} catch (error) {
			throw new Error(
				`Redis connection failed - unable to acquire lock: ${error}`,
				{ cause: error },
			);
		}
	}

	/**
	 * Check if we still hold the lock and refresh it if needed.
	 * The watchdog handles automatic refresh, this verifies ownership.
	 *
	 * @throws Error if Redis is unavailable
	 */
	async refreshLock(): Promise<boolean> {
		if (!this.lockHeldByThisInstance) {
			return false;
		}

		try {
			const refreshed = await this.executeRefreshScript();

			if (!refreshed) {
				this.logger.warn("Lost lock - refresh failed (lock expired or stolen)");
				this.lockHeldByThisInstance = false;
				this.stopWatchdog();
				return false;
			}

			this.logger.debug(
				{ instanceId: this.config.instanceId },
				"Lock refreshed",
			);
			return true;
		} catch (error) {
			throw new Error(
				`Redis connection failed - unable to refresh lock: ${error}`,
				{ cause: error },
			);
		}
	}

	/**
	 * Release the distributed lock immediately (for graceful shutdown).
	 * Uses atomic Lua script to only release if we own it.
	 */
	async releaseLock(): Promise<boolean> {
		this.stopWatchdog();

		if (!this.lockHeldByThisInstance) {
			this.logger.debug("Lock not held by this instance, nothing to release");
			return false;
		}

		try {
			const { lockKey, instanceId } = this.config;
			const result = await this.redis.eval(
				RELEASE_SCRIPT,
				1,
				lockKey,
				instanceId,
			);

			this.lockHeldByThisInstance = false;

			if (result === 1) {
				this.logger.info({ instanceId }, "Lock released");
				return true;
			}

			this.logger.warn("Lock was not held by this instance (already expired?)");
			return false;
		} catch (error) {
			this.logger.warn(
				{ err: error },
				"Error releasing lock (will expire automatically)",
			);
			this.lockHeldByThisInstance = false;
			return false;
		}
	}

	/**
	 * Check if we currently hold the lock.
	 */
	isHoldingLock(): boolean {
		return this.lockHeldByThisInstance;
	}

	/**
	 * Get current lock holder info (for monitoring/debugging).
	 */
	async getCurrentLockHolder(): Promise<string> {
		try {
			const owner = await this.redis.get(this.config.lockKey);
			return owner ?? "unlocked";
		} catch (error) {
			this.logger.warn({ err: error }, "Failed to get lock status");
			return "unknown";
		}
	}

	/**
	 * Stop the watchdog and release resources.
	 */
	shutdown(): void {
		this.stopWatchdog();
	}

	/**
	 * Start the watchdog task that automatically refreshes the lock TTL.
	 * Runs at TTL/3 interval to ensure lock is refreshed well before expiry.
	 */
	private startWatchdog(): void {
		if (this.watchdogInterval) {
			return; // Already running
		}

		const ttlSeconds = this.config.lockTtlSeconds;
		const refreshIntervalMs =
			Math.max(1, Math.floor(ttlSeconds / REFRESH_DIVISOR)) * 1000;

		this.watchdogInterval = setInterval(() => {
			this.watchdogRefresh();
		}, refreshIntervalMs);

		this.logger.info(
			{ refreshIntervalMs, ttlSeconds },
			"Lock watchdog started",
		);
	}

	private stopWatchdog(): void {
		if (this.watchdogInterval) {
			clearInterval(this.watchdogInterval);
			this.watchdogInterval = null;
			this.logger.debug("Lock watchdog stopped");
		}
	}

	/**
	 * Watchdog refresh task - called periodically to refresh lock TTL.
	 */
	private async watchdogRefresh(): Promise<void> {
		if (!this.lockHeldByThisInstance) {
			this.stopWatchdog();
			return;
		}

		try {
			const refreshed = await this.executeRefreshScript();

			if (refreshed) {
				this.logger.debug("Watchdog: Lock TTL refreshed");
			} else {
				this.logger.error("Watchdog: Lost lock - TTL refresh failed!");
				this.lockHeldByThisInstance = false;
				this.stopWatchdog();
			}
		} catch (error) {
			this.logger.error(
				{ err: error },
				"Watchdog: Error refreshing lock",
			);
			// Don't set lockHeldByThisInstance to false here - might be transient network issue
			// The main StandbyService refresh loop will detect the failure
		}
	}

	/**
	 * Execute the refresh Lua script atomically.
	 */
	private async executeRefreshScript(): Promise<boolean> {
		const { lockKey, instanceId, lockTtlSeconds } = this.config;
		const result = await this.redis.eval(
			REFRESH_SCRIPT,
			1,
			lockKey,
			instanceId,
			String(lockTtlSeconds),
		);
		return result === 1;
	}
}
