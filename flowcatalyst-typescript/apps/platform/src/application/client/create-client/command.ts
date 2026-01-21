/**
 * Create Client Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to create a new client.
 */
export interface CreateClientCommand extends Command {
	readonly name: string;
	readonly identifier: string;
}
