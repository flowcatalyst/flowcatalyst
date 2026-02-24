import type { Logger } from "@flowcatalyst/logging";
import type { TrafficManager } from "../traffic/index.js";
import type { WarningService } from "../services/warning-service.js";
import type { StandbyConfig } from "./types.js";
import { LockManager } from "./lock-manager.js";
import { StandbyService, NoOpStandbyService } from "./standby-service.js";

export type { StandbyConfig, StandbyStatus, StandbyError } from "./types.js";
export { LockManager } from "./lock-manager.js";
export { StandbyService, NoOpStandbyService } from "./standby-service.js";

/** Union type for both real and no-op standby service */
export type StandbyServiceInstance = StandbyService | NoOpStandbyService;

/**
 * Create standby service from configuration.
 *
 * If enabled: creates ioredis client, LockManager, and StandbyService.
 * If disabled: returns a NoOpStandbyService (always primary, no Redis).
 */
export async function createStandbyService(
	config: StandbyConfig,
	traffic: TrafficManager,
	warnings: WarningService,
	logger: Logger,
): Promise<StandbyServiceInstance> {
	if (!config.enabled) {
		logger.info("Hot standby mode disabled - running as single instance");
		return new NoOpStandbyService();
	}

	if (!config.redisUrl) {
		logger.warn(
			"Standby enabled but no REDIS_URL configured - falling back to single instance mode",
		);
		return new NoOpStandbyService();
	}

	// Dynamic import ioredis to avoid loading it when not needed
	const { Redis } = await import("ioredis");

	const redis = new Redis(config.redisUrl!, {
		maxRetriesPerRequest: 3,
		retryStrategy(times: number) {
			// Exponential backoff: 200ms, 400ms, 800ms, then cap at 5s
			return Math.min(times * 200, 5000);
		},
		lazyConnect: true,
	});

	// Attempt to connect
	try {
		await redis.connect();
		logger.info({ redisUrl: config.redisUrl }, "Redis connected for standby mode");
	} catch (error) {
		logger.error(
			{ err: error, redisUrl: config.redisUrl },
			"Failed to connect to Redis for standby mode",
		);
		// Return real StandbyService anyway - it will handle Redis unavailability
		// by firing warnings and staying in standby
	}

	const lockManager = new LockManager(redis, config, logger);

	return new StandbyService(config, lockManager, traffic, warnings, logger);
}
