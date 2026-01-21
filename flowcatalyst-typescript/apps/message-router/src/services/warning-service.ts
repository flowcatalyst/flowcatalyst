import type { Logger } from '@flowcatalyst/logging';
import type { Warning, WarningCategory, WarningSeverity } from '@flowcatalyst/shared-types';
import { randomUUID } from 'node:crypto';
import type { BatchingNotificationService, WarningNotification, Severity } from '../notifications/index.js';

/**
 * Service for managing system warnings
 */
export class WarningService {
	private readonly warnings = new Map<string, Warning>();
	private readonly logger: Logger;
	private notificationService: BatchingNotificationService | null = null;

	constructor(logger: Logger) {
		this.logger = logger.child({ component: 'WarningService' });
	}

	/**
	 * Set the notification service for sending alerts
	 */
	setNotificationService(notificationService: BatchingNotificationService): void {
		this.notificationService = notificationService;
		this.logger.info('Notification service attached to warning service');
	}

	/**
	 * Add a new warning
	 */
	add(
		category: WarningCategory,
		severity: WarningSeverity,
		message: string,
		source: string,
	): Warning {
		const warning: Warning = {
			id: randomUUID(),
			category,
			severity,
			message,
			timestamp: new Date().toISOString(),
			source,
			acknowledged: false,
		};

		this.warnings.set(warning.id, warning);
		this.logger.warn({ warning }, 'Warning added');

		// Send notification
		this.sendNotification(warning);

		return warning;
	}

	/**
	 * Send notification for a warning
	 */
	private sendNotification(warning: Warning): void {
		if (!this.notificationService?.isEnabled()) return;

		const notification: WarningNotification = {
			id: warning.id,
			category: warning.category,
			severity: warning.severity as Severity,
			message: warning.message,
			timestamp: new Date(warning.timestamp),
			source: warning.source,
		};

		// Critical errors are sent immediately, others are batched
		if (warning.severity === 'CRITICAL') {
			this.notificationService.notifyCriticalError(notification).catch((error) => {
				this.logger.error({ error }, 'Failed to send critical error notification');
			});
		} else {
			this.notificationService.notifyWarning(notification).catch((error) => {
				this.logger.error({ error }, 'Failed to queue warning notification');
			});
		}
	}

	/**
	 * Get all warnings
	 */
	getAll(): Warning[] {
		return Array.from(this.warnings.values());
	}

	/**
	 * Get unacknowledged warnings
	 */
	getUnacknowledged(): Warning[] {
		return this.getAll().filter((w) => !w.acknowledged);
	}

	/**
	 * Get warnings by severity
	 */
	getBySeverity(severity: string): Warning[] {
		return this.getAll().filter((w) => w.severity === severity);
	}

	/**
	 * Acknowledge a warning
	 */
	acknowledge(warningId: string): boolean {
		const warning = this.warnings.get(warningId);
		if (warning) {
			warning.acknowledged = true;
			this.logger.info({ warningId }, 'Warning acknowledged');
			return true;
		}
		return false;
	}

	/**
	 * Clear all warnings
	 */
	clearAll(): void {
		this.warnings.clear();
		this.logger.info('All warnings cleared');
	}

	/**
	 * Clear warnings older than specified hours
	 */
	clearOlderThan(hours: number): void {
		const cutoff = Date.now() - hours * 60 * 60 * 1000;
		let cleared = 0;

		for (const [id, warning] of this.warnings) {
			const warningTime = new Date(warning.timestamp).getTime();
			if (warningTime < cutoff) {
				this.warnings.delete(id);
				cleared++;
			}
		}

		this.logger.info({ hours, cleared }, 'Old warnings cleared');
	}

	/**
	 * Get warning count by severity
	 */
	getCountBySeverity(): Record<string, number> {
		const counts: Record<string, number> = {};
		for (const warning of this.warnings.values()) {
			counts[warning.severity] = (counts[warning.severity] || 0) + 1;
		}
		return counts;
	}
}
