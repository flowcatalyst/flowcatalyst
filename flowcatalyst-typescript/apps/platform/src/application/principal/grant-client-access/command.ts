/**
 * Grant Client Access Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to grant client access to a user.
 */
export interface GrantClientAccessCommand extends Command {
	readonly userId: string;
	readonly clientId: string;
}
