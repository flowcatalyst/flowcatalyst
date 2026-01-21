import type { Logger } from '@flowcatalyst/logging';
import type {
	NotificationService,
	WarningNotification,
	SystemEventNotification,
	Severity,
} from './types.js';
import { meetsSeverityThreshold } from './types.js';
import type { EmailNotificationService } from './email-service.js';
import type { TeamsNotificationService } from './teams-service.js';

/**
 * Batching notification configuration
 */
export interface BatchingNotificationConfig {
	/** Whether notifications are enabled globally */
	enabled: boolean;
	/** Batch interval in milliseconds */
	batchIntervalMs: number;
	/** Minimum severity to send notifications */
	minSeverity: Severity;
	/** Instance identifier */
	instanceId: string;
}

/**
 * Batching notification service that collects warnings and sends them periodically.
 * Critical errors bypass batching and are sent immediately.
 */
export class BatchingNotificationService implements NotificationService {
	private readonly config: BatchingNotificationConfig;
	private readonly logger: Logger;
	private readonly emailService: EmailNotificationService | null;
	private readonly teamsService: TeamsNotificationService | null;

	private pendingWarnings: WarningNotification[] = [];
	private batchTimer: ReturnType<typeof setInterval> | null = null;
	private isRunning = false;

	constructor(
		config: BatchingNotificationConfig,
		emailService: EmailNotificationService | null,
		teamsService: TeamsNotificationService | null,
		logger: Logger,
	) {
		this.config = config;
		this.emailService = emailService;
		this.teamsService = teamsService;
		this.logger = logger.child({ component: 'BatchingNotification' });

		if (config.enabled && (emailService?.isEnabled() || teamsService?.isEnabled())) {
			this.start();
		}
	}

	isEnabled(): boolean {
		return (
			this.config.enabled &&
			(this.emailService?.isEnabled() || this.teamsService?.isEnabled() || false)
		);
	}

	/**
	 * Start the batching timer
	 */
	private start(): void {
		if (this.isRunning) return;

		this.isRunning = true;
		this.batchTimer = setInterval(() => {
			this.flush().catch((error) => {
				this.logger.error({ error }, 'Failed to flush notification batch');
			});
		}, this.config.batchIntervalMs);

		this.logger.info(
			{ batchIntervalMs: this.config.batchIntervalMs, minSeverity: this.config.minSeverity },
			'Batching notification service started',
		);
	}

	/**
	 * Stop the batching timer and flush pending notifications
	 */
	async stop(): Promise<void> {
		if (!this.isRunning) return;

		this.isRunning = false;

		if (this.batchTimer) {
			clearInterval(this.batchTimer);
			this.batchTimer = null;
		}

		// Flush any remaining warnings
		await this.flush();

		this.logger.info('Batching notification service stopped');
	}

	/**
	 * Queue a warning notification for batching.
	 * Warnings are collected and sent periodically.
	 */
	async notifyWarning(warning: WarningNotification): Promise<void> {
		if (!this.isEnabled()) return;

		// Check if warning meets minimum severity threshold
		if (!meetsSeverityThreshold(warning.severity, this.config.minSeverity)) {
			this.logger.debug(
				{ severity: warning.severity, minSeverity: this.config.minSeverity },
				'Warning below severity threshold, skipping',
			);
			return;
		}

		// Add to pending batch
		this.pendingWarnings.push(warning);

		this.logger.debug(
			{ category: warning.category, severity: warning.severity, pendingCount: this.pendingWarnings.length },
			'Warning queued for batch notification',
		);
	}

	/**
	 * Send a critical error immediately, bypassing the batch.
	 * Critical errors are too important to wait for batching.
	 */
	async notifyCriticalError(error: WarningNotification): Promise<void> {
		if (!this.isEnabled()) return;

		this.logger.info({ category: error.category }, 'Sending critical error notification immediately');

		// Send to both channels immediately
		const promises: Promise<void>[] = [];

		if (this.emailService?.isEnabled()) {
			promises.push(this.emailService.notifyCriticalError(error));
		}

		if (this.teamsService?.isEnabled()) {
			promises.push(this.teamsService.notifyCriticalError(error));
		}

		await Promise.allSettled(promises);
	}

	/**
	 * Send a system event notification immediately.
	 * System events (startup, shutdown, etc.) are not batched.
	 */
	async notifySystemEvent(event: SystemEventNotification): Promise<void> {
		if (!this.isEnabled()) return;

		this.logger.debug({ eventType: event.eventType }, 'Sending system event notification');

		const promises: Promise<void>[] = [];

		if (this.emailService?.isEnabled()) {
			promises.push(this.emailService.notifySystemEvent(event));
		}

		if (this.teamsService?.isEnabled()) {
			promises.push(this.teamsService.notifySystemEvent(event));
		}

		await Promise.allSettled(promises);
	}

	/**
	 * Flush all pending warnings as a batch notification
	 */
	async flush(): Promise<void> {
		if (this.pendingWarnings.length === 0) {
			return;
		}

		// Take all pending warnings
		const warnings = this.pendingWarnings;
		this.pendingWarnings = [];

		this.logger.info(
			{ count: warnings.length },
			'Flushing batch notification',
		);

		// Deduplicate warnings by category + message
		const deduplicated = this.deduplicateWarnings(warnings);

		if (deduplicated.length === 0) {
			return;
		}

		// If only one warning, send as individual notification
		if (deduplicated.length === 1) {
			const warning = deduplicated[0];
			if (!warning) return; // Type guard

			const promises: Promise<void>[] = [];

			if (this.emailService?.isEnabled()) {
				promises.push(this.emailService.notifyWarning(warning));
			}

			if (this.teamsService?.isEnabled()) {
				promises.push(this.teamsService.notifyWarning(warning));
			}

			await Promise.allSettled(promises);
			return;
		}

		// Send batch notification
		const promises: Promise<void>[] = [];

		if (this.emailService?.isEnabled()) {
			promises.push(this.emailService.notifyBatch(deduplicated));
		}

		if (this.teamsService?.isEnabled()) {
			promises.push(this.teamsService.notifyBatch(deduplicated));
		}

		await Promise.allSettled(promises);
	}

	/**
	 * Deduplicate warnings by category + message, keeping the most recent
	 */
	private deduplicateWarnings(warnings: WarningNotification[]): WarningNotification[] {
		const seen = new Map<string, WarningNotification>();

		for (const warning of warnings) {
			const key = `${warning.category}:${warning.message}`;
			const existing = seen.get(key);

			// Keep the one with higher severity, or more recent if same severity
			if (!existing) {
				seen.set(key, warning);
			} else if (
				this.compareSeverity(warning.severity, existing.severity) > 0 ||
				(warning.severity === existing.severity && warning.timestamp > existing.timestamp)
			) {
				seen.set(key, warning);
			}
		}

		return Array.from(seen.values());
	}

	/**
	 * Compare severity levels (higher severity = higher number)
	 */
	private compareSeverity(a: Severity, b: Severity): number {
		const order: Record<Severity, number> = {
			INFO: 0,
			WARNING: 1,
			ERROR: 2,
			CRITICAL: 3,
		};
		return order[a] - order[b];
	}

	/**
	 * Get current pending warnings count (for monitoring)
	 */
	getPendingCount(): number {
		return this.pendingWarnings.length;
	}
}
