/**
 * Principal Domain
 *
 * Exports for the principal aggregate and related types.
 */

// Types
export { PrincipalType } from "./principal-type.js";
export { PrincipalScope } from "./principal-scope.js";
export { IdpType } from "./idp-type.js";

// Entities
export {
	type Principal,
	type NewPrincipal,
	createUserPrincipal,
	createServicePrincipal,
	getRoleNames,
	hasRole,
	hasApplicationAccess,
	updatePrincipal,
	assignRoles,
} from "./principal.js";

export {
	type UserIdentity,
	createUserIdentity,
	extractEmailDomain,
} from "./user-identity.js";

export {
	type RoleAssignment,
	createRoleAssignment,
} from "./role-assignment.js";

export {
	type ClientAccessGrant,
	type NewClientAccessGrant,
	createClientAccessGrant,
} from "./client-access-grant.js";

// Scope resolution
export {
	type ResolvedScope,
	resolveScopeForEmail,
} from "./scope-resolution.js";

// Events
export {
	UserCreated,
	UserUpdated,
	UserActivated,
	UserDeactivated,
	UserDeleted,
	RolesAssigned,
	ApplicationAccessAssigned,
	ClientAccessGranted,
	ClientAccessRevoked,
	PrincipalsSynced,
	type UserCreatedData,
	type UserUpdatedData,
	type UserActivatedData,
	type UserDeactivatedData,
	type UserDeletedData,
	type RolesAssignedData,
	type ApplicationAccessAssignedData,
	type ClientAccessGrantedData,
	type ClientAccessRevokedData,
	type PrincipalsSyncedData,
} from "./events.js";
