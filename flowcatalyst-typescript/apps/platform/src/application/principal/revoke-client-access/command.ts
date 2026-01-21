/**
 * Revoke Client Access Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to revoke client access from a user.
 */
export interface RevokeClientAccessCommand extends Command {
	readonly userId: string;
	readonly clientId: string;
}
