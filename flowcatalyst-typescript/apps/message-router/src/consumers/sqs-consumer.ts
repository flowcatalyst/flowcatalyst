import {
	SQSClient,
	ReceiveMessageCommand,
	DeleteMessageCommand,
	ChangeMessageVisibilityCommand,
	GetQueueAttributesCommand,
	type Message,
} from '@aws-sdk/client-sqs';
import type { Logger } from '@flowcatalyst/logging';
import type {
	MessageBatch,
	MessagePointer,
	MessagePointerSchema,
	QueueMessage,
	SqsQueueConfig,
} from '@flowcatalyst/shared-types';
import { randomUUID } from 'node:crypto';
import { env } from '../env.js';

/**
 * Message callback for ACK/NACK operations
 */
export interface SqsMessageCallback {
	ack(): Promise<void>;
	nack(visibilityTimeoutSeconds?: number): Promise<void>;
	updateReceiptHandle(newHandle: string): void;
}

/**
 * Batch handler callback type
 */
export type BatchHandler = (batch: MessageBatch, callbacks: Map<string, SqsMessageCallback>) => Promise<void>;

/**
 * SQS Consumer configuration
 */
export interface SqsConsumerConfig {
	queueUrl: string;
	queueName: string;
	region: string;
	waitTimeSeconds: number;
	maxMessages: number;
	visibilityTimeout: number;
	connections: number;
	metricsPollIntervalMs: number;
}

/**
 * Consumer health status
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
 * SQS Queue Consumer
 * Matches Java SqsQueueConsumer behavior exactly
 */
export class SqsConsumer {
	private readonly config: SqsConsumerConfig;
	private readonly client: SQSClient;
	private readonly handler: BatchHandler;
	private readonly logger: Logger;
	private readonly instanceId: string;

	private running = false;
	private lastPollTimeMs = 0;
	private pollingTasks: Promise<void>[] = [];
	private metricsTask: Promise<void> | null = null;

	// Track pending deletes for expired receipt handles
	private readonly pendingDeleteSqsMessageIds = new Map<string, string>();

	// Queue metrics
	private pendingMessages = 0;
	private messagesNotVisible = 0;

	// Health check timeout (60 seconds)
	private static readonly POLL_TIMEOUT_MS = 60_000;

	// Adaptive delays
	private static readonly EMPTY_BATCH_DELAY_MS = 1000;
	private static readonly PARTIAL_BATCH_DELAY_MS = 50;

	constructor(
		config: SqsConsumerConfig,
		handler: BatchHandler,
		logger: Logger,
		instanceId: string,
	) {
		this.config = config;
		this.handler = handler;
		this.logger = logger.child({
			component: 'SqsConsumer',
			queueUrl: config.queueUrl,
			queueName: config.queueName,
		});
		this.instanceId = instanceId;

		// Create SQS client
		this.client = new SQSClient({
			region: config.region,
			...(env.SQS_ENDPOINT && { endpoint: env.SQS_ENDPOINT }),
		});
	}

	/**
	 * Start consuming messages
	 */
	async start(): Promise<void> {
		if (this.running) {
			this.logger.warn('Consumer already running');
			return;
		}

		this.running = true;
		this.logger.info(
			{ connections: this.config.connections },
			'Starting SQS consumer',
		);

		// Start polling threads (one per connection)
		for (let i = 0; i < this.config.connections; i++) {
			const task = this.pollLoop(i);
			this.pollingTasks.push(task);
		}

		// Start metrics polling
		this.metricsTask = this.metricsLoop();
	}

	/**
	 * Stop consuming messages gracefully
	 */
	async stop(): Promise<void> {
		this.logger.info('Stopping SQS consumer');
		this.running = false;

		// Wait for all polling tasks to complete
		await Promise.allSettled(this.pollingTasks);
		if (this.metricsTask) {
			await this.metricsTask;
		}

		this.pollingTasks = [];
		this.metricsTask = null;
		this.logger.info('SQS consumer stopped');
	}

	/**
	 * Get consumer health status
	 */
	getHealth(): ConsumerHealthInfo {
		const currentTimeMs = Date.now();
		const timeSinceLastPollMs =
			this.lastPollTimeMs > 0 ? currentTimeMs - this.lastPollTimeMs : -1;
		const timeSinceLastPollSeconds =
			timeSinceLastPollMs > 0 ? Math.floor(timeSinceLastPollMs / 1000) : -1;

		const isHealthy =
			this.running &&
			(this.lastPollTimeMs === 0 || timeSinceLastPollMs < SqsConsumer.POLL_TIMEOUT_MS);

		return {
			mapKey: this.config.queueUrl,
			queueIdentifier: this.config.queueUrl,
			consumerQueueIdentifier: this.config.queueUrl,
			instanceId: this.instanceId,
			isHealthy,
			lastPollTimeMs: this.lastPollTimeMs,
			lastPollTime:
				this.lastPollTimeMs > 0 ? new Date(this.lastPollTimeMs).toISOString() : 'never',
			timeSinceLastPollMs,
			timeSinceLastPollSeconds,
			isRunning: this.running,
		};
	}

	/**
	 * Get queue metrics
	 */
	getQueueMetrics(): { pendingMessages: number; messagesNotVisible: number } {
		return {
			pendingMessages: this.pendingMessages,
			messagesNotVisible: this.messagesNotVisible,
		};
	}

	/**
	 * Check if consumer is running
	 */
	isRunning(): boolean {
		return this.running;
	}

	/**
	 * Check if consumer is fully stopped (not running and no pending tasks)
	 * Used by cleanup task to know when it's safe to remove from draining list
	 */
	isFullyStopped(): boolean {
		return !this.running && this.pollingTasks.length === 0;
	}

	/**
	 * Main polling loop
	 */
	private async pollLoop(connectionIndex: number): Promise<void> {
		this.logger.debug({ connectionIndex }, 'Poll loop started');

		while (this.running) {
			const loopStart = Date.now();

			try {
				// Record heartbeat
				this.lastPollTimeMs = Date.now();

				// Long poll for messages
				const command = new ReceiveMessageCommand({
					QueueUrl: this.config.queueUrl,
					MaxNumberOfMessages: this.config.maxMessages,
					WaitTimeSeconds: this.config.waitTimeSeconds,
					VisibilityTimeout: this.config.visibilityTimeout,
					MessageSystemAttributeNames: ['ApproximateReceiveCount', 'MessageGroupId'],
					MessageAttributeNames: ['All'],
				});

				const response = await this.client.send(command);
				const messages = response.Messages || [];

				this.logger.debug(
					{ messageCount: messages.length, connectionIndex },
					'Received messages from SQS',
				);

				if (messages.length > 0) {
					await this.processBatch(messages);
				}

				// Adaptive delay based on batch size
				const delay = this.getAdaptiveDelay(messages.length);
				if (delay > 0) {
					await sleep(delay);
				}

				// Check for thread starvation
				const loopDuration = Date.now() - loopStart;
				if (loopDuration > 30_000) {
					this.logger.warn(
						{ loopDuration, connectionIndex },
						'Poll loop took longer than 30 seconds - possible thread starvation',
					);
				}
			} catch (error) {
				if (this.running) {
					this.logger.error({ err: error, connectionIndex }, 'Error polling SQS');
					await sleep(1000); // Back off on error
				}
			}
		}

		this.logger.debug({ connectionIndex }, 'Poll loop stopped');
	}

	/**
	 * Process a batch of messages
	 */
	private async processBatch(sqsMessages: Message[]): Promise<void> {
		const batchId = randomUUID();
		const receivedAt = new Date();
		const messages: QueueMessage[] = [];
		const callbacks = new Map<string, SqsMessageCallback>();
		const seenMessageIds = new Set<string>();

		for (const sqsMsg of sqsMessages) {
			if (!sqsMsg.Body || !sqsMsg.ReceiptHandle || !sqsMsg.MessageId) {
				this.logger.warn('Received SQS message without body, receipt handle, or message ID');
				continue;
			}

			// Check for pending deletes (messages that were processed but delete failed)
			if (this.pendingDeleteSqsMessageIds.has(sqsMsg.MessageId)) {
				this.logger.info(
					{ sqsMessageId: sqsMsg.MessageId },
					'Found pending delete - deleting message',
				);
				await this.deleteMessage(sqsMsg.ReceiptHandle);
				this.pendingDeleteSqsMessageIds.delete(sqsMsg.MessageId);
				continue;
			}

			// Parse message body
			let pointer: MessagePointer;
			try {
				const parsed = JSON.parse(sqsMsg.Body);
				pointer = {
					messageId: parsed.id || parsed.messageId || sqsMsg.MessageId,
					poolCode: parsed.poolCode || 'DEFAULT',
					messageGroupId:
						parsed.messageGroupId ||
						sqsMsg.Attributes?.MessageGroupId ||
						'__DEFAULT__',
					payload: parsed.payload || parsed,
					authToken: parsed.authToken,
					callbackUrl: parsed.mediationTarget || parsed.callbackUrl,
					createdAt: parsed.createdAt,
				};
			} catch (error) {
				this.logger.warn(
					{ err: error, sqsMessageId: sqsMsg.MessageId },
					'Failed to parse message body - ACKing to prevent infinite retry',
				);
				await this.deleteMessage(sqsMsg.ReceiptHandle);
				continue;
			}

			// Within-batch deduplication
			if (seenMessageIds.has(pointer.messageId)) {
				this.logger.debug(
					{ messageId: pointer.messageId },
					'Duplicate message in batch - ACKing',
				);
				await this.deleteMessage(sqsMsg.ReceiptHandle);
				continue;
			}
			seenMessageIds.add(pointer.messageId);

			// Create queue message
			const queueMessage: QueueMessage = {
				brokerMessageId: sqsMsg.MessageId,
				messageId: pointer.messageId,
				receiptHandle: sqsMsg.ReceiptHandle,
				pointer,
				receiveCount: Number.parseInt(
					sqsMsg.Attributes?.ApproximateReceiveCount || '1',
					10,
				),
				receivedAt,
				batchId,
				queueId: this.config.queueUrl,
			};

			// Create callback with current receipt handle
			let currentReceiptHandle = sqsMsg.ReceiptHandle;
			const callback: SqsMessageCallback = {
				ack: async () => {
					await this.ackMessage(sqsMsg.MessageId!, currentReceiptHandle, pointer);
				},
				nack: async (visibilityTimeoutSeconds?: number) => {
					await this.nackMessage(currentReceiptHandle, visibilityTimeoutSeconds);
				},
				updateReceiptHandle: (newHandle: string) => {
					currentReceiptHandle = newHandle;
				},
			};

			messages.push(queueMessage);
			callbacks.set(sqsMsg.MessageId, callback);
		}

		if (messages.length === 0) {
			return;
		}

		// Create batch and pass to handler
		const batch: MessageBatch = {
			batchId,
			messages,
			queueId: this.config.queueUrl,
			receivedAt,
		};

		try {
			await this.handler(batch, callbacks);
		} catch (error) {
			this.logger.error({ err: error, batchId }, 'Error handling batch');
			// NACK all messages in batch
			for (const callback of callbacks.values()) {
				try {
					await callback.nack();
				} catch (nackError) {
					this.logger.error({ err: nackError }, 'Error NACKing message');
				}
			}
		}
	}

	/**
	 * ACK a message (delete from queue)
	 */
	private async ackMessage(
		sqsMessageId: string,
		receiptHandle: string,
		pointer: MessagePointer,
	): Promise<void> {
		try {
			await this.deleteMessage(receiptHandle);
			this.logger.debug({ messageId: pointer.messageId }, 'Message ACKed');
		} catch (error) {
			// Check if receipt handle expired
			const errorMessage = error instanceof Error ? error.message : String(error);
			if (
				errorMessage.includes('ReceiptHandleIsInvalid') ||
				errorMessage.includes('receipt handle has expired')
			) {
				this.logger.warn(
					{ messageId: pointer.messageId, sqsMessageId },
					'Receipt handle expired - adding to pending deletes',
				);
				this.pendingDeleteSqsMessageIds.set(sqsMessageId, pointer.messageId);
			} else {
				this.logger.error(
					{ err: error, messageId: pointer.messageId },
					'Unexpected error deleting message',
				);
			}
		}
	}

	/**
	 * Delete a message from the queue
	 */
	private async deleteMessage(receiptHandle: string): Promise<void> {
		const command = new DeleteMessageCommand({
			QueueUrl: this.config.queueUrl,
			ReceiptHandle: receiptHandle,
		});
		await this.client.send(command);
	}

	/**
	 * NACK a message (change visibility for retry)
	 */
	private async nackMessage(
		receiptHandle: string,
		visibilityTimeoutSeconds = 30,
	): Promise<void> {
		try {
			// Clamp to SQS limits (0-43200 seconds)
			const timeout = Math.max(0, Math.min(43200, visibilityTimeoutSeconds));

			const command = new ChangeMessageVisibilityCommand({
				QueueUrl: this.config.queueUrl,
				ReceiptHandle: receiptHandle,
				VisibilityTimeout: timeout,
			});
			await this.client.send(command);
			this.logger.debug({ visibilityTimeout: timeout }, 'Message NACKed');
		} catch (error) {
			this.logger.error({ err: error }, 'Error changing message visibility');
		}
	}

	/**
	 * Get adaptive delay based on batch size
	 */
	private getAdaptiveDelay(messageCount: number): number {
		if (messageCount === 0) {
			return SqsConsumer.EMPTY_BATCH_DELAY_MS;
		}
		if (messageCount < this.config.maxMessages) {
			return SqsConsumer.PARTIAL_BATCH_DELAY_MS;
		}
		return 0; // Full batch - no delay
	}

	/**
	 * Metrics polling loop
	 */
	private async metricsLoop(): Promise<void> {
		this.logger.debug('Metrics loop started');

		while (this.running) {
			try {
				const command = new GetQueueAttributesCommand({
					QueueUrl: this.config.queueUrl,
					AttributeNames: [
						'ApproximateNumberOfMessages',
						'ApproximateNumberOfMessagesNotVisible',
					],
				});

				const response = await this.client.send(command);
				const attrs = response.Attributes || {};

				this.pendingMessages = Number.parseInt(
					attrs['ApproximateNumberOfMessages'] || '0',
					10,
				);
				this.messagesNotVisible = Number.parseInt(
					attrs['ApproximateNumberOfMessagesNotVisible'] || '0',
					10,
				);

				this.logger.debug(
					{ pendingMessages: this.pendingMessages, messagesNotVisible: this.messagesNotVisible },
					'Queue metrics updated',
				);
			} catch (error) {
				if (this.running) {
					this.logger.error({ err: error }, 'Error polling queue metrics');
				}
			}

			await sleep(this.config.metricsPollIntervalMs);
		}

		this.logger.debug('Metrics loop stopped');
	}
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}
