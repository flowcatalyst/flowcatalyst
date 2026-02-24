/**
 * Hot standby configuration for distributed primary/standby deployment.
 * When enabled, uses Redis for leader election with automatic failover.
 */
export interface StandbyConfig {
	/** Enable hot standby mode with Redis-based leader election */
	enabled: boolean;

	/** Unique instance identifier for this server */
	instanceId: string;

	/** Redis key name for the distributed lock */
	lockKey: string;

	/** Lock TTL in seconds */
	lockTtlSeconds: number;

	/** Redis URL (e.g., "redis://localhost:6379") */
	redisUrl?: string | undefined;
}

/**
 * Standby status for monitoring
 */
export interface StandbyStatus {
	instanceId: string;
	isPrimary: boolean;
	redisAvailable: boolean;
	lastSuccessfulRefresh: string | null;
	currentLockHolder: string;
	hasWarning: boolean;
}

/**
 * Standby error discriminated union
 */
export type StandbyError =
	| { type: "redis_unavailable"; cause: Error }
	| { type: "lock_lost"; instanceId: string }
	| { type: "lock_acquisition_failed"; cause: Error };
