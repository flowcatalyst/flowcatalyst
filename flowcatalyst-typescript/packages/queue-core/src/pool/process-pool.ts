import type { Logger } from '@flowcatalyst/logging';
import type {
	PoolConfig,
	PoolState,
	PoolStats,
	QueueMessage,
} from '@flowcatalyst/shared-types';
import pLimit from 'p-limit';
import { RateLimiterMemory } from 'rate-limiter-flexible';
import type { HttpMediator } from '../mediation/http-mediator.js';
import { MessageGroupHandler } from './message-group-handler.js';

/**
 * Message callback for ack/nack
 */
export interface MessageCallback {
	ack(): Promise<void>;
	nack(visibilityTimeoutSeconds?: number): Promise<void>;
}

/**
 * Process pool implementation - matches Java ProcessPoolImpl
 * Uses per-message-group handlers for FIFO ordering within groups
 */
export class ProcessPool {
	private readonly config: PoolConfig;
	private readonly logger: Logger;
	private readonly mediator: HttpMediator;

	private state: PoolState = 'STARTING';
	private readonly messageGroups = new Map<string, MessageGroupHandler>();

	// Concurrency control
	private concurrencyLimiter: ReturnType<typeof pLimit>;
	// Rate limiting
	private rateLimiter: RateLimiterMemory | null;

	// Statistics tracking
	private totalProcessed = 0;
	private totalSucceeded = 0;
	private totalFailed = 0;
	private totalRateLimited = 0;
	private totalDeferred = 0;
	private processingTimes: number[] = [];

	// Windowed stats (simplified - use sliding window in production)
	private stats5min = { processed: 0, succeeded: 0, failed: 0, rateLimited: 0 };
	private stats30min = { processed: 0, succeeded: 0, failed: 0, rateLimited: 0 };

	// Batch+group failure tracking for FIFO
	private readonly failedBatchGroups = new Set<string>();
	private readonly batchGroupMessageCount = new Map<string, number>();

	// Capacity management
	private queuedMessages = 0;
	private readonly maxCapacity: number;

	constructor(config: PoolConfig, mediator: HttpMediator, logger: Logger) {
		this.config = config;
		this.mediator = mediator;
		this.logger = logger.child({ component: 'ProcessPool', poolCode: config.code });

		// Capacity = max(concurrency * 2, 50)
		this.maxCapacity = Math.max(config.concurrency * 2, 50);

		// Initialize concurrency limiter
		this.concurrencyLimiter = pLimit(config.concurrency);

		// Initialize rate limiter
		if (config.rateLimitPerMinute && config.rateLimitPerMinute > 0) {
			this.rateLimiter = new RateLimiterMemory({
				points: config.rateLimitPerMinute,
				duration: 60, // Per minute
			});
		} else {
			this.rateLimiter = null;
		}

		this.state = 'RUNNING';
		this.logger.info(
			{ concurrency: config.concurrency, capacity: this.maxCapacity },
			'Pool started',
		);
	}

	/**
	 * Submit a message for processing
	 * Returns false if pool is at capacity or draining
	 */
	async submit(
		message: QueueMessage,
		callback: MessageCallback,
	): Promise<boolean> {
		if (this.state !== 'RUNNING') {
			this.logger.warn({ state: this.state }, 'Pool not accepting messages');
			return false;
		}

		// Check capacity (Backpressure)
		if (this.queuedMessages >= this.maxCapacity) {
			this.logger.warn(
				{ queued: this.queuedMessages, capacity: this.maxCapacity },
				'Pool at capacity',
			);
			return false;
		}

		this.queuedMessages++;

		// Track batch+group count
		const batchGroupKey = `${message.batchId}|${message.pointer.messageGroupId}`;
		const currentCount = this.batchGroupMessageCount.get(batchGroupKey) || 0;
		this.batchGroupMessageCount.set(batchGroupKey, currentCount + 1);

		// Get or create message group handler
		let groupHandler = this.messageGroups.get(message.pointer.messageGroupId);
		if (!groupHandler) {
			groupHandler = new MessageGroupHandler(
				message.pointer.messageGroupId,
				this.processMessage.bind(this),
				() => {
					this.messageGroups.delete(message.pointer.messageGroupId);
					this.logger.debug(
						{ messageGroupId: message.pointer.messageGroupId },
						'Message group handler cleaned up',
					);
				},
				this.logger,
			);
			this.messageGroups.set(message.pointer.messageGroupId, groupHandler);
		}

		// Enqueue for processing
		groupHandler.enqueue(message, callback);
		return true;
	}

	/**
	 * Process a single message (called by message group handler)
	 */
	private async processMessage(
		message: QueueMessage,
		callback: MessageCallback,
	): Promise<void> {
		const batchGroupKey = `${message.batchId}|${message.pointer.messageGroupId}`;

		// Check if batch+group already failed (FIFO preservation)
		if (this.failedBatchGroups.has(batchGroupKey)) {
			this.logger.debug(
				{ messageId: message.messageId, batchGroupKey },
				'Skipping message due to batch+group failure',
			);
			await callback.nack(); // Default visibility
			this.decrementBatchGroupCount(batchGroupKey);
			this.queuedMessages--;
			return;
		}

		// Step 1: Rate Limiting (Wait logic)
		if (this.rateLimiter) {
			try {
				await this.rateLimiter.consume(1);
			} catch (rej) {
				// Rate limited
				this.totalRateLimited++;
				this.stats5min.rateLimited++;
				this.stats30min.rateLimited++;

				const msBeforeNext = (rej as { msBeforeNext: number }).msBeforeNext;
				// Wait in "Heap" (non-blocking for concurrency limiter)
				await new Promise((resolve) => setTimeout(resolve, msBeforeNext));
				// After waking up, we technically should try consuming again or just proceed
				// rate-limiter-flexible strict mode would require retry, but for a simple
				// smooth-out, proceeding is usually acceptable if we trust the wait time.
				// For strict enforcement, we'd loop, but that risks infinite loops.
				// Given msBeforeNext is precise, we proceed.
			}
		}

		// Step 2: Concurrency Control (Work logic)
		await this.concurrencyLimiter(async () => {
			try {
				const startTime = Date.now();
				const result = await this.mediator.process(message);
				const durationMs = Date.now() - startTime;

				this.recordProcessingTime(durationMs);
				this.totalProcessed++;
				this.stats5min.processed++;
				this.stats30min.processed++;

				switch (result.outcome) {
					case 'SUCCESS':
						this.totalSucceeded++;
						this.stats5min.succeeded++;
						this.stats30min.succeeded++;
						await callback.ack();
						this.decrementBatchGroupCount(batchGroupKey);
						break;

					case 'ERROR_CONFIG':
						// 4xx errors - ack to prevent infinite retry
						this.totalFailed++;
						this.stats5min.failed++;
						this.stats30min.failed++;
						await callback.ack();
						this.decrementBatchGroupCount(batchGroupKey);
						break;

					case 'DEFERRED':
						// Message not ready - nack with visibility
						this.totalDeferred++;
						await callback.nack(result.delaySeconds || 30);
						this.decrementBatchGroupCount(batchGroupKey);
						break;

					case 'ERROR_PROCESS':
					case 'BATCH_FAILED':
					case 'ERROR_CONNECTION':
					default:
						// 5xx or timeout - nack for retry, mark batch+group as failed
						this.totalFailed++;
						this.stats5min.failed++;
						this.stats30min.failed++;
						this.failedBatchGroups.add(batchGroupKey);
						await callback.nack(30); // 30s visibility for errors
						this.decrementBatchGroupCount(batchGroupKey);
						break;
				}
			} catch (error) {
				this.logger.error(
					{ err: error, messageId: message.messageId },
					'Processing error',
				);
				this.totalFailed++;
				this.stats5min.failed++;
				this.stats30min.failed++;
				this.failedBatchGroups.add(batchGroupKey);
				await callback.nack(30);
				this.decrementBatchGroupCount(batchGroupKey);
			} finally {
				this.queuedMessages--;
			}
		});
	}

	/**
	 * Decrement batch group count and cleanup if zero
	 */
	private decrementBatchGroupCount(key: string): void {
		const current = this.batchGroupMessageCount.get(key);
		if (current !== undefined) {
			const next = current - 1;
			if (next <= 0) {
				this.batchGroupMessageCount.delete(key);
				this.failedBatchGroups.delete(key);
			} else {
				this.batchGroupMessageCount.set(key, next);
			}
		}
	}

	/**
	 * Get pool statistics
	 */
	getStats(): PoolStats {
		const activeWorkers = this.concurrencyLimiter.activeCount;
		const successRate =
			this.totalProcessed > 0 ? this.totalSucceeded / this.totalProcessed : 1.0;

		return {
			poolCode: this.config.code,
			totalProcessed: this.totalProcessed,
			totalSucceeded: this.totalSucceeded,
			totalFailed: this.totalFailed,
			totalRateLimited: this.totalRateLimited,
			successRate,
			activeWorkers,
			availablePermits: this.config.concurrency - activeWorkers,
			maxConcurrency: this.config.concurrency,
			queueSize: this.queuedMessages,
			maxQueueCapacity: this.maxCapacity,
			averageProcessingTimeMs: this.getAverageProcessingTime(),
			totalProcessed5min: this.stats5min.processed,
			totalSucceeded5min: this.stats5min.succeeded,
			totalFailed5min: this.stats5min.failed,
			successRate5min:
				this.stats5min.processed > 0
					? this.stats5min.succeeded / this.stats5min.processed
					: 1.0,
			totalProcessed30min: this.stats30min.processed,
			totalSucceeded30min: this.stats30min.succeeded,
			totalFailed30min: this.stats30min.failed,
			successRate30min:
				this.stats30min.processed > 0
					? this.stats30min.succeeded / this.stats30min.processed
					: 1.0,
			totalRateLimited5min: this.stats5min.rateLimited,
			totalRateLimited30min: this.stats30min.rateLimited,
		};
	}

	/**
	 * Update pool configuration in-place
	 */
	updateConfig(newConfig: Partial<PoolConfig>): void {
		if (newConfig.rateLimitPerMinute !== undefined) {
			const rateLimit = newConfig.rateLimitPerMinute;
			if (rateLimit && rateLimit > 0) {
				this.rateLimiter = new RateLimiterMemory({
					points: rateLimit,
					duration: 60,
				});
				this.logger.info({ rateLimitPerMinute: rateLimit }, 'Rate limit updated');
			} else {
				this.rateLimiter = null;
				this.logger.info('Rate limit disabled');
			}
		}

		if (newConfig.concurrency !== undefined) {
			// p-limit cannot update concurrency dynamically.
			// We create a new limiter for future tasks.
			// Existing tasks in the old queue will finish there.
			this.concurrencyLimiter = pLimit(newConfig.concurrency);
			this.logger.info(
				{ concurrency: newConfig.concurrency },
				'Concurrency updated (applies to new tasks)',
			);
		}
	}

	/**
	 * Start draining - stop accepting new messages
	 */
	drain(): void {
		this.state = 'DRAINING';
		this.logger.info('Pool draining started');
	}

	/**
	 * Check if pool is fully drained
	 */
	isDrained(): boolean {
		return (
			this.state === 'DRAINING' &&
			this.queuedMessages === 0 &&
			this.concurrencyLimiter.activeCount === 0
		);
	}

	/**
	 * Shutdown the pool
	 */
	async shutdown(): Promise<void> {
		this.state = 'STOPPED';
		this.messageGroups.clear();
		this.failedBatchGroups.clear();
		this.batchGroupMessageCount.clear();
		this.logger.info('Pool shutdown complete');
	}

	/**
	 * Get current state
	 */
	getState(): PoolState {
		return this.state;
	}

	/**
	 * Get pool code
	 */
	getCode(): string {
		return this.config.code;
	}

	private recordProcessingTime(durationMs: number): void {
		this.processingTimes.push(durationMs);
		// Keep last 1000 samples
		if (this.processingTimes.length > 1000) {
			this.processingTimes.shift();
		}
	}

	private getAverageProcessingTime(): number {
		if (this.processingTimes.length === 0) return 0;
		const sum = this.processingTimes.reduce((a, b) => a + b, 0);
		return sum / this.processingTimes.length;
	}
}
