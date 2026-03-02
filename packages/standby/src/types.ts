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
 * Callbacks invoked by StandbyService on mode transitions and Redis events.
 */
export interface StandbyCallbacks {
	/** Called when this instance wins the lock and becomes primary. */
	onBecomePrimary: () => Promise<void>;
	/** Called when this instance is in standby (at startup or after losing lock). */
	onBecomeStandby: () => Promise<void>;
	/**
	 * Called when Redis becomes unavailable.
	 * Return an opaque ID that will be passed to onWarningCleared when Redis recovers.
	 */
	onWarning?: (message: string) => string;
	/** Called when Redis recovers after being unavailable. Receives the ID from onWarning. */
	onWarningCleared?: (warningId: string) => void;
}

/**
 * Standby status for monitoring.
 */
export interface StandbyStatus {
	instanceId: string;
	isPrimary: boolean;
	redisAvailable: boolean;
	lastSuccessfulRefresh: string | null;
	currentLockHolder: string;
	hasWarning: boolean;
}
