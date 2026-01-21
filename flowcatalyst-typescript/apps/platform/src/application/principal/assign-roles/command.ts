/**
 * Assign Roles Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to assign roles to a user.
 */
export interface AssignRolesCommand extends Command {
	readonly userId: string;
	readonly roles: readonly string[];
}
