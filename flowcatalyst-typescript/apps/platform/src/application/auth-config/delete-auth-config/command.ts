/**
 * Delete Auth Config Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to delete an auth config.
 */
export interface DeleteAuthConfigCommand extends Command {
	readonly authConfigId: string;
}
