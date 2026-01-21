import initSqlJs, { type Database } from 'sql.js';
import type { Logger } from '@flowcatalyst/logging';
import { initializeSchema } from './schema.js';
import { EmbeddedQueuePublisher, type EmbeddedQueueMessage, type PublishResult } from './publisher.js';
import {
	EmbeddedQueueConsumer,
	type EmbeddedConsumerConfig,
	type EmbeddedBatchHandler,
} from './consumer.js';
import * as fs from 'node:fs';
import * as path from 'node:path';

/**
 * Embedded queue configuration
 */
export interface EmbeddedQueueConfig {
	/** Database file path (use ':memory:' for in-memory) */
	dbPath: string;
	/** Queue name */
	queueName: string;
	/** Visibility timeout in seconds (default: 30) */
	visibilityTimeoutSeconds?: number;
	/** Poll interval when empty in ms (default: 1000) */
	receiveTimeoutMs?: number;
	/** Max messages per batch (default: 10) */
	maxMessages?: number;
	/** Metrics poll interval in ms (default: 5000) */
	metricsPollIntervalMs?: number;
}

/**
 * Embedded queue - SQLite-backed message queue
 * Provides SQS-like semantics with FIFO ordering per message group
 */
export class EmbeddedQueue {
	private db: Database | null = null;
	private publisher: EmbeddedQueuePublisher | null = null;
	private consumer: EmbeddedQueueConsumer | null = null;
	private readonly config: EmbeddedQueueConfig;
	private readonly logger: Logger;
	private readonly instanceId: string;
	private saveInterval: ReturnType<typeof setInterval> | null = null;

	constructor(config: EmbeddedQueueConfig, logger: Logger, instanceId: string) {
		this.config = config;
		this.logger = logger.child({ component: 'EmbeddedQueue', queue: config.queueName });
		this.instanceId = instanceId;
	}

	/**
	 * Initialize the embedded queue
	 */
	async initialize(): Promise<void> {
		this.logger.info({ dbPath: this.config.dbPath }, 'Initializing embedded queue');

		// Initialize sql.js
		const SQL = await initSqlJs();

		// Load existing database or create new one
		if (this.config.dbPath !== ':memory:' && fs.existsSync(this.config.dbPath)) {
			const buffer = fs.readFileSync(this.config.dbPath);
			this.db = new SQL.Database(buffer);
			this.logger.info('Loaded existing database');
		} else {
			this.db = new SQL.Database();
			this.logger.info('Created new database');
		}

		// Initialize schema
		initializeSchema(this.db);

		// Create publisher
		this.publisher = new EmbeddedQueuePublisher(this.db, this.logger);

		// Start periodic save for file-based databases
		if (this.config.dbPath !== ':memory:') {
			this.startPeriodicSave();
		}

		this.logger.info('Embedded queue initialized');
	}

	/**
	 * Start consuming messages
	 */
	async startConsumer(handler: EmbeddedBatchHandler): Promise<void> {
		if (!this.db) {
			throw new Error('Queue not initialized');
		}

		const consumerConfig: EmbeddedConsumerConfig = {
			queueName: this.config.queueName,
			visibilityTimeoutSeconds: this.config.visibilityTimeoutSeconds ?? 30,
			receiveTimeoutMs: this.config.receiveTimeoutMs ?? 1000,
			maxMessages: this.config.maxMessages ?? 10,
			metricsPollIntervalMs: this.config.metricsPollIntervalMs ?? 5000,
		};

		this.consumer = new EmbeddedQueueConsumer(
			this.db,
			consumerConfig,
			handler,
			this.logger,
			this.instanceId,
		);

		await this.consumer.start();
	}

	/**
	 * Stop consuming messages
	 */
	async stopConsumer(): Promise<void> {
		if (this.consumer) {
			await this.consumer.stop();
			this.consumer = null;
		}
	}

	/**
	 * Publish a message to the queue
	 */
	publish(message: EmbeddedQueueMessage): PublishResult {
		if (!this.publisher) {
			throw new Error('Queue not initialized');
		}
		return this.publisher.publish(message);
	}

	/**
	 * Publish multiple messages to the queue
	 */
	publishBatch(messages: EmbeddedQueueMessage[]): PublishResult[] {
		if (!this.publisher) {
			throw new Error('Queue not initialized');
		}
		return this.publisher.publishBatch(messages);
	}

	/**
	 * Get queue statistics
	 */
	getStats(): { totalMessages: number; visibleMessages: number; invisibleMessages: number } {
		if (!this.publisher) {
			return { totalMessages: 0, visibleMessages: 0, invisibleMessages: 0 };
		}
		return this.publisher.getStats();
	}

	/**
	 * Get consumer metrics
	 */
	getConsumerMetrics(): { pendingMessages: number; messagesNotVisible: number } | null {
		return this.consumer?.getQueueMetrics() ?? null;
	}

	/**
	 * Get consumer health
	 */
	getConsumerHealth() {
		return this.consumer?.getHealth() ?? null;
	}

	/**
	 * Save database to disk (for file-based databases)
	 */
	save(): void {
		if (!this.db || this.config.dbPath === ':memory:') {
			return;
		}

		try {
			const data = this.db.export();
			const buffer = Buffer.from(data);

			// Ensure directory exists
			const dir = path.dirname(this.config.dbPath);
			if (!fs.existsSync(dir)) {
				fs.mkdirSync(dir, { recursive: true });
			}

			fs.writeFileSync(this.config.dbPath, buffer);
			this.logger.debug('Database saved to disk');
		} catch (error) {
			this.logger.error({ err: error }, 'Failed to save database');
		}
	}

	/**
	 * Start periodic save
	 */
	private startPeriodicSave(): void {
		// Save every 10 seconds
		this.saveInterval = setInterval(() => {
			this.save();
		}, 10_000);
	}

	/**
	 * Close the embedded queue
	 */
	async close(): Promise<void> {
		this.logger.info('Closing embedded queue');

		// Stop consumer
		if (this.consumer) {
			await this.consumer.stop();
			this.consumer = null;
		}

		// Stop periodic save
		if (this.saveInterval) {
			clearInterval(this.saveInterval);
			this.saveInterval = null;
		}

		// Final save
		this.save();

		// Close database
		if (this.db) {
			this.db.close();
			this.db = null;
		}

		this.publisher = null;

		this.logger.info('Embedded queue closed');
	}
}
