import { okAsync, type ResultAsync } from 'neverthrow';
import type { Logger } from '@flowcatalyst/logging';
import type { TrafficManagementStrategy } from './strategy.js';
import type { TrafficError } from './errors.js';

/**
 * No-op traffic management strategy
 *
 * Used when traffic management is disabled or no specific
 * load balancer integration is configured.
 *
 * Matches Java NoOpTrafficStrategy behavior.
 */
export class NoOpTrafficStrategy implements TrafficManagementStrategy {
	private readonly logger: Logger;
	private registered = false;

	constructor(logger: Logger) {
		this.logger = logger.child({ component: 'NoOpTrafficStrategy' });
	}

	getName(): string {
		return 'NONE';
	}

	registerAsActive(): ResultAsync<void, TrafficError> {
		this.logger.debug('No-op register as active');
		this.registered = true;
		return okAsync(undefined);
	}

	deregisterFromActive(): ResultAsync<void, TrafficError> {
		this.logger.debug('No-op deregister from active');
		this.registered = false;
		return okAsync(undefined);
	}

	isRegistered(): boolean {
		return this.registered;
	}
}
