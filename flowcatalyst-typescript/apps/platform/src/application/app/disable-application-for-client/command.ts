/**
 * Disable Application For Client Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to disable an application for a client.
 */
export interface DisableApplicationForClientCommand extends Command {
	readonly applicationId: string;
	readonly clientId: string;
}
