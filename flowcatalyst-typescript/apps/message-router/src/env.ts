import { parseEnv, z } from '@flowcatalyst/config';

/**
 * Message router environment configuration
 */
export const envSchema = z.object({
	// Server
	PORT: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('8080'),
	HOST: z.string().default('0.0.0.0'),
	NODE_ENV: z.enum(['development', 'production', 'test']).default('development'),

	// Logging
	LOG_LEVEL: z.enum(['trace', 'debug', 'info', 'warn', 'error', 'fatal']).default('info'),

	// Router configuration
	MESSAGE_ROUTER_ENABLED: z
		.string()
		.transform((v) => v === 'true')
		.default('true'),
	QUEUE_TYPE: z.enum(['SQS', 'NATS', 'ACTIVEMQ', 'EMBEDDED']).default('EMBEDDED'),
	SYNC_INTERVAL_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('300000'), // 5 minutes
	MAX_POOLS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('2000'),
	POOL_WARNING_THRESHOLD: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('1000'),
	DEFAULT_CONNECTIONS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('1'),

	// AWS/SQS Configuration
	AWS_REGION: z.string().default('eu-west-1'),
	AWS_ACCESS_KEY_ID: z.string().optional(),
	AWS_SECRET_ACCESS_KEY: z.string().optional(),
	SQS_ENDPOINT: z.string().optional(), // For LocalStack

	// Platform Configuration Client
	PLATFORM_URL: z.string().url().optional(),
	PLATFORM_API_KEY: z.string().optional(),

	// HTTP Mediation Configuration
	/**
	 * Use HTTP/2 for mediation calls.
	 * Set to 'false' for local development (HTTP/1.1)
	 * Default: true for production, false for development
	 */
	MEDIATION_HTTP2: z
		.string()
		.transform((v) => v === 'true')
		.optional()
		.transform((v) => v ?? undefined), // Will be set based on NODE_ENV if not specified

	/**
	 * Connection timeout in milliseconds.
	 * How long to wait for TCP connection to be established.
	 * Default: 5000ms (5 seconds)
	 */
	MEDIATION_CONNECT_TIMEOUT_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('5000'),

	/**
	 * Request timeout in milliseconds.
	 * How long to wait for the response body.
	 * Default: 900000ms (15 minutes) - allows long-running operations
	 */
	MEDIATION_REQUEST_TIMEOUT_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('900000'),

	/**
	 * Headers timeout in milliseconds.
	 * How long to wait for response headers after connection.
	 * Default: 30000ms (30 seconds)
	 */
	MEDIATION_HEADERS_TIMEOUT_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('30000'),

	/**
	 * Number of retries for failed HTTP calls.
	 * Only retries on 5xx errors and timeouts, not 4xx.
	 * Default: 3
	 */
	MEDIATION_RETRIES: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('3'),

	/**
	 * Initial retry delay in milliseconds.
	 * Uses exponential backoff: delay * 2^attempt
	 * Default: 1000ms (1 second)
	 */
	MEDIATION_RETRY_DELAY_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('1000'),

	// Instance identification
	INSTANCE_ID: z.string().default(() => `router-${Date.now()}`),

	// Authentication Configuration
	/**
	 * Enable authentication for protected endpoints.
	 * Default: false (open access for development)
	 */
	AUTHENTICATION_ENABLED: z
		.string()
		.transform((v) => v === 'true')
		.default('false'),

	/**
	 * Authentication mode: NONE, BASIC, or OIDC.
	 * Default: NONE
	 */
	AUTHENTICATION_MODE: z.enum(['NONE', 'BASIC', 'OIDC']).default('NONE'),

	/**
	 * BasicAuth username.
	 */
	AUTH_BASIC_USERNAME: z.string().optional(),

	/**
	 * BasicAuth password.
	 */
	AUTH_BASIC_PASSWORD: z.string().optional(),

	/**
	 * OIDC issuer URL (e.g., https://keycloak.example.com/realms/myrealm).
	 * Used to discover JWKS endpoint for token validation.
	 */
	OIDC_ISSUER_URL: z.string().optional(),

	/**
	 * OIDC client ID for audience validation.
	 */
	OIDC_CLIENT_ID: z.string().optional(),

	/**
	 * OIDC audience to validate (defaults to client ID if not set).
	 */
	OIDC_AUDIENCE: z.string().optional(),

	// Notification Configuration
	/**
	 * Enable notifications globally.
	 * Default: false
	 */
	NOTIFICATION_ENABLED: z
		.string()
		.transform((v) => v === 'true')
		.default('false'),

	/**
	 * Batch interval in milliseconds for grouping warnings.
	 * Default: 300000 (5 minutes)
	 */
	NOTIFICATION_BATCH_INTERVAL_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('300000'),

	/**
	 * Minimum severity for notifications: INFO, WARNING, ERROR, CRITICAL.
	 * Default: WARNING
	 */
	NOTIFICATION_MIN_SEVERITY: z
		.enum(['INFO', 'WARNING', 'ERROR', 'CRITICAL'])
		.default('WARNING'),

	// Email Notification Configuration
	/**
	 * Enable email notifications.
	 * Default: false
	 */
	NOTIFICATION_EMAIL_ENABLED: z
		.string()
		.transform((v) => v === 'true')
		.default('false'),

	/**
	 * Email sender address.
	 */
	NOTIFICATION_EMAIL_FROM: z.string().optional(),

	/**
	 * Email recipient address(es), comma-separated.
	 */
	NOTIFICATION_EMAIL_TO: z.string().optional(),

	/**
	 * SMTP host for email.
	 */
	SMTP_HOST: z.string().optional(),

	/**
	 * SMTP port.
	 * Default: 587
	 */
	SMTP_PORT: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('587'),

	/**
	 * SMTP username for authentication.
	 */
	SMTP_USERNAME: z.string().optional(),

	/**
	 * SMTP password for authentication.
	 */
	SMTP_PASSWORD: z.string().optional(),

	/**
	 * Use TLS for SMTP connection.
	 * Default: true
	 */
	SMTP_SECURE: z
		.string()
		.transform((v) => v === 'true')
		.default('true'),

	// Teams Webhook Notification Configuration
	/**
	 * Enable Microsoft Teams webhook notifications.
	 * Default: false
	 */
	NOTIFICATION_TEAMS_ENABLED: z
		.string()
		.transform((v) => v === 'true')
		.default('false'),

	/**
	 * Microsoft Teams incoming webhook URL.
	 */
	NOTIFICATION_TEAMS_WEBHOOK_URL: z.string().optional(),

	// NATS JetStream Configuration
	/**
	 * NATS server URLs (comma-separated).
	 * Default: nats://localhost:4222
	 */
	NATS_SERVERS: z.string().default('nats://localhost:4222'),

	/**
	 * NATS connection name for identification.
	 * Default: flowcatalyst-router
	 */
	NATS_CONNECTION_NAME: z.string().default('flowcatalyst-router'),

	/**
	 * NATS username for authentication (optional).
	 */
	NATS_USERNAME: z.string().optional(),

	/**
	 * NATS password for authentication (optional).
	 */
	NATS_PASSWORD: z.string().optional(),

	/**
	 * NATS JetStream stream name.
	 * Default: FLOWCATALYST
	 */
	NATS_STREAM_NAME: z.string().default('FLOWCATALYST'),

	/**
	 * NATS JetStream consumer name.
	 * Default: flowcatalyst-router
	 */
	NATS_CONSUMER_NAME: z.string().default('flowcatalyst-router'),

	/**
	 * NATS subject filter (supports wildcards).
	 * Default: flowcatalyst.dispatch.>
	 */
	NATS_SUBJECT: z.string().default('flowcatalyst.dispatch.>'),

	/**
	 * Max messages per poll batch.
	 * Default: 10
	 */
	NATS_MAX_MESSAGES_PER_POLL: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('10'),

	/**
	 * Poll timeout in seconds.
	 * Default: 20 (matches SQS long polling)
	 */
	NATS_POLL_TIMEOUT_SECONDS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('20'),

	/**
	 * Ack wait timeout in seconds.
	 * Default: 120 (matches SQS visibility timeout)
	 */
	NATS_ACK_WAIT_SECONDS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('120'),

	/**
	 * Max delivery attempts before message goes to DLQ.
	 * Default: 10
	 */
	NATS_MAX_DELIVER: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('10'),

	/**
	 * Max messages awaiting acknowledgment.
	 * Default: 1000
	 */
	NATS_MAX_ACK_PENDING: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('1000'),

	/**
	 * Storage type: 'file' for production, 'memory' for dev.
	 * Default: file
	 */
	NATS_STORAGE_TYPE: z.enum(['file', 'memory']).default('file'),

	/**
	 * Number of stream replicas.
	 * Default: 1
	 */
	NATS_REPLICAS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('1'),

	/**
	 * Max age of messages in days.
	 * Default: 7
	 */
	NATS_MAX_AGE_DAYS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('7'),

	// ActiveMQ Configuration
	/**
	 * ActiveMQ broker host.
	 * Default: localhost
	 */
	ACTIVEMQ_HOST: z.string().default('localhost'),

	/**
	 * ActiveMQ STOMP port.
	 * Default: 61613 (STOMP protocol, not 61616 which is OpenWire)
	 */
	ACTIVEMQ_PORT: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('61613'),

	/**
	 * ActiveMQ username.
	 * Default: admin
	 */
	ACTIVEMQ_USERNAME: z.string().default('admin'),

	/**
	 * ActiveMQ password.
	 * Default: admin
	 */
	ACTIVEMQ_PASSWORD: z.string().default('admin'),

	/**
	 * ActiveMQ receive timeout in milliseconds.
	 * Default: 1000ms
	 */
	ACTIVEMQ_RECEIVE_TIMEOUT_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('1000'),

	/**
	 * ActiveMQ prefetch count per consumer.
	 * Default: 1 (process one message at a time per consumer)
	 */
	ACTIVEMQ_PREFETCH_COUNT: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('1'),

	/**
	 * ActiveMQ redelivery delay in milliseconds.
	 * Default: 30000ms (30 seconds)
	 */
	ACTIVEMQ_REDELIVERY_DELAY_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('30000'),

	// Embedded Queue Configuration
	/**
	 * Database file path for embedded queue.
	 * Use ':memory:' for in-memory only (data lost on restart).
	 * Default: ':memory:'
	 */
	EMBEDDED_DB_PATH: z.string().default(':memory:'),

	/**
	 * Visibility timeout in seconds for embedded queue.
	 * Messages become visible again after this timeout if not ACKed.
	 * Default: 30 seconds (matches SQS)
	 */
	EMBEDDED_VISIBILITY_TIMEOUT_SECONDS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('30'),

	/**
	 * Poll interval when queue is empty in milliseconds.
	 * Default: 1000ms (1 second)
	 */
	EMBEDDED_RECEIVE_TIMEOUT_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('1000'),

	/**
	 * Maximum messages per batch for embedded queue.
	 * Default: 10 (matches SQS)
	 */
	EMBEDDED_MAX_MESSAGES: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('10'),

	/**
	 * Metrics poll interval for embedded queue in milliseconds.
	 * Default: 5000ms (5 seconds)
	 */
	EMBEDDED_METRICS_POLL_INTERVAL_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('5000'),

	// Traffic Management Configuration (Standby Mode)
	/**
	 * Enable traffic management (standby mode support).
	 * When enabled, supports PRIMARY/STANDBY mode transitions
	 * and load balancer registration/deregistration.
	 * Default: false
	 */
	TRAFFIC_MANAGEMENT_ENABLED: z
		.string()
		.transform((v) => v === 'true')
		.default('false'),

	/**
	 * Traffic management strategy name.
	 * e.g., 'AWS_ALB_DEREGISTRATION' for AWS ALB integration.
	 * Default: none (basic implementation)
	 */
	TRAFFIC_STRATEGY_NAME: z.string().optional(),

	// AWS ALB Traffic Strategy Configuration
	/**
	 * AWS ALB target group ARN for traffic management.
	 * Required when TRAFFIC_STRATEGY_NAME is 'AWS_ALB_DEREGISTRATION'.
	 */
	ALB_TARGET_GROUP_ARN: z.string().optional(),

	/**
	 * Target ID for ALB registration (EC2 instance ID or IP address).
	 * Required when TRAFFIC_STRATEGY_NAME is 'AWS_ALB_DEREGISTRATION'.
	 */
	ALB_TARGET_ID: z.string().optional(),

	/**
	 * Target port for ALB registration.
	 * Default: 8080
	 */
	ALB_TARGET_PORT: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('8080'),

	/**
	 * Deregistration delay in seconds (time to wait for connection draining).
	 * Default: 300 (5 minutes)
	 */
	ALB_DEREGISTRATION_DELAY_SECONDS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('300'),

	// Health Check Configuration
	/**
	 * Enable broker health checks.
	 * Default: true
	 */
	HEALTH_CHECK_ENABLED: z
		.string()
		.transform((v) => v === 'true')
		.default('true'),

	/**
	 * Broker health check interval in milliseconds.
	 * Default: 60000 (1 minute)
	 */
	HEALTH_CHECK_INTERVAL_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('60000'),

	/**
	 * Broker health check timeout in milliseconds.
	 * Default: 5000 (5 seconds)
	 */
	HEALTH_CHECK_TIMEOUT_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('5000'),

	/**
	 * Number of consecutive failures before generating warning.
	 * Default: 3
	 */
	HEALTH_CHECK_FAILURE_THRESHOLD: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('3'),

	// Queue Health Monitor Configuration
	/**
	 * Enable queue health monitoring.
	 * Default: true
	 */
	QUEUE_HEALTH_MONITOR_ENABLED: z
		.string()
		.transform((v) => v === 'true')
		.default('true'),

	/**
	 * Queue backlog threshold - depth above this generates warning.
	 * Default: 1000
	 */
	QUEUE_HEALTH_BACKLOG_THRESHOLD: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('1000'),

	/**
	 * Queue growth threshold - per-period growth above this counts as growing.
	 * Default: 100
	 */
	QUEUE_HEALTH_GROWTH_THRESHOLD: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('100'),

	/**
	 * Queue health check interval in milliseconds.
	 * Default: 30000 (30 seconds)
	 */
	QUEUE_HEALTH_INTERVAL_MS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('30000'),

	/**
	 * Number of consecutive growth periods before warning.
	 * Default: 3 (90 seconds of growth)
	 */
	QUEUE_HEALTH_GROWTH_PERIODS: z
		.string()
		.transform((v) => Number.parseInt(v, 10))
		.default('3'),
});

export type Env = z.infer<typeof envSchema>;

const parsedEnv = parseEnv(envSchema);

// Derive HTTP/2 setting from NODE_ENV if not explicitly set
export const env = {
	...parsedEnv,
	MEDIATION_HTTP2:
		parsedEnv.MEDIATION_HTTP2 ?? parsedEnv.NODE_ENV === 'production',
};
