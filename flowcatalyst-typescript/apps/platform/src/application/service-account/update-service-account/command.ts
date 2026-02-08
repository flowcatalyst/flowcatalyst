/**
 * Update Service Account Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to update a service account's name and description.
 */
export interface UpdateServiceAccountCommand extends Command {
	/** Principal ID of the service account */
	readonly serviceAccountId: string;

	/** New display name */
	readonly name?: string | undefined;

	/** New description */
	readonly description?: string | null | undefined;
}
