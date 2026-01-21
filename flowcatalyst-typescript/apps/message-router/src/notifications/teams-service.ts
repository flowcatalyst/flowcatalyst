import type { Logger } from '@flowcatalyst/logging';
import type {
	NotificationService,
	WarningNotification,
	SystemEventNotification,
	Severity,
} from './types.js';
import { SEVERITY_COLORS, SEVERITY_EMOJIS } from './types.js';

/**
 * Teams webhook notification configuration
 */
export interface TeamsNotificationConfig {
	enabled: boolean;
	webhookUrl: string;
	instanceId: string;
}

/**
 * Adaptive Card color mapping for Teams
 */
const TEAMS_COLORS: Record<Severity, 'attention' | 'warning' | 'good' | 'accent'> = {
	CRITICAL: 'attention',
	ERROR: 'attention',
	WARNING: 'warning',
	INFO: 'accent',
};

/**
 * Microsoft Teams webhook notification service using Adaptive Cards
 */
export class TeamsNotificationService implements NotificationService {
	private readonly config: TeamsNotificationConfig;
	private readonly logger: Logger;

	constructor(config: TeamsNotificationConfig, logger: Logger) {
		this.config = config;
		this.logger = logger.child({ component: 'TeamsNotification' });

		if (config.enabled) {
			this.logger.info('Teams webhook notification service initialized');
		}
	}

	isEnabled(): boolean {
		return this.config.enabled && !!this.config.webhookUrl;
	}

	async notifyWarning(warning: WarningNotification): Promise<void> {
		if (!this.isEnabled()) return;

		const card = this.createWarningCard(warning);
		await this.sendCard(card);
	}

	async notifyCriticalError(error: WarningNotification): Promise<void> {
		if (!this.isEnabled()) return;

		const card = this.createWarningCard(error);
		await this.sendCard(card);
	}

	async notifySystemEvent(event: SystemEventNotification): Promise<void> {
		if (!this.isEnabled()) return;

		const card = this.createSystemEventCard(event);
		await this.sendCard(card);
	}

	/**
	 * Send batch notification for multiple warnings
	 */
	async notifyBatch(warnings: WarningNotification[]): Promise<void> {
		if (!this.isEnabled() || warnings.length === 0) return;

		const card = this.createBatchCard(warnings);
		await this.sendCard(card);
	}

	private async sendCard(card: AdaptiveCard): Promise<void> {
		try {
			const response = await fetch(this.config.webhookUrl, {
				method: 'POST',
				headers: {
					'Content-Type': 'application/json',
				},
				body: JSON.stringify(card),
			});

			if (!response.ok) {
				const text = await response.text();
				throw new Error(`Teams webhook failed: ${response.status} ${text}`);
			}

			this.logger.debug('Teams notification sent');
		} catch (error) {
			this.logger.error({ error }, 'Failed to send Teams notification');
		}
	}

	private createWarningCard(warning: WarningNotification): AdaptiveCard {
		const color = TEAMS_COLORS[warning.severity];

		return {
			type: 'message',
			attachments: [
				{
					contentType: 'application/vnd.microsoft.card.adaptive',
					content: {
						$schema: 'http://adaptivecards.io/schemas/adaptive-card.json',
						type: 'AdaptiveCard',
						version: '1.4',
						body: [
							{
								type: 'Container',
								style: color,
								items: [
									{
										type: 'TextBlock',
										text: `${SEVERITY_EMOJIS[warning.severity]} ${warning.severity}: ${warning.category}`,
										weight: 'bolder',
										size: 'large',
										color: warning.severity === 'CRITICAL' || warning.severity === 'ERROR' ? 'attention' : 'default',
									},
								],
							},
							{
								type: 'FactSet',
								facts: [
									{ title: 'Source', value: warning.source },
									{ title: 'Instance', value: this.config.instanceId },
									{ title: 'Time', value: warning.timestamp.toISOString() },
								],
							},
							{
								type: 'TextBlock',
								text: 'Message',
								weight: 'bolder',
								spacing: 'medium',
							},
							{
								type: 'TextBlock',
								text: warning.message,
								wrap: true,
								fontType: 'monospace',
							},
						],
						msteams: {
							width: 'full',
						},
					},
				},
			],
		};
	}

	private createBatchCard(warnings: WarningNotification[]): AdaptiveCard {
		// Group by category
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

		const categoryContainers = Object.entries(groupedByCategory).map(
			([category, categoryWarnings]) => ({
				type: 'Container' as const,
				items: [
					{
						type: 'TextBlock' as const,
						text: `**${category}** (${categoryWarnings.length})`,
						weight: 'bolder' as const,
						spacing: 'medium' as const,
					},
					...categoryWarnings.slice(0, 5).map((w) => ({
						type: 'Container' as const,
						style: TEAMS_COLORS[w.severity],
						items: [
							{
								type: 'TextBlock' as const,
								text: `${SEVERITY_EMOJIS[w.severity]} **${w.severity}** - ${w.source}`,
								size: 'small' as const,
							},
							{
								type: 'TextBlock' as const,
								text: w.message.substring(0, 200) + (w.message.length > 200 ? '...' : ''),
								wrap: true,
								size: 'small' as const,
								fontType: 'monospace' as const,
							},
						],
					})),
					...(categoryWarnings.length > 5
						? [
								{
									type: 'TextBlock' as const,
									text: `_... and ${categoryWarnings.length - 5} more_`,
									size: 'small' as const,
									isSubtle: true,
								},
							]
						: []),
				],
			}),
		);

		return {
			type: 'message',
			attachments: [
				{
					contentType: 'application/vnd.microsoft.card.adaptive',
					content: {
						$schema: 'http://adaptivecards.io/schemas/adaptive-card.json',
						type: 'AdaptiveCard',
						version: '1.4',
						body: [
							{
								type: 'Container',
								style: 'warning',
								items: [
									{
										type: 'TextBlock',
										text: `${SEVERITY_EMOJIS.WARNING} Batch Notification: ${warnings.length} Warnings`,
										weight: 'bolder',
										size: 'large',
									},
								],
							},
							{
								type: 'FactSet',
								facts: [
									{ title: 'Instance', value: this.config.instanceId },
									{ title: 'Categories', value: Object.keys(groupedByCategory).join(', ') },
									{ title: 'Total Warnings', value: String(warnings.length) },
								],
							},
							...categoryContainers,
						],
						msteams: {
							width: 'full',
						},
					},
				},
			],
		};
	}

	private createSystemEventCard(event: SystemEventNotification): AdaptiveCard {
		const facts = [
			{ title: 'Event Type', value: event.eventType },
			{ title: 'Instance', value: this.config.instanceId },
			{ title: 'Time', value: event.timestamp.toISOString() },
		];

		if (event.metadata) {
			Object.entries(event.metadata).forEach(([key, value]) => {
				facts.push({ title: key, value: String(value) });
			});
		}

		return {
			type: 'message',
			attachments: [
				{
					contentType: 'application/vnd.microsoft.card.adaptive',
					content: {
						$schema: 'http://adaptivecards.io/schemas/adaptive-card.json',
						type: 'AdaptiveCard',
						version: '1.4',
						body: [
							{
								type: 'Container',
								style: 'accent',
								items: [
									{
										type: 'TextBlock',
										text: `${SEVERITY_EMOJIS.INFO} System Event: ${event.eventType}`,
										weight: 'bolder',
										size: 'large',
									},
								],
							},
							{
								type: 'FactSet',
								facts,
							},
							{
								type: 'TextBlock',
								text: 'Message',
								weight: 'bolder',
								spacing: 'medium',
							},
							{
								type: 'TextBlock',
								text: event.message,
								wrap: true,
								fontType: 'monospace',
							},
						],
						msteams: {
							width: 'full',
						},
					},
				},
			],
		};
	}
}

/**
 * Adaptive Card type definitions for Teams
 */
interface AdaptiveCard {
	type: 'message';
	attachments: Array<{
		contentType: 'application/vnd.microsoft.card.adaptive';
		content: {
			$schema: string;
			type: 'AdaptiveCard';
			version: string;
			body: unknown[];
			msteams?: {
				width: 'full';
			};
		};
	}>;
}
