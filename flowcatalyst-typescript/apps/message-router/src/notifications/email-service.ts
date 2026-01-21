import nodemailer from 'nodemailer';
import type { Transporter } from 'nodemailer';
import type { Logger } from '@flowcatalyst/logging';
import type {
	NotificationService,
	WarningNotification,
	SystemEventNotification,
} from './types.js';
import { SEVERITY_COLORS, SEVERITY_EMOJIS } from './types.js';

/**
 * Email notification configuration
 */
export interface EmailNotificationConfig {
	enabled: boolean;
	from: string;
	to: string[];
	smtp: {
		host: string;
		port: number;
		secure: boolean;
		auth?:
			| {
					user: string;
					pass: string;
			  }
			| undefined;
	};
	instanceId: string;
}

/**
 * Email notification service using nodemailer
 */
export class EmailNotificationService implements NotificationService {
	private readonly config: EmailNotificationConfig;
	private readonly logger: Logger;
	private transporter: Transporter | null = null;

	constructor(config: EmailNotificationConfig, logger: Logger) {
		this.config = config;
		this.logger = logger.child({ component: 'EmailNotification' });

		if (config.enabled) {
			this.initializeTransporter();
		}
	}

	private initializeTransporter(): void {
		this.transporter = nodemailer.createTransport({
			host: this.config.smtp.host,
			port: this.config.smtp.port,
			secure: this.config.smtp.secure,
			auth: this.config.smtp.auth,
		});

		this.logger.info(
			{ host: this.config.smtp.host, port: this.config.smtp.port },
			'Email transporter initialized',
		);
	}

	isEnabled(): boolean {
		return this.config.enabled && this.transporter !== null;
	}

	async notifyWarning(warning: WarningNotification): Promise<void> {
		if (!this.isEnabled()) return;

		const subject = `${SEVERITY_EMOJIS[warning.severity]} [${warning.severity}] ${warning.category} - ${this.config.instanceId}`;
		const html = this.formatWarningHtml(warning);

		await this.sendEmail(subject, html);
	}

	async notifyCriticalError(error: WarningNotification): Promise<void> {
		if (!this.isEnabled()) return;

		const subject = `${SEVERITY_EMOJIS.CRITICAL} [CRITICAL] ${error.category} - ${this.config.instanceId}`;
		const html = this.formatWarningHtml(error);

		await this.sendEmail(subject, html);
	}

	async notifySystemEvent(event: SystemEventNotification): Promise<void> {
		if (!this.isEnabled()) return;

		const subject = `[${event.eventType}] ${this.config.instanceId}`;
		const html = this.formatSystemEventHtml(event);

		await this.sendEmail(subject, html);
	}

	/**
	 * Send batch notification for multiple warnings
	 */
	async notifyBatch(warnings: WarningNotification[]): Promise<void> {
		if (!this.isEnabled() || warnings.length === 0) return;

		const subject = `${SEVERITY_EMOJIS.WARNING} [BATCH] ${warnings.length} warnings - ${this.config.instanceId}`;
		const html = this.formatBatchHtml(warnings);

		await this.sendEmail(subject, html);
	}

	private async sendEmail(subject: string, html: string): Promise<void> {
		if (!this.transporter) return;

		try {
			await this.transporter.sendMail({
				from: this.config.from,
				to: this.config.to.join(', '),
				subject,
				html,
			});

			this.logger.debug({ subject }, 'Email notification sent');
		} catch (error) {
			this.logger.error({ error, subject }, 'Failed to send email notification');
		}
	}

	private formatWarningHtml(warning: WarningNotification): string {
		const color = SEVERITY_COLORS[warning.severity];

		return `
<!DOCTYPE html>
<html>
<head>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
    .container { max-width: 600px; margin: 0 auto; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
    .header { background: ${color}; color: white; padding: 20px; }
    .header h1 { margin: 0; font-size: 20px; }
    .content { padding: 20px; }
    .field { margin-bottom: 15px; }
    .field-label { font-weight: 600; color: #666; font-size: 12px; text-transform: uppercase; margin-bottom: 4px; }
    .field-value { color: #333; font-size: 14px; }
    .message { background: #f9f9f9; padding: 15px; border-radius: 4px; font-family: monospace; white-space: pre-wrap; }
    .footer { padding: 15px 20px; background: #f9f9f9; border-top: 1px solid #eee; font-size: 12px; color: #999; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>${SEVERITY_EMOJIS[warning.severity]} ${warning.severity}: ${warning.category}</h1>
    </div>
    <div class="content">
      <div class="field">
        <div class="field-label">Source</div>
        <div class="field-value">${warning.source}</div>
      </div>
      <div class="field">
        <div class="field-label">Timestamp</div>
        <div class="field-value">${warning.timestamp.toISOString()}</div>
      </div>
      <div class="field">
        <div class="field-label">Message</div>
        <div class="message">${this.escapeHtml(warning.message)}</div>
      </div>
    </div>
    <div class="footer">
      Instance: ${this.config.instanceId} | ID: ${warning.id}
    </div>
  </div>
</body>
</html>`;
	}

	private formatBatchHtml(warnings: WarningNotification[]): string {
		const groupedByCategory = warnings.reduce(
			(acc, warning) => {
				const category = warning.category;
				const existing = acc[category] ?? [];
				existing.push(warning);
				acc[category] = existing;
				return acc;
			},
			{} as Record<string, WarningNotification[]>,
		);

		const categorySections = Object.entries(groupedByCategory)
			.map(
				([category, categoryWarnings]) => `
      <div class="category">
        <h3>${category} (${categoryWarnings.length})</h3>
        ${categoryWarnings
					.map(
						(w) => `
          <div class="warning-item" style="border-left: 3px solid ${SEVERITY_COLORS[w.severity]};">
            <div class="warning-header">
              ${SEVERITY_EMOJIS[w.severity]} <strong>${w.severity}</strong> - ${w.source}
              <span class="timestamp">${w.timestamp.toISOString()}</span>
            </div>
            <div class="warning-message">${this.escapeHtml(w.message)}</div>
          </div>
        `,
					)
					.join('')}
      </div>
    `,
			)
			.join('');

		return `
<!DOCTYPE html>
<html>
<head>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
    .container { max-width: 800px; margin: 0 auto; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
    .header { background: #ffc107; color: #333; padding: 20px; }
    .header h1 { margin: 0; font-size: 20px; }
    .summary { padding: 15px 20px; background: #f9f9f9; border-bottom: 1px solid #eee; }
    .content { padding: 20px; }
    .category { margin-bottom: 25px; }
    .category h3 { margin: 0 0 15px 0; padding-bottom: 10px; border-bottom: 2px solid #eee; color: #333; }
    .warning-item { padding: 10px 15px; margin-bottom: 10px; background: #f9f9f9; border-radius: 4px; }
    .warning-header { font-size: 13px; margin-bottom: 8px; }
    .timestamp { float: right; color: #999; font-size: 11px; }
    .warning-message { font-family: monospace; font-size: 13px; white-space: pre-wrap; color: #555; }
    .footer { padding: 15px 20px; background: #f9f9f9; border-top: 1px solid #eee; font-size: 12px; color: #999; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>${SEVERITY_EMOJIS.WARNING} Batch Notification: ${warnings.length} Warnings</h1>
    </div>
    <div class="summary">
      <strong>Categories:</strong> ${Object.keys(groupedByCategory).join(', ')}
    </div>
    <div class="content">
      ${categorySections}
    </div>
    <div class="footer">
      Instance: ${this.config.instanceId}
    </div>
  </div>
</body>
</html>`;
	}

	private formatSystemEventHtml(event: SystemEventNotification): string {
		const metadataHtml = event.metadata
			? `
      <div class="field">
        <div class="field-label">Metadata</div>
        <div class="message">${this.escapeHtml(JSON.stringify(event.metadata, null, 2))}</div>
      </div>
    `
			: '';

		return `
<!DOCTYPE html>
<html>
<head>
  <style>
    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 0; padding: 20px; background: #f5f5f5; }
    .container { max-width: 600px; margin: 0 auto; background: white; border-radius: 8px; overflow: hidden; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
    .header { background: #17a2b8; color: white; padding: 20px; }
    .header h1 { margin: 0; font-size: 20px; }
    .content { padding: 20px; }
    .field { margin-bottom: 15px; }
    .field-label { font-weight: 600; color: #666; font-size: 12px; text-transform: uppercase; margin-bottom: 4px; }
    .field-value { color: #333; font-size: 14px; }
    .message { background: #f9f9f9; padding: 15px; border-radius: 4px; font-family: monospace; white-space: pre-wrap; }
    .footer { padding: 15px 20px; background: #f9f9f9; border-top: 1px solid #eee; font-size: 12px; color: #999; }
  </style>
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>${SEVERITY_EMOJIS.INFO} System Event: ${event.eventType}</h1>
    </div>
    <div class="content">
      <div class="field">
        <div class="field-label">Timestamp</div>
        <div class="field-value">${event.timestamp.toISOString()}</div>
      </div>
      <div class="field">
        <div class="field-label">Message</div>
        <div class="message">${this.escapeHtml(event.message)}</div>
      </div>
      ${metadataHtml}
    </div>
    <div class="footer">
      Instance: ${this.config.instanceId}
    </div>
  </div>
</body>
</html>`;
	}

	private escapeHtml(text: string): string {
		return text
			.replace(/&/g, '&amp;')
			.replace(/</g, '&lt;')
			.replace(/>/g, '&gt;')
			.replace(/"/g, '&quot;')
			.replace(/'/g, '&#039;');
	}
}
