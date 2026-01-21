import type { Logger } from '@flowcatalyst/logging';
import { SQSClient, GetQueueUrlCommand } from '@aws-sdk/client-sqs';
import stompit from 'stompit';
import { connect as natsConnect, type NatsConnection } from 'nats';
import type { WarningService } from './warning-service.js';
import { env } from '../env.js';

/**
 * Queue configuration for validation.
 * At least one of queueUri or queueName must be provided.
 */
export type QueueConfigForValidation =
	| { queueUri: string; queueName?: string }
	| { queueUri?: string; queueName: string };

/**
 * Validation result
 */
export interface ValidationResult {
	issues: string[];
	validated: number;
	failed: number;
}

/**
 * Service for validating queue existence and accessibility.
 * Raises warnings for missing or inaccessible queues without stopping the system.
 *
 * Matches Java QueueValidationService behavior.
 */
export class QueueValidationService {
	private readonly logger: Logger;
	private readonly warnings: WarningService;
	private readonly queueType: string;

	constructor(warnings: WarningService, logger: Logger) {
		this.warnings = warnings;
		this.logger = logger.child({ component: 'QueueValidationService' });
		this.queueType = env.QUEUE_TYPE;
	}

	/**
	 * Validate all configured queues and raise warnings for any that are missing or inaccessible.
	 * Does NOT throw exceptions - continues processing even if some queues are missing.
	 *
	 * @param queueConfigs the list of queue configurations to validate
	 * @returns validation result with list of issues
	 */
	async validateQueues(queueConfigs: QueueConfigForValidation[]): Promise<ValidationResult> {
		if (!env.MESSAGE_ROUTER_ENABLED) {
			this.logger.debug('Message router disabled, skipping queue validation');
			return { issues: [], validated: 0, failed: 0 };
		}

		const issues: string[] = [];
		let validated = 0;
		let failed = 0;

		for (const config of queueConfigs) {
			const queueIdentifier = config.queueName || config.queueUri || 'unknown';

			try {
				let accessible: boolean;

				switch (this.queueType) {
					case 'SQS':
						accessible = await this.validateSqsQueue(config);
						break;
					case 'ACTIVEMQ':
						accessible = await this.validateActiveMqQueue(config);
						break;
					case 'NATS':
						// NATS streams are created automatically
						accessible = true;
						break;
					case 'EMBEDDED':
						// Embedded queues are always accessible (SQLite file-based)
						accessible = true;
						break;
					default:
						accessible = true;
				}

				if (accessible) {
					validated++;
				} else {
					failed++;
					const issue = `Queue [${queueIdentifier}] is not accessible`;
					issues.push(issue);
					this.raiseQueueWarning(queueIdentifier, 'Queue is not accessible');
				}
			} catch (error) {
				failed++;
				const errorMessage = error instanceof Error ? error.message : String(error);
				const issue = `Failed to validate queue [${queueIdentifier}]: ${errorMessage}`;
				issues.push(issue);
				this.raiseQueueWarning(queueIdentifier, `Validation failed: ${errorMessage}`);
			}
		}

		if (issues.length === 0) {
			this.logger.info({ count: queueConfigs.length }, 'All queues validated successfully');
		} else {
			this.logger.warn(
				{ issueCount: issues.length },
				'Queue validation found issues (system will continue processing other queues)',
			);
		}

		return { issues, validated, failed };
	}

	/**
	 * Validate a single SQS queue.
	 * Checks if the queue exists and is accessible with current credentials.
	 */
	private async validateSqsQueue(config: QueueConfigForValidation): Promise<boolean> {
		const sqsClient = new SQSClient({
			region: env.AWS_REGION,
			...(env.SQS_ENDPOINT && { endpoint: env.SQS_ENDPOINT }),
		});

		try {
			const queueName = config.queueName;
			const queueUrl = config.queueUri;

			if (queueUrl && queueUrl.trim() !== '') {
				// If URI is provided, validate it's a valid URL format
				if (!queueUrl.startsWith('http')) {
					this.logger.warn({ queueUrl }, "Queue URI doesn't appear to be a valid URL");
					return false;
				}
				this.logger.debug({ queueUrl }, 'Queue URI provided, assuming valid');
				return true;
			} else if (queueName && queueName.trim() !== '') {
				// Validate queue name by getting its URL
				await sqsClient.send(
					new GetQueueUrlCommand({
						QueueName: queueName,
					}),
				);
				this.logger.debug({ queueName }, 'SQS queue validated successfully');
				return true;
			} else {
				this.logger.warn('Queue configuration has neither name nor URI');
				return false;
			}
		} catch (error) {
			const errorName = error instanceof Error ? error.name : 'Unknown';
			if (errorName === 'QueueDoesNotExist' || errorName === 'AWS.SimpleQueueService.NonExistentQueue') {
				this.logger.warn({ queueName: config.queueName }, 'SQS queue does not exist');
				return false;
			}
			this.logger.error({ err: error, queueName: config.queueName }, 'Failed to validate SQS queue');
			return false;
		} finally {
			sqsClient.destroy();
		}
	}

	/**
	 * Validate a single ActiveMQ queue.
	 * Checks if we can create a session and access the queue.
	 * Note: ActiveMQ will create the queue if it doesn't exist (auto-create behavior)
	 * So this mainly validates that we can connect to the broker.
	 */
	private async validateActiveMqQueue(config: QueueConfigForValidation): Promise<boolean> {
		return new Promise((resolve) => {
			const queueName = config.queueName;
			if (!queueName || queueName.trim() === '') {
				this.logger.warn('ActiveMQ queue configuration has no queue name');
				resolve(false);
				return;
			}

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
				this.logger.warn({ queueName }, 'ActiveMQ validation timed out');
				resolve(false);
			}, 10000);

			stompit.connect(connectOptions, (error, client) => {
				clearTimeout(timeout);

				if (error) {
					this.logger.error(
						{ err: error, queueName },
						'Failed to connect to ActiveMQ for validation',
					);
					resolve(false);
					return;
				}

				this.logger.debug({ queueName }, 'ActiveMQ queue validated successfully');
				client.disconnect();
				resolve(true);
			});
		});
	}

	/**
	 * Validate NATS connectivity.
	 * Creates a temporary connection to verify NATS is accessible.
	 */
	async validateNatsConnectivity(): Promise<boolean> {
		let connection: NatsConnection | null = null;
		try {
			connection = await natsConnect({
				servers: env.NATS_SERVERS.split(','),
				name: `${env.NATS_CONNECTION_NAME}-validation`,
				...(env.NATS_USERNAME && { user: env.NATS_USERNAME }),
				...(env.NATS_PASSWORD && { pass: env.NATS_PASSWORD }),
				timeout: 5000,
			});

			this.logger.debug('NATS connectivity validated successfully');
			return true;
		} catch (error) {
			this.logger.error({ err: error }, 'Failed to validate NATS connectivity');
			return false;
		} finally {
			if (connection) {
				await connection.close();
			}
		}
	}

	/**
	 * Raise a warning for a queue issue.
	 * This warning will be visible in the monitoring dashboard.
	 */
	private raiseQueueWarning(queueIdentifier: string, reason: string): void {
		this.warnings.add(
			'QUEUE_VALIDATION',
			'WARNING',
			`Queue [${queueIdentifier}] validation failed: ${reason}. System will continue processing other queues.`,
			'QueueValidationService',
		);

		this.logger.warn(
			{ queueIdentifier, reason },
			'Queue validation issue (system will continue)',
		);
	}
}
