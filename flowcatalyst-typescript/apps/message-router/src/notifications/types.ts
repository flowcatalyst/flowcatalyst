/**
 * Notification severity levels
 */
export type Severity = 'INFO' | 'WARNING' | 'ERROR' | 'CRITICAL';

/**
 * Severity order for comparison
 */
export const SEVERITY_ORDER: Record<Severity, number> = {
	INFO: 0,
	WARNING: 1,
	ERROR: 2,
	CRITICAL: 3,
};

/**
 * Check if severity meets minimum threshold
 */
export function meetsSeverityThreshold(severity: Severity, minSeverity: Severity): boolean {
	return SEVERITY_ORDER[severity] >= SEVERITY_ORDER[minSeverity];
}

/**
 * Warning notification data
 */
export interface WarningNotification {
	id: string;
	category: string;
	severity: Severity;
	message: string;
	timestamp: Date;
	source: string;
}

/**
 * System event notification data
 */
export interface SystemEventNotification {
	eventType: string;
	message: string;
	timestamp: Date;
	metadata?: Record<string, unknown>;
}

/**
 * Notification service interface
 */
export interface NotificationService {
	/**
	 * Send a warning notification
	 */
	notifyWarning(warning: WarningNotification): Promise<void>;

	/**
	 * Send a critical error notification (immediate, not batched)
	 */
	notifyCriticalError(error: WarningNotification): Promise<void>;

	/**
	 * Send a system event notification
	 */
	notifySystemEvent(event: SystemEventNotification): Promise<void>;

	/**
	 * Check if the notification service is enabled
	 */
	isEnabled(): boolean;

	/**
	 * Flush any pending batched notifications
	 */
	flush?(): Promise<void>;

	/**
	 * Stop the notification service
	 */
	stop?(): Promise<void>;
}

/**
 * Severity colors for HTML formatting
 */
export const SEVERITY_COLORS: Record<Severity, string> = {
	CRITICAL: '#dc3545', // Red
	ERROR: '#fd7e14', // Orange
	WARNING: '#ffc107', // Yellow
	INFO: '#17a2b8', // Cyan
};

/**
 * Severity emojis for text formatting
 */
export const SEVERITY_EMOJIS: Record<Severity, string> = {
	CRITICAL: '🚨',
	ERROR: '❌',
	WARNING: '⚠️',
	INFO: 'ℹ️',
};
