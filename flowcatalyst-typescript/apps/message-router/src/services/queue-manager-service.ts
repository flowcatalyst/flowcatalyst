import type { Logger } from '@flowcatalyst/logging';
import type {
	ConsumerHealthResponse,
	InFlightMessage,
	LocalConfigResponse,
	MessageBatch,
	MessagePointer,
	PoolConfig,
	PoolStats,
	QueueMessage,
	QueueStats,
} from '@flowcatalyst/shared-types';
import {
	CircuitBreakerManager,
	HttpMediator,
	type HttpMediatorConfig,
	ProcessPool,
	type MessageCallback,
} from '@flowcatalyst/queue-core';
import type { WarningService } from './warning-service.js';
import type { QueueValidationService } from './queue-validation-service.js';
import {
	PlatformConfigClient,
	ConfigSyncService,
	type MessageRouterConfig,
} from '../clients/platform-config-client.js';
import { SqsConsumer, type SqsConsumerConfig, type SqsMessageCallback } from '../consumers/sqs-consumer.js';
import {
	ActiveMqConsumer,
	type ActiveMqConsumerConfig,
	type ActiveMqBatch,
	type ActiveMqMessageCallback,
} from '../consumers/activemq-consumer.js';
import {
	NatsConsumer,
	type NatsConsumerConfig,
	type NatsBatch,
	type NatsMessageCallback,
} from '../consumers/nats-consumer.js';
import {
	EmbeddedQueue,
	type EmbeddedBatch,
	type EmbeddedMessageCallback,
} from '../embedded/index.js';
import type { TrafficManager } from '../traffic/index.js';
import { env } from '../env.js';

/**
 * In-flight message tracking
 */
interface InFlightMessageInfo {
	messageId: string;
	brokerMessageId: string;
	queueId: string;
	poolCode: string;
	addedAt: number;
}

/**
 * Queue manager service - orchestrates consumers, pools, and mediation
 */
export class QueueManagerService {
	private readonly circuitBreakers: CircuitBreakerManager;
	private readonly warnings: WarningService;
	private readonly traffic: TrafficManager;
	private readonly queueValidation: QueueValidationService;
	private readonly logger: Logger;

	private running = false;
	private readonly startTime: number;

	// HTTP Mediation
	private readonly httpMediator: HttpMediator;

	// Configuration
	private currentConfig: MessageRouterConfig | null = null;
	private configSyncService: ConfigSyncService | null = null;

	// Consumers
	private readonly consumers = new Map<string, SqsConsumer>();
	private readonly activeMqConsumers = new Map<string, ActiveMqConsumer>();
	private readonly natsConsumers = new Map<string, NatsConsumer>();

	// Embedded queue (for EMBEDDED mode)
	private embeddedQueue: EmbeddedQueue | null = null;

	// Process pools
	private readonly processPools = new Map<string, ProcessPool>();
	private readonly drainingPools = new Map<string, ProcessPool>();
	private readonly drainingConsumers = new Map<string, SqsConsumer>();

	// Pool limits (matching Java)
	private readonly maxPools = env.MAX_POOLS;
	private readonly poolWarningThreshold = Math.floor(env.MAX_POOLS * 0.5); // 50% threshold

	// Sync lock to prevent concurrent config syncs
	private syncLock: Promise<void> | null = null;

	// Cleanup interval
	private cleanupInterval: ReturnType<typeof setInterval> | null = null;

	// Queue and pool statistics
	private readonly queueStats = new Map<string, QueueStats>();

	// In-flight message tracking
	private readonly inFlightMessages = new Map<string, InFlightMessageInfo>();
	private readonly messageCallbacks = new Map<string, SqsMessageCallback>();
	private readonly appMessageIdToPipelineKey = new Map<string, string>();

	constructor(
		circuitBreakers: CircuitBreakerManager,
		warnings: WarningService,
		traffic: TrafficManager,
		queueValidation: QueueValidationService,
		logger: Logger,
	) {
		this.circuitBreakers = circuitBreakers;
		this.warnings = warnings;
		this.traffic = traffic;
		this.queueValidation = queueValidation;
		this.logger = logger.child({ component: 'QueueManager' });
		this.startTime = Date.now();

		// Create HTTP mediator with configuration from env
		const mediatorConfig: HttpMediatorConfig = {
			callbackUrl: '', // Will be overridden per-message
			useHttp2: env.MEDIATION_HTTP2,
			connectTimeoutMs: env.MEDIATION_CONNECT_TIMEOUT_MS,
			headersTimeoutMs: env.MEDIATION_HEADERS_TIMEOUT_MS,
			bodyTimeoutMs: env.MEDIATION_REQUEST_TIMEOUT_MS,
			retries: env.MEDIATION_RETRIES,
			retryDelayMs: env.MEDIATION_RETRY_DELAY_MS,
		};

		this.httpMediator = new HttpMediator(mediatorConfig, circuitBreakers, logger);
	}

	/**
	 * Start the queue manager
	 */
	async start(): Promise<void> {
		this.logger.info({ queueType: env.QUEUE_TYPE }, 'Starting queue manager');

		// Register mode change listener for standby support
		this.traffic.addModeChangeListener((newMode, previousMode) => {
			this.handleModeChange(newMode, previousMode);
		});

		// Start traffic manager (handles ALB registration)
		const trafficResult = await this.traffic.start();
		trafficResult.match(
			() => this.logger.info('Traffic manager started'),
			(error) => {
				this.logger.error({ err: error }, 'Failed to start traffic manager');
				this.warnings.add(
					'CONFIGURATION',
					'WARNING',
					`Traffic manager failed to start: ${error.type}`,
					'QueueManager',
				);
			},
		);

		if (env.QUEUE_TYPE === 'EMBEDDED') {
			// Use embedded mode with SQLite-backed queue
			await this.initializeEmbeddedMode();
			this.startCleanupTask();
			this.running = true;
			this.logger.info('Queue manager started in embedded mode');
			return;
		}

		if (env.QUEUE_TYPE === 'ACTIVEMQ') {
			// Use ActiveMQ mode
			await this.initializeActiveMqMode();
			this.startCleanupTask();
			this.running = true;
			this.logger.info('Queue manager started in ActiveMQ mode');
			return;
		}

		if (env.QUEUE_TYPE === 'NATS') {
			// Use NATS JetStream mode
			await this.initializeNatsMode();
			this.startCleanupTask();
			this.running = true;
			this.logger.info('Queue manager started in NATS mode');
			return;
		}

		// SQS mode - fetch config from platform
		if (env.PLATFORM_URL) {
			const configClient = new PlatformConfigClient(
				{
					baseUrl: env.PLATFORM_URL,
					apiKey: env.PLATFORM_API_KEY,
				},
				this.logger,
			);

			this.configSyncService = new ConfigSyncService(
				configClient,
				env.SYNC_INTERVAL_MS,
				async (config) => this.applyConfiguration(config),
				this.logger,
			);

			const success = await this.configSyncService.start();
			if (!success) {
				this.warnings.add(
					'CONFIGURATION',
					'CRITICAL',
					'Failed to fetch initial configuration from platform',
					'QueueManager',
				);
				// Continue with embedded mode
				await this.initializeEmbeddedMode();
			}
		} else {
			// No platform URL - use embedded mode
			this.logger.warn('No PLATFORM_URL configured, using embedded mode');
			await this.initializeEmbeddedMode();
		}

		// Start cleanup task (matches Java scheduled tasks)
		this.startCleanupTask();

		this.running = true;
		this.logger.info('Queue manager started');
	}

	/**
	 * Start scheduled cleanup task for draining pools and consumers
	 * Matches Java QueueManager.cleanupDrainingResources()
	 */
	private startCleanupTask(): void {
		// Run every 10 seconds (matches Java)
		this.cleanupInterval = setInterval(() => {
			this.cleanupDrainingResources();
		}, 10_000);
		this.logger.debug('Cleanup task started (10s interval)');
	}

	/**
	 * Cleanup drained pools and consumers
	 * Called periodically to remove fully drained resources
	 */
	private cleanupDrainingResources(): void {
		if (!this.running) return;

		// Cleanup fully drained pools
		for (const [code, pool] of this.drainingPools) {
			if (pool.isDrained()) {
				this.logger.info({ poolCode: code }, 'Draining pool fully drained, shutting down');
				pool.shutdown().catch((err) => {
					this.logger.error({ err, poolCode: code }, 'Error shutting down drained pool');
				});
				this.drainingPools.delete(code);
			} else {
				const stats = pool.getStats();
				this.logger.debug(
					{
						poolCode: code,
						queueSize: stats.queueSize,
						activeWorkers: stats.activeWorkers,
					},
					'Pool still draining',
				);
			}
		}

		// Cleanup fully stopped consumers
		for (const [queueUri, consumer] of this.drainingConsumers) {
			if (consumer.isFullyStopped()) {
				this.logger.info({ queueUri }, 'Draining consumer fully stopped');
				this.drainingConsumers.delete(queueUri);
			} else {
				this.logger.debug({ queueUri }, 'Consumer still draining');
			}
		}

		// Log cleanup state periodically
		if (this.drainingPools.size > 0 || this.drainingConsumers.size > 0) {
			this.logger.debug(
				{
					drainingPools: this.drainingPools.size,
					drainingConsumers: this.drainingConsumers.size,
				},
				'Resources still draining',
			);
		}
	}

	/**
	 * Stop the queue manager
	 */
	async stop(): Promise<void> {
		this.logger.info('Stopping queue manager');
		this.running = false;

		// Stop cleanup interval
		if (this.cleanupInterval) {
			clearInterval(this.cleanupInterval);
			this.cleanupInterval = null;
		}

		// Stop traffic manager (handles ALB deregistration)
		const trafficResult = await this.traffic.stop();
		trafficResult.match(
			() => this.logger.info('Traffic manager stopped'),
			(error) => this.logger.error({ err: error }, 'Error stopping traffic manager'),
		);

		// Stop config sync
		if (this.configSyncService) {
			await this.configSyncService.stop();
		}

		// Stop embedded queue
		if (this.embeddedQueue) {
			await this.embeddedQueue.close();
			this.embeddedQueue = null;
		}

		// Stop all SQS consumers
		const stopConsumerPromises = Array.from(this.consumers.values()).map((c) => c.stop());
		await Promise.allSettled(stopConsumerPromises);
		this.consumers.clear();

		// Stop all ActiveMQ consumers
		const stopActiveMqPromises = Array.from(this.activeMqConsumers.values()).map((c) => c.stop());
		await Promise.allSettled(stopActiveMqPromises);
		this.activeMqConsumers.clear();

		// Stop all NATS consumers
		const stopNatsPromises = Array.from(this.natsConsumers.values()).map((c) => c.stop());
		await Promise.allSettled(stopNatsPromises);
		this.natsConsumers.clear();

		// Drain and shutdown pools
		for (const pool of this.processPools.values()) {
			pool.drain();
		}
		// Wait for pools to drain (up to 30 seconds)
		const drainStart = Date.now();
		while (Date.now() - drainStart < 30_000) {
			let allDrained = true;
			for (const pool of this.processPools.values()) {
				if (!pool.isDrained()) {
					allDrained = false;
					break;
				}
			}
			if (allDrained) break;
			await sleep(100);
		}
		// Shutdown pools
		const shutdownPoolPromises = Array.from(this.processPools.values()).map((p) => p.shutdown());
		await Promise.allSettled(shutdownPoolPromises);
		this.processPools.clear();

		// NACK all in-flight messages
		for (const [key, info] of this.inFlightMessages) {
			const callback = this.messageCallbacks.get(key);
			if (callback) {
				try {
					await callback.nack();
				} catch (error) {
					this.logger.error({ err: error, messageId: info.messageId }, 'Error NACKing message during shutdown');
				}
			}
		}

		// Clear tracking maps
		this.inFlightMessages.clear();
		this.messageCallbacks.clear();
		this.appMessageIdToPipelineKey.clear();

		// Close HTTP mediator
		await this.httpMediator.close();

		this.logger.info('Queue manager stopped');
	}

	/**
	 * Check if running
	 */
	isRunning(): boolean {
		return this.running;
	}

	/**
	 * Handle traffic mode changes (PRIMARY/STANDBY transitions)
	 * Pauses consumers on STANDBY, resumes on PRIMARY
	 */
	private handleModeChange(newMode: 'PRIMARY' | 'STANDBY', previousMode: 'PRIMARY' | 'STANDBY'): void {
		this.logger.info({ newMode, previousMode }, 'Traffic mode changed');

		if (newMode === 'STANDBY') {
			// Pause all consumers - they will stop polling but can be resumed
			this.pauseAllConsumers();
		} else if (newMode === 'PRIMARY' && previousMode === 'STANDBY') {
			// Resume all consumers
			this.resumeAllConsumers();
		}
	}

	/**
	 * Pause all consumers (stop polling without full shutdown)
	 */
	private pauseAllConsumers(): void {
		this.logger.info('Pausing all consumers for standby mode');

		// Stop SQS consumers
		for (const consumer of this.consumers.values()) {
			consumer.stop().catch((err) => {
				this.logger.error({ err }, 'Error stopping SQS consumer');
			});
		}

		// Stop ActiveMQ consumers
		for (const consumer of this.activeMqConsumers.values()) {
			consumer.stop().catch((err) => {
				this.logger.error({ err }, 'Error stopping ActiveMQ consumer');
			});
		}

		// Stop NATS consumers
		for (const consumer of this.natsConsumers.values()) {
			consumer.stop().catch((err) => {
				this.logger.error({ err }, 'Error stopping NATS consumer');
			});
		}
	}

	/**
	 * Resume all consumers (restart polling)
	 */
	private resumeAllConsumers(): void {
		this.logger.info('Resuming all consumers from standby mode');

		// Restart SQS consumers
		for (const consumer of this.consumers.values()) {
			consumer.start().catch((err) => {
				this.logger.error({ err }, 'Error starting SQS consumer');
			});
		}

		// Restart ActiveMQ consumers
		for (const consumer of this.activeMqConsumers.values()) {
			consumer.start().catch((err) => {
				this.logger.error({ err }, 'Error starting ActiveMQ consumer');
			});
		}

		// Restart NATS consumers
		for (const consumer of this.natsConsumers.values()) {
			consumer.start().catch((err) => {
				this.logger.error({ err }, 'Error starting NATS consumer');
			});
		}
	}

	/**
	 * Apply new configuration (with sync lock to prevent concurrent syncs)
	 * Matches Java QueueManager.syncConfiguration() behavior
	 */
	private async applyConfiguration(config: MessageRouterConfig): Promise<void> {
		// Acquire sync lock (wait for any in-progress sync to complete)
		if (this.syncLock) {
			this.logger.debug('Waiting for existing sync to complete');
			await this.syncLock;
		}

		// Create lock promise that resolves when we're done
		let releaseLock: () => void;
		this.syncLock = new Promise((resolve) => {
			releaseLock = resolve;
		});

		try {
			await this.doApplyConfiguration(config);
		} finally {
			// Release lock
			releaseLock!();
			this.syncLock = null;
		}
	}

	/**
	 * Internal: Apply configuration (called with lock held)
	 */
	private async doApplyConfiguration(config: MessageRouterConfig): Promise<void> {
		this.logger.info(
			{
				queues: config.queues.length,
				pools: config.processingPools.length,
				connections: config.connections,
			},
			'Applying configuration',
		);

		this.currentConfig = config;

		// Filter queues with valid identifiers, warn about invalid ones
		const validQueueConfigs: Array<{ queueUri: string } | { queueName: string }> = [];
		for (const q of config.queues) {
			const queueUri = q.queueUri?.trim() || null;
			const queueName = q.queueName?.trim() || null;

			if (queueUri) {
				validQueueConfigs.push({ queueUri, ...(queueName && { queueName }) });
			} else if (queueName) {
				validQueueConfigs.push({ queueName });
			} else {
				this.warnings.add(
					'CONFIGURATION',
					'WARNING',
					'Queue configuration missing both queueUri and queueName - skipping validation',
					'QueueManagerService',
				);
				this.logger.warn({ queue: q }, 'Queue configuration missing identifier, skipping');
			}
		}

		// Validate queues (raises warnings for missing queues but doesn't stop)
		const validationResult = await this.queueValidation.validateQueues(validQueueConfigs);
		if (validationResult.failed > 0) {
			this.logger.warn(
				{ validated: validationResult.validated, failed: validationResult.failed },
				'Some queues failed validation (continuing with available queues)',
			);
		}

		// Sync process pools
		await this.syncProcessPools(config.processingPools);

		// Handle queue consumers
		if (env.QUEUE_TYPE === 'SQS') {
			await this.syncSqsConsumers(config);
		}

		// Update queue stats
		for (const queue of config.queues) {
			const queueName = queue.queueName || this.extractQueueName(queue.queueUri);
			if (!this.queueStats.has(queueName)) {
				this.queueStats.set(queueName, this.createEmptyQueueStats(queueName));
			}
		}
	}

	/**
	 * Sync process pools with configuration (matches Java QueueManager.syncConfiguration)
	 */
	private async syncProcessPools(poolConfigs: PoolConfig[]): Promise<void> {
		const newPoolCodes = new Set(poolConfigs.map((p) => p.code));
		const newPoolConfigs = new Map(poolConfigs.map((p) => [p.code, p]));

		// Step 1: Handle pool changes - update in-place or move to draining
		for (const [code, existingPool] of this.processPools) {
			const newConfig = newPoolConfigs.get(code);

			if (!newConfig) {
				// Pool removed from config - drain asynchronously (matches Java)
				this.logger.info(
					{
						poolCode: code,
						queueSize: existingPool.getStats().queueSize,
						activeWorkers: existingPool.getStats().activeWorkers,
					},
					'Pool removed from config - draining asynchronously',
				);
				existingPool.drain();
				this.processPools.delete(code);
				this.drainingPools.set(code, existingPool);
			} else {
				// Pool exists in new config - check for changes
				const stats = existingPool.getStats();
				const concurrencyChanged = newConfig.concurrency !== stats.maxConcurrency;
				const rateLimitChanged = newConfig.rateLimitPerMinute !== (stats.totalRateLimited > 0 ? stats.totalRateLimited : undefined);

				if (concurrencyChanged || rateLimitChanged) {
					this.logger.info(
						{
							poolCode: code,
							oldConcurrency: stats.maxConcurrency,
							newConcurrency: newConfig.concurrency,
							rateLimitChanged,
						},
						'Updating pool configuration in-place',
					);
					existingPool.updateConfig(newConfig);
				}
			}
		}

		// Step 2: Create new pools (with limit checks)
		for (const poolConfig of poolConfigs) {
			if (!this.processPools.has(poolConfig.code)) {
				const currentPoolCount = this.processPools.size;

				// Check pool limit
				if (currentPoolCount >= this.maxPools) {
					this.logger.error(
						{
							poolCode: poolConfig.code,
							currentCount: currentPoolCount,
							maxPools: this.maxPools,
						},
						'Cannot create pool: maximum pool limit reached',
					);
					this.warnings.add(
						'POOL_LIMIT',
						'CRITICAL',
						`Max pool limit reached (${currentPoolCount}/${this.maxPools}) - cannot create pool [${poolConfig.code}]`,
						'QueueManager',
					);
					continue;
				}

				// Warn if approaching limit
				if (currentPoolCount >= this.poolWarningThreshold) {
					this.logger.warn(
						{
							currentCount: currentPoolCount,
							maxPools: this.maxPools,
							threshold: this.poolWarningThreshold,
						},
						'Pool count approaching limit',
					);
					this.warnings.add(
						'POOL_LIMIT',
						'WARNING',
						`Pool count ${currentPoolCount} approaching limit ${this.maxPools}`,
						'QueueManager',
					);
				}

				// Calculate queue capacity (matches Java)
				const queueCapacity = Math.max(poolConfig.concurrency * 2, 50);

				this.logger.info(
					{
						poolCode: poolConfig.code,
						concurrency: poolConfig.concurrency,
						queueCapacity,
						poolNumber: currentPoolCount + 1,
						maxPools: this.maxPools,
					},
					'Creating new process pool',
				);

				const pool = new ProcessPool(poolConfig, this.httpMediator, this.logger);
				this.processPools.set(poolConfig.code, pool);
			}
		}
	}

	/**
	 * Sync SQS consumers with configuration (matches Java QueueManager pattern)
	 */
	private async syncSqsConsumers(config: MessageRouterConfig): Promise<void> {
		const activeQueueUris = new Set(config.queues.map((q) => q.queueUri));

		// Phase out consumers for queues no longer in config (async draining)
		for (const [queueUri, consumer] of this.consumers) {
			if (!activeQueueUris.has(queueUri)) {
				this.logger.info({ queueUri }, 'Phasing out consumer for removed queue');
				// Stop consumer (sets running=false, initiates graceful shutdown)
				consumer.stop();
				// Move to draining for async cleanup
				this.consumers.delete(queueUri);
				this.drainingConsumers.set(queueUri, consumer);
				this.logger.info({ queueUri }, 'Consumer moved to draining state');
			}
		}

		// Start consumers for new queues
		for (const queue of config.queues) {
			if (!this.consumers.has(queue.queueUri)) {
				this.logger.info({ queueUri: queue.queueUri }, 'Starting consumer for new queue');
				const consumer = this.createSqsConsumer(queue.queueUri, queue.queueName || '', queue.connections || config.connections);
				await consumer.start();
				this.consumers.set(queue.queueUri, consumer);
			}
		}
	}

	/**
	 * Create an SQS consumer
	 */
	private createSqsConsumer(queueUrl: string, queueName: string, connections: number): SqsConsumer {
		const config: SqsConsumerConfig = {
			queueUrl,
			queueName: queueName || this.extractQueueName(queueUrl),
			region: env.AWS_REGION,
			waitTimeSeconds: 20,
			maxMessages: 10,
			visibilityTimeout: 30,
			connections,
			metricsPollIntervalMs: 300_000, // 5 minutes
		};

		return new SqsConsumer(
			config,
			async (batch, callbacks) => {
				await this.handleBatch(batch, callbacks);
			},
			this.logger,
			env.INSTANCE_ID,
		);
	}

	/**
	 * Handle a batch of messages from an SQS consumer
	 */
	private async handleBatch(
		batch: MessageBatch,
		callbacks: Map<string, SqsMessageCallback>,
	): Promise<void> {
		const queueName = this.extractQueueName(batch.queueId);

		for (const message of batch.messages) {
			const pipelineKey = message.brokerMessageId;
			const callback = callbacks.get(message.brokerMessageId);

			// Check for physical redelivery (same SQS message ID in pipeline)
			if (this.inFlightMessages.has(pipelineKey)) {
				this.logger.debug(
					{ brokerMessageId: message.brokerMessageId },
					'Physical redelivery detected - updating receipt handle',
				);
				// Update receipt handle and NACK
				if (callback) {
					await callback.nack();
				}
				continue;
			}

			// Check for requeue (different SQS ID, same app message ID)
			const existingPipelineKey = this.appMessageIdToPipelineKey.get(message.messageId);
			if (existingPipelineKey && existingPipelineKey !== pipelineKey) {
				this.logger.debug(
					{ messageId: message.messageId, existingKey: existingPipelineKey, newKey: pipelineKey },
					'Requeue detected - ACKing duplicate',
				);
				if (callback) {
					await callback.ack();
				}
				continue;
			}

			// Track message in pipeline
			this.inFlightMessages.set(pipelineKey, {
				messageId: message.messageId,
				brokerMessageId: message.brokerMessageId,
				queueId: batch.queueId,
				poolCode: message.pointer.poolCode,
				addedAt: Date.now(),
			});
			this.appMessageIdToPipelineKey.set(message.messageId, pipelineKey);

			if (callback) {
				this.messageCallbacks.set(pipelineKey, callback);
			}

			// Update queue stats
			const queueStat = this.queueStats.get(queueName);
			if (queueStat) {
				queueStat.totalMessages++;
				queueStat.totalMessages5min++;
				queueStat.totalMessages30min++;
			}

			// Route to process pool
			const pool = this.processPools.get(message.pointer.poolCode);
			if (!pool) {
				this.logger.warn(
					{ poolCode: message.pointer.poolCode, messageId: message.messageId },
					'No pool found for message - NACKing',
				);
				if (callback) {
					await callback.nack();
				}
				this.cleanupMessage(pipelineKey, message.messageId);
				continue;
			}

			// Create QueueMessage for pool
			const queueMessage: QueueMessage = {
				messageId: message.messageId,
				brokerMessageId: message.brokerMessageId,
				receiptHandle: message.receiptHandle,
				batchId: batch.batchId,
				queueId: batch.queueId,
				receiveCount: message.receiveCount,
				receivedAt: message.receivedAt,
				pointer: {
					messageId: message.messageId,
					poolCode: message.pointer.poolCode,
					messageGroupId: message.pointer.messageGroupId,
					callbackUrl: message.pointer.callbackUrl,
					authToken: message.pointer.authToken,
					payload: message.pointer.payload,
				},
			};

			// Create callback wrapper that cleans up tracking on completion
			const poolCallback: MessageCallback = {
				ack: async () => {
					if (callback) {
						await callback.ack();
					}
					this.cleanupMessage(pipelineKey, message.messageId);
					// Update stats
					if (queueStat) {
						queueStat.totalConsumed++;
						queueStat.totalConsumed5min++;
						queueStat.totalConsumed30min++;
					}
				},
				nack: async (visibilityTimeoutSeconds?: number) => {
					if (callback) {
						await callback.nack(visibilityTimeoutSeconds);
					}
					this.cleanupMessage(pipelineKey, message.messageId);
					// Update stats
					if (queueStat) {
						queueStat.totalFailed++;
						queueStat.totalFailed5min++;
						queueStat.totalFailed30min++;
					}
				},
			};

			// Submit to pool
			const accepted = await pool.submit(queueMessage, poolCallback);
			if (!accepted) {
				this.logger.warn(
					{ poolCode: message.pointer.poolCode, messageId: message.messageId },
					'Pool rejected message (at capacity) - NACKing',
				);
				if (callback) {
					await callback.nack(5); // Short visibility for retry
				}
				this.cleanupMessage(pipelineKey, message.messageId);
			}
		}
	}

	/**
	 * Clean up message tracking
	 */
	private cleanupMessage(pipelineKey: string, messageId: string): void {
		this.inFlightMessages.delete(pipelineKey);
		this.messageCallbacks.delete(pipelineKey);
		this.appMessageIdToPipelineKey.delete(messageId);
	}

	/**
	 * Handle a batch of messages from the embedded queue
	 */
	private async handleEmbeddedBatch(
		batch: EmbeddedBatch,
		callbacks: Map<string, EmbeddedMessageCallback>,
	): Promise<void> {
		for (const message of batch.messages) {
			const pipelineKey = message.messageId;
			const callback = callbacks.get(message.messageId);

			// Parse message payload to get the pointer
			let pointer: MessagePointer;

			try {
				const parsed = message.payload as Record<string, unknown>;
				pointer = {
					messageId: message.messageId,
					poolCode: (parsed['poolCode'] as string) || 'POOL-MEDIUM',
					messageGroupId: message.messageGroupId,
					callbackUrl: parsed['callbackUrl'] as string | undefined,
					authToken: parsed['authToken'] as string | undefined,
					payload: parsed['payload'] ?? parsed,
				};
			} catch {
				pointer = {
					messageId: message.messageId,
					poolCode: 'POOL-MEDIUM',
					messageGroupId: message.messageGroupId,
					payload: message.payload,
				};
			}

			// Check for physical redelivery
			if (this.inFlightMessages.has(pipelineKey)) {
				this.logger.debug(
					{ messageId: message.messageId },
					'Physical redelivery detected - NACKing',
				);
				if (callback) {
					callback.nack();
				}
				continue;
			}

			// Track message in pipeline
			this.inFlightMessages.set(pipelineKey, {
				messageId: message.messageId,
				brokerMessageId: message.messageId,
				queueId: batch.queueId,
				poolCode: pointer.poolCode,
				addedAt: Date.now(),
			});
			this.appMessageIdToPipelineKey.set(message.messageId, pipelineKey);

			// Update queue stats
			const queueStat = this.queueStats.get(batch.queueId);
			if (queueStat) {
				queueStat.totalMessages++;
				queueStat.totalMessages5min++;
				queueStat.totalMessages30min++;
			}

			// Route to process pool
			const pool = this.processPools.get(pointer.poolCode);
			if (!pool) {
				this.logger.warn(
					{ poolCode: pointer.poolCode, messageId: message.messageId },
					'No pool found for message - NACKing',
				);
				const callback = callbacks.get(message.messageId);
				if (callback) {
					callback.nack();
				}
				this.cleanupMessage(pipelineKey, message.messageId);
				continue;
			}

			// Create QueueMessage for pool
			const queueMessage: QueueMessage = {
				messageId: message.messageId,
				brokerMessageId: message.messageId,
				receiptHandle: message.receiptHandle,
				receiveCount: message.receiveCount,
				receivedAt: new Date(),
				batchId: batch.batchId,
				queueId: batch.queueId,
				pointer,
			};

			// Create callback wrapper that cleans up tracking on completion
			const poolCallback: MessageCallback = {
				ack: async () => {
					if (callback) {
						callback.ack();
					}
					this.cleanupMessage(pipelineKey, message.messageId);
					// Update stats
					if (queueStat) {
						queueStat.totalConsumed++;
						queueStat.totalConsumed5min++;
						queueStat.totalConsumed30min++;
					}
				},
				nack: async (visibilityTimeoutSeconds?: number) => {
					if (callback) {
						callback.nack(visibilityTimeoutSeconds);
					}
					this.cleanupMessage(pipelineKey, message.messageId);
					// Update stats
					if (queueStat) {
						queueStat.totalFailed++;
						queueStat.totalFailed5min++;
						queueStat.totalFailed30min++;
					}
				},
			};

			// Submit to pool
			const accepted = await pool.submit(queueMessage, poolCallback);
			if (!accepted) {
				this.logger.warn(
					{ poolCode: pointer.poolCode, messageId: message.messageId },
					'Pool rejected message (at capacity) - NACKing',
				);
				if (callback) {
					callback.nack(5); // Short visibility for retry
				}
				this.cleanupMessage(pipelineKey, message.messageId);
			}
		}
	}

	/**
	 * Initialize embedded mode with SQLite-backed queue
	 */
	private async initializeEmbeddedMode(): Promise<void> {
		const queueName = 'embedded-queue';

		// Create embedded queue
		this.embeddedQueue = new EmbeddedQueue(
			{
				dbPath: env.EMBEDDED_DB_PATH,
				queueName,
				visibilityTimeoutSeconds: env.EMBEDDED_VISIBILITY_TIMEOUT_SECONDS,
				receiveTimeoutMs: env.EMBEDDED_RECEIVE_TIMEOUT_MS,
				maxMessages: env.EMBEDDED_MAX_MESSAGES,
				metricsPollIntervalMs: env.EMBEDDED_METRICS_POLL_INTERVAL_MS,
			},
			this.logger,
			env.INSTANCE_ID,
		);

		// Initialize the queue
		await this.embeddedQueue.initialize();

		// Add queue stats
		this.queueStats.set(queueName, this.createEmptyQueueStats(queueName));

		// Create default pools
		const poolConfigs: PoolConfig[] = [
			{ code: 'POOL-HIGH', concurrency: 10, rateLimitPerMinute: null },
			{ code: 'POOL-MEDIUM', concurrency: 10, rateLimitPerMinute: null },
			{ code: 'POOL-LOW', concurrency: 10, rateLimitPerMinute: null },
		];

		for (const config of poolConfigs) {
			const pool = new ProcessPool(config, this.httpMediator, this.logger);
			this.processPools.set(config.code, pool);
		}

		// Start the consumer
		await this.embeddedQueue.startConsumer(
			async (batch, callbacks) => this.handleEmbeddedBatch(batch, callbacks),
		);

		this.logger.info(
			{ dbPath: env.EMBEDDED_DB_PATH, queueName },
			'Embedded queue initialized',
		);
	}

	/**
	 * Initialize ActiveMQ mode
	 */
	private async initializeActiveMqMode(): Promise<void> {
		// Default queue names for ActiveMQ mode
		const queueNames = [
			'flow-catalyst-high-priority',
			'flow-catalyst-medium-priority',
			'flow-catalyst-low-priority',
		];

		// Create default pools
		const poolConfigs: PoolConfig[] = [
			{ code: 'POOL-HIGH', concurrency: 10, rateLimitPerMinute: null },
			{ code: 'POOL-MEDIUM', concurrency: 10, rateLimitPerMinute: null },
			{ code: 'POOL-LOW', concurrency: 10, rateLimitPerMinute: null },
		];

		for (const config of poolConfigs) {
			const pool = new ProcessPool(config, this.httpMediator, this.logger);
			this.processPools.set(config.code, pool);
		}

		// Create consumers for each queue
		for (const queueName of queueNames) {
			const consumerConfig: ActiveMqConsumerConfig = {
				host: env.ACTIVEMQ_HOST,
				port: env.ACTIVEMQ_PORT,
				username: env.ACTIVEMQ_USERNAME,
				password: env.ACTIVEMQ_PASSWORD,
				queueName,
				connections: env.DEFAULT_CONNECTIONS,
				receiveTimeoutMs: env.ACTIVEMQ_RECEIVE_TIMEOUT_MS,
				metricsPollIntervalMs: env.SYNC_INTERVAL_MS,
				prefetchCount: env.ACTIVEMQ_PREFETCH_COUNT,
				redeliveryDelayMs: env.ACTIVEMQ_REDELIVERY_DELAY_MS,
			};

			const consumer = new ActiveMqConsumer(
				consumerConfig,
				async (batch, callbacks) => this.handleActiveMqBatch(batch, callbacks),
				this.logger,
				env.INSTANCE_ID,
			);

			await consumer.start();
			this.activeMqConsumers.set(queueName, consumer);
			this.queueStats.set(queueName, this.createEmptyQueueStats(queueName));
		}

		this.logger.info(
			{
				host: env.ACTIVEMQ_HOST,
				port: env.ACTIVEMQ_PORT,
				queues: queueNames,
			},
			'ActiveMQ mode initialized',
		);
	}

	/**
	 * Handle a batch of messages from ActiveMQ
	 */
	private async handleActiveMqBatch(
		batch: ActiveMqBatch,
		callbacks: Map<string, ActiveMqMessageCallback>,
	): Promise<void> {
		for (const message of batch.messages) {
			const pipelineKey = message.brokerMessageId;
			const callback = callbacks.get(message.brokerMessageId);

			// Parse message body to get the pointer
			let pointer: MessagePointer;

			try {
				const parsed = JSON.parse(message.body) as Record<string, unknown>;
				pointer = {
					messageId: message.messageId,
					poolCode: (parsed['poolCode'] as string) || 'POOL-MEDIUM',
					messageGroupId: (parsed['messageGroupId'] as string) || message.messageId,
					callbackUrl: parsed['callbackUrl'] as string | undefined,
					authToken: parsed['authToken'] as string | undefined,
					payload: parsed['payload'] ?? parsed,
				};
			} catch {
				// If not JSON, treat the entire body as payload
				pointer = {
					messageId: message.messageId,
					poolCode: 'POOL-MEDIUM',
					messageGroupId: message.messageId,
					payload: message.body,
				};
			}

			// Check for physical redelivery
			if (this.inFlightMessages.has(pipelineKey)) {
				this.logger.debug(
					{ messageId: message.messageId },
					'Physical redelivery detected - NACKing',
				);
				if (callback) {
					await callback.nack();
				}
				continue;
			}

			// Track message in pipeline
			this.inFlightMessages.set(pipelineKey, {
				messageId: message.messageId,
				brokerMessageId: message.brokerMessageId,
				queueId: batch.queueId,
				poolCode: pointer.poolCode,
				addedAt: Date.now(),
			});
			this.appMessageIdToPipelineKey.set(message.messageId, pipelineKey);

			// Update queue stats
			const queueStat = this.queueStats.get(batch.queueId);
			if (queueStat) {
				queueStat.totalMessages++;
				queueStat.totalMessages5min++;
				queueStat.totalMessages30min++;
			}

			// Route to process pool
			const pool = this.processPools.get(pointer.poolCode);
			if (!pool) {
				this.logger.warn(
					{ poolCode: pointer.poolCode, messageId: message.messageId },
					'No pool found for message - NACKing',
				);
				if (callback) {
					await callback.nack();
				}
				this.cleanupMessage(pipelineKey, message.messageId);
				continue;
			}

			// Create QueueMessage for pool
			const queueMessage: QueueMessage = {
				messageId: message.messageId,
				brokerMessageId: message.brokerMessageId,
				receiptHandle: message.brokerMessageId, // ActiveMQ uses brokerMessageId for ack/nack
				receiveCount: message.receiveCount,
				receivedAt: new Date(),
				batchId: batch.batchId,
				queueId: batch.queueId,
				pointer,
			};

			// Create callback wrapper that cleans up tracking on completion
			const poolCallback: MessageCallback = {
				ack: async () => {
					if (callback) {
						await callback.ack();
					}
					this.cleanupMessage(pipelineKey, message.messageId);
					// Update stats
					if (queueStat) {
						queueStat.totalConsumed++;
						queueStat.totalConsumed5min++;
						queueStat.totalConsumed30min++;
					}
				},
				nack: async (visibilityTimeoutSeconds?: number) => {
					if (callback) {
						await callback.nack(visibilityTimeoutSeconds);
					}
					this.cleanupMessage(pipelineKey, message.messageId);
					// Update stats
					if (queueStat) {
						queueStat.totalFailed++;
						queueStat.totalFailed5min++;
						queueStat.totalFailed30min++;
					}
				},
			};

			// Submit to pool
			const accepted = await pool.submit(queueMessage, poolCallback);
			if (!accepted) {
				this.logger.warn(
					{ poolCode: pointer.poolCode, messageId: message.messageId },
					'Pool rejected message (at capacity) - NACKing',
				);
				if (callback) {
					await callback.nack(10); // Short visibility for retry (fast-fail)
				}
				this.cleanupMessage(pipelineKey, message.messageId);
			}
		}
	}

	/**
	 * Initialize NATS JetStream mode
	 */
	private async initializeNatsMode(): Promise<void> {
		// Create default pools
		const poolConfigs: PoolConfig[] = [
			{ code: 'POOL-HIGH', concurrency: 10, rateLimitPerMinute: null },
			{ code: 'POOL-MEDIUM', concurrency: 10, rateLimitPerMinute: null },
			{ code: 'POOL-LOW', concurrency: 10, rateLimitPerMinute: null },
		];

		for (const config of poolConfigs) {
			const pool = new ProcessPool(config, this.httpMediator, this.logger);
			this.processPools.set(config.code, pool);
		}

		// Create NATS consumer
		const consumerConfig: NatsConsumerConfig = {
			servers: env.NATS_SERVERS,
			connectionName: env.NATS_CONNECTION_NAME,
			username: env.NATS_USERNAME,
			password: env.NATS_PASSWORD,
			streamName: env.NATS_STREAM_NAME,
			consumerName: env.NATS_CONSUMER_NAME,
			subject: env.NATS_SUBJECT,
			maxMessagesPerPoll: env.NATS_MAX_MESSAGES_PER_POLL,
			pollTimeoutSeconds: env.NATS_POLL_TIMEOUT_SECONDS,
			ackWaitSeconds: env.NATS_ACK_WAIT_SECONDS,
			maxDeliver: env.NATS_MAX_DELIVER,
			maxAckPending: env.NATS_MAX_ACK_PENDING,
			storageType: env.NATS_STORAGE_TYPE,
			replicas: env.NATS_REPLICAS,
			maxAgeDays: env.NATS_MAX_AGE_DAYS,
			metricsPollIntervalMs: env.SYNC_INTERVAL_MS,
		};

		const consumer = new NatsConsumer(
			consumerConfig,
			async (batch, callbacks) => this.handleNatsBatch(batch, callbacks),
			this.logger,
			env.INSTANCE_ID,
		);

		await consumer.start();
		const queueId = `${env.NATS_STREAM_NAME}:${env.NATS_CONSUMER_NAME}`;
		this.natsConsumers.set(queueId, consumer);
		this.queueStats.set(queueId, this.createEmptyQueueStats(queueId));

		this.logger.info(
			{
				servers: env.NATS_SERVERS,
				stream: env.NATS_STREAM_NAME,
				consumer: env.NATS_CONSUMER_NAME,
			},
			'NATS mode initialized',
		);
	}

	/**
	 * Handle a batch of messages from NATS JetStream
	 */
	private async handleNatsBatch(
		batch: NatsBatch,
		callbacks: Map<string, NatsMessageCallback>,
	): Promise<void> {
		for (const message of batch.messages) {
			const pipelineKey = message.brokerMessageId;
			const callback = callbacks.get(message.brokerMessageId);

			// Parse message data to get the pointer
			let pointer: MessagePointer;

			try {
				const parsed = JSON.parse(message.data) as Record<string, unknown>;
				pointer = {
					messageId: message.messageId,
					poolCode: (parsed['poolCode'] as string) || 'POOL-MEDIUM',
					messageGroupId: (parsed['messageGroupId'] as string) || message.messageId,
					callbackUrl: parsed['callbackUrl'] as string | undefined,
					authToken: parsed['authToken'] as string | undefined,
					payload: parsed['payload'] ?? parsed,
				};
			} catch {
				// If not JSON, treat the entire data as payload
				pointer = {
					messageId: message.messageId,
					poolCode: 'POOL-MEDIUM',
					messageGroupId: message.messageId,
					payload: message.data,
				};
			}

			// Check for physical redelivery
			if (this.inFlightMessages.has(pipelineKey)) {
				this.logger.debug(
					{ messageId: message.messageId },
					'Physical redelivery detected - NACKing',
				);
				if (callback) {
					await callback.nack(10); // Fast-fail
				}
				continue;
			}

			// Track message in pipeline
			this.inFlightMessages.set(pipelineKey, {
				messageId: message.messageId,
				brokerMessageId: message.brokerMessageId,
				queueId: batch.queueId,
				poolCode: pointer.poolCode,
				addedAt: Date.now(),
			});
			this.appMessageIdToPipelineKey.set(message.messageId, pipelineKey);

			// Update queue stats
			const queueStat = this.queueStats.get(batch.queueId);
			if (queueStat) {
				queueStat.totalMessages++;
				queueStat.totalMessages5min++;
				queueStat.totalMessages30min++;
			}

			// Route to process pool
			const pool = this.processPools.get(pointer.poolCode);
			if (!pool) {
				this.logger.warn(
					{ poolCode: pointer.poolCode, messageId: message.messageId },
					'No pool found for message - NACKing',
				);
				if (callback) {
					await callback.nack();
				}
				this.cleanupMessage(pipelineKey, message.messageId);
				continue;
			}

			// Create QueueMessage for pool
			const queueMessage: QueueMessage = {
				messageId: message.messageId,
				brokerMessageId: message.brokerMessageId,
				receiptHandle: message.brokerMessageId, // NATS uses seq for ack/nack
				receiveCount: message.redeliveryCount + 1, // redeliveryCount starts at 0
				receivedAt: new Date(),
				batchId: batch.batchId,
				queueId: batch.queueId,
				pointer,
			};

			// Mark message as in-progress for long-running operations
			if (callback) {
				callback.inProgress();
			}

			// Create callback wrapper that cleans up tracking on completion
			const poolCallback: MessageCallback = {
				ack: async () => {
					if (callback) {
						await callback.ack();
					}
					this.cleanupMessage(pipelineKey, message.messageId);
					// Update stats
					if (queueStat) {
						queueStat.totalConsumed++;
						queueStat.totalConsumed5min++;
						queueStat.totalConsumed30min++;
					}
				},
				nack: async (visibilityTimeoutSeconds?: number) => {
					if (callback) {
						// Use 120s default (matching SQS visibility timeout)
						await callback.nack(visibilityTimeoutSeconds ?? 120);
					}
					this.cleanupMessage(pipelineKey, message.messageId);
					// Update stats
					if (queueStat) {
						queueStat.totalFailed++;
						queueStat.totalFailed5min++;
						queueStat.totalFailed30min++;
					}
				},
			};

			// Submit to pool
			const accepted = await pool.submit(queueMessage, poolCallback);
			if (!accepted) {
				this.logger.warn(
					{ poolCode: pointer.poolCode, messageId: message.messageId },
					'Pool rejected message (at capacity) - NACKing',
				);
				if (callback) {
					await callback.nack(10); // Fast-fail (10s)
				}
				this.cleanupMessage(pipelineKey, message.messageId);
			}
		}
	}

	/**
	 * Get local configuration - matches Java LocalConfigResponse
	 */
	getConfig(): LocalConfigResponse {
		if (this.currentConfig) {
			return {
				queues: this.currentConfig.queues,
				connections: this.currentConfig.connections,
				processingPools: this.currentConfig.processingPools,
			};
		}

		// Return mock config for embedded mode
		return {
			queues: Array.from(this.queueStats.values()).map((q) => ({
				queueUri: q.name,
				queueName: q.name,
				connections: env.DEFAULT_CONNECTIONS,
			})),
			connections: env.DEFAULT_CONNECTIONS,
			processingPools: Array.from(this.processPools.values()).map((p) => ({
				code: p.getCode(),
				concurrency: p.getStats().maxConcurrency,
				rateLimitPerMinute: null,
			})),
		};
	}

	/**
	 * Get queue statistics - matches Java response format
	 */
	getQueueStats(): Record<string, QueueStats> {
		// Update consumer metrics from SQS consumers
		for (const [queueUri, consumer] of this.consumers) {
			const queueName = this.extractQueueName(queueUri);
			const stats = this.queueStats.get(queueName);
			if (stats) {
				const metrics = consumer.getQueueMetrics();
				stats.pendingMessages = metrics.pendingMessages;
				stats.messagesNotVisible = metrics.messagesNotVisible;
				stats.currentSize = metrics.pendingMessages;
			}
		}

		// Update metrics from embedded queue
		if (this.embeddedQueue) {
			const embeddedMetrics = this.embeddedQueue.getConsumerMetrics();
			const embeddedStats = this.embeddedQueue.getStats();
			const queueName = 'embedded-queue';
			const stats = this.queueStats.get(queueName);
			if (stats && embeddedMetrics) {
				stats.pendingMessages = embeddedMetrics.pendingMessages;
				stats.messagesNotVisible = embeddedMetrics.messagesNotVisible;
				stats.currentSize = embeddedStats.visibleMessages;
			}
		}

		// Update metrics from ActiveMQ consumers
		for (const [queueName, consumer] of this.activeMqConsumers) {
			const stats = this.queueStats.get(queueName);
			if (stats) {
				const metrics = consumer.getQueueMetrics();
				stats.pendingMessages = metrics.pendingMessages;
				stats.messagesNotVisible = metrics.messagesNotVisible;
				stats.currentSize = metrics.pendingMessages;
			}
		}

		// Update metrics from NATS consumers
		for (const [queueId, consumer] of this.natsConsumers) {
			const stats = this.queueStats.get(queueId);
			if (stats) {
				const metrics = consumer.getQueueMetrics();
				stats.pendingMessages = metrics.pendingMessages;
				stats.messagesNotVisible = metrics.messagesNotVisible;
				stats.currentSize = metrics.pendingMessages;
			}
		}

		const result: Record<string, QueueStats> = {};
		for (const [name, stats] of this.queueStats) {
			result[name] = { ...stats };
		}
		return result;
	}

	/**
	 * Get pool statistics - matches Java response format
	 */
	getPoolStats(): Record<string, PoolStats> {
		const result: Record<string, PoolStats> = {};
		for (const [code, pool] of this.processPools) {
			result[code] = pool.getStats();
		}
		return result;
	}

	/**
	 * Get in-flight messages
	 */
	getInFlightMessages(limit: number, messageId?: string): InFlightMessage[] {
		let messages = Array.from(this.inFlightMessages.values()).map((info) => ({
			messageId: info.messageId,
			brokerMessageId: info.brokerMessageId,
			queueId: info.queueId,
			addedToInPipelineAt: new Date(info.addedAt).toISOString(),
			elapsedTimeMs: Date.now() - info.addedAt,
			poolCode: info.poolCode,
		}));

		if (messageId) {
			messages = messages.filter(
				(m) => m.messageId.includes(messageId) || m.brokerMessageId.includes(messageId),
			);
		}

		return messages.slice(0, limit);
	}

	/**
	 * Get consumer health - matches Java ConsumerHealthResponse
	 */
	getConsumerHealth(): ConsumerHealthResponse {
		const currentTimeMs = Date.now();
		const consumers: Record<string, ReturnType<SqsConsumer['getHealth']>> = {};

		// SQS consumers
		for (const [queueUri, consumer] of this.consumers) {
			consumers[queueUri] = consumer.getHealth();
		}

		// Embedded queue health
		if (this.embeddedQueue) {
			const health = this.embeddedQueue.getConsumerHealth();
			if (health) {
				consumers['embedded-queue'] = health;
			}
		}

		// ActiveMQ consumer health
		for (const [queueName, consumer] of this.activeMqConsumers) {
			consumers[queueName] = consumer.getHealth();
		}

		// NATS consumer health
		for (const [queueId, consumer] of this.natsConsumers) {
			consumers[queueId] = consumer.getHealth();
		}

		return {
			currentTimeMs,
			currentTime: new Date(currentTimeMs).toISOString(),
			consumers,
		};
	}

	/**
	 * Get circuit breaker statistics
	 */
	getCircuitBreakerStats() {
		return this.circuitBreakers.getAllStats();
	}

	/**
	 * Get HTTP mediator statistics
	 * Note: HttpMediator doesn't track stats currently
	 */
	getMediatorStats() {
		return {};
	}

	/**
	 * Get traffic management statistics
	 */
	getTrafficStats() {
		return this.traffic.getStats();
	}

	/**
	 * Get traffic statistics (standby mode status)
	 */
	getDetailedTrafficStats() {
		return this.traffic.getStats();
	}

	/**
	 * Check if a message is in the pipeline
	 */
	isMessageInPipeline(pipelineKey: string): boolean {
		return this.inFlightMessages.has(pipelineKey);
	}

	/**
	 * Publish a message to the embedded queue (only works in EMBEDDED mode)
	 */
	publishToEmbeddedQueue(message: {
		messageId: string;
		messageGroupId: string;
		messageDeduplicationId?: string;
		payload: unknown;
	}): { success: boolean; error?: string; deduplicated?: boolean } {
		if (!this.embeddedQueue) {
			return { success: false, error: 'Embedded queue not available' };
		}

		return this.embeddedQueue.publish(message);
	}

	/**
	 * Check if embedded queue is available
	 */
	hasEmbeddedQueue(): boolean {
		return this.embeddedQueue !== null;
	}

	/**
	 * Extract queue name from URL
	 */
	private extractQueueName(queueUri: string): string {
		// Handle SQS URL format: https://sqs.region.amazonaws.com/account/queue-name
		const parts = queueUri.split('/');
		return parts[parts.length - 1] || queueUri;
	}

	/**
	 * Create empty queue stats
	 */
	private createEmptyQueueStats(name: string): QueueStats {
		return {
			name,
			totalMessages: 0,
			totalConsumed: 0,
			totalFailed: 0,
			successRate: 1.0,
			currentSize: 0,
			throughput: 0,
			pendingMessages: 0,
			messagesNotVisible: 0,
			totalMessages5min: 0,
			totalConsumed5min: 0,
			totalFailed5min: 0,
			successRate5min: 1.0,
			totalMessages30min: 0,
			totalConsumed30min: 0,
			totalFailed30min: 0,
			successRate30min: 1.0,
			totalDeferred: 0,
		};
	}
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}
