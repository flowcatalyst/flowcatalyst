import type { Logger } from "@flowcatalyst/logging";
import type { CircuitBreakerStats } from "@flowcatalyst/contracts";
import {
	circuitBreaker,
	CountBreaker,
	CircuitState,
	handleAll,
	type CircuitBreakerPolicy,
	isBrokenCircuitError,
} from "cockatiel";
import type { MessageRouterMetrics } from "../metrics.js";

/**
 * Circuit breaker state
 */
export type CircuitBreakerState = "CLOSED" | "OPEN" | "HALF_OPEN";

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
 * Map cockatiel CircuitState enum to our string type
 */
function mapState(state: CircuitState): CircuitBreakerState {
	switch (state) {
		case CircuitState.Closed:
			return "CLOSED";
		case CircuitState.Open:
		case CircuitState.Isolated:
			return "OPEN";
		case CircuitState.HalfOpen:
			return "HALF_OPEN";
	}
}

/**
 * Map our string state to Prometheus gauge value
 */
function stateToMetricValue(state: CircuitState): number {
	switch (state) {
		case CircuitState.Closed:
			return 0;
		case CircuitState.Open:
		case CircuitState.Isolated:
			return 1;
		case CircuitState.HalfOpen:
			return 2;
	}
}

/**
 * Circuit breaker implementation backed by cockatiel
 */
export class CircuitBreaker {
	private readonly name: string;
	private readonly config: CircuitBreakerConfig;
	private readonly logger: Logger;
	private readonly metrics: MessageRouterMetrics | undefined;

	private policy: CircuitBreakerPolicy;
	private successCount = 0;
	private failureCount = 0;
	private rejectedCount = 0;

	// Lightweight sliding window for stats reporting
	private readonly callResults: boolean[] = [];

	constructor(
		name: string,
		config: CircuitBreakerConfig,
		logger: Logger,
		metrics?: MessageRouterMetrics,
	) {
		this.name = name;
		this.config = config;
		this.logger = logger.child({ component: "CircuitBreaker", name });
		this.metrics = metrics;
		this.policy = this.createPolicy();
	}

	/**
	 * Create a cockatiel circuit breaker policy with event wiring
	 */
	private createPolicy(): CircuitBreakerPolicy {
		const policy = circuitBreaker(handleAll, {
			halfOpenAfter: this.config.waitDurationMs,
			breaker: new CountBreaker({
				threshold: this.config.failureRateThreshold,
				size: this.config.slidingWindowSize,
				minimumNumberOfCalls: this.config.minimumCalls,
			}),
		});

		policy.onStateChange((state) => {
			const mapped = mapState(state);
			this.logger.info(
				{ newState: mapped, name: this.name },
				"Circuit breaker state change",
			);

			if (mapped === "CLOSED") {
				this.callResults.length = 0;
			}

			if (this.metrics) {
				this.metrics.circuitBreakerState.set(
					{ name: this.name },
					stateToMetricValue(state),
				);
			}
		});

		policy.onSuccess(() => {
			this.successCount++;
			this.addToWindow(true);
			if (this.metrics) {
				this.metrics.circuitBreakerCalls.inc({
					name: this.name,
					result: "success",
				});
			}
		});

		policy.onFailure(() => {
			this.failureCount++;
			this.addToWindow(false);
			if (this.metrics) {
				this.metrics.circuitBreakerCalls.inc({
					name: this.name,
					result: "failure",
				});
			}
		});

		return policy;
	}

	/**
	 * Add result to sliding window (for stats reporting)
	 */
	private addToWindow(success: boolean): void {
		this.callResults.push(success);
		if (this.callResults.length > this.config.slidingWindowSize) {
			this.callResults.shift();
		}
	}

	/**
	 * Execute a function with circuit breaker protection
	 */
	async execute<T>(fn: () => Promise<T>): Promise<T> {
		try {
			return await this.policy.execute(fn);
		} catch (error) {
			if (isBrokenCircuitError(error)) {
				this.rejectedCount++;
				if (this.metrics) {
					this.metrics.circuitBreakerCalls.inc({
						name: this.name,
						result: "rejected",
					});
				}
				throw new Error(`Circuit breaker is open for ${this.name}`);
			}
			throw error;
		}
	}

	/**
	 * Get current state
	 */
	getState(): CircuitBreakerState {
		return mapState(this.policy.state);
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
		this.policy = this.createPolicy();
		this.successCount = 0;
		this.failureCount = 0;
		this.rejectedCount = 0;
		this.callResults.length = 0;
		this.logger.info({ name: this.name }, "Circuit breaker reset");
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
	private readonly metrics: MessageRouterMetrics | undefined;

	constructor(
		config: CircuitBreakerConfig,
		logger: Logger,
		metrics?: MessageRouterMetrics,
	) {
		this.config = config;
		this.logger = logger;
		this.metrics = metrics;
	}

	/**
	 * Get or create a circuit breaker for the given name
	 */
	getOrCreate(name: string): CircuitBreaker {
		let breaker = this.breakers.get(name);
		if (!breaker) {
			breaker = new CircuitBreaker(
				name,
				this.config,
				this.logger,
				this.metrics,
			);
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
