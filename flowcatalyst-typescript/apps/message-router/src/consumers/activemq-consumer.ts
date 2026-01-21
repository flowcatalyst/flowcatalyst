import stompit from 'stompit';
import type { Logger } from '@flowcatalyst/logging';
import { randomUUID } from 'node:crypto';

// Local type declarations for stompit (the @types/stompit package is incomplete)
declare module 'stompit' {
	interface ChannelPoolOptions {
		minChannels?: number;
		maxChannels?: number;
		requestTimeout?: number;
	}
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type StompitSubscription = any;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type StompitSubscribeOptions = Record<string, string>;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type StompitMessage = any;

/**
 * Message received from ActiveMQ
 */
export interface ActiveMqMessage {
	messageId: string;
	brokerMessageId: string;
	body: string;
	headers: Record<string, string>;
	receiveCount: number;
}

/**
 * Message callback for ACK/NACK
 */
export interface ActiveMqMessageCallback {
	ack: () => Promise<void>;
	nack: (visibilityDelaySeconds?: number) => Promise<void>;
}

/**
 * Batch of messages from ActiveMQ
 */
export interface ActiveMqBatch {
	batchId: string;
	messages: ActiveMqMessage[];
	queueId: string;
}

/**
 * Batch handler type
 */
export type ActiveMqBatchHandler = (
	batch: ActiveMqBatch,
	callbacks: Map<string, ActiveMqMessageCallback>,
) => Promise<void>;

/**
 * ActiveMQ consumer configuration
 */
export interface ActiveMqConsumerConfig {
	/** Broker host */
	host: string;
	/** Broker port (STOMP port, typically 61613) */
	port: number;
	/** Username for authentication */
	username: string;
	/** Password for authentication */
	password: string;
	/** Queue name to consume from */
	queueName: string;
	/** Number of concurrent consumers */
	connections: number;
	/** Receive timeout in milliseconds (default: 1000) */
	receiveTimeoutMs: number;
	/** Metrics poll interval in milliseconds (default: 300000) */
	metricsPollIntervalMs: number;
	/** Prefetch count per consumer (default: 1) */
	prefetchCount: number;
	/** Redelivery delay in milliseconds (default: 30000) */
	redeliveryDelayMs: number;
}

/**
 * Default redelivery delay (30 seconds)
 */
const DEFAULT_REDELIVERY_DELAY_MS = 30_000;

/**
 * Fast fail delay (10 seconds)
 */
const FAST_FAIL_DELAY_MS = 10_000;

/**
 * ActiveMQ consumer using STOMP protocol
 * Implements similar semantics to the Java ActiveMqQueueConsumer
 */
export class ActiveMqConsumer {
	private readonly config: ActiveMqConsumerConfig;
	private readonly handler: ActiveMqBatchHandler;
	private readonly logger: Logger;
	private readonly instanceId: string;

	private running = false;
	private lastPollTimeMs = 0;
	private connectionManager: stompit.ConnectFailover | null = null;
	private channelPool: stompit.ChannelPool | null = null;
	private subscriptions: StompitSubscription[] = [];

	private metrics = {
		pendingMessages: 0,
		messagesNotVisible: 0,
	};

	constructor(
		config: ActiveMqConsumerConfig,
		handler: ActiveMqBatchHandler,
		logger: Logger,
		instanceId: string,
	) {
		this.config = config;
		this.handler = handler;
		this.logger = logger.child({
			component: 'ActiveMqConsumer',
			queue: config.queueName,
		});
		this.instanceId = instanceId;
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
			{
				host: this.config.host,
				port: this.config.port,
				queue: this.config.queueName,
				connections: this.config.connections,
			},
			'Starting ActiveMQ consumer',
		);

		// Create connection manager with failover support
		const servers = [
			{
				host: this.config.host,
				port: this.config.port,
				connectHeaders: {
					host: '/',
					login: this.config.username,
					passcode: this.config.password,
					'heart-beat': '10000,10000',
				},
			},
		];

		this.connectionManager = new stompit.ConnectFailover(servers, {
			maxReconnects: -1, // Unlimited reconnects
			initialReconnectDelay: 1000,
			maxReconnectDelay: 30000,
			useExponentialBackOff: true,
			reconnectDelayExponent: 2,
		});

		// Create channel pool for connection reuse
		this.channelPool = new stompit.ChannelPool(this.connectionManager, {
			minChannels: 1,
			maxChannels: this.config.connections,
			requestTimeout: 30000,
		} as stompit.ChannelPoolOptions);

		// Start consumers
		for (let i = 0; i < this.config.connections; i++) {
			this.startConsumer(i);
		}

		// Start metrics polling
		this.startMetricsPolling();

		this.logger.info('ActiveMQ consumer started');
	}

	/**
	 * Stop consuming messages
	 */
	async stop(): Promise<void> {
		this.logger.info('Stopping ActiveMQ consumer');
		this.running = false;

		// Unsubscribe all subscriptions
		for (const subscription of this.subscriptions) {
			try {
				subscription.unsubscribe();
			} catch (error) {
				this.logger.warn({ err: error }, 'Error unsubscribing');
			}
		}
		this.subscriptions = [];

		// Close channel pool
		if (this.channelPool) {
			this.channelPool.close();
			this.channelPool = null;
		}

		// Close connection manager
		if (this.connectionManager) {
			this.connectionManager = null;
		}

		this.logger.info('ActiveMQ consumer stopped');
	}

	/**
	 * Start a single consumer
	 */
	private startConsumer(consumerId: number): void {
		if (!this.channelPool || !this.running) return;

		// @ts-expect-error - stompit types are incomplete, callback has 3 params
		this.channelPool.channel((error: Error | null, channel: stompit.Channel, done: () => void) => {
			if (error) {
				this.logger.error({ err: error, consumerId }, 'Error getting channel');
				// Retry after delay
				if (this.running) {
					setTimeout(() => this.startConsumer(consumerId), 5000);
				}
				return;
			}

			const subscribeHeaders: StompitSubscribeOptions = {
				destination: `/queue/${this.config.queueName}`,
				ack: 'client-individual', // Individual acknowledgment
				'activemq.prefetchSize': String(this.config.prefetchCount),
			};

			const subscription = channel.subscribe(
				subscribeHeaders,
				(subscribeError: Error | null, message: StompitMessage, rawSubscription: StompitSubscription) => {
					if (subscribeError) {
						this.logger.error({ err: subscribeError, consumerId }, 'Subscribe error');
						done();
						if (this.running) {
							setTimeout(() => this.startConsumer(consumerId), 5000);
						}
						return;
					}

					this.lastPollTimeMs = Date.now();

					// Read message body
					let body = '';
					message.on('data', (chunk: Buffer) => {
						body += chunk.toString();
					});

					message.on('end', () => {
						this.handleMessage(channel, message, body, consumerId)
							.catch((err) => {
								this.logger.error({ err, consumerId }, 'Error handling message');
							});
					});

					message.on('error', (msgError: Error) => {
						this.logger.error({ err: msgError, consumerId }, 'Message read error');
					});
				},
			);

			this.subscriptions.push(subscription);
		});
	}

	/**
	 * Handle a received message
	 */
	private async handleMessage(
		channel: stompit.Channel,
		message: StompitMessage,
		body: string,
		consumerId: number,
	): Promise<void> {
		const headers = (message.headers ?? {}) as Record<string, string>;
		const brokerMessageId = headers['message-id'] || randomUUID();
		const messageId = headers['correlation-id'] || brokerMessageId;
		const redelivered = headers['redelivered'] === 'true';
		const receiveCount = redelivered ? 2 : 1; // STOMP doesn't provide exact count

		this.logger.debug(
			{
				messageId,
				brokerMessageId,
				consumerId,
				redelivered,
			},
			'Received message',
		);

		const activeMqMessage: ActiveMqMessage = {
			messageId,
			brokerMessageId,
			body,
			headers,
			receiveCount,
		};

		const batch: ActiveMqBatch = {
			batchId: randomUUID(),
			messages: [activeMqMessage],
			queueId: this.config.queueName,
		};

		// Create callback map
		const callbacks = new Map<string, ActiveMqMessageCallback>();
		callbacks.set(brokerMessageId, {
			ack: async () => {
				try {
					channel.ack(message);
					this.logger.debug({ messageId }, 'Message ACKed');
				} catch (error) {
					this.logger.error({ err: error, messageId }, 'Failed to ACK message');
				}
			},
			nack: async (visibilityDelaySeconds?: number) => {
				try {
					// For STOMP, NACK will cause redelivery
					// We can use scheduled redelivery by sending to a delay queue
					// or just NACK and let ActiveMQ handle redelivery
					const delay = visibilityDelaySeconds
						? visibilityDelaySeconds * 1000
						: this.config.redeliveryDelayMs;

					// STOMP 1.1+ supports NACK
					channel.nack(message);
					this.logger.debug(
						{ messageId, delayMs: delay },
						'Message NACKed (will be redelivered by broker)',
					);
				} catch (error) {
					this.logger.error({ err: error, messageId }, 'Failed to NACK message');
				}
			},
		});

		// Process batch through handler
		try {
			await this.handler(batch, callbacks);
		} catch (error) {
			this.logger.error({ err: error, messageId }, 'Error in batch handler');
			// NACK on handler error
			const callback = callbacks.get(brokerMessageId);
			if (callback) {
				await callback.nack();
			}
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
	 * Note: STOMP protocol has limited queue browsing capabilities
	 * For accurate metrics, JMX or management API would be needed
	 */
	private updateMetrics(): void {
		// STOMP doesn't provide a standard way to get queue depth
		// In production, you'd use the ActiveMQ management API or JMX
		// For now, we track what we can locally
		this.logger.debug('Metrics polling (limited with STOMP protocol)');
	}

	/**
	 * Get queue metrics
	 */
	getQueueMetrics(): { pendingMessages: number; messagesNotVisible: number } {
		return { ...this.metrics };
	}

	/**
	 * Get consumer health status
	 */
	getHealth(): {
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
	} {
		const now = Date.now();
		const timeSinceLastPoll = now - this.lastPollTimeMs;
		const healthTimeoutMs = 60_000; // 60 seconds

		return {
			mapKey: this.config.queueName,
			queueIdentifier: this.config.queueName,
			consumerQueueIdentifier: `activemq://${this.config.host}:${this.config.port}/${this.config.queueName}`,
			instanceId: this.instanceId,
			isHealthy: this.running && (this.lastPollTimeMs === 0 || timeSinceLastPoll < healthTimeoutMs),
			lastPollTimeMs: this.lastPollTimeMs,
			lastPollTime: this.lastPollTimeMs ? new Date(this.lastPollTimeMs).toISOString() : '',
			timeSinceLastPollMs: timeSinceLastPoll,
			timeSinceLastPollSeconds: Math.floor(timeSinceLastPoll / 1000),
			isRunning: this.running,
		};
	}
}
