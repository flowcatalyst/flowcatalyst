import type { Logger } from "@flowcatalyst/logging";
import type {
	PoolConfig,
	PoolState,
	PoolStats,
	QueueMessage,
} from "@flowcatalyst/shared-types";
import { RateLimiterMemory, RateLimiterQueue } from "rate-limiter-flexible";
import type { HttpMediator } from "../mediation/http-mediator.js";
import { DynamicSemaphore } from "./dynamic-semaphore.js";
import { MessageGroupHandler } from "./message-group-handler.js";

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

	private state: PoolState = "STARTING";
	private readonly messageGroups = new Map<string, MessageGroupHandler>();

	// Concurrency control (in-place adjustable, unlike p-limit)
	private readonly concurrencyLimiter: DynamicSemaphore;
	// Rate limiting — RateLimiterQueue wraps a drip-rate limiter to smooth
	// bursts into evenly-spaced requests (leaky bucket behaviour).
	private rateLimiterQueue: RateLimiterQueue | null;

	// Statistics tracking
	private totalProcessed = 0;
	private totalSucceeded = 0;
	private totalFailed = 0;
	private totalTransient = 0;
	private totalRateLimited = 0;
	private totalDeferred = 0;
	private processingTimes: number[] = [];

	// Windowed stats (simplified - use sliding window in production)
	private stats5min = {
		processed: 0,
		succeeded: 0,
		failed: 0,
		transient: 0,
		rateLimited: 0,
	};
	private stats30min = {
		processed: 0,
		succeeded: 0,
		failed: 0,
		transient: 0,
		rateLimited: 0,
	};

	// Batch+group failure tracking for FIFO
	private readonly failedBatchGroups = new Set<string>();
	private readonly batchGroupMessageCount = new Map<string, number>();

	// Capacity management
	private queuedMessages = 0;
	private readonly maxCapacity: number;

	constructor(config: PoolConfig, mediator: HttpMediator, logger: Logger) {
		this.config = config;
		this.mediator = mediator;
		this.logger = logger.child({
			component: "ProcessPool",
			poolCode: config.code,
		});

		// Capacity = max(concurrency * 20, 50) — matches Java QUEUE_CAPACITY_MULTIPLIER
		this.maxCapacity = Math.max(config.concurrency * 20, 50);

		// Initialize concurrency limiter
		this.concurrencyLimiter = new DynamicSemaphore(config.concurrency);

		// Initialize rate limiter
		this.rateLimiterQueue = buildRateLimiterQueue(
			config.rateLimitPerMinute,
			this.maxCapacity,
		);

		this.state = "RUNNING";
		this.logger.info(
			{ concurrency: config.concurrency, capacity: this.maxCapacity },
			"Pool started",
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
		if (this.state !== "RUNNING") {
			this.logger.warn({ state: this.state }, "Pool not accepting messages");
			return false;
		}

		// Check capacity (Backpressure)
		if (this.queuedMessages >= this.maxCapacity) {
			this.logger.warn(
				{ queued: this.queuedMessages, capacity: this.maxCapacity },
				"Pool at capacity",
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
						"Message group handler cleaned up",
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
				"Skipping message due to batch+group failure",
			);
			await callback.nack(10); // 10s fast-fail visibility, matching Java
			this.decrementBatchGroupCount(batchGroupKey);
			this.queuedMessages--;
			return;
		}

		// Step 1: Rate Limiting (Wait logic)
		// RateLimiterQueue.removeTokens() resolves when the token is available,
		// smoothing bursts into evenly-spaced requests (leaky bucket).
		if (this.rateLimiterQueue) {
			try {
				await this.rateLimiterQueue.removeTokens(1);
			} catch {
				// Queue is full (maxQueueSize exceeded) — count as rate-limited
				this.totalRateLimited++;
				this.stats5min.rateLimited++;
				this.stats30min.rateLimited++;
				await callback.nack(10);
				this.decrementBatchGroupCount(batchGroupKey);
				this.queuedMessages--;
				return;
			}
		}

		// Step 2: Concurrency Control (Work logic)
		await this.concurrencyLimiter.run(async () => {
			try {
				const startTime = Date.now();
				const result = await this.mediator.process(message);
				const durationMs = Date.now() - startTime;

				this.recordProcessingTime(durationMs);
				this.totalProcessed++;
				this.stats5min.processed++;
				this.stats30min.processed++;

				switch (result.outcome) {
					case "SUCCESS":
						this.totalSucceeded++;
						this.stats5min.succeeded++;
						this.stats30min.succeeded++;
						await callback.ack();
						this.decrementBatchGroupCount(batchGroupKey);
						break;

					case "ERROR_CONFIG":
						// 4xx errors - ack to prevent infinite retry
						this.totalFailed++;
						this.stats5min.failed++;
						this.stats30min.failed++;
						await callback.ack();
						this.decrementBatchGroupCount(batchGroupKey);
						break;

					case "DEFERRED":
						// Message deferred (ack=false) - nack with visibility, mark batch+group failed (FIFO)
						this.totalDeferred++;
						this.failedBatchGroups.add(batchGroupKey);
						await callback.nack(result.delaySeconds || 30);
						this.decrementBatchGroupCount(batchGroupKey);
						break;

					case "ERROR_PROCESS":
					case "BATCH_FAILED":
						// 5xx or timeout — transient, NOT counted as failure (matches Java)
						this.totalTransient++;
						this.stats5min.transient++;
						this.stats30min.transient++;
						this.failedBatchGroups.add(batchGroupKey);
						await callback.nack(result.delaySeconds ?? 30);
						this.decrementBatchGroupCount(batchGroupKey);
						break;

					case "ERROR_CONNECTION":
					default:
						// Connection/network errors — counted as failure (matches Java)
						this.totalFailed++;
						this.stats5min.failed++;
						this.stats30min.failed++;
						this.failedBatchGroups.add(batchGroupKey);
						await callback.nack(result.delaySeconds ?? 30);
						this.decrementBatchGroupCount(batchGroupKey);
						break;
				}
			} catch (error) {
				this.logger.error(
					{ err: error, messageId: message.messageId },
					"Processing error",
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

		// Success rate excludes transient errors (matches Java)
		const completed = this.totalSucceeded + this.totalFailed;
		const successRate = completed > 0 ? this.totalSucceeded / completed : 1.0;

		const completed5min = this.stats5min.succeeded + this.stats5min.failed;
		const successRate5min =
			completed5min > 0 ? this.stats5min.succeeded / completed5min : 1.0;

		const completed30min =
			this.stats30min.succeeded + this.stats30min.failed;
		const successRate30min =
			completed30min > 0
				? this.stats30min.succeeded / completed30min
				: 1.0;

		return {
			poolCode: this.config.code,
			totalProcessed: this.totalProcessed,
			totalSucceeded: this.totalSucceeded,
			totalFailed: this.totalFailed,
			totalTransient: this.totalTransient,
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
			totalTransient5min: this.stats5min.transient,
			successRate5min,
			totalProcessed30min: this.stats30min.processed,
			totalSucceeded30min: this.stats30min.succeeded,
			totalFailed30min: this.stats30min.failed,
			totalTransient30min: this.stats30min.transient,
			successRate30min,
			totalRateLimited5min: this.stats5min.rateLimited,
			totalRateLimited30min: this.stats30min.rateLimited,
		};
	}

	/**
	 * Update pool configuration in-place
	 */
	updateConfig(newConfig: Partial<PoolConfig>): void {
		if (newConfig.rateLimitPerMinute !== undefined) {
			this.rateLimiterQueue = buildRateLimiterQueue(
				newConfig.rateLimitPerMinute,
				this.maxCapacity,
			);
			if (this.rateLimiterQueue) {
				this.logger.info(
					{ rateLimitPerMinute: newConfig.rateLimitPerMinute },
					"Rate limit updated",
				);
			} else {
				this.logger.info("Rate limit disabled");
			}
		}

		if (newConfig.concurrency !== undefined) {
			this.concurrencyLimiter.setLimit(newConfig.concurrency);
			this.logger.info(
				{ concurrency: newConfig.concurrency },
				"Concurrency updated in-place",
			);
		}
	}

	/**
	 * Start draining - stop accepting new messages
	 */
	drain(): void {
		this.state = "DRAINING";
		this.logger.info("Pool draining started");
	}

	/**
	 * Check if pool is fully drained
	 */
	isDrained(): boolean {
		return (
			this.state === "DRAINING" &&
			this.queuedMessages === 0 &&
			this.concurrencyLimiter.activeCount === 0
		);
	}

	/**
	 * Shutdown the pool
	 */
	async shutdown(): Promise<void> {
		this.state = "STOPPED";
		this.messageGroups.clear();
		this.failedBatchGroups.clear();
		this.batchGroupMessageCount.clear();
		this.logger.info("Pool shutdown complete");
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

/**
 * Build a RateLimiterQueue that smooths bursts into evenly-spaced requests
 * (leaky bucket). Returns null if rate limiting is disabled.
 *
 * The inner RateLimiterMemory defines the drip rate: 1 point every
 * (60 / ratePerMinute) seconds. RateLimiterQueue queues callers and
 * releases them one-by-one as tokens become available.
 */
function buildRateLimiterQueue(
	ratePerMinute: number | null | undefined,
	maxQueueSize: number,
): RateLimiterQueue | null {
	if (!ratePerMinute || ratePerMinute <= 0) return null;

	const limiter = new RateLimiterMemory({
		points: 1,
		duration: 60 / ratePerMinute, // seconds per token
	});

	return new RateLimiterQueue(limiter, { maxQueueSize });
}
