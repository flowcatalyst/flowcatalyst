import type { Logger } from '@flowcatalyst/logging';
import type { TrafficConfig } from './types.js';
import type { TrafficManagementStrategy } from './strategy.js';
import { TrafficManager } from './traffic-manager.js';
import { NoOpTrafficStrategy } from './noop-strategy.js';
import { AwsAlbStrategy, type AwsAlbStrategyConfig } from './aws-alb-strategy.js';

export * from './types.js';
export * from './errors.js';
export * from './strategy.js';
export * from './traffic-manager.js';
export * from './noop-strategy.js';
export * from './aws-alb-strategy.js';

/**
 * Traffic manager factory configuration
 */
export interface TrafficManagerFactoryConfig {
	/** Enable traffic management */
	enabled: boolean;
	/** Strategy name: 'AWS_ALB_DEREGISTRATION' or undefined for no-op */
	strategyName?: string | undefined;
	/** AWS ALB configuration (required if strategyName is 'AWS_ALB_DEREGISTRATION') */
	awsAlb?: AwsAlbStrategyConfig | undefined;
}

/**
 * Create traffic manager from environment configuration
 *
 * Note: Traffic management is about standby mode support
 * (load balancer registration), NOT rate limiting.
 * Per-pool rate limiting is handled in ProcessPool.
 */
export function createTrafficManager(
	config: TrafficManagerFactoryConfig,
	logger: Logger,
): TrafficManager {
	const trafficConfig: TrafficConfig = {
		enabled: config.enabled,
		strategyName: config.strategyName,
	};

	// Select strategy based on configuration
	let strategy: TrafficManagementStrategy;

	if (config.strategyName === 'AWS_ALB_DEREGISTRATION' && config.awsAlb) {
		strategy = new AwsAlbStrategy(config.awsAlb, logger);
	} else {
		strategy = new NoOpTrafficStrategy(logger);
	}

	return new TrafficManager(trafficConfig, strategy, logger);
}
