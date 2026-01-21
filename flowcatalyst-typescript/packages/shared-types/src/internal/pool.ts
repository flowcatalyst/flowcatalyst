import { z } from 'zod';

/**
 * Pool configuration - internal representation
 * Used for pool management, matches Java ProcessingPool
 */
export const PoolConfigSchema = z.object({
	/** Unique pool code */
	code: z.string(),
	/** Max concurrent messages being processed */
	concurrency: z.number().int().min(1).max(1000).default(10),
	/** Rate limit in messages per minute (null = unlimited) */
	rateLimitPerMinute: z.number().int().min(0).nullable().default(null),
	/** Callback URL for HTTP mediation (optional for local/embedded pools) */
	callbackUrl: z.string().url().optional(),
	/** Timeout for HTTP calls in milliseconds */
	timeoutMs: z.number().int().min(1000).max(900000).default(60000).optional(),
	/** Number of retries for failed HTTP calls */
	retries: z.number().int().min(0).max(10).default(3).optional(),
});

export type PoolConfig = z.infer<typeof PoolConfigSchema>;

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
