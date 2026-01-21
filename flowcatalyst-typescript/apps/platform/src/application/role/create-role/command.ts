/**
 * Create Role Command
 */

import type { Command } from '@flowcatalyst/application';
import type { RoleSource } from '../../../domain/index.js';

/**
 * Command to create a new role.
 */
export interface CreateRoleCommand extends Command {
	/** The application this role belongs to (optional) */
	readonly applicationId?: string | null;
	/** The application code (optional) */
	readonly applicationCode?: string | null;
	/** Short role name (will be prefixed with applicationCode if provided) */
	readonly shortName: string;
	/** Human-readable display name */
	readonly displayName: string;
	readonly description?: string | null;
	readonly permissions?: readonly string[];
	/** Source of this role definition (defaults to DATABASE) */
	readonly source?: RoleSource;
	/** If true, this role syncs to IDPs configured for client-managed roles */
	readonly clientManaged?: boolean;
}
