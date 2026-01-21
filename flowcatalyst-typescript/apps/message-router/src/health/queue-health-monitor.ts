import { ok, err, Result, ResultAsync } from 'neverthrow';
import type { Logger } from '@flowcatalyst/logging';
import type { QueueStats, WarningCategory, WarningSeverity } from '@flowcatalyst/shared-types';
import type { WarningService } from '../services/warning-service.js';
import type { QueueHealthError, QueueHealthStatus } from './errors.js';
import { QueueHealthErrors } from './errors.js';

/**
 * Queue size history for growth detection
 */
interface QueueSizeHistory {
	lastSize: number; // -1 = no history yet
	consecutiveGrowthPeriods: number;
}

/**
 * Queue health monitor configuration
 */
export interface QueueHealthMonitorConfig {
	/** Enable monitoring (default: true) */
	enabled: boolean;
	/** Backlog threshold - queue depth above this generates warning (default: 1000) */
	backlogThreshold: number;
	/** Growth threshold - per-period growth above this counts as growing (default: 100) */
	growthThreshold: number;
	/** Monitor interval in milliseconds (default: 30000) */
	intervalMs: number;
	/** Number of consecutive growth periods before warning (default: 3) */
	growthPeriodsForWarning: number;
}

/**
 * Default configuration
 */
const DEFAULT_CONFIG: QueueHealthMonitorConfig = {
	enabled: true,
	backlogThreshold: 1000,
	growthThreshold: 100,
	intervalMs: 30_000,
	growthPeriodsForWarning: 3,
};

/**
 * Function type for getting queue stats
 * Accepts either a Map or Record for flexibility
 */
export type QueueStatsProvider = () => Map<string, QueueStats> | Record<string, QueueStats>;

/**
 * Monitors queue health and generates warnings for operational issues:
 * - QUEUE_BACKLOG: Queue depth exceeds threshold
 * - QUEUE_GROWING: Queue growing for 3+ consecutive check periods
 */
export class QueueHealthMonitor {
	private readonly config: QueueHealthMonitorConfig;
	private readonly warningService: WarningService;
	private readonly getQueueStats: QueueStatsProvider;
	private readonly logger: Logger;

	private readonly queueHistory = new Map<string, QueueSizeHistory>();
	private intervalHandle: NodeJS.Timeout | null = null;
	private running = false;

	constructor(
		warningService: WarningService,
		getQueueStats: QueueStatsProvider,
		logger: Logger,
		config: Partial<QueueHealthMonitorConfig> = {},
	) {
		this.config = { ...DEFAULT_CONFIG, ...config };
		this.warningService = warningService;
		this.getQueueStats = getQueueStats;
		this.logger = logger.child({ component: 'QueueHealthMonitor' });
	}

	/**
	 * Start the health monitor
	 */
	start(): Result<void, never> {
		if (this.running) {
			this.logger.warn('Queue health monitor already running');
			return ok(undefined);
		}

		if (!this.config.enabled) {
			this.logger.info('Queue health monitor disabled');
			return ok(undefined);
		}

		this.running = true;
		this.logger.info(
			{
				backlogThreshold: this.config.backlogThreshold,
				growthThreshold: this.config.growthThreshold,
				intervalMs: this.config.intervalMs,
			},
			'Starting queue health monitor',
		);

		// Run immediately, then on interval
		this.runHealthCheck();
		this.intervalHandle = setInterval(() => this.runHealthCheck(), this.config.intervalMs);

		return ok(undefined);
	}

	/**
	 * Stop the health monitor
	 */
	stop(): Result<void, never> {
		this.logger.info('Stopping queue health monitor');
		this.running = false;

		if (this.intervalHandle) {
			clearInterval(this.intervalHandle);
			this.intervalHandle = null;
		}

		return ok(undefined);
	}

	/**
	 * Run a single health check cycle
	 */
	private runHealthCheck(): void {
		if (!this.running) return;

		try {
			const allStats = this.getQueueStats();

			// Handle both Map and Record types
			const entries: [string, QueueStats][] =
				allStats instanceof Map ? Array.from(allStats.entries()) : Object.entries(allStats);

			for (const [queueName, stats] of entries) {
				this.checkQueueBacklog(queueName, stats);
				this.checkQueueGrowth(queueName, stats);
			}
		} catch (error) {
			this.logger.error({ err: error }, 'Error monitoring queue health');
		}
	}

	/**
	 * Check if queue depth exceeds backlog threshold
	 */
	private checkQueueBacklog(queueName: string, stats: QueueStats): void {
		const currentSize = stats.pendingMessages;

		if (currentSize > this.config.backlogThreshold) {
			this.logger.warn(
				{
					queueName,
					currentSize,
					threshold: this.config.backlogThreshold,
				},
				'Queue backlog detected',
			);

			this.warningService.add(
				'QUEUE_BACKLOG' as WarningCategory,
				'WARNING' as WarningSeverity,
				`Queue ${queueName} depth is ${currentSize} (threshold: ${this.config.backlogThreshold})`,
				'QueueHealthMonitor',
			);
		}
	}

	/**
	 * Check if queue is growing for consecutive periods
	 */
	private checkQueueGrowth(queueName: string, stats: QueueStats): void {
		const currentSize = stats.pendingMessages;

		let history = this.queueHistory.get(queueName);
		if (!history) {
			history = { lastSize: -1, consecutiveGrowthPeriods: 0 };
			this.queueHistory.set(queueName, history);
		}

		const previousSize = history.lastSize;
		history.lastSize = currentSize;

		// Skip first check (no history yet)
		if (previousSize < 0) {
			return;
		}

		const growth = currentSize - previousSize;

		if (growth >= this.config.growthThreshold) {
			// Queue is growing
			history.consecutiveGrowthPeriods++;

			if (history.consecutiveGrowthPeriods >= this.config.growthPeriodsForWarning) {
				this.logger.warn(
					{
						queueName,
						consecutiveGrowthPeriods: history.consecutiveGrowthPeriods,
						currentSize,
						growth,
					},
					'Queue growth detected',
				);

				this.warningService.add(
					'QUEUE_GROWING' as WarningCategory,
					'WARNING' as WarningSeverity,
					`Queue ${queueName} growing for ${history.consecutiveGrowthPeriods} periods (current depth: ${currentSize}, growth rate: +${growth}/${this.config.intervalMs / 1000}s)`,
					'QueueHealthMonitor',
				);

				// Cap at 10 to avoid warning spam
				if (history.consecutiveGrowthPeriods > 10) {
					history.consecutiveGrowthPeriods = 10;
				}
			}
		} else {
			// Reset counter if queue stopped growing
			if (history.consecutiveGrowthPeriods > 0) {
				this.logger.debug(
					{
						queueName,
						previousGrowthPeriods: history.consecutiveGrowthPeriods,
					},
					'Queue stopped growing',
				);
			}
			history.consecutiveGrowthPeriods = 0;
		}
	}

	/**
	 * Get current health status for all monitored queues
	 */
	getHealthStatus(): Result<QueueHealthStatus[], QueueHealthError> {
		try {
			const allStats = this.getQueueStats();
			const statuses: QueueHealthStatus[] = [];

			// Handle both Map and Record types
			const entries: [string, QueueStats][] =
				allStats instanceof Map ? Array.from(allStats.entries()) : Object.entries(allStats);

			for (const [queueName, stats] of entries) {
				const history = this.queueHistory.get(queueName);
				const consecutiveGrowthPeriods = history?.consecutiveGrowthPeriods ?? 0;

				statuses.push({
					queueName,
					pendingMessages: stats.pendingMessages,
					messagesNotVisible: stats.messagesNotVisible,
					isBacklogged: stats.pendingMessages > this.config.backlogThreshold,
					isGrowing: consecutiveGrowthPeriods >= this.config.growthPeriodsForWarning,
					consecutiveGrowthPeriods,
				});
			}

			return ok(statuses);
		} catch (error) {
			return err(QueueHealthErrors.metricsUnavailable('all', error as Error));
		}
	}

	/**
	 * Check if monitor is running
	 */
	isRunning(): boolean {
		return this.running;
	}

	/**
	 * Get current configuration
	 */
	getConfig(): QueueHealthMonitorConfig {
		return { ...this.config };
	}

	/**
	 * Clear history for a specific queue (useful for testing)
	 */
	clearHistory(queueName?: string): void {
		if (queueName) {
			this.queueHistory.delete(queueName);
		} else {
			this.queueHistory.clear();
		}
	}
}
