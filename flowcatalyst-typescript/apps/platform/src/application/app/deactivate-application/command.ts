/**
 * Deactivate Application Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to deactivate an application.
 */
export interface DeactivateApplicationCommand extends Command {
	readonly applicationId: string;
}
