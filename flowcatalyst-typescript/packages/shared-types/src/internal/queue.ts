import { z } from 'zod';

/**
 * Supported queue types
 */
export const QueueType = {
	SQS: 'SQS',
	NATS: 'NATS',
	EMBEDDED: 'EMBEDDED',
} as const;

export type QueueType = (typeof QueueType)[keyof typeof QueueType];

/**
 * Base queue configuration - internal representation
 * Matches Java QueueConfig
 */
export const BaseQueueConfigSchema = z.object({
	/** Queue URI (unique identifier) */
	queueUri: z.string(),
	/** Human-readable queue name */
	queueName: z.string().nullable(),
	/** Number of parallel polling connections (null = use global) */
	connections: z.number().int().min(1).max(10).nullable(),
});

/**
 * SQS-specific configuration
 */
export const SqsQueueConfigSchema = BaseQueueConfigSchema.extend({
	type: z.literal('SQS'),
	/** AWS region */
	region: z.string().default('us-east-1'),
	/** Long poll wait time in seconds */
	waitTimeSeconds: z.number().int().min(0).max(20).default(20),
	/** Max messages per poll */
	maxMessages: z.number().int().min(1).max(10).default(10),
	/** Visibility timeout in seconds */
	visibilityTimeout: z.number().int().min(0).max(43200).default(30),
});

export type SqsQueueConfig = z.infer<typeof SqsQueueConfigSchema>;

/**
 * NATS-specific configuration
 */
export const NatsQueueConfigSchema = BaseQueueConfigSchema.extend({
	type: z.literal('NATS'),
	/** NATS server URLs */
	servers: z.array(z.string()).min(1),
	/** Stream name */
	stream: z.string(),
	/** Consumer name */
	consumer: z.string(),
	/** Subject to subscribe to */
	subject: z.string(),
});

export type NatsQueueConfig = z.infer<typeof NatsQueueConfigSchema>;

/**
 * Embedded queue configuration (for local dev)
 */
export const EmbeddedQueueConfigSchema = BaseQueueConfigSchema.extend({
	type: z.literal('EMBEDDED'),
	/** SQLite database path */
	dbPath: z.string().default(':memory:'),
	/** Visibility timeout in seconds */
	visibilityTimeout: z.number().int().min(0).max(3600).default(30),
});

export type EmbeddedQueueConfig = z.infer<typeof EmbeddedQueueConfigSchema>;

/**
 * Union of all queue configurations
 */
export const QueueConfigSchema = z.discriminatedUnion('type', [
	SqsQueueConfigSchema,
	NatsQueueConfigSchema,
	EmbeddedQueueConfigSchema,
]);

export type QueueConfig = z.infer<typeof QueueConfigSchema>;

/**
 * Internal queue statistics tracking
 */
export interface QueueStatsInternal {
	/** Queue name */
	name: string;
	/** Queue URI */
	queueUri: string;
	/** Total messages received */
	totalMessages: number;
	/** Total successfully consumed */
	totalConsumed: number;
	/** Total failed */
	totalFailed: number;
	/** Total deferred */
	totalDeferred: number;
	/** Current queue size (from SQS attributes) */
	currentSize: number;
	/** Pending messages */
	pendingMessages: number;
	/** Messages not visible */
	messagesNotVisible: number;
	/** Windowed stats */
	windowedStats: {
		messages5min: number;
		consumed5min: number;
		failed5min: number;
		messages30min: number;
		consumed30min: number;
		failed30min: number;
	};
}

/**
 * Message callback interface for ack/nack operations
 */
export interface MessageCallback {
	/** Acknowledge the message (delete from queue) */
	ack(): Promise<void>;
	/** Negative acknowledge (change visibility for retry) */
	nack(visibilityTimeoutSeconds?: number): Promise<void>;
	/** Update the receipt handle (for redelivered messages) */
	updateReceiptHandle(newHandle: string): void;
}
