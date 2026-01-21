/**
 * Update Role Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to update an existing role.
 */
export interface UpdateRoleCommand extends Command {
	readonly roleId: string;
	/** Human-readable display name */
	readonly displayName: string;
	readonly description?: string | null;
	readonly permissions?: readonly string[];
	/** If true, this role syncs to IDPs configured for client-managed roles */
	readonly clientManaged?: boolean;
}
