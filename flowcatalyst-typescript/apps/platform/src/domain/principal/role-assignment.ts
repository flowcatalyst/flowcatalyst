/**
 * Role Assignment
 *
 * Embedded role assignment within a Principal.
 */

/**
 * Represents a role assigned to a principal.
 */
export interface RoleAssignment {
	/** Name of the role */
	readonly roleName: string;

	/** How this role was assigned (e.g., "ADMIN_ASSIGNED", "DEFAULT_ROLE") */
	readonly assignmentSource: string;

	/** When the role was assigned */
	readonly assignedAt: Date;
}

/**
 * Create a new role assignment.
 */
export function createRoleAssignment(roleName: string, assignmentSource: string): RoleAssignment {
	return {
		roleName,
		assignmentSource,
		assignedAt: new Date(),
	};
}
