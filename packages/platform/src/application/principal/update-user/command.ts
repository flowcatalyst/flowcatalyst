/**
 * Update User Command
 *
 * Input data for updating an existing user.
 */

import type { Command } from "@flowcatalyst/application";

/**
 * Command to update an existing user.
 */
export interface UpdateUserCommand extends Command {
	/** User ID to update */
	readonly userId: string;

	/** New display name */
	readonly name: string;

	/** New user scope (ANCHOR, PARTNER, CLIENT) — optional, only updated if provided */
	readonly scope?: string | undefined;

	/** Client ID for CLIENT scope users — optional, only updated if provided */
	readonly clientId?: string | null | undefined;
}
