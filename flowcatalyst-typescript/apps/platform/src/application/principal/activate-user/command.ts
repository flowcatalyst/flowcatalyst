/**
 * Activate User Command
 *
 * Input data for activating a deactivated user.
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to activate a user.
 */
export interface ActivateUserCommand extends Command {
	/** User ID to activate */
	readonly userId: string;
}
