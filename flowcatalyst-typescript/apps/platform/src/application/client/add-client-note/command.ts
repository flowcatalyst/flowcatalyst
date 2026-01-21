/**
 * Add Client Note Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to add a note to a client.
 */
export interface AddClientNoteCommand extends Command {
	readonly clientId: string;
	readonly category: string;
	readonly text: string;
}
