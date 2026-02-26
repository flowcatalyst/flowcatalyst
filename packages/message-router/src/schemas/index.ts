/**
 * TypeBox route schemas for message-router endpoints.
 *
 * These mirror the shapes from shared-types and the old zod schemas,
 * providing Fastify route validation and OpenAPI documentation.
 */

import { Type } from "@sinclair/typebox";

// ── Health ──

export const HealthCheckResponseSchema = Type.Object({
	status: Type.String(),
	timestamp: Type.String(),
	issues: Type.Array(Type.String()),
});

export const MonitoringHealthResponseSchema = Type.Object({
	status: Type.String(),
	timestamp: Type.String(),
	uptimeMillis: Type.Number(),
	details: Type.Object({
		totalQueues: Type.Number(),
		healthyQueues: Type.Number(),
		totalPools: Type.Number(),
		healthyPools: Type.Number(),
		activeWarnings: Type.Number(),
		criticalWarnings: Type.Number(),
		circuitBreakersOpen: Type.Number(),
		degradationReason: Type.Union([Type.String(), Type.Null()]),
	}),
});

// ── Config ──

export const QueueConfigDtoSchema = Type.Object({
	queueUri: Type.String(),
	queueName: Type.Union([Type.String(), Type.Null()]),
	connections: Type.Union([Type.Number(), Type.Null()]),
});

export const ProcessingPoolDtoSchema = Type.Object({
	code: Type.String(),
	concurrency: Type.Number(),
	rateLimitPerMinute: Type.Union([Type.Number(), Type.Null()]),
});

export const LocalConfigResponseSchema = Type.Object({
	queues: Type.Array(QueueConfigDtoSchema),
	connections: Type.Number(),
	processingPools: Type.Array(ProcessingPoolDtoSchema),
});

// ── Monitoring ──

export const QueueStatsSchema = Type.Object({
	name: Type.String(),
	totalMessages: Type.Number(),
	totalConsumed: Type.Number(),
	totalFailed: Type.Number(),
	successRate: Type.Number(),
	currentSize: Type.Number(),
	throughput: Type.Number(),
	pendingMessages: Type.Number(),
	messagesNotVisible: Type.Number(),
	totalMessages5min: Type.Number(),
	totalConsumed5min: Type.Number(),
	totalFailed5min: Type.Number(),
	successRate5min: Type.Number(),
	totalMessages30min: Type.Number(),
	totalConsumed30min: Type.Number(),
	totalFailed30min: Type.Number(),
	successRate30min: Type.Number(),
	totalDeferred: Type.Number(),
});

export const PoolStatsSchema = Type.Object({
	poolCode: Type.String(),
	totalProcessed: Type.Number(),
	totalSucceeded: Type.Number(),
	totalFailed: Type.Number(),
	totalRateLimited: Type.Number(),
	successRate: Type.Number(),
	activeWorkers: Type.Number(),
	availablePermits: Type.Number(),
	maxConcurrency: Type.Number(),
	queueSize: Type.Number(),
	maxQueueCapacity: Type.Number(),
	averageProcessingTimeMs: Type.Number(),
	totalProcessed5min: Type.Number(),
	totalSucceeded5min: Type.Number(),
	totalFailed5min: Type.Number(),
	successRate5min: Type.Number(),
	totalProcessed30min: Type.Number(),
	totalSucceeded30min: Type.Number(),
	totalFailed30min: Type.Number(),
	successRate30min: Type.Number(),
	totalRateLimited5min: Type.Number(),
	totalRateLimited30min: Type.Number(),
});

export const WarningSchema = Type.Object({
	id: Type.String(),
	category: Type.String(),
	severity: Type.String(),
	message: Type.String(),
	timestamp: Type.String(),
	source: Type.String(),
	acknowledged: Type.Boolean(),
});

export const WarningAcknowledgeResponseSchema = Type.Object({
	status: Type.String(),
	message: Type.Optional(Type.String()),
});

export const CircuitBreakerStatsSchema = Type.Object({
	name: Type.String(),
	state: Type.String(),
	successfulCalls: Type.Number(),
	failedCalls: Type.Number(),
	rejectedCalls: Type.Number(),
	failureRate: Type.Number(),
	bufferedCalls: Type.Number(),
	bufferSize: Type.Number(),
});

export const CircuitBreakerStateResponseSchema = Type.Object({
	name: Type.String(),
	state: Type.String(),
});

export const InFlightMessageSchema = Type.Object({
	messageId: Type.String(),
	brokerMessageId: Type.String(),
	queueId: Type.String(),
	addedToInPipelineAt: Type.String(),
	elapsedTimeMs: Type.Number(),
	poolCode: Type.String(),
});

export const StandbyStatusDisabledSchema = Type.Object({
	standbyEnabled: Type.Literal(false),
});

export const StandbyStatusEnabledSchema = Type.Object({
	standbyEnabled: Type.Literal(true),
	instanceId: Type.String(),
	role: Type.String(),
	redisAvailable: Type.Boolean(),
	currentLockHolder: Type.String(),
	lastSuccessfulRefresh: Type.Union([Type.String(), Type.Null()]),
	hasWarning: Type.Boolean(),
});

export const StandbyStatusResponseSchema = Type.Union([
	StandbyStatusDisabledSchema,
	StandbyStatusEnabledSchema,
]);

export const TrafficStatusDisabledSchema = Type.Object({
	enabled: Type.Literal(false),
	message: Type.String(),
});

export const TrafficStatusEnabledSchema = Type.Object({
	enabled: Type.Literal(true),
	strategyType: Type.String(),
	registered: Type.Boolean(),
	targetInfo: Type.String(),
	lastOperation: Type.String(),
	lastError: Type.String(),
});

export const TrafficStatusResponseSchema = Type.Union([
	TrafficStatusDisabledSchema,
	TrafficStatusEnabledSchema,
]);

export const ConsumerHealthInfoSchema = Type.Object({
	mapKey: Type.String(),
	queueIdentifier: Type.String(),
	consumerQueueIdentifier: Type.String(),
	instanceId: Type.String(),
	isHealthy: Type.Boolean(),
	lastPollTimeMs: Type.Number(),
	lastPollTime: Type.String(),
	timeSinceLastPollMs: Type.Number(),
	timeSinceLastPollSeconds: Type.Number(),
	isRunning: Type.Boolean(),
});

export const ConsumerHealthResponseSchema = Type.Object({
	currentTimeMs: Type.Number(),
	currentTime: Type.String(),
	consumers: Type.Record(Type.String(), ConsumerHealthInfoSchema),
});

export const StatusResponseSchema = Type.Object({
	status: Type.String(),
	message: Type.Optional(Type.String()),
});

// ── Test ──

export const TestEndpointResponseSchema = Type.Object({
	status: Type.String(),
	endpoint: Type.String(),
	requestId: Type.Number(),
	messageId: Type.Optional(Type.String()),
	error: Type.Optional(Type.String()),
});

export const MediationResponseSchema = Type.Object({
	ack: Type.Boolean(),
	message: Type.String(),
});

export const TestStatsResponseSchema = Type.Object({
	totalRequests: Type.Number(),
});

export const TestStatsResetResponseSchema = Type.Object({
	previousCount: Type.Number(),
	currentCount: Type.Number(),
});

// ── Seed ──

export const SeedMessageRequestSchema = Type.Object({
	count: Type.Optional(Type.Number()),
	queue: Type.Optional(Type.String()),
	endpoint: Type.Optional(Type.String()),
	messageGroupMode: Type.Optional(Type.String()),
});

export const SeedMessageResponseSchema = Type.Object({
	status: Type.String(),
	messagesSent: Type.Optional(Type.Number()),
	totalRequested: Type.Optional(Type.Number()),
	message: Type.Optional(Type.String()),
});

// ── Benchmark ──

export const BenchmarkProcessResponseSchema = Type.Object({
	status: Type.String(),
	requestId: Type.Number(),
	timestamp: Type.Number(),
});

export const BenchmarkProcessSlowResponseSchema = Type.Object({
	status: Type.String(),
	requestId: Type.Number(),
	delayMs: Type.Number(),
	timestamp: Type.Number(),
});

export const BenchmarkStatsResponseSchema = Type.Object({
	totalRequests: Type.Number(),
	elapsedMs: Type.Number(),
	throughputPerSecond: Type.Number(),
});

export const BenchmarkResetResponseSchema = Type.Object({
	status: Type.String(),
});

// ── Benchmark query schemas ──

export const BenchmarkSlowQuerySchema = Type.Object({
	delayMs: Type.Optional(Type.String({ default: "100" })),
});

// ── Monitoring: OIDC Diagnostics ──

export const OidcDiagnosticsResponseSchema = Type.Object({
	authenticationEnabled: Type.Boolean(),
	authenticationMode: Type.Union([
		Type.Literal("NONE"),
		Type.Literal("BASIC"),
		Type.Literal("OIDC"),
	]),
	oidcConfigured: Type.Boolean(),
	issuerUrl: Type.Union([Type.String(), Type.Null()]),
	clientId: Type.Union([Type.String(), Type.Null()]),
	audience: Type.Union([Type.String(), Type.Null()]),
	discoveryEndpoint: Type.Union([Type.String(), Type.Null()]),
	jwksUri: Type.Union([Type.String(), Type.Null()]),
	discoveryStatus: Type.Union([
		Type.Literal("OK"),
		Type.Literal("ERROR"),
		Type.Literal("NOT_CONFIGURED"),
	]),
	discoveryError: Type.Union([Type.String(), Type.Null()]),
});

// ── Monitoring: Infrastructure Health ──

export const InfrastructureHealthCheckSchema = Type.Object({
	name: Type.String(),
	healthy: Type.Boolean(),
	message: Type.String(),
});

export const InfrastructureHealthResponseSchema = Type.Object({
	healthy: Type.Boolean(),
	checks: Type.Array(InfrastructureHealthCheckSchema),
	timestamp: Type.String(),
});

// ── Monitoring: query schemas ──

export const InFlightMessagesQuerySchema = Type.Object({
	limit: Type.Optional(Type.String({ default: "100" })),
	messageId: Type.Optional(Type.String()),
	poolCode: Type.Optional(Type.String()),
});

export const ClearOldWarningsQuerySchema = Type.Object({
	hours: Type.Optional(Type.String({ default: "24" })),
});

// ── Error ──

export const ErrorResponseSchema = Type.Object({
	status: Type.String(),
	message: Type.String(),
});
