/**
 * Delete Client Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to delete a client.
 */
export interface DeleteClientCommand extends Command {
	readonly clientId: string;
}
