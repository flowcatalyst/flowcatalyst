import { ok, err, type Result, type ResultAsync } from 'neverthrow';
import type { Logger } from '@flowcatalyst/logging';
import type { TrafficConfig, TrafficMode, TrafficStats } from './types.js';
import type { TrafficManagementStrategy } from './strategy.js';
import type { TrafficError } from './errors.js';

/**
 * Listener for mode change events
 */
export type ModeChangeListener = (newMode: TrafficMode, previousMode: TrafficMode) => void;

/**
 * Traffic manager for standby mode support
 *
 * Matches Java TrafficManagementService behavior:
 * - Manages PRIMARY/STANDBY mode transitions
 * - Delegates load balancer registration/deregistration to strategy
 * - Notifies listeners on mode changes (for consumer pause/resume)
 *
 * Note: Per-pool rate limiting and concurrency are handled in ProcessPool,
 * NOT in this traffic manager.
 */
export class TrafficManager {
	private readonly config: TrafficConfig;
	private readonly strategy: TrafficManagementStrategy;
	private readonly logger: Logger;
	private readonly modeChangeListeners: ModeChangeListener[] = [];
	private mode: TrafficMode = 'PRIMARY';
	private started = false;

	constructor(config: TrafficConfig, strategy: TrafficManagementStrategy, logger: Logger) {
		this.config = config;
		this.strategy = strategy;
		this.logger = logger.child({ component: 'TrafficManager' });

		this.logger.info(
			{
				enabled: config.enabled,
				strategyName: strategy.getName(),
			},
			'Traffic manager initialized',
		);
	}

	/**
	 * Start traffic management
	 */
	start(): ResultAsync<void, TrafficError> {
		if (this.started) {
			this.logger.warn('Traffic manager already started');
			return ok(undefined) as unknown as ResultAsync<void, TrafficError>;
		}

		this.started = true;

		if (!this.config.enabled) {
			this.logger.info('Traffic management disabled');
			return ok(undefined) as unknown as ResultAsync<void, TrafficError>;
		}

		this.logger.info({ mode: this.mode }, 'Starting traffic manager');

		// Register as active when starting in PRIMARY mode
		if (this.mode === 'PRIMARY') {
			return this.strategy.registerAsActive().map(() => {
				this.logger.info({ mode: this.mode }, 'Traffic manager started');
			});
		}

		return ok(undefined) as unknown as ResultAsync<void, TrafficError>;
	}

	/**
	 * Stop traffic management
	 */
	stop(): ResultAsync<void, TrafficError> {
		this.logger.info('Stopping traffic manager');

		if (this.strategy.isRegistered()) {
			return this.strategy.deregisterFromActive().map(() => {
				this.started = false;
				this.logger.info('Traffic manager stopped');
			});
		}

		this.started = false;
		return ok(undefined) as unknown as ResultAsync<void, TrafficError>;
	}

	/**
	 * Switch to PRIMARY mode
	 */
	becomePrimary(): ResultAsync<void, TrafficError> {
		if (this.mode === 'PRIMARY') {
			return ok(undefined) as unknown as ResultAsync<void, TrafficError>;
		}

		const previousMode = this.mode;
		this.logger.info('Transitioning to PRIMARY mode');
		this.mode = 'PRIMARY';

		// Notify listeners (e.g., consumers to resume)
		this.notifyModeChange(this.mode, previousMode);

		if (this.config.enabled) {
			return this.strategy.registerAsActive();
		}

		return ok(undefined) as unknown as ResultAsync<void, TrafficError>;
	}

	/**
	 * Switch to STANDBY mode
	 */
	becomeStandby(): ResultAsync<void, TrafficError> {
		if (this.mode === 'STANDBY') {
			return ok(undefined) as unknown as ResultAsync<void, TrafficError>;
		}

		const previousMode = this.mode;
		this.logger.info('Transitioning to STANDBY mode');
		this.mode = 'STANDBY';

		// Notify listeners (e.g., consumers to pause)
		this.notifyModeChange(this.mode, previousMode);

		if (this.strategy.isRegistered()) {
			return this.strategy.deregisterFromActive();
		}

		return ok(undefined) as unknown as ResultAsync<void, TrafficError>;
	}

	/**
	 * Add a listener for mode changes
	 * Used by consumers to pause/resume based on standby state
	 */
	addModeChangeListener(listener: ModeChangeListener): void {
		this.modeChangeListeners.push(listener);
	}

	/**
	 * Remove a mode change listener
	 */
	removeModeChangeListener(listener: ModeChangeListener): void {
		const index = this.modeChangeListeners.indexOf(listener);
		if (index >= 0) {
			this.modeChangeListeners.splice(index, 1);
		}
	}

	/**
	 * Check if this instance is in PRIMARY mode
	 */
	isPrimary(): boolean {
		return this.mode === 'PRIMARY';
	}

	/**
	 * Check if this instance is in STANDBY mode
	 */
	isStandby(): boolean {
		return this.mode === 'STANDBY';
	}

	/**
	 * Get current mode
	 */
	getMode(): TrafficMode {
		return this.mode;
	}

	/**
	 * Check if registered with load balancer
	 */
	isRegisteredWithLoadBalancer(): boolean {
		return this.strategy.isRegistered();
	}

	/**
	 * Check if traffic management is enabled
	 */
	isEnabled(): boolean {
		return this.config.enabled;
	}

	/**
	 * Get traffic statistics
	 */
	getStats(): TrafficStats {
		return {
			enabled: this.config.enabled,
			mode: this.mode,
			isRegistered: this.strategy.isRegistered(),
			strategyName: this.strategy.getName(),
		};
	}

	/**
	 * Notify all listeners of a mode change
	 */
	private notifyModeChange(newMode: TrafficMode, previousMode: TrafficMode): void {
		for (const listener of this.modeChangeListeners) {
			try {
				listener(newMode, previousMode);
			} catch (error) {
				this.logger.error(
					{ err: error, newMode, previousMode },
					'Error notifying mode change listener',
				);
			}
		}
	}
}
