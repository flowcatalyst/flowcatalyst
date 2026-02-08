/**
 * Pool configuration - internal representation
 * Used for pool management, matches Java ProcessingPool
 */
export interface PoolConfig {
	/** Unique pool code */
	code: string;
	/** Max concurrent messages being processed */
	concurrency: number;
	/** Rate limit in messages per minute (null = unlimited) */
	rateLimitPerMinute: number | null;
	/** Callback URL for HTTP mediation (optional for local/embedded pools) */
	callbackUrl?: string | undefined;
	/** Timeout for HTTP calls in milliseconds */
	timeoutMs?: number | undefined;
	/** Number of retries for failed HTTP calls */
	retries?: number | undefined;
}

/**
 * Pool state for internal tracking
 */
export const PoolState = {
	STARTING: 'STARTING',
	RUNNING: 'RUNNING',
	DRAINING: 'DRAINING',
	STOPPED: 'STOPPED',
} as const;

export type PoolState = (typeof PoolState)[keyof typeof PoolState];

/**
 * Internal pool statistics tracking
 */
export interface PoolStatsInternal {
	/** Pool code */
	poolCode: string;
	/** Total messages ever processed */
	totalProcessed: number;
	/** Total successful */
	totalSucceeded: number;
	/** Total failed */
	totalFailed: number;
	/** Total rate limited */
	totalRateLimited: number;
	/** Total deferred (ack=false) */
	totalDeferred: number;
	/** Processing time samples for average calculation */
	processingTimes: number[];
	/** Messages processed in time windows */
	windowedStats: {
		processed5min: number;
		succeeded5min: number;
		failed5min: number;
		rateLimited5min: number;
		processed30min: number;
		succeeded30min: number;
		failed30min: number;
		rateLimited30min: number;
	};
}
