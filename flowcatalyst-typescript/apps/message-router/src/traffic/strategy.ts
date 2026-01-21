import { ResultAsync } from 'neverthrow';
import type { TrafficError } from './errors.js';

/**
 * Strategy interface for traffic management
 *
 * Implementations handle load balancer registration/deregistration
 * for standby mode support (PRIMARY/STANDBY transitions).
 *
 * Matches Java TrafficManagementStrategy interface.
 */
export interface TrafficManagementStrategy {
	/**
	 * Get the strategy name for identification
	 */
	getName(): string;

	/**
	 * Register this instance as active with the load balancer.
	 * Called when transitioning to PRIMARY mode.
	 */
	registerAsActive(): ResultAsync<void, TrafficError>;

	/**
	 * Deregister this instance from the load balancer.
	 * Called when transitioning to STANDBY mode.
	 */
	deregisterFromActive(): ResultAsync<void, TrafficError>;

	/**
	 * Check if this instance is currently registered with the load balancer.
	 */
	isRegistered(): boolean;
}
