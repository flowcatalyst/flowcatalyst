/**
 * Delete Role Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to delete a role.
 */
export interface DeleteRoleCommand extends Command {
	readonly roleId: string;
}
