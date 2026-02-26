import type { MessageBatch } from "@flowcatalyst/contracts";

/**
 * Queue Consumer Interface
 *
 * Matches Java QueueConsumer contract. All queue implementations (SQS, NATS, ActiveMQ, etc.)
 * implement this interface to provide a consistent lifecycle and health monitoring contract.
 */

/**
 * Standard batch handler type - the unified callback signature that all consumers use.
 * Consumers normalize broker-specific messages into `MessageBatch` + `MessageCallbackFns`
 * before invoking this handler, so the queue manager never sees broker-specific types.
 */
export type StandardBatchHandler = (
	batch: MessageBatch,
	callbacks: Map<string, MessageCallbackFns>,
) => Promise<void>;

/**
 * Queue consumer interface - the contract all broker implementations must satisfy.
 */
export interface QueueConsumer {
	/** Start consuming messages from the queue */
	start(): Promise<void>;

	/** Stop consuming messages gracefully */
	stop(): Promise<void>;

	/** Get the queue identifier (URL, name, or URI) */
	getQueueIdentifier(): string;

	/** Check if consumer is currently running */
	isRunning(): boolean;

	/** Check if consumer is fully stopped (not running and no pending tasks) */
	isFullyStopped(): boolean;

	/** Get consumer health status */
	getHealth(): ConsumerHealth;

	/** Get queue metrics (pending messages, in-flight count) */
	getQueueMetrics(): QueueMetrics;

	/** Create a new consumer with the same configuration but a different handler */
	recreate(handler: StandardBatchHandler): QueueConsumer;
}

/**
 * Consumer health info - matches Java ConsumerHealthInfo
 */
export interface ConsumerHealth {
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
 * Queue metrics
 */
export interface QueueMetrics {
	pendingMessages: number;
	messagesNotVisible: number;
}

/**
 * Acknowledge a message (delete from queue)
 */
export type AckFn = () => Promise<void>;

/**
 * Negative acknowledge a message (change visibility for retry)
 */
export type NackFn = (delaySeconds?: number) => Promise<void>;

/**
 * Message callback created by consumer for each message.
 * Matches Java MessageCallback + MessageVisibilityControl combined.
 */
export interface MessageCallbackFns {
	ack: AckFn;
	nack: NackFn;
	updateReceiptHandle: (newHandle: string) => void;
	getReceiptHandle: () => string;
	inProgress: () => void;
}
