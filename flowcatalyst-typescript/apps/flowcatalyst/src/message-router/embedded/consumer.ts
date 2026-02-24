import type { Database } from "sql.js";
import type { Logger } from "@flowcatalyst/logging";
import type {
	QueueConsumer,
	ConsumerHealth,
	QueueMetrics,
	MessageCallbackFns,
	StandardBatchHandler,
} from "@flowcatalyst/queue-core";
import type { MessageBatch, QueueMessage } from "@flowcatalyst/shared-types";
import { randomUUID } from "node:crypto";
import { parseMessagePointer } from "../consumers/parse-pointer.js";

/**
 * Internal message from dequeue (not exported)
 */
interface DequeuedMessage {
	id: number;
	messageId: string;
	messageGroupId: string;
	receiptHandle: string;
	receiveCount: number;
	payload: unknown;
}

/**
 * Consumer configuration
 */
export interface EmbeddedConsumerConfig {
	/** Queue name/identifier */
	queueName: string;
	/** Visibility timeout in seconds (default: 30) */
	visibilityTimeoutSeconds: number;
	/** Poll interval when queue is empty in milliseconds (default: 1000) */
	receiveTimeoutMs: number;
	/** Maximum messages per batch (default: 10) */
	maxMessages: number;
	/** Metrics poll interval in milliseconds (default: 5000) */
	metricsPollIntervalMs: number;
}

/**
 * Embedded queue consumer - consumes messages from SQLite-backed queue
 * Implements FIFO ordering per message group with visibility timeout
 */
export class EmbeddedQueueConsumer implements QueueConsumer {
	private readonly db: Database;
	private readonly config: EmbeddedConsumerConfig;
	private readonly handler: StandardBatchHandler;
	private readonly logger: Logger;
	private readonly instanceId: string;

	private running = false;
	private lastPollTimeMs = 0;
	private metrics = {
		pendingMessages: 0,
		messagesNotVisible: 0,
	};

	constructor(
		db: Database,
		config: EmbeddedConsumerConfig,
		handler: StandardBatchHandler,
		logger: Logger,
		instanceId: string,
	) {
		this.db = db;
		this.config = config;
		this.handler = handler;
		this.logger = logger.child({
			component: "EmbeddedQueueConsumer",
			queue: config.queueName,
		});
		this.instanceId = instanceId;
	}

	/**
	 * Start consuming messages
	 */
	async start(): Promise<void> {
		if (this.running) {
			this.logger.warn("Consumer already running");
			return;
		}

		this.running = true;
		this.logger.info(
			{ config: this.config },
			"Starting embedded queue consumer",
		);

		// Start metrics polling
		this.startMetricsPolling();

		// Start consumption loop
		this.consumeLoop();
	}

	/**
	 * Stop consuming messages
	 */
	async stop(): Promise<void> {
		this.logger.info("Stopping embedded queue consumer");
		this.running = false;
	}

	/**
	 * Main consumption loop
	 */
	private async consumeLoop(): Promise<void> {
		while (this.running) {
			try {
				this.lastPollTimeMs = Date.now();

				// Dequeue batch of raw messages
				const dequeued = this.dequeueBatch();

				if (dequeued.length === 0) {
					// No messages available - wait before polling again
					await sleep(this.config.receiveTimeoutMs);
					continue;
				}

				const batchId = randomUUID();
				const receivedAt = new Date();

				// Convert to standard types
				const messages: QueueMessage[] = [];
				const callbacks = new Map<string, MessageCallbackFns>();

				for (const msg of dequeued) {
					const pointer = parseMessagePointer(msg.messageId, msg.payload);
					// Preserve the embedded queue's message group ID (from SQLite column)
					pointer.messageGroupId = msg.messageGroupId;

					const queueMessage: QueueMessage = {
						messageId: msg.messageId,
						brokerMessageId: msg.messageId,
						receiptHandle: msg.receiptHandle,
						receiveCount: msg.receiveCount,
						receivedAt,
						batchId,
						queueId: this.config.queueName,
						pointer,
					};

					messages.push(queueMessage);

					const receiptHandle = msg.receiptHandle;
					callbacks.set(msg.messageId, {
						ack: async () => this.ack(receiptHandle),
						nack: async (visibilityTimeoutSeconds?: number) =>
							this.nack(receiptHandle, visibilityTimeoutSeconds),
						updateReceiptHandle: () => {},
						getReceiptHandle: () => receiptHandle,
						inProgress: () => {},
					});
				}

				// Create standard batch and process
				const batch: MessageBatch = {
					batchId,
					messages,
					queueId: this.config.queueName,
					receivedAt,
				};

				await this.handler(batch, callbacks);
			} catch (error) {
				this.logger.error({ err: error }, "Error in consumption loop");
				await sleep(1000); // Back off on error
			}
		}

		this.logger.info("Consumption loop stopped");
	}

	/**
	 * Dequeue a batch of messages with FIFO ordering per message group
	 */
	private dequeueBatch(): DequeuedMessage[] {
		const now = Date.now();
		const visibleAt = now + this.config.visibilityTimeoutSeconds * 1000;
		const messages: DequeuedMessage[] = [];

		// Process up to maxMessages, respecting message group ordering
		const processedGroups = new Set<string>();

		for (let i = 0; i < this.config.maxMessages; i++) {
			const message = this.dequeueOne(now, visibleAt, processedGroups);
			if (!message) break;

			messages.push(message);
			processedGroups.add(message.messageGroupId);
		}

		return messages;
	}

	/**
	 * Dequeue a single message using FIFO ordering
	 * Uses CTE to find the oldest message from the oldest available group
	 */
	private dequeueOne(
		now: number,
		visibleAt: number,
		excludeGroups: Set<string>,
	): DequeuedMessage | null {
		const newReceiptHandle = randomUUID();

		// Build exclusion clause for already-processed groups in this batch
		const excludeClause =
			excludeGroups.size > 0
				? `AND message_group_id NOT IN (${Array.from(excludeGroups)
						.map(() => "?")
						.join(",")})`
				: "";
		const excludeParams = Array.from(excludeGroups);

		// Find and update the next available message using FIFO ordering
		// This query finds the oldest message from the oldest available message group
		const selectSql = `
			WITH next_group AS (
				SELECT message_group_id
				FROM queue_messages
				WHERE visible_at <= ?
				${excludeClause}
				ORDER BY id
				LIMIT 1
			)
			SELECT id, message_id, message_group_id, message_json, receipt_handle, receive_count
			FROM queue_messages
			WHERE message_group_id IN (SELECT message_group_id FROM next_group)
				AND visible_at <= ?
			ORDER BY id
			LIMIT 1
		`;

		try {
			// First, find the message to update
			const selectResult = this.db.exec(selectSql, [
				now,
				...excludeParams,
				now,
			]);

			if (
				selectResult.length === 0 ||
				!selectResult[0] ||
				selectResult[0].values.length === 0
			) {
				return null;
			}

			const row = selectResult[0].values[0];
			if (!row) {
				return null;
			}
			const id = Number(row[0]);
			const messageId = String(row[1]);
			const messageGroupId = String(row[2]);
			const messageJson = String(row[3]);
			const receiveCount = Number(row[5]);

			// Update the message with new visibility and receipt handle
			this.db.run(
				`UPDATE queue_messages
				SET visible_at = ?,
					receipt_handle = ?,
					receive_count = receive_count + 1,
					first_received_at = COALESCE(first_received_at, ?)
				WHERE id = ?`,
				[visibleAt, newReceiptHandle, now, id],
			);

			// Parse payload
			let payload: unknown;
			try {
				payload = JSON.parse(messageJson);
			} catch {
				payload = messageJson;
			}

			return {
				id,
				messageId,
				messageGroupId,
				receiptHandle: newReceiptHandle,
				receiveCount: receiveCount + 1,
				payload,
			};
		} catch (error) {
			this.logger.error({ err: error }, "Error dequeuing message");
			return null;
		}
	}

	/**
	 * ACK a message - permanently remove from queue
	 */
	private ack(receiptHandle: string): void {
		try {
			this.db.run("DELETE FROM queue_messages WHERE receipt_handle = ?", [
				receiptHandle,
			]);
			this.logger.debug({ receiptHandle }, "Message ACKed");
		} catch (error) {
			this.logger.error({ err: error, receiptHandle }, "Failed to ACK message");
		}
	}

	/**
	 * NACK a message - reset visibility timeout for retry
	 */
	private nack(receiptHandle: string, visibilityTimeoutSeconds?: number): void {
		const timeout =
			visibilityTimeoutSeconds ?? this.config.visibilityTimeoutSeconds;
		const visibleAt = Date.now() + timeout * 1000;

		try {
			this.db.run(
				"UPDATE queue_messages SET visible_at = ? WHERE receipt_handle = ?",
				[visibleAt, receiptHandle],
			);
			this.logger.debug(
				{ receiptHandle, visibilityTimeoutSeconds: timeout },
				"Message NACKed",
			);
		} catch (error) {
			this.logger.error(
				{ err: error, receiptHandle },
				"Failed to NACK message",
			);
		}
	}

	/**
	 * Start periodic metrics polling
	 */
	private startMetricsPolling(): void {
		const poll = () => {
			if (!this.running) return;

			this.updateMetrics();
			setTimeout(poll, this.config.metricsPollIntervalMs);
		};

		setTimeout(poll, this.config.metricsPollIntervalMs);
	}

	/**
	 * Update queue metrics
	 */
	private updateMetrics(): void {
		const now = Date.now();

		try {
			const result = this.db.exec(
				`SELECT
					COUNT(CASE WHEN visible_at <= ? THEN 1 END) as visible,
					COUNT(CASE WHEN visible_at > ? THEN 1 END) as invisible
				FROM queue_messages`,
				[now, now],
			);

			if (result.length > 0 && result[0] && result[0].values.length > 0) {
				const row = result[0].values[0];
				if (row) {
					this.metrics.pendingMessages = Number(row[0]) || 0;
					this.metrics.messagesNotVisible = Number(row[1]) || 0;
				}
			}
		} catch (error) {
			this.logger.warn({ err: error }, "Failed to update metrics");
		}
	}

	/**
	 * Get the queue identifier
	 */
	getQueueIdentifier(): string {
		return this.config.queueName;
	}

	/**
	 * Check if consumer is currently running
	 */
	isRunning(): boolean {
		return this.running;
	}

	/**
	 * Check if consumer is fully stopped
	 */
	isFullyStopped(): boolean {
		return !this.running;
	}

	/**
	 * Get queue metrics
	 */
	getQueueMetrics(): QueueMetrics {
		return { ...this.metrics };
	}

	/**
	 * Get consumer health status
	 */
	getHealth(): ConsumerHealth {
		const now = Date.now();
		const timeSinceLastPoll = now - this.lastPollTimeMs;
		const healthTimeoutMs = 60_000; // 60 seconds

		return {
			mapKey: this.config.queueName,
			queueIdentifier: this.config.queueName,
			consumerQueueIdentifier: this.config.queueName,
			instanceId: this.instanceId,
			isHealthy:
				this.running &&
				(this.lastPollTimeMs === 0 || timeSinceLastPoll < healthTimeoutMs),
			lastPollTimeMs: this.lastPollTimeMs,
			lastPollTime: new Date(this.lastPollTimeMs).toISOString(),
			timeSinceLastPollMs: timeSinceLastPoll,
			timeSinceLastPollSeconds: Math.floor(timeSinceLastPoll / 1000),
			isRunning: this.running,
		};
	}

	/**
	 * Create a new consumer with the same configuration but a different handler
	 */
	recreate(handler: StandardBatchHandler): EmbeddedQueueConsumer {
		return new EmbeddedQueueConsumer(
			this.db,
			this.config,
			handler,
			this.logger,
			this.instanceId,
		);
	}
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}
