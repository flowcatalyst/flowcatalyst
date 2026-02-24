/**
 * Queue statistics - GET /monitoring/queue-stats
 * Map values keyed by queue name
 */
export interface QueueStats {
	name: string;
	totalMessages: number;
	totalConsumed: number;
	totalFailed: number;
	successRate: number;
	currentSize: number;
	throughput: number;
	pendingMessages: number;
	messagesNotVisible: number;
	totalMessages5min: number;
	totalConsumed5min: number;
	totalFailed5min: number;
	successRate5min: number;
	totalMessages30min: number;
	totalConsumed30min: number;
	totalFailed30min: number;
	successRate30min: number;
	totalDeferred: number;
}

/**
 * Pool statistics - GET /monitoring/pool-stats
 * Map values keyed by pool code
 */
export interface PoolStats {
	poolCode: string;
	totalProcessed: number;
	totalSucceeded: number;
	totalFailed: number;
	totalTransient: number;
	totalRateLimited: number;
	successRate: number;
	activeWorkers: number;
	availablePermits: number;
	maxConcurrency: number;
	queueSize: number;
	maxQueueCapacity: number;
	averageProcessingTimeMs: number;
	totalProcessed5min: number;
	totalSucceeded5min: number;
	totalFailed5min: number;
	totalTransient5min: number;
	successRate5min: number;
	totalProcessed30min: number;
	totalSucceeded30min: number;
	totalFailed30min: number;
	totalTransient30min: number;
	successRate30min: number;
	totalRateLimited5min: number;
	totalRateLimited30min: number;
}

/**
 * Warning categories - exact match to Java enum
 */
export const WarningCategory = {
	QUEUE_BACKLOG: "QUEUE_BACKLOG",
	QUEUE_GROWING: "QUEUE_GROWING",
	QUEUE_FULL: "QUEUE_FULL",
	QUEUE_VALIDATION: "QUEUE_VALIDATION",
	MEDIATION: "MEDIATION",
	CONFIGURATION: "CONFIGURATION",
	CONFIG_SYNC_FAILED: "CONFIG_SYNC_FAILED",
	POOL_LIMIT: "POOL_LIMIT",
	PIPELINE_MAP_LEAK: "PIPELINE_MAP_LEAK",
	BROKER_HEALTH: "BROKER_HEALTH",
	CONSUMER_RESTART: "CONSUMER_RESTART",
	CONSUMER_RESTART_FAILED: "CONSUMER_RESTART_FAILED",
	ROUTING: "ROUTING",
	SHUTDOWN_CLEANUP_ERRORS: "SHUTDOWN_CLEANUP_ERRORS",
	STANDBY_REDIS: "STANDBY_REDIS",
} as const;

export type WarningCategory =
	(typeof WarningCategory)[keyof typeof WarningCategory];

/**
 * Warning severity - exact match to Java enum
 */
export const WarningSeverity = {
	CRITICAL: "CRITICAL",
	ERROR: "ERROR",
	WARNING: "WARNING",
	INFO: "INFO",
} as const;

export type WarningSeverity =
	(typeof WarningSeverity)[keyof typeof WarningSeverity];

/**
 * Warning - GET /monitoring/warnings
 */
export interface Warning {
	id: string;
	category: string;
	severity: string;
	message: string;
	timestamp: string;
	source: string;
	acknowledged: boolean;
}

/**
 * Warning acknowledge response
 */
export interface WarningAcknowledgeResponse {
	status: string;
	message?: string | undefined;
}

/**
 * Circuit breaker state - exact match to Java
 */
export const CircuitBreakerState = {
	CLOSED: "CLOSED",
	OPEN: "OPEN",
	HALF_OPEN: "HALF_OPEN",
} as const;

export type CircuitBreakerState =
	(typeof CircuitBreakerState)[keyof typeof CircuitBreakerState];

/**
 * Circuit breaker stats - GET /monitoring/circuit-breakers
 * Map values keyed by circuit breaker name (URL)
 */
export interface CircuitBreakerStats {
	name: string;
	state: string;
	successfulCalls: number;
	failedCalls: number;
	rejectedCalls: number;
	failureRate: number;
	bufferedCalls: number;
	bufferSize: number;
}

/**
 * Circuit breaker state response
 */
export interface CircuitBreakerStateResponse {
	name: string;
	state: string;
}

/**
 * In-flight message - GET /monitoring/in-flight-messages
 */
export interface InFlightMessage {
	messageId: string;
	brokerMessageId: string;
	queueId: string;
	addedToInPipelineAt: string;
	elapsedTimeMs: number;
	poolCode: string;
}

/**
 * Standby status - GET /monitoring/standby-status (disabled)
 */
export interface StandbyStatusDisabled {
	standbyEnabled: false;
}

/**
 * Standby status - GET /monitoring/standby-status (enabled)
 */
export interface StandbyStatusEnabled {
	standbyEnabled: true;
	instanceId: string;
	role: string;
	redisAvailable: boolean;
	currentLockHolder: string;
	lastSuccessfulRefresh: string | null;
	hasWarning: boolean;
}

export type StandbyStatusResponse =
	| StandbyStatusDisabled
	| StandbyStatusEnabled;

/**
 * Traffic status - GET /monitoring/traffic-status (disabled)
 */
export interface TrafficStatusDisabled {
	enabled: false;
	message: string;
}

/**
 * Traffic status - GET /monitoring/traffic-status (enabled)
 */
export interface TrafficStatusEnabled {
	enabled: true;
	strategyType: string;
	registered: boolean;
	targetInfo: string;
	lastOperation: string;
	lastError: string;
}

export type TrafficStatusResponse =
	| TrafficStatusDisabled
	| TrafficStatusEnabled;

/**
 * Consumer health info - part of /monitoring/consumer-health
 */
export interface ConsumerHealthInfo {
	mapKey: string;
	queueIdentifier: string;
	consumerQueueIdentifier: string;
	instanceId: string;
	isHealthy: boolean;
	lastPollTimeMs: number;
	lastPollTime: string;
	timeSinceLastPollMs: number;
	timeSinceLastPollSeconds: number;
	isRunning: boolean;
}

/**
 * Consumer health response - GET /monitoring/consumer-health
 */
export interface ConsumerHealthResponse {
	currentTimeMs: number;
	currentTime: string;
	consumers: Record<string, ConsumerHealthInfo>;
}

/**
 * Simple status response for various operations
 */
export interface StatusResponse {
	status: string;
	message?: string | undefined;
}
