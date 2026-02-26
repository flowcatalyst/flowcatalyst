/**
 * Outbox Processor Configuration
 *
 * Environment variables and configuration for the outbox processor.
 */

import { z } from "zod/v4";

const envSchema = z.object({
	// Connection to the CUSTOMER database (NOT the platform database)
	OUTBOX_PROCESSOR_DATABASE_URL: z.string(),

	// FlowCatalyst platform API base URL
	OUTBOX_PROCESSOR_API_BASE_URL: z.string(),

	// Optional bearer token for API authentication
	OUTBOX_PROCESSOR_API_TOKEN: z.string().optional(),

	// Polling
	OUTBOX_PROCESSOR_POLL_INTERVAL_MS: z.coerce.number().default(1000),
	OUTBOX_PROCESSOR_POLL_BATCH_SIZE: z.coerce.number().default(500),

	// API batching
	OUTBOX_PROCESSOR_API_BATCH_SIZE: z.coerce.number().default(100),

	// Concurrency
	OUTBOX_PROCESSOR_MAX_CONCURRENT_GROUPS: z.coerce.number().default(10),
	OUTBOX_PROCESSOR_GLOBAL_BUFFER_SIZE: z.coerce.number().default(1000),
	OUTBOX_PROCESSOR_MAX_IN_FLIGHT: z.coerce.number().default(5000),

	// Retry
	OUTBOX_PROCESSOR_MAX_RETRIES: z.coerce.number().default(3),

	// Recovery
	OUTBOX_PROCESSOR_PROCESSING_TIMEOUT_SECONDS: z.coerce.number().default(300),
	OUTBOX_PROCESSOR_RECOVERY_INTERVAL_MS: z.coerce.number().default(60000),

	// Table names (shared-table pattern by default)
	OUTBOX_PROCESSOR_EVENTS_TABLE: z.string().default("outbox_messages"),
	OUTBOX_PROCESSOR_DISPATCH_JOBS_TABLE: z.string().default("outbox_messages"),
	OUTBOX_PROCESSOR_AUDIT_LOGS_TABLE: z.string().default("outbox_messages"),
});

export type OutboxProcessorEnv = z.infer<typeof envSchema>;

export interface OutboxProcessorConfig {
	readonly databaseUrl: string;
	readonly apiBaseUrl: string;
	readonly apiToken: string | undefined;
	readonly pollIntervalMs: number;
	readonly pollBatchSize: number;
	readonly apiBatchSize: number;
	readonly maxConcurrentGroups: number;
	readonly globalBufferSize: number;
	readonly maxInFlight: number;
	readonly maxRetries: number;
	readonly processingTimeoutSeconds: number;
	readonly recoveryIntervalMs: number;
	readonly eventsTable: string;
	readonly dispatchJobsTable: string;
	readonly auditLogsTable: string;
}

export function loadOutboxProcessorConfig(): OutboxProcessorConfig {
	const env = envSchema.parse(process.env);

	return {
		databaseUrl: env.OUTBOX_PROCESSOR_DATABASE_URL,
		apiBaseUrl: env.OUTBOX_PROCESSOR_API_BASE_URL,
		apiToken: env.OUTBOX_PROCESSOR_API_TOKEN,
		pollIntervalMs: env.OUTBOX_PROCESSOR_POLL_INTERVAL_MS,
		pollBatchSize: env.OUTBOX_PROCESSOR_POLL_BATCH_SIZE,
		apiBatchSize: env.OUTBOX_PROCESSOR_API_BATCH_SIZE,
		maxConcurrentGroups: env.OUTBOX_PROCESSOR_MAX_CONCURRENT_GROUPS,
		globalBufferSize: env.OUTBOX_PROCESSOR_GLOBAL_BUFFER_SIZE,
		maxInFlight: env.OUTBOX_PROCESSOR_MAX_IN_FLIGHT,
		maxRetries: env.OUTBOX_PROCESSOR_MAX_RETRIES,
		processingTimeoutSeconds: env.OUTBOX_PROCESSOR_PROCESSING_TIMEOUT_SECONDS,
		recoveryIntervalMs: env.OUTBOX_PROCESSOR_RECOVERY_INTERVAL_MS,
		eventsTable: env.OUTBOX_PROCESSOR_EVENTS_TABLE,
		dispatchJobsTable: env.OUTBOX_PROCESSOR_DISPATCH_JOBS_TABLE,
		auditLogsTable: env.OUTBOX_PROCESSOR_AUDIT_LOGS_TABLE,
	};
}
