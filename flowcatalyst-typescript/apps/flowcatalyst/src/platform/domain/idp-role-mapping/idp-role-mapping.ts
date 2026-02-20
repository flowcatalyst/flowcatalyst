/**
 * IDP Role Mapping Domain Model
 *
 * Maps external IDP role names to internal platform role names.
 * This is a CRITICAL SECURITY control - only explicitly authorized
 * IDP roles can be mapped to internal roles.
 */

export interface IdpRoleMapping {
	readonly id: string;
	/** Role name from external IDP token (e.g., from realm_access.roles) */
	readonly idpRoleName: string;
	/** Internal platform role name this maps to */
	readonly internalRoleName: string;
	readonly createdAt: Date;
	readonly updatedAt: Date;
}
