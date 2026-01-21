/**
 * Delete OAuth Client Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to delete an OAuth client.
 */
export interface DeleteOAuthClientCommand extends Command {
	readonly oauthClientId: string;
}
