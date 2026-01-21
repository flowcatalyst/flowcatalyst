/**
 * Traffic management configuration
 *
 * Note: Traffic management in FlowCatalyst is about standby mode support
 * (load balancer registration for PRIMARY/STANDBY), NOT rate limiting.
 *
 * Per-pool rate limiting and concurrency are handled in ProcessPool.
 */
export interface TrafficConfig {
	/** Global enable/disable for traffic management */
	enabled: boolean;

	/** Strategy name (e.g., 'AWS_ALB_DEREGISTRATION') */
	strategyName?: string | undefined;
}

/**
 * Traffic management mode
 */
export type TrafficMode = 'PRIMARY' | 'STANDBY';

/**
 * Traffic management statistics
 */
export interface TrafficStats {
	/** Whether traffic management is enabled */
	enabled: boolean;
	/** Current mode */
	mode: TrafficMode;
	/** Whether this instance is registered with load balancer */
	isRegistered: boolean;
	/** Strategy name */
	strategyName: string;
}
