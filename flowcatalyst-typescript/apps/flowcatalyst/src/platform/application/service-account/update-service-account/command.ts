/**
 * Update Service Account Command
 */

import type { Command } from "@flowcatalyst/application";
import type { PrincipalScope } from "../../../domain/index.js";

/**
 * Command to update a service account's name, description, and scope.
 */
export interface UpdateServiceAccountCommand extends Command {
	/** Principal ID of the service account */
	readonly serviceAccountId: string;

	/** New display name */
	readonly name?: string | undefined;

	/** New description */
	readonly description?: string | null | undefined;

	/** New access scope */
	readonly scope?: PrincipalScope | undefined;
}
