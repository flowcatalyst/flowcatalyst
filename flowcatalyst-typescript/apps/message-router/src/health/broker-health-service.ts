import { ok, err, Result, ResultAsync } from 'neverthrow';
import type { Logger } from '@flowcatalyst/logging';
import type { WarningCategory, WarningSeverity } from '@flowcatalyst/shared-types';
import { SQSClient, ListQueuesCommand, GetQueueUrlCommand } from '@aws-sdk/client-sqs';
import stompit from 'stompit';
import { connect as natsConnect, type NatsConnection } from 'nats';
import type { WarningService } from '../services/warning-service.js';
import { env } from '../env.js';
import type { BrokerHealthError, BrokerHealthResult } from './errors.js';
import { BrokerHealthErrors } from './errors.js';

/**
 * Broker health statistics
 */
export interface BrokerHealthStats {
	connectionAttempts: number;
	connectionSuccesses: number;
	connectionFailures: number;
	brokerAvailable: boolean;
	brokerType: string;
	lastCheckTime: Date | null;
	lastCheckDurationMs: number;
}

/**
 * Broker health service configuration
 */
export interface BrokerHealthServiceConfig {
	/** Enable scheduled health checks (default: true) */
	enabled: boolean;
	/** Health check interval in milliseconds (default: 60000) */
	intervalMs: number;
	/** Connection timeout in milliseconds (default: 5000) */
	timeoutMs: number;
	/** Generate warning after this many consecutive failures (default: 3) */
	failureThresholdForWarning: number;
}

/**
 * Default configuration
 */
const DEFAULT_CONFIG: BrokerHealthServiceConfig = {
	enabled: true,
	intervalMs: 60_000,
	timeoutMs: 5000,
	failureThresholdForWarning: 3,
};

/**
 * Queue type enum matching Java
 */
export type QueueType = 'SQS' | 'ACTIVEMQ' | 'NATS' | 'EMBEDDED';

/**
 * Service for checking broker (SQS/ActiveMQ/NATS) connectivity and health.
 * Provides explicit health checks for external messaging dependencies.
 *
 * Uses neverthrow for typed error handling.
 */
export class BrokerHealthService {
	private readonly config: BrokerHealthServiceConfig;
	private readonly warningService: WarningService | null;
	private readonly logger: Logger;
	private readonly queueType: QueueType;

	// Metrics
	private connectionAttempts = 0;
	private connectionSuccesses = 0;
	private connectionFailures = 0;
	private consecutiveFailures = 0;
	private brokerAvailable = false;
	private lastCheckTime: Date | null = null;
	private lastCheckDurationMs = 0;

	// Scheduled check
	private intervalHandle: NodeJS.Timeout | null = null;
	private running = false;

	constructor(
		logger: Logger,
		warningService: WarningService | null = null,
		config: Partial<BrokerHealthServiceConfig> = {},
	) {
		this.config = { ...DEFAULT_CONFIG, ...config };
		this.warningService = warningService;
		this.logger = logger.child({ component: 'BrokerHealthService' });
		this.queueType = env.QUEUE_TYPE as QueueType;
		this.logger.info({ queueType: this.queueType }, 'BrokerHealthService initialized');
	}

	/**
	 * Start scheduled health checks
	 */
	start(): Result<void, never> {
		if (this.running) {
			this.logger.warn('Broker health service already running');
			return ok(undefined);
		}

		if (!this.config.enabled) {
			this.logger.info('Broker health service disabled');
			return ok(undefined);
		}

		this.running = true;
		this.logger.info(
			{
				intervalMs: this.config.intervalMs,
				queueType: this.queueType,
			},
			'Starting broker health service',
		);

		// Run immediately, then on interval
		this.runScheduledCheck();
		this.intervalHandle = setInterval(() => this.runScheduledCheck(), this.config.intervalMs);

		return ok(undefined);
	}

	/**
	 * Stop scheduled health checks
	 */
	stop(): Result<void, never> {
		this.logger.info('Stopping broker health service');
		this.running = false;

		if (this.intervalHandle) {
			clearInterval(this.intervalHandle);
			this.intervalHandle = null;
		}

		return ok(undefined);
	}

	/**
	 * Run a scheduled health check
	 */
	private runScheduledCheck(): void {
		if (!this.running) return;

		this.checkBrokerConnectivity().match(
			(result) => {
				if (!result.healthy) {
					this.handleUnhealthyBroker(result);
				} else {
					this.handleHealthyBroker();
				}
			},
			(error) => {
				this.logger.error({ error }, 'Broker health check returned error');
				this.handleBrokerError(error);
			},
		);
	}

	/**
	 * Handle healthy broker state
	 */
	private handleHealthyBroker(): void {
		if (this.consecutiveFailures > 0) {
			this.logger.info(
				{ previousFailures: this.consecutiveFailures },
				'Broker connectivity restored',
			);
		}
		this.consecutiveFailures = 0;
	}

	/**
	 * Handle unhealthy broker state
	 */
	private handleUnhealthyBroker(result: BrokerHealthResult): void {
		this.consecutiveFailures++;

		if (
			this.consecutiveFailures >= this.config.failureThresholdForWarning &&
			this.warningService
		) {
			this.warningService.add(
				'BROKER_HEALTH' as WarningCategory,
				'ERROR' as WarningSeverity,
				`${this.queueType} broker unhealthy for ${this.consecutiveFailures} consecutive checks. Details: ${result.details || 'unknown'}`,
				'BrokerHealthService',
			);
		}
	}

	/**
	 * Handle broker check error
	 */
	private handleBrokerError(error: BrokerHealthError): void {
		this.consecutiveFailures++;

		if (
			this.consecutiveFailures >= this.config.failureThresholdForWarning &&
			this.warningService
		) {
			const message = this.formatErrorMessage(error);
			this.warningService.add(
				'BROKER_HEALTH' as WarningCategory,
				'CRITICAL' as WarningSeverity,
				message,
				'BrokerHealthService',
			);
		}
	}

	/**
	 * Format error message based on error type
	 */
	private formatErrorMessage(error: BrokerHealthError): string {
		switch (error.type) {
			case 'broker_unreachable':
				return `${error.broker} broker unreachable: ${error.cause.message}`;
			case 'auth_failed':
				return `${error.broker} authentication failed: ${error.message}`;
			case 'timeout':
				return `${error.broker} connection timed out after ${error.durationMs}ms`;
			case 'unknown':
				return `${error.broker} unknown error: ${error.cause.message}`;
		}
	}

	/**
	 * Check broker connectivity based on configured queue type.
	 * Returns a typed Result with either health result or error.
	 */
	checkBrokerConnectivity(): ResultAsync<BrokerHealthResult, BrokerHealthError> {
		if (!env.MESSAGE_ROUTER_ENABLED) {
			this.logger.debug('Message router disabled, skipping broker connectivity check');
			return ResultAsync.fromSafePromise(
				Promise.resolve({
					broker: this.queueType,
					healthy: true,
					latencyMs: 0,
					details: 'Message router disabled',
				}),
			);
		}

		this.connectionAttempts++;
		const startTime = Date.now();

		const checkPromise = this.performConnectivityCheck();

		return ResultAsync.fromPromise(checkPromise, (error) =>
			BrokerHealthErrors.unknown(this.queueType, error as Error),
		).andThen((connected) => {
			const durationMs = Date.now() - startTime;
			this.lastCheckTime = new Date();
			this.lastCheckDurationMs = durationMs;

			if (connected) {
				this.connectionSuccesses++;
				this.brokerAvailable = true;
				this.logger.debug(
					{ queueType: this.queueType, durationMs },
					'Broker connectivity check passed',
				);

				return ok({
					broker: this.queueType,
					healthy: true,
					latencyMs: durationMs,
				});
			} else {
				this.connectionFailures++;
				this.brokerAvailable = false;

				return ok({
					broker: this.queueType,
					healthy: false,
					latencyMs: durationMs,
					details: `${this.queueType} broker is not accessible`,
				});
			}
		});
	}

	/**
	 * Perform the actual connectivity check based on queue type
	 */
	private async performConnectivityCheck(): Promise<boolean> {
		switch (this.queueType) {
			case 'SQS':
				return this.checkSqsConnectivity();
			case 'ACTIVEMQ':
				return this.checkActiveMqConnectivity();
			case 'NATS':
				return this.checkNatsConnectivity();
			case 'EMBEDDED':
				// Embedded queue is always available (SQLite)
				return true;
			default:
				return true;
		}
	}

	/**
	 * Check if a specific SQS queue is accessible.
	 * This verifies both AWS credentials and queue existence.
	 */
	checkSqsQueueAccessible(queueName: string): ResultAsync<string, BrokerHealthError> {
		const sqsClient = new SQSClient({
			region: env.AWS_REGION,
			...(env.SQS_ENDPOINT && { endpoint: env.SQS_ENDPOINT }),
		});

		return ResultAsync.fromPromise(
			sqsClient.send(new GetQueueUrlCommand({ QueueName: queueName })).finally(() => {
				sqsClient.destroy();
			}),
			(error) => BrokerHealthErrors.unreachable('SQS', error as Error),
		).andThen((response) => {
			if (!response.QueueUrl) {
				return err(
					BrokerHealthErrors.unreachable(
						'SQS',
						new Error(`Queue ${queueName} URL is empty`),
					),
				);
			}
			return ok(response.QueueUrl);
		});
	}

	/**
	 * Get broker health statistics
	 */
	getStats(): BrokerHealthStats {
		return {
			connectionAttempts: this.connectionAttempts,
			connectionSuccesses: this.connectionSuccesses,
			connectionFailures: this.connectionFailures,
			brokerAvailable: this.brokerAvailable,
			brokerType: this.queueType,
			lastCheckTime: this.lastCheckTime,
			lastCheckDurationMs: this.lastCheckDurationMs,
		};
	}

	/**
	 * Get the current broker type
	 */
	getBrokerType(): QueueType {
		return this.queueType;
	}

	/**
	 * Check if service is running
	 */
	isRunning(): boolean {
		return this.running;
	}

	/**
	 * Check basic SQS connectivity by attempting to list queues.
	 */
	private async checkSqsConnectivity(): Promise<boolean> {
		const sqsClient = new SQSClient({
			region: env.AWS_REGION,
			...(env.SQS_ENDPOINT && { endpoint: env.SQS_ENDPOINT }),
		});

		try {
			await sqsClient.send(new ListQueuesCommand({}));
			return true;
		} catch (error) {
			this.logger.error({ err: error }, 'SQS connectivity check failed');
			return false;
		} finally {
			sqsClient.destroy();
		}
	}

	/**
	 * Check ActiveMQ connectivity by creating and closing a test connection.
	 */
	private async checkActiveMqConnectivity(): Promise<boolean> {
		return new Promise((resolve) => {
			const connectOptions = {
				host: env.ACTIVEMQ_HOST,
				port: env.ACTIVEMQ_PORT,
				connectHeaders: {
					host: '/',
					login: env.ACTIVEMQ_USERNAME,
					passcode: env.ACTIVEMQ_PASSWORD,
					'heart-beat': '0,0',
				},
			};

			const timeout = setTimeout(() => {
				this.logger.warn('ActiveMQ connectivity check timed out');
				resolve(false);
			}, this.config.timeoutMs);

			stompit.connect(connectOptions, (error, client) => {
				clearTimeout(timeout);

				if (error) {
					this.logger.error({ err: error }, 'ActiveMQ connectivity check failed');
					resolve(false);
					return;
				}

				client.disconnect();
				resolve(true);
			});
		});
	}

	/**
	 * Check NATS JetStream connectivity.
	 */
	private async checkNatsConnectivity(): Promise<boolean> {
		let connection: NatsConnection | null = null;
		try {
			connection = await natsConnect({
				servers: env.NATS_SERVERS.split(','),
				name: `${env.NATS_CONNECTION_NAME}-health-check`,
				...(env.NATS_USERNAME && { user: env.NATS_USERNAME }),
				...(env.NATS_PASSWORD && { pass: env.NATS_PASSWORD }),
				timeout: this.config.timeoutMs,
			});

			return true;
		} catch (error) {
			this.logger.error({ err: error }, 'NATS connectivity check failed');
			return false;
		} finally {
			if (connection) {
				await connection.close();
			}
		}
	}
}
