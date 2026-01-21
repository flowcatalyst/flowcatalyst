/**
 * Health check error types using discriminated unions for neverthrow
 */

/**
 * Broker connectivity errors
 */
export type BrokerHealthError =
	| { type: 'broker_unreachable'; broker: string; cause: Error }
	| { type: 'auth_failed'; broker: string; message: string }
	| { type: 'timeout'; broker: string; durationMs: number }
	| { type: 'unknown'; broker: string; cause: Error };

/**
 * Queue health check errors
 */
export type QueueHealthError =
	| { type: 'queue_not_found'; queueUrl: string }
	| { type: 'permission_denied'; queueUrl: string; message: string }
	| { type: 'metrics_unavailable'; queueName: string; cause: Error };

/**
 * Health check result for a single broker
 */
export interface BrokerHealthResult {
	broker: string;
	healthy: boolean;
	latencyMs: number;
	details?: string;
}

/**
 * Queue health status
 */
export interface QueueHealthStatus {
	queueName: string;
	pendingMessages: number;
	messagesNotVisible: number;
	isBacklogged: boolean;
	isGrowing: boolean;
	consecutiveGrowthPeriods: number;
}

/**
 * Overall health check result
 */
export interface HealthCheckResult {
	healthy: boolean;
	brokers: BrokerHealthResult[];
	queues: QueueHealthStatus[];
	issues: string[];
	checkedAt: Date;
}

/**
 * Helper to create broker errors
 */
export const BrokerHealthErrors = {
	unreachable: (broker: string, cause: Error): BrokerHealthError => ({
		type: 'broker_unreachable',
		broker,
		cause,
	}),
	authFailed: (broker: string, message: string): BrokerHealthError => ({
		type: 'auth_failed',
		broker,
		message,
	}),
	timeout: (broker: string, durationMs: number): BrokerHealthError => ({
		type: 'timeout',
		broker,
		durationMs,
	}),
	unknown: (broker: string, cause: Error): BrokerHealthError => ({
		type: 'unknown',
		broker,
		cause,
	}),
};

/**
 * Helper to create queue errors
 */
export const QueueHealthErrors = {
	notFound: (queueUrl: string): QueueHealthError => ({
		type: 'queue_not_found',
		queueUrl,
	}),
	permissionDenied: (queueUrl: string, message: string): QueueHealthError => ({
		type: 'permission_denied',
		queueUrl,
		message,
	}),
	metricsUnavailable: (queueName: string, cause: Error): QueueHealthError => ({
		type: 'metrics_unavailable',
		queueName,
		cause,
	}),
};
