/**
 * Delete Application Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to delete an application.
 */
export interface DeleteApplicationCommand extends Command {
	readonly applicationId: string;
}
