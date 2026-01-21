/**
 * Principal Domain
 *
 * Exports for the principal aggregate and related types.
 */

// Types
export { PrincipalType } from './principal-type.js';
export { UserScope } from './user-scope.js';
export { IdpType } from './idp-type.js';

// Entities
export {
	type Principal,
	type NewPrincipal,
	createUserPrincipal,
	getRoleNames,
	hasRole,
	updatePrincipal,
	assignRoles,
} from './principal.js';

export {
	type UserIdentity,
	createUserIdentity,
	extractEmailDomain,
} from './user-identity.js';

export {
	type RoleAssignment,
	createRoleAssignment,
} from './role-assignment.js';

export {
	type ClientAccessGrant,
	type NewClientAccessGrant,
	createClientAccessGrant,
} from './client-access-grant.js';

// Events
export {
	UserCreated,
	UserUpdated,
	UserActivated,
	UserDeactivated,
	UserDeleted,
	RolesAssigned,
	ClientAccessGranted,
	ClientAccessRevoked,
	type UserCreatedData,
	type UserUpdatedData,
	type UserActivatedData,
	type UserDeactivatedData,
	type UserDeletedData,
	type RolesAssignedData,
	type ClientAccessGrantedData,
	type ClientAccessRevokedData,
} from './events.js';
