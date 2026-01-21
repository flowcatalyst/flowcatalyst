/**
 * Delete User Command
 *
 * Input data for deleting a user.
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to delete a user.
 */
export interface DeleteUserCommand extends Command {
	/** User ID to delete */
	readonly userId: string;
}
