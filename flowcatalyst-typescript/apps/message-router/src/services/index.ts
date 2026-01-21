import type { Logger } from '@flowcatalyst/logging';
import { CircuitBreakerManager, defaultCircuitBreakerConfig } from '@flowcatalyst/queue-core';
import { HealthService } from './health-service.js';
import { WarningService } from './warning-service.js';
import { QueueManagerService } from './queue-manager-service.js';
import { QueueValidationService } from './queue-validation-service.js';
import { SeederService } from './seeder-service.js';
import { createNotificationService, type BatchingNotificationService } from '../notifications/index.js';
import { createTrafficManager, type TrafficManager } from '../traffic/index.js';
import { BrokerHealthService, QueueHealthMonitor } from '../health/index.js';
import { env } from '../env.js';

/**
 * All application services
 */
export interface Services {
	health: HealthService;
	warnings: WarningService;
	queueManager: QueueManagerService;
	queueValidation: QueueValidationService;
	brokerHealth: BrokerHealthService;
	queueHealthMonitor: QueueHealthMonitor;
	circuitBreakers: CircuitBreakerManager;
	seeder: SeederService;
	notifications: BatchingNotificationService;
	traffic: TrafficManager;
}

export { BrokerHealthService, QueueHealthMonitor } from '../health/index.js';
export type { BrokerHealthResult, BrokerHealthStats } from '../health/index.js';

/**
 * Create all services
 */
export function createServices(logger: Logger): Services {
	const warnings = new WarningService(logger);
	const circuitBreakers = new CircuitBreakerManager(defaultCircuitBreakerConfig, logger);

	// Create traffic manager (for standby mode support)
	const traffic = createTrafficManager(
		{
			enabled: env.TRAFFIC_MANAGEMENT_ENABLED,
			strategyName: env.TRAFFIC_STRATEGY_NAME,
			awsAlb:
				env.TRAFFIC_STRATEGY_NAME === 'AWS_ALB_DEREGISTRATION' &&
				env.ALB_TARGET_GROUP_ARN &&
				env.ALB_TARGET_ID
					? {
							region: env.AWS_REGION,
							targetGroupArn: env.ALB_TARGET_GROUP_ARN,
							targetId: env.ALB_TARGET_ID,
							targetPort: env.ALB_TARGET_PORT,
							deregistrationDelaySeconds: env.ALB_DEREGISTRATION_DELAY_SECONDS,
						}
					: undefined,
		},
		logger,
	);

	const queueValidation = new QueueValidationService(warnings, logger);
	const queueManager = new QueueManagerService(circuitBreakers, warnings, traffic, queueValidation, logger);

	// Create broker health service with warning integration
	const brokerHealth = new BrokerHealthService(logger, warnings, {
		enabled: env.HEALTH_CHECK_ENABLED,
		intervalMs: env.HEALTH_CHECK_INTERVAL_MS,
		timeoutMs: env.HEALTH_CHECK_TIMEOUT_MS,
		failureThresholdForWarning: env.HEALTH_CHECK_FAILURE_THRESHOLD,
	});

	// Create queue health monitor with warning integration
	const queueHealthMonitor = new QueueHealthMonitor(
		warnings,
		() => queueManager.getQueueStats(),
		logger,
		{
			enabled: env.QUEUE_HEALTH_MONITOR_ENABLED,
			backlogThreshold: env.QUEUE_HEALTH_BACKLOG_THRESHOLD,
			growthThreshold: env.QUEUE_HEALTH_GROWTH_THRESHOLD,
			intervalMs: env.QUEUE_HEALTH_INTERVAL_MS,
			growthPeriodsForWarning: env.QUEUE_HEALTH_GROWTH_PERIODS,
		},
	);

	const health = new HealthService(queueManager, warnings, brokerHealth, circuitBreakers, logger);
	const seeder = new SeederService(queueManager, logger);

	// Create notification service
	const notifications = createNotificationService(
		{
			enabled: env.NOTIFICATION_ENABLED,
			batchIntervalMs: env.NOTIFICATION_BATCH_INTERVAL_MS,
			minSeverity: env.NOTIFICATION_MIN_SEVERITY,
			instanceId: env.INSTANCE_ID,
			email: {
				enabled: env.NOTIFICATION_EMAIL_ENABLED,
				from: env.NOTIFICATION_EMAIL_FROM,
				to: env.NOTIFICATION_EMAIL_TO,
				smtp: {
					host: env.SMTP_HOST,
					port: env.SMTP_PORT,
					secure: env.SMTP_SECURE,
					username: env.SMTP_USERNAME,
					password: env.SMTP_PASSWORD,
				},
			},
			teams: {
				enabled: env.NOTIFICATION_TEAMS_ENABLED,
				webhookUrl: env.NOTIFICATION_TEAMS_WEBHOOK_URL,
			},
		},
		logger,
	);

	// Connect notification service to warning service
	warnings.setNotificationService(notifications);

	// Start services if message router is enabled
	if (env.MESSAGE_ROUTER_ENABLED) {
		queueManager.start().catch((err) => {
			logger.error({ err }, 'Failed to start queue manager');
		});

		// Start health monitoring services
		brokerHealth.start();
		queueHealthMonitor.start();
	}

	return {
		health,
		warnings,
		queueManager,
		queueValidation,
		brokerHealth,
		queueHealthMonitor,
		circuitBreakers,
		seeder,
		notifications,
		traffic,
	};
}
