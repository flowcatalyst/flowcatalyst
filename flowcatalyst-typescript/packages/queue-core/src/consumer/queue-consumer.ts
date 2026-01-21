import type { Logger } from '@flowcatalyst/logging';
import type { MessageBatch, QueueConfig } from '@flowcatalyst/shared-types';

/**
 * Message handler callback type
 */
export type MessageBatchHandler = (batch: MessageBatch) => Promise<void>;

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
 * Abstract base class for queue consumers.
 * Implementations handle queue-specific polling and message parsing.
 */
export abstract class QueueConsumer {
	protected readonly config: QueueConfig;
	protected readonly logger: Logger;
	protected readonly handler: MessageBatchHandler;
	protected readonly instanceId: string;

	protected running = false;
	protected lastPollTimeMs = 0;
	protected errorCount = 0;
	protected lastError: string | null = null;

	constructor(
		config: QueueConfig,
		handler: MessageBatchHandler,
		logger: Logger,
		instanceId: string,
	) {
		this.config = config;
		this.handler = handler;
		this.logger = logger.child({ component: 'QueueConsumer', queueId: config.queueUri });
		this.instanceId = instanceId;
	}

	/**
	 * Start consuming messages
	 */
	abstract start(): Promise<void>;

	/**
	 * Stop consuming messages gracefully
	 */
	abstract stop(): Promise<void>;

	/**
	 * Get consumer health status - matches Java format exactly
	 */
	getHealth(): ConsumerHealth {
		const currentTimeMs = Date.now();
		const timeSinceLastPollMs =
			this.lastPollTimeMs > 0 ? currentTimeMs - this.lastPollTimeMs : -1;
		const timeSinceLastPollSeconds =
			timeSinceLastPollMs > 0 ? Math.floor(timeSinceLastPollMs / 1000) : -1;

		const staleThresholdMs = 60_000; // 60 seconds
		const isHealthy =
			this.running && (this.lastPollTimeMs === 0 || timeSinceLastPollMs < staleThresholdMs);

		return {
			mapKey: this.config.queueUri,
			queueIdentifier: this.config.queueUri,
			consumerQueueIdentifier: this.config.queueUri,
			instanceId: this.instanceId,
			isHealthy,
			lastPollTimeMs: this.lastPollTimeMs,
			lastPollTime: this.lastPollTimeMs > 0 ? new Date(this.lastPollTimeMs).toISOString() : 'never',
			timeSinceLastPollMs,
			timeSinceLastPollSeconds,
			isRunning: this.running,
		};
	}

	/**
	 * Check if consumer is running
	 */
	isRunning(): boolean {
		return this.running;
	}

	/**
	 * Record successful poll
	 */
	protected recordPoll(): void {
		this.lastPollTimeMs = Date.now();
		this.errorCount = 0;
		this.lastError = null;
	}

	/**
	 * Record poll error
	 */
	protected recordError(error: Error): void {
		this.errorCount++;
		this.lastError = error.message;
		this.logger.error({ err: error }, 'Consumer poll error');
	}
}

/**
 * Acknowledge a message (delete from queue)
 */
export type AckFn = () => Promise<void>;

/**
 * Negative acknowledge a message (change visibility for retry)
 */
export type NackFn = (visibilityTimeoutSeconds?: number) => Promise<void>;

/**
 * Message callback created by consumer for each message
 */
export interface MessageCallbackFns {
	ack: AckFn;
	nack: NackFn;
	updateReceiptHandle: (newHandle: string) => void;
}

/**
 * Factory function type for creating queue consumers
 */
export type QueueConsumerFactory = (
	config: QueueConfig,
	handler: MessageBatchHandler,
	logger: Logger,
	instanceId: string,
) => QueueConsumer;
