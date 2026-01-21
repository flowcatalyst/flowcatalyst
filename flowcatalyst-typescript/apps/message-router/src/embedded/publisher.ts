import type { Database } from 'sql.js';
import type { Logger } from '@flowcatalyst/logging';
import { randomUUID } from 'node:crypto';

/**
 * Message to publish to the embedded queue
 */
export interface EmbeddedQueueMessage {
	messageId: string;
	messageGroupId: string;
	messageDeduplicationId?: string;
	payload: unknown;
}

/**
 * Result of publishing a message
 */
export interface PublishResult {
	messageId: string;
	success: boolean;
	error?: string;
	deduplicated?: boolean;
}

/**
 * Deduplication window in milliseconds (5 minutes, matches SQS)
 */
const DEDUPLICATION_WINDOW_MS = 5 * 60 * 1000;

/**
 * Embedded queue publisher - publishes messages to SQLite-backed queue
 * Matches Java EmbeddedQueuePublisher behavior
 */
export class EmbeddedQueuePublisher {
	private readonly db: Database;
	private readonly logger: Logger;

	constructor(db: Database, logger: Logger) {
		this.db = db;
		this.logger = logger.child({ component: 'EmbeddedQueuePublisher' });
	}

	/**
	 * Publish a single message to the queue
	 */
	publish(message: EmbeddedQueueMessage): PublishResult {
		const now = Date.now();

		try {
			// Check deduplication if deduplication ID provided
			if (message.messageDeduplicationId) {
				const isDuplicate = this.checkDeduplication(message.messageDeduplicationId, now);
				if (isDuplicate) {
					this.logger.debug(
						{ messageId: message.messageId, deduplicationId: message.messageDeduplicationId },
						'Message deduplicated',
					);
					return {
						messageId: message.messageId,
						success: true,
						deduplicated: true,
					};
				}
			}

			// Insert message
			const receiptHandle = randomUUID();
			const messageJson = JSON.stringify(message.payload);

			this.db.run(
				`INSERT INTO queue_messages
				(message_id, message_group_id, message_deduplication_id, message_json, created_at, visible_at, receipt_handle)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				[
					message.messageId,
					message.messageGroupId,
					message.messageDeduplicationId || null,
					messageJson,
					now,
					now, // Immediately visible
					receiptHandle,
				],
			);

			// Record deduplication entry
			if (message.messageDeduplicationId) {
				this.db.run(
					`INSERT OR REPLACE INTO message_deduplication
					(message_deduplication_id, message_id, created_at)
					VALUES (?, ?, ?)`,
					[message.messageDeduplicationId, message.messageId, now],
				);
			}

			// Clean up old deduplication entries (older than 5 minutes)
			this.cleanupDeduplication(now);

			this.logger.debug(
				{ messageId: message.messageId, messageGroupId: message.messageGroupId },
				'Message published',
			);

			return {
				messageId: message.messageId,
				success: true,
			};
		} catch (error) {
			const errorMessage = error instanceof Error ? error.message : String(error);

			// Check for duplicate key error
			if (errorMessage.includes('UNIQUE constraint failed: queue_messages.message_id')) {
				this.logger.debug(
					{ messageId: message.messageId },
					'Duplicate message ID - already in queue',
				);
				return {
					messageId: message.messageId,
					success: true,
					deduplicated: true,
				};
			}

			this.logger.error(
				{ err: error, messageId: message.messageId },
				'Failed to publish message',
			);
			return {
				messageId: message.messageId,
				success: false,
				error: errorMessage,
			};
		}
	}

	/**
	 * Publish multiple messages to the queue
	 */
	publishBatch(messages: EmbeddedQueueMessage[]): PublishResult[] {
		return messages.map((message) => this.publish(message));
	}

	/**
	 * Check if a message with the given deduplication ID exists within the window
	 */
	private checkDeduplication(deduplicationId: string, now: number): boolean {
		const cutoff = now - DEDUPLICATION_WINDOW_MS;

		const result = this.db.exec(
			`SELECT 1 FROM message_deduplication
			WHERE message_deduplication_id = ? AND created_at > ?`,
			[deduplicationId, cutoff],
		);

		return result.length > 0 && result[0] !== undefined && result[0].values.length > 0;
	}

	/**
	 * Clean up deduplication entries older than the window
	 */
	private cleanupDeduplication(now: number): void {
		const cutoff = now - DEDUPLICATION_WINDOW_MS;

		try {
			this.db.run(
				'DELETE FROM message_deduplication WHERE created_at < ?',
				[cutoff],
			);
		} catch (error) {
			this.logger.warn({ err: error }, 'Failed to cleanup deduplication entries');
		}
	}

	/**
	 * Get queue statistics
	 */
	getStats(): { totalMessages: number; visibleMessages: number; invisibleMessages: number } {
		const now = Date.now();

		try {
			const result = this.db.exec(
				`SELECT
					COUNT(*) as total,
					COUNT(CASE WHEN visible_at <= ? THEN 1 END) as visible,
					COUNT(CASE WHEN visible_at > ? THEN 1 END) as invisible
				FROM queue_messages`,
				[now, now],
			);

			if (result.length > 0 && result[0] && result[0].values.length > 0) {
				const row = result[0].values[0];
				if (row) {
					return {
						totalMessages: Number(row[0]) || 0,
						visibleMessages: Number(row[1]) || 0,
						invisibleMessages: Number(row[2]) || 0,
					};
				}
			}
		} catch (error) {
			this.logger.warn({ err: error }, 'Failed to get queue stats');
		}

		return { totalMessages: 0, visibleMessages: 0, invisibleMessages: 0 };
	}
}
