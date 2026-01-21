import {
	connect,
	type NatsConnection,
	type JetStreamClient,
	type JetStreamManager,
	type Consumer,
	type ConsumerConfig,
	type StreamConfig,
	AckPolicy,
	DeliverPolicy,
	RetentionPolicy,
	StorageType,
	type JsMsg,
	nanos,
} from 'nats';
import type { Logger } from '@flowcatalyst/logging';
import { randomUUID } from 'node:crypto';

/**
 * Message received from NATS JetStream
 */
export interface NatsMessage {
	messageId: string;
	brokerMessageId: string;
	subject: string;
	data: string;
	headers: Record<string, string>;
	redelivered: boolean;
	redeliveryCount: number;
	seq: number;
}

/**
 * Message callback for ACK/NACK
 */
export interface NatsMessageCallback {
	ack: () => Promise<void>;
	nack: (delaySeconds?: number) => Promise<void>;
	inProgress: () => void;
}

/**
 * Batch of messages from NATS
 */
export interface NatsBatch {
	batchId: string;
	messages: NatsMessage[];
	queueId: string;
}

/**
 * Batch handler type
 */
export type NatsBatchHandler = (
	batch: NatsBatch,
	callbacks: Map<string, NatsMessageCallback>,
) => Promise<void>;

/**
 * NATS JetStream consumer configuration
 */
export interface NatsConsumerConfig {
	/** NATS server URLs (comma-separated) */
	servers: string;
	/** Connection name for identification */
	connectionName: string;
	/** Username for authentication (optional) */
	username?: string | undefined;
	/** Password for authentication (optional) */
	password?: string | undefined;
	/** Stream name */
	streamName: string;
	/** Durable consumer name */
	consumerName: string;
	/** Subject filter (e.g., "flowcatalyst.dispatch.>") */
	subject: string;
	/** Max messages per poll (default: 10) */
	maxMessagesPerPoll: number;
	/** Poll timeout in seconds (default: 20) */
	pollTimeoutSeconds: number;
	/** Ack wait in seconds (default: 120) */
	ackWaitSeconds: number;
	/** Max delivery attempts before DLQ (default: 10) */
	maxDeliver: number;
	/** Max messages awaiting ack (default: 1000) */
	maxAckPending: number;
	/** Storage type: 'file' or 'memory' */
	storageType: 'file' | 'memory';
	/** Number of replicas (default: 1) */
	replicas: number;
	/** Max age in days (default: 7) */
	maxAgeDays: number;
	/** Metrics poll interval in milliseconds */
	metricsPollIntervalMs: number;
}

/**
 * Default visibility delays (matching Java implementation)
 */
const DEFAULT_VISIBILITY_DELAY_SECONDS = 120;
const FAST_FAIL_DELAY_SECONDS = 10;

/**
 * NATS JetStream consumer
 * Implements pull-based durable consumer with explicit acknowledgment
 */
export class NatsConsumer {
	private readonly config: NatsConsumerConfig;
	private readonly handler: NatsBatchHandler;
	private readonly logger: Logger;
	private readonly instanceId: string;

	private running = false;
	private lastPollTimeMs = 0;
	private connection: NatsConnection | null = null;
	private jetStream: JetStreamClient | null = null;
	private jetStreamManager: JetStreamManager | null = null;
	private consumer: Consumer | null = null;

	private metrics = {
		pendingMessages: 0,
		messagesNotVisible: 0,
	};

	// Track sequences that failed to ack (for deduplication)
	private readonly pendingAckSequences = new Set<number>();

	constructor(
		config: NatsConsumerConfig,
		handler: NatsBatchHandler,
		logger: Logger,
		instanceId: string,
	) {
		this.config = config;
		this.handler = handler;
		this.logger = logger.child({
			component: 'NatsConsumer',
			stream: config.streamName,
			consumer: config.consumerName,
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

		this.logger.info(
			{
				servers: this.config.servers,
				stream: this.config.streamName,
				consumer: this.config.consumerName,
				subject: this.config.subject,
			},
			'Starting NATS JetStream consumer',
		);

		try {
			// Connect to NATS
			const connectOptions: Parameters<typeof connect>[0] = {
				servers: this.config.servers.split(',').map((s) => s.trim()),
				name: this.config.connectionName,
				reconnect: true,
				maxReconnectAttempts: -1, // Unlimited
				reconnectTimeWait: 2000,
				timeout: 10000,
			};

			if (this.config.username && this.config.password) {
				connectOptions.user = this.config.username;
				connectOptions.pass = this.config.password;
			}

			this.connection = await connect(connectOptions);
			this.logger.info('Connected to NATS');

			// Get JetStream context
			this.jetStream = this.connection.jetstream();
			this.jetStreamManager = await this.connection.jetstreamManager();

			// Ensure stream exists
			await this.ensureStream();

			// Ensure consumer exists
			await this.ensureConsumer();

			// Get consumer handle
			this.consumer = await this.jetStream.consumers.get(
				this.config.streamName,
				this.config.consumerName,
			);

			this.running = true;

			// Start consumption loop
			this.consumeLoop();

			// Start metrics polling
			this.startMetricsPolling();

			this.logger.info('NATS JetStream consumer started');
		} catch (error) {
			this.logger.error({ err: error }, 'Failed to start NATS consumer');
			throw error;
		}
	}

	/**
	 * Stop consuming messages
	 */
	async stop(): Promise<void> {
		this.logger.info('Stopping NATS JetStream consumer');
		this.running = false;

		// Close connection
		if (this.connection) {
			await this.connection.drain();
			await this.connection.close();
			this.connection = null;
		}

		this.jetStream = null;
		this.jetStreamManager = null;
		this.consumer = null;

		this.logger.info('NATS JetStream consumer stopped');
	}

	/**
	 * Ensure the stream exists
	 */
	private async ensureStream(): Promise<void> {
		if (!this.jetStreamManager) return;

		const streamConfig: Partial<StreamConfig> = {
			name: this.config.streamName,
			subjects: [this.config.subject],
			retention: RetentionPolicy.Workqueue,
			storage: this.config.storageType === 'file' ? StorageType.File : StorageType.Memory,
			num_replicas: this.config.replicas,
			max_age: nanos(this.config.maxAgeDays * 24 * 60 * 60 * 1000), // Convert days to nanos
		};

		try {
			// Try to get existing stream
			await this.jetStreamManager.streams.info(this.config.streamName);
			this.logger.debug({ stream: this.config.streamName }, 'Stream exists');
		} catch {
			// Stream doesn't exist, create it
			this.logger.info({ stream: this.config.streamName }, 'Creating stream');
			await this.jetStreamManager.streams.add(streamConfig as StreamConfig);
		}
	}

	/**
	 * Ensure the consumer exists
	 */
	private async ensureConsumer(): Promise<void> {
		if (!this.jetStreamManager) return;

		const consumerConfig: Partial<ConsumerConfig> = {
			durable_name: this.config.consumerName,
			ack_policy: AckPolicy.Explicit,
			ack_wait: nanos(this.config.ackWaitSeconds * 1000), // Convert seconds to nanos
			max_deliver: this.config.maxDeliver,
			max_ack_pending: this.config.maxAckPending,
			deliver_policy: DeliverPolicy.All,
			filter_subject: this.config.subject,
		};

		try {
			// Try to get existing consumer
			await this.jetStreamManager.consumers.info(this.config.streamName, this.config.consumerName);
			this.logger.debug({ consumer: this.config.consumerName }, 'Consumer exists');
		} catch {
			// Consumer doesn't exist, create it
			this.logger.info({ consumer: this.config.consumerName }, 'Creating consumer');
			await this.jetStreamManager.consumers.add(
				this.config.streamName,
				consumerConfig as ConsumerConfig,
			);
		}
	}

	/**
	 * Main consumption loop
	 */
	private async consumeLoop(): Promise<void> {
		while (this.running && this.consumer) {
			try {
				this.lastPollTimeMs = Date.now();

				// Fetch batch of messages
				const messages = await this.consumer.fetch({
					max_messages: this.config.maxMessagesPerPoll,
					expires: this.config.pollTimeoutSeconds * 1000,
				});

				const batch: NatsMessage[] = [];
				const jsMessages = new Map<string, JsMsg>();

				for await (const msg of messages) {
					const messageId = msg.headers?.get('message-id') || randomUUID();
					const brokerMessageId = `${msg.info.stream}-${msg.seq}`;

					// Check for duplicate (failed ack)
					if (this.pendingAckSequences.has(msg.seq)) {
						this.logger.debug(
							{ seq: msg.seq },
							'Skipping message with pending ack (deduplication)',
						);
						continue;
					}

					const natsMessage: NatsMessage = {
						messageId,
						brokerMessageId,
						subject: msg.subject,
						data: msg.string(),
						headers: this.extractHeaders(msg),
						redelivered: msg.redelivered,
						redeliveryCount: msg.info.redeliveryCount,
						seq: msg.seq,
					};

					batch.push(natsMessage);
					jsMessages.set(brokerMessageId, msg);
				}

				if (batch.length === 0) {
					continue;
				}

				// Create batch
				const natsBatch: NatsBatch = {
					batchId: randomUUID(),
					messages: batch,
					queueId: `${this.config.streamName}:${this.config.consumerName}`,
				};

				// Create callbacks map
				const callbacks = new Map<string, NatsMessageCallback>();
				for (const message of batch) {
					const jsMsg = jsMessages.get(message.brokerMessageId);
					if (!jsMsg) continue;

					callbacks.set(message.brokerMessageId, {
						ack: async () => {
							try {
								jsMsg.ack();
								this.pendingAckSequences.delete(message.seq);
								this.logger.debug({ seq: message.seq }, 'Message ACKed');
							} catch (error) {
								this.logger.error({ err: error, seq: message.seq }, 'Failed to ACK message');
								this.pendingAckSequences.add(message.seq);
							}
						},
						nack: async (delaySeconds?: number) => {
							try {
								const delay = delaySeconds ?? DEFAULT_VISIBILITY_DELAY_SECONDS;
								jsMsg.nak(delay * 1000); // nak accepts milliseconds
								this.logger.debug({ seq: message.seq, delaySeconds: delay }, 'Message NACKed');
							} catch (error) {
								this.logger.error({ err: error, seq: message.seq }, 'Failed to NACK message');
							}
						},
						inProgress: () => {
							try {
								jsMsg.working();
								this.logger.debug({ seq: message.seq }, 'Message marked in-progress');
							} catch (error) {
								this.logger.error(
									{ err: error, seq: message.seq },
									'Failed to mark message in-progress',
								);
							}
						},
					});
				}

				// Process batch through handler
				await this.handler(natsBatch, callbacks);
			} catch (error) {
				this.logger.error({ err: error }, 'Error in consumption loop');
				await sleep(1000); // Back off on error
			}
		}

		this.logger.info('Consumption loop stopped');
	}

	/**
	 * Extract headers from message
	 */
	private extractHeaders(msg: JsMsg): Record<string, string> {
		const headers: Record<string, string> = {};
		if (msg.headers) {
			for (const [key, values] of msg.headers) {
				headers[key] = values.join(',');
			}
		}
		return headers;
	}

	/**
	 * Start periodic metrics polling
	 */
	private startMetricsPolling(): void {
		const poll = async () => {
			if (!this.running) return;

			await this.updateMetrics();
			setTimeout(poll, this.config.metricsPollIntervalMs);
		};

		setTimeout(poll, this.config.metricsPollIntervalMs);
	}

	/**
	 * Update queue metrics
	 */
	private async updateMetrics(): Promise<void> {
		if (!this.jetStreamManager) return;

		try {
			const consumerInfo = await this.jetStreamManager.consumers.info(
				this.config.streamName,
				this.config.consumerName,
			);

			this.metrics.pendingMessages = consumerInfo.num_pending;
			this.metrics.messagesNotVisible = consumerInfo.num_ack_pending;
		} catch (error) {
			this.logger.warn({ err: error }, 'Failed to update metrics');
		}
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

		const queueIdentifier = `${this.config.streamName}:${this.config.consumerName}`;

		return {
			mapKey: queueIdentifier,
			queueIdentifier,
			consumerQueueIdentifier: `nats://${this.config.servers}/${this.config.streamName}/${this.config.consumerName}`,
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

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}
