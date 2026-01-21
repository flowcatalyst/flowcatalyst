import { z } from 'zod';

/**
 * Queue statistics - GET /monitoring/queue-stats
 * Map values keyed by queue name
 */
export const QueueStatsSchema = z.object({
	name: z.string(),
	totalMessages: z.number(),
	totalConsumed: z.number(),
	totalFailed: z.number(),
	successRate: z.number(),
	currentSize: z.number(),
	throughput: z.number(),
	pendingMessages: z.number(),
	messagesNotVisible: z.number(),
	totalMessages5min: z.number(),
	totalConsumed5min: z.number(),
	totalFailed5min: z.number(),
	successRate5min: z.number(),
	totalMessages30min: z.number(),
	totalConsumed30min: z.number(),
	totalFailed30min: z.number(),
	successRate30min: z.number(),
	totalDeferred: z.number(),
});

export type QueueStats = z.infer<typeof QueueStatsSchema>;

/**
 * Pool statistics - GET /monitoring/pool-stats
 * Map values keyed by pool code
 */
export const PoolStatsSchema = z.object({
	poolCode: z.string(),
	totalProcessed: z.number(),
	totalSucceeded: z.number(),
	totalFailed: z.number(),
	totalRateLimited: z.number(),
	successRate: z.number(),
	activeWorkers: z.number().int(),
	availablePermits: z.number().int(),
	maxConcurrency: z.number().int(),
	queueSize: z.number().int(),
	maxQueueCapacity: z.number().int(),
	averageProcessingTimeMs: z.number(),
	totalProcessed5min: z.number(),
	totalSucceeded5min: z.number(),
	totalFailed5min: z.number(),
	successRate5min: z.number(),
	totalProcessed30min: z.number(),
	totalSucceeded30min: z.number(),
	totalFailed30min: z.number(),
	successRate30min: z.number(),
	totalRateLimited5min: z.number(),
	totalRateLimited30min: z.number(),
});

export type PoolStats = z.infer<typeof PoolStatsSchema>;

/**
 * Warning categories - exact match to Java enum
 */
export const WarningCategory = {
	QUEUE_BACKLOG: 'QUEUE_BACKLOG',
	QUEUE_GROWING: 'QUEUE_GROWING',
	QUEUE_VALIDATION: 'QUEUE_VALIDATION',
	MEDIATION: 'MEDIATION',
	CONFIGURATION: 'CONFIGURATION',
	POOL_LIMIT: 'POOL_LIMIT',
	BROKER_HEALTH: 'BROKER_HEALTH',
} as const;

export type WarningCategory = (typeof WarningCategory)[keyof typeof WarningCategory];

/**
 * Warning severity - exact match to Java enum
 */
export const WarningSeverity = {
	CRITICAL: 'CRITICAL',
	ERROR: 'ERROR',
	WARNING: 'WARNING',
	INFO: 'INFO',
} as const;

export type WarningSeverity = (typeof WarningSeverity)[keyof typeof WarningSeverity];

/**
 * Warning - GET /monitoring/warnings
 */
export const WarningSchema = z.object({
	id: z.string(),
	category: z.string(),
	severity: z.string(),
	message: z.string(),
	timestamp: z.string(),
	source: z.string(),
	acknowledged: z.boolean(),
});

export type Warning = z.infer<typeof WarningSchema>;

/**
 * Warning acknowledge response
 */
export const WarningAcknowledgeResponseSchema = z.object({
	status: z.string(),
	message: z.string().optional(),
});

export type WarningAcknowledgeResponse = z.infer<typeof WarningAcknowledgeResponseSchema>;

/**
 * Circuit breaker state - exact match to Java
 */
export const CircuitBreakerState = {
	CLOSED: 'CLOSED',
	OPEN: 'OPEN',
	HALF_OPEN: 'HALF_OPEN',
} as const;

export type CircuitBreakerState = (typeof CircuitBreakerState)[keyof typeof CircuitBreakerState];

/**
 * Circuit breaker stats - GET /monitoring/circuit-breakers
 * Map values keyed by circuit breaker name (URL)
 */
export const CircuitBreakerStatsSchema = z.object({
	name: z.string(),
	state: z.string(),
	successfulCalls: z.number(),
	failedCalls: z.number(),
	rejectedCalls: z.number(),
	failureRate: z.number(),
	bufferedCalls: z.number().int(),
	bufferSize: z.number().int(),
});

export type CircuitBreakerStats = z.infer<typeof CircuitBreakerStatsSchema>;

/**
 * Circuit breaker state response
 */
export const CircuitBreakerStateResponseSchema = z.object({
	name: z.string(),
	state: z.string(),
});

export type CircuitBreakerStateResponse = z.infer<typeof CircuitBreakerStateResponseSchema>;

/**
 * In-flight message - GET /monitoring/in-flight-messages
 */
export const InFlightMessageSchema = z.object({
	messageId: z.string(),
	brokerMessageId: z.string(),
	queueId: z.string(),
	addedToInPipelineAt: z.string(),
	elapsedTimeMs: z.number(),
	poolCode: z.string(),
});

export type InFlightMessage = z.infer<typeof InFlightMessageSchema>;

/**
 * Standby status - GET /monitoring/standby-status (disabled)
 */
export const StandbyStatusDisabledSchema = z.object({
	standbyEnabled: z.literal(false),
});

/**
 * Standby status - GET /monitoring/standby-status (enabled)
 */
export const StandbyStatusEnabledSchema = z.object({
	standbyEnabled: z.literal(true),
	instanceId: z.string(),
	role: z.string(),
	redisAvailable: z.boolean(),
	currentLockHolder: z.string(),
	lastSuccessfulRefresh: z.string().nullable(),
	hasWarning: z.boolean(),
});

export const StandbyStatusResponseSchema = z.discriminatedUnion('standbyEnabled', [
	StandbyStatusDisabledSchema,
	StandbyStatusEnabledSchema,
]);

export type StandbyStatusResponse = z.infer<typeof StandbyStatusResponseSchema>;

/**
 * Traffic status - GET /monitoring/traffic-status (disabled)
 */
export const TrafficStatusDisabledSchema = z.object({
	enabled: z.literal(false),
	message: z.string(),
});

/**
 * Traffic status - GET /monitoring/traffic-status (enabled)
 */
export const TrafficStatusEnabledSchema = z.object({
	enabled: z.literal(true),
	strategyType: z.string(),
	registered: z.boolean(),
	targetInfo: z.string(),
	lastOperation: z.string(),
	lastError: z.string(),
});

export const TrafficStatusResponseSchema = z.discriminatedUnion('enabled', [
	TrafficStatusDisabledSchema,
	TrafficStatusEnabledSchema,
]);

export type TrafficStatusResponse = z.infer<typeof TrafficStatusResponseSchema>;

/**
 * Consumer health info - part of /monitoring/consumer-health
 */
export const ConsumerHealthInfoSchema = z.object({
	mapKey: z.string(),
	queueIdentifier: z.string(),
	consumerQueueIdentifier: z.string(),
	instanceId: z.string(),
	isHealthy: z.boolean(),
	lastPollTimeMs: z.number(),
	lastPollTime: z.string(),
	timeSinceLastPollMs: z.number(),
	timeSinceLastPollSeconds: z.number(),
	isRunning: z.boolean(),
});

export type ConsumerHealthInfo = z.infer<typeof ConsumerHealthInfoSchema>;

/**
 * Consumer health response - GET /monitoring/consumer-health
 */
export const ConsumerHealthResponseSchema = z.object({
	currentTimeMs: z.number(),
	currentTime: z.string(),
	consumers: z.record(z.string(), ConsumerHealthInfoSchema),
});

export type ConsumerHealthResponse = z.infer<typeof ConsumerHealthResponseSchema>;

/**
 * Simple status response for various operations
 */
export const StatusResponseSchema = z.object({
	status: z.string(),
	message: z.string().optional(),
});

export type StatusResponse = z.infer<typeof StatusResponseSchema>;
