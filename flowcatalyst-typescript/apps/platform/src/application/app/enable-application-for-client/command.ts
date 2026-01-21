/**
 * Enable Application For Client Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to enable an application for a client.
 */
export interface EnableApplicationForClientCommand extends Command {
	readonly applicationId: string;
	readonly clientId: string;
}
