import { Counter, Gauge, Histogram, Registry } from 'prom-client';

/**
 * Metrics registry and collectors for the message router
 * Matches Java Micrometer metrics
 */
export class MessageRouterMetrics {
	public readonly registry: Registry;

	// Queue metrics
	public readonly messagesReceived: Counter;
	public readonly messagesConsumed: Counter;
	public readonly messagesFailed: Counter;
	public readonly messagesDeferred: Counter;
	public readonly queueSize: Gauge;
	public readonly pendingMessages: Gauge;
	public readonly messagesNotVisible: Gauge;

	// Pool metrics
	public readonly messagesProcessed: Counter;
	public readonly messagesSucceeded: Counter;
	public readonly poolMessagesFailed: Counter;
	public readonly messagesRateLimited: Counter;
	public readonly activeWorkers: Gauge;
	public readonly availablePermits: Gauge;
	public readonly poolQueueSize: Gauge;
	public readonly processingDuration: Histogram;

	// Consumer metrics
	public readonly consumerPollCount: Counter;
	public readonly consumerErrors: Counter;
	public readonly lastPollTime: Gauge;

	// Circuit breaker metrics
	public readonly circuitBreakerState: Gauge;
	public readonly circuitBreakerCalls: Counter;

	constructor(prefix = 'message_router') {
		this.registry = new Registry();

		// Queue metrics
		this.messagesReceived = new Counter({
			name: `${prefix}_queue_messages_received_total`,
			help: 'Total messages received from queue',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		this.messagesConsumed = new Counter({
			name: `${prefix}_queue_messages_consumed_total`,
			help: 'Total messages successfully consumed',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		this.messagesFailed = new Counter({
			name: `${prefix}_queue_messages_failed_total`,
			help: 'Total messages that failed processing',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		this.messagesDeferred = new Counter({
			name: `${prefix}_queue_messages_deferred_total`,
			help: 'Total messages deferred (ack=false)',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		this.queueSize = new Gauge({
			name: `${prefix}_queue_size`,
			help: 'Current queue size',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		this.pendingMessages = new Gauge({
			name: `${prefix}_queue_pending_messages`,
			help: 'Pending messages in queue',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		this.messagesNotVisible = new Gauge({
			name: `${prefix}_queue_messages_not_visible`,
			help: 'Messages not visible (in flight)',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		// Pool metrics
		this.messagesProcessed = new Counter({
			name: `${prefix}_pool_messages_processed_total`,
			help: 'Total messages processed by pool',
			labelNames: ['pool'],
			registers: [this.registry],
		});

		this.messagesSucceeded = new Counter({
			name: `${prefix}_pool_messages_succeeded_total`,
			help: 'Total messages successfully processed',
			labelNames: ['pool'],
			registers: [this.registry],
		});

		this.poolMessagesFailed = new Counter({
			name: `${prefix}_pool_messages_failed_total`,
			help: 'Total messages failed in pool',
			labelNames: ['pool'],
			registers: [this.registry],
		});

		this.messagesRateLimited = new Counter({
			name: `${prefix}_pool_messages_rate_limited_total`,
			help: 'Total messages rate limited',
			labelNames: ['pool'],
			registers: [this.registry],
		});

		this.activeWorkers = new Gauge({
			name: `${prefix}_pool_active_workers`,
			help: 'Active workers in pool',
			labelNames: ['pool'],
			registers: [this.registry],
		});

		this.availablePermits = new Gauge({
			name: `${prefix}_pool_available_permits`,
			help: 'Available permits in pool',
			labelNames: ['pool'],
			registers: [this.registry],
		});

		this.poolQueueSize = new Gauge({
			name: `${prefix}_pool_queue_size`,
			help: 'Messages queued in pool',
			labelNames: ['pool'],
			registers: [this.registry],
		});

		this.processingDuration = new Histogram({
			name: `${prefix}_pool_processing_duration_seconds`,
			help: 'Message processing duration',
			labelNames: ['pool'],
			buckets: [0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10, 30, 60],
			registers: [this.registry],
		});

		// Consumer metrics
		this.consumerPollCount = new Counter({
			name: `${prefix}_consumer_poll_total`,
			help: 'Total consumer poll operations',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		this.consumerErrors = new Counter({
			name: `${prefix}_consumer_errors_total`,
			help: 'Total consumer errors',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		this.lastPollTime = new Gauge({
			name: `${prefix}_consumer_last_poll_timestamp`,
			help: 'Timestamp of last successful poll',
			labelNames: ['queue'],
			registers: [this.registry],
		});

		// Circuit breaker metrics
		this.circuitBreakerState = new Gauge({
			name: `${prefix}_circuit_breaker_state`,
			help: 'Circuit breaker state (0=closed, 1=open, 2=half_open)',
			labelNames: ['name'],
			registers: [this.registry],
		});

		this.circuitBreakerCalls = new Counter({
			name: `${prefix}_circuit_breaker_calls_total`,
			help: 'Circuit breaker call counts',
			labelNames: ['name', 'result'],
			registers: [this.registry],
		});
	}

	/**
	 * Get metrics in Prometheus format
	 */
	async getMetrics(): Promise<string> {
		return this.registry.metrics();
	}

	/**
	 * Get content type for Prometheus
	 */
	getContentType(): string {
		return this.registry.contentType;
	}
}

/**
 * Global metrics instance
 */
let globalMetrics: MessageRouterMetrics | null = null;

/**
 * Get or create global metrics instance
 */
export function getMetrics(): MessageRouterMetrics {
	if (!globalMetrics) {
		globalMetrics = new MessageRouterMetrics();
	}
	return globalMetrics;
}

/**
 * Set global metrics instance (for testing)
 */
export function setMetrics(metrics: MessageRouterMetrics): void {
	globalMetrics = metrics;
}
