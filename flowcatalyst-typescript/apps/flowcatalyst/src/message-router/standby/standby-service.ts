import type { Logger } from "@flowcatalyst/logging";
import type { TrafficManager } from "../traffic/index.js";
import type { WarningService } from "../services/warning-service.js";
import type { LockManager } from "./lock-manager.js";
import type { StandbyConfig, StandbyStatus } from "./types.js";

/** Refresh interval in milliseconds (matches Java: hardcoded 10s) */
const REFRESH_INTERVAL_MS = 10_000;

/**
 * Manages hot standby mode with distributed primary/standby election using Redis.
 *
 * Direct port of Java StandbyService:
 * - Acquire lock on startup (becomes primary or waits as standby)
 * - Refresh lock periodically to maintain primary status
 * - Release lock gracefully on shutdown
 * - Detect Redis failures and mark system as unhealthy
 * - Fire warnings when Redis is unavailable
 */
export class StandbyService {
	private readonly config: StandbyConfig;
	private readonly lockManager: LockManager;
	private readonly traffic: TrafficManager;
	private readonly warnings: WarningService;
	private readonly logger: Logger;

	private isPrimaryState = false;
	private lastSuccessfulRefresh: Date | null = null;
	private redisAvailableState = true;
	private warningId: string | null = null;
	private refreshInterval: ReturnType<typeof setInterval> | null = null;

	constructor(
		config: StandbyConfig,
		lockManager: LockManager,
		traffic: TrafficManager,
		warnings: WarningService,
		logger: Logger,
	) {
		this.config = config;
		this.lockManager = lockManager;
		this.traffic = traffic;
		this.warnings = warnings;
		this.logger = logger.child({ component: "StandbyService" });
	}

	/**
	 * Start the standby service.
	 * Attempts lock acquisition and sets initial mode on TrafficManager.
	 */
	async start(): Promise<void> {
		this.logger.info(
			{
				instanceId: this.config.instanceId,
				lockKey: this.config.lockKey,
				lockTtlSeconds: this.config.lockTtlSeconds,
			},
			"Hot standby mode enabled",
		);

		// Attempt initial lock acquisition
		try {
			const acquired = await this.lockManager.acquireLock();

			if (acquired) {
				this.isPrimaryState = true;
				this.lastSuccessfulRefresh = new Date();
				this.redisAvailableState = true;
				this.logger.info("Acquired primary lock. Starting processing.");
				await this.traffic.becomePrimary();
			} else {
				this.isPrimaryState = false;
				this.logger.info(
					{ lockTtlSeconds: this.config.lockTtlSeconds },
					"Standby mode: Waiting for primary to fail",
				);
				await this.traffic.becomeStandby();
			}
		} catch (error) {
			// Redis unavailable at startup
			this.redisAvailableState = false;
			this.isPrimaryState = false;
			this.logger.error(
				{ err: error },
				"Redis unavailable at startup. System will not process until Redis is restored.",
			);
			await this.traffic.becomeStandby();
			this.fireRedisUnavailableWarning();
		}

		// Start periodic refresh task (matches Java @Scheduled(every = "10s"))
		this.refreshInterval = setInterval(() => {
			this.refreshLockTask().catch((err) => {
				this.logger.error({ err }, "Unhandled error in refresh task");
			});
		}, REFRESH_INTERVAL_MS);
	}

	/**
	 * Stop the standby service and release the lock.
	 * Allows standby instance to immediately take over without waiting for timeout.
	 */
	async stop(): Promise<void> {
		// Stop refresh interval
		if (this.refreshInterval) {
			clearInterval(this.refreshInterval);
			this.refreshInterval = null;
		}

		if (!this.isPrimaryState) {
			this.lockManager.shutdown();
			return;
		}

		try {
			await this.lockManager.releaseLock();
			this.logger.info(
				"Primary lock released. Standby instance can now take over immediately.",
			);
		} catch (error) {
			this.logger.warn(
				{ err: error },
				"Error releasing lock during shutdown (will expire automatically)",
			);
		}

		this.lockManager.shutdown();
	}

	/**
	 * Check if this instance is the primary.
	 */
	isPrimary(): boolean {
		return this.isPrimaryState;
	}

	/**
	 * Check if Redis is currently available.
	 */
	isRedisAvailable(): boolean {
		return this.redisAvailableState;
	}

	/**
	 * Check if standby mode is enabled.
	 */
	isEnabled(): boolean {
		return this.config.enabled;
	}

	/**
	 * Get current state information (for monitoring).
	 */
	async getStatus(): Promise<StandbyStatus> {
		const currentLockHolder =
			await this.lockManager.getCurrentLockHolder();

		return {
			instanceId: this.config.instanceId,
			isPrimary: this.isPrimaryState,
			redisAvailable: this.redisAvailableState,
			lastSuccessfulRefresh: this.lastSuccessfulRefresh?.toISOString() ?? null,
			currentLockHolder,
			hasWarning: this.warningId !== null,
		};
	}

	/**
	 * Periodic task to refresh the lock (primary) or attempt acquisition (standby).
	 * Matches Java StandbyService.refreshLockTask()
	 */
	private async refreshLockTask(): Promise<void> {
		try {
			if (this.isPrimaryState) {
				// Try to refresh our lock
				const refreshed = await this.lockManager.refreshLock();

				if (refreshed) {
					this.lastSuccessfulRefresh = new Date();
					this.redisAvailableState = true;
					this.clearWarningIfNeeded();
				} else {
					// Lost the lock unexpectedly
					this.logger.error(
						"CRITICAL: Primary lost lock unexpectedly. Shutting down.",
					);
					this.isPrimaryState = false;

					// Match Java: Quarkus.asyncExit(1) - exit with error code
					// Brief delay to allow log message to flush
					setTimeout(() => {
						process.exit(1);
					}, 1000);
				}
			} else {
				// Standby mode: try to acquire the lock
				const acquired = await this.lockManager.acquireLock();

				if (acquired) {
					this.isPrimaryState = true;
					this.lastSuccessfulRefresh = new Date();
					this.redisAvailableState = true;
					this.logger.info("Acquired primary lock. Taking over processing.");
					this.clearWarningIfNeeded();
					await this.traffic.becomePrimary();
				} else {
					this.logger.debug(
						"Still in standby. Waiting for primary lock to expire.",
					);
				}
			}
		} catch (error) {
			// Redis connection failed
			this.logger.error(
				{ err: error },
				"Redis connection failed during refresh",
			);
			this.redisAvailableState = false;

			if (this.warningId === null) {
				this.fireRedisUnavailableWarning();
			}
		}
	}

	/**
	 * Fire CRITICAL warning when Redis becomes unavailable.
	 */
	private fireRedisUnavailableWarning(): void {
		const warning = this.warnings.add(
			"STANDBY_REDIS",
			"CRITICAL",
			`Redis is unavailable and standby mode cannot function. Instance: ${this.config.instanceId}. Manual intervention required. Server health checks will report FAILED.`,
			"StandbyService",
		);
		this.warningId = warning.id;
		this.logger.error(
			{ warningId: this.warningId },
			"CRITICAL: Redis unavailable",
		);
	}

	/**
	 * Clear warning if Redis was previously unavailable but is now restored.
	 */
	private clearWarningIfNeeded(): void {
		if (this.warningId !== null) {
			this.warnings.acknowledge(this.warningId);
			this.warningId = null;
		}
	}
}

/**
 * No-op standby service for when standby mode is disabled.
 * Always reports as primary, no Redis connection needed.
 */
export class NoOpStandbyService {
	isPrimary(): boolean {
		return true;
	}

	isRedisAvailable(): boolean {
		return false;
	}

	isEnabled(): boolean {
		return false;
	}

	async getStatus(): Promise<StandbyStatus> {
		return {
			instanceId: "single-instance",
			isPrimary: true,
			redisAvailable: false,
			lastSuccessfulRefresh: null,
			currentLockHolder: "single-instance",
			hasWarning: false,
		};
	}

	async start(): Promise<void> {
		// No-op
	}

	async stop(): Promise<void> {
		// No-op
	}
}
