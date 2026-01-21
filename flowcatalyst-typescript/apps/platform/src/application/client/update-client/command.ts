/**
 * Update Client Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to update a client.
 */
export interface UpdateClientCommand extends Command {
	readonly clientId: string;
	readonly name: string;
}
