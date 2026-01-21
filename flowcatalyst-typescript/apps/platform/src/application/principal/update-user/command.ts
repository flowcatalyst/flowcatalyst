/**
 * Update User Command
 *
 * Input data for updating an existing user.
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to update an existing user.
 */
export interface UpdateUserCommand extends Command {
	/** User ID to update */
	readonly userId: string;

	/** New display name */
	readonly name: string;
}
