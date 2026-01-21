import type { Logger } from '@flowcatalyst/logging';
import type { Severity } from './types.js';
import { EmailNotificationService, type EmailNotificationConfig } from './email-service.js';
import { TeamsNotificationService, type TeamsNotificationConfig } from './teams-service.js';
import { BatchingNotificationService, type BatchingNotificationConfig } from './batching-service.js';

export * from './types.js';
export * from './email-service.js';
export * from './teams-service.js';
export * from './batching-service.js';

/**
 * Notification service configuration from environment
 */
export interface NotificationConfig {
	/** Global enable/disable */
	enabled: boolean;
	/** Batch interval in milliseconds */
	batchIntervalMs: number;
	/** Minimum severity to notify */
	minSeverity: Severity;
	/** Instance identifier */
	instanceId: string;

	/** Email configuration */
	email: {
		enabled: boolean;
		from?: string | undefined;
		to?: string | undefined;
		smtp: {
			host?: string | undefined;
			port: number;
			secure: boolean;
			username?: string | undefined;
			password?: string | undefined;
		};
	};

	/** Teams webhook configuration */
	teams: {
		enabled: boolean;
		webhookUrl?: string | undefined;
	};
}

/**
 * Create the notification service from configuration
 */
export function createNotificationService(
	config: NotificationConfig,
	logger: Logger,
): BatchingNotificationService {
	const childLogger = logger.child({ component: 'Notifications' });

	// Create email service if configured
	let emailService: EmailNotificationService | null = null;
	if (config.email.enabled && config.email.from && config.email.to && config.email.smtp.host) {
		const emailConfig: EmailNotificationConfig = {
			enabled: true,
			from: config.email.from,
			to: config.email.to.split(',').map((e) => e.trim()),
			smtp: {
				host: config.email.smtp.host,
				port: config.email.smtp.port,
				secure: config.email.smtp.secure,
				auth:
					config.email.smtp.username && config.email.smtp.password
						? {
								user: config.email.smtp.username,
								pass: config.email.smtp.password,
							}
						: undefined,
			},
			instanceId: config.instanceId,
		};

		emailService = new EmailNotificationService(emailConfig, childLogger);
		childLogger.info('Email notification service created');
	}

	// Create Teams service if configured
	let teamsService: TeamsNotificationService | null = null;
	if (config.teams.enabled && config.teams.webhookUrl) {
		const teamsConfig: TeamsNotificationConfig = {
			enabled: true,
			webhookUrl: config.teams.webhookUrl,
			instanceId: config.instanceId,
		};

		teamsService = new TeamsNotificationService(teamsConfig, childLogger);
		childLogger.info('Teams notification service created');
	}

	// Create the batching service that wraps both
	const batchingConfig: BatchingNotificationConfig = {
		enabled: config.enabled,
		batchIntervalMs: config.batchIntervalMs,
		minSeverity: config.minSeverity,
		instanceId: config.instanceId,
	};

	const batchingService = new BatchingNotificationService(
		batchingConfig,
		emailService,
		teamsService,
		childLogger,
	);

	if (batchingService.isEnabled()) {
		childLogger.info(
			{
				email: emailService?.isEnabled() ?? false,
				teams: teamsService?.isEnabled() ?? false,
				batchIntervalMs: config.batchIntervalMs,
				minSeverity: config.minSeverity,
			},
			'Notification service initialized',
		);
	} else {
		childLogger.info('Notification service disabled');
	}

	return batchingService;
}
