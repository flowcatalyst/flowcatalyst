/**
 * Delete Service Account Command
 */

import type { Command } from "@flowcatalyst/application";

/**
 * Command to delete a service account and its linked OAuth client.
 */
export interface DeleteServiceAccountCommand extends Command {
	/** Principal ID of the service account */
	readonly serviceAccountId: string;
}
