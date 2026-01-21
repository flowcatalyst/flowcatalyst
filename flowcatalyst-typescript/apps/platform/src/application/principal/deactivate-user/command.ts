/**
 * Deactivate User Command
 *
 * Input data for deactivating an active user.
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to deactivate a user.
 */
export interface DeactivateUserCommand extends Command {
	/** User ID to deactivate */
	readonly userId: string;
}
