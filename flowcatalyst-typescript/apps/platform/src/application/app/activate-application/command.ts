/**
 * Activate Application Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to activate an application.
 */
export interface ActivateApplicationCommand extends Command {
	readonly applicationId: string;
}
