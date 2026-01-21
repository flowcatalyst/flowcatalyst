import type { Logger } from '@flowcatalyst/logging';
import type { CircuitBreakerStats } from '@flowcatalyst/shared-types';

/**
 * Circuit breaker state
 */
export type CircuitBreakerState = 'CLOSED' | 'OPEN' | 'HALF_OPEN';

/**
 * Circuit breaker configuration
 */
export interface CircuitBreakerConfig {
	/** Failure rate threshold (0-1) to open circuit */
	failureRateThreshold: number;
	/** Minimum calls before evaluating failure rate */
	minimumCalls: number;
	/** Time in ms to wait before transitioning to half-open */
	waitDurationMs: number;
	/** Number of permitted calls in half-open state */
	permittedCallsInHalfOpen: number;
	/** Sliding window size for tracking calls */
	slidingWindowSize: number;
}

/**
 * Default circuit breaker configuration
 */
export const defaultCircuitBreakerConfig: CircuitBreakerConfig = {
	failureRateThreshold: 0.5,
	minimumCalls: 10,
	waitDurationMs: 5000,
	permittedCallsInHalfOpen: 3,
	slidingWindowSize: 100,
};

/**
 * Circuit breaker implementation
 * Matches Java Resilience4j circuit breaker behavior
 */
export class CircuitBreaker {
	private readonly name: string;
	private readonly config: CircuitBreakerConfig;
	private readonly logger: Logger;

	private state: CircuitBreakerState = 'CLOSED';
	private successCount = 0;
	private failureCount = 0;
	private rejectedCount = 0;
	private halfOpenCallCount = 0;
	private lastStateChangeTime = Date.now();

	// Sliding window for recent calls
	private readonly callResults: boolean[] = [];

	constructor(name: string, config: CircuitBreakerConfig, logger: Logger) {
		this.name = name;
		this.config = config;
		this.logger = logger.child({ component: 'CircuitBreaker', name });
	}

	/**
	 * Execute a function with circuit breaker protection
	 */
	async execute<T>(fn: () => Promise<T>): Promise<T> {
		if (!this.canExecute()) {
			this.rejectedCount++;
			throw new Error(`Circuit breaker is open for ${this.name}`);
		}

		try {
			const result = await fn();
			this.recordSuccess();
			return result;
		} catch (error) {
			this.recordFailure();
			throw error;
		}
	}

	/**
	 * Check if execution is allowed
	 */
	private canExecute(): boolean {
		switch (this.state) {
			case 'CLOSED':
				return true;

			case 'OPEN': {
				const elapsed = Date.now() - this.lastStateChangeTime;
				if (elapsed >= this.config.waitDurationMs) {
					this.transitionTo('HALF_OPEN');
					return true;
				}
				return false;
			}

			case 'HALF_OPEN':
				return this.halfOpenCallCount < this.config.permittedCallsInHalfOpen;
		}
	}

	/**
	 * Record successful call
	 */
	private recordSuccess(): void {
		this.successCount++;
		this.addToWindow(true);

		if (this.state === 'HALF_OPEN') {
			this.halfOpenCallCount++;
			if (this.halfOpenCallCount >= this.config.permittedCallsInHalfOpen) {
				this.transitionTo('CLOSED');
			}
		}
	}

	/**
	 * Record failed call
	 */
	private recordFailure(): void {
		this.failureCount++;
		this.addToWindow(false);

		if (this.state === 'HALF_OPEN') {
			this.transitionTo('OPEN');
			return;
		}

		if (this.state === 'CLOSED') {
			this.checkThreshold();
		}
	}

	/**
	 * Add result to sliding window
	 */
	private addToWindow(success: boolean): void {
		this.callResults.push(success);
		if (this.callResults.length > this.config.slidingWindowSize) {
			this.callResults.shift();
		}
	}

	/**
	 * Check if failure threshold is exceeded
	 */
	private checkThreshold(): void {
		if (this.callResults.length < this.config.minimumCalls) {
			return;
		}

		const failures = this.callResults.filter((r) => !r).length;
		const failureRate = failures / this.callResults.length;

		if (failureRate >= this.config.failureRateThreshold) {
			this.transitionTo('OPEN');
		}
	}

	/**
	 * Transition to a new state
	 */
	private transitionTo(newState: CircuitBreakerState): void {
		const oldState = this.state;
		this.state = newState;
		this.lastStateChangeTime = Date.now();

		if (newState === 'HALF_OPEN') {
			this.halfOpenCallCount = 0;
		}

		if (newState === 'CLOSED') {
			this.callResults.length = 0;
		}

		this.logger.info({ oldState, newState, name: this.name }, 'Circuit breaker state change');
	}

	/**
	 * Get current state
	 */
	getState(): CircuitBreakerState {
		// Check if we should transition from OPEN to HALF_OPEN
		if (this.state === 'OPEN') {
			const elapsed = Date.now() - this.lastStateChangeTime;
			if (elapsed >= this.config.waitDurationMs) {
				this.transitionTo('HALF_OPEN');
			}
		}
		return this.state;
	}

	/**
	 * Get statistics - matches Java CircuitBreakerStats
	 */
	getStats(): CircuitBreakerStats {
		const failures = this.callResults.filter((r) => !r).length;
		const failureRate =
			this.callResults.length > 0 ? failures / this.callResults.length : 0;

		return {
			name: this.name,
			state: this.getState(),
			successfulCalls: this.successCount,
			failedCalls: this.failureCount,
			rejectedCalls: this.rejectedCount,
			failureRate,
			bufferedCalls: this.callResults.length,
			bufferSize: this.config.slidingWindowSize,
		};
	}

	/**
	 * Reset the circuit breaker
	 */
	reset(): void {
		this.transitionTo('CLOSED');
		this.successCount = 0;
		this.failureCount = 0;
		this.rejectedCount = 0;
		this.halfOpenCallCount = 0;
		this.callResults.length = 0;
		this.logger.info({ name: this.name }, 'Circuit breaker reset');
	}

	/**
	 * Get circuit breaker name
	 */
	getName(): string {
		return this.name;
	}
}

/**
 * Manager for multiple circuit breakers
 */
export class CircuitBreakerManager {
	private readonly breakers = new Map<string, CircuitBreaker>();
	private readonly config: CircuitBreakerConfig;
	private readonly logger: Logger;

	constructor(config: CircuitBreakerConfig, logger: Logger) {
		this.config = config;
		this.logger = logger;
	}

	/**
	 * Get or create a circuit breaker for the given name
	 */
	getOrCreate(name: string): CircuitBreaker {
		let breaker = this.breakers.get(name);
		if (!breaker) {
			breaker = new CircuitBreaker(name, this.config, this.logger);
			this.breakers.set(name, breaker);
		}
		return breaker;
	}

	/**
	 * Get all circuit breakers
	 */
	getAll(): Map<string, CircuitBreaker> {
		return new Map(this.breakers);
	}

	/**
	 * Get all circuit breaker stats - matches Java response format
	 */
	getAllStats(): Record<string, CircuitBreakerStats> {
		const stats: Record<string, CircuitBreakerStats> = {};
		for (const [name, breaker] of this.breakers) {
			stats[name] = breaker.getStats();
		}
		return stats;
	}

	/**
	 * Reset a specific circuit breaker
	 */
	reset(name: string): boolean {
		const breaker = this.breakers.get(name);
		if (breaker) {
			breaker.reset();
			return true;
		}
		return false;
	}

	/**
	 * Reset all circuit breakers
	 */
	resetAll(): void {
		for (const breaker of this.breakers.values()) {
			breaker.reset();
		}
	}
}
