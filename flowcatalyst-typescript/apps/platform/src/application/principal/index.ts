/**
 * Principal Application Layer
 *
 * Use cases for managing principals (users).
 */

// Create User
export {
	type CreateUserCommand,
	createCreateUserUseCase,
	type CreateUserUseCaseDeps,
} from './create-user/index.js';

// Update User
export {
	type UpdateUserCommand,
	createUpdateUserUseCase,
	type UpdateUserUseCaseDeps,
} from './update-user/index.js';

// Activate User
export {
	type ActivateUserCommand,
	createActivateUserUseCase,
	type ActivateUserUseCaseDeps,
} from './activate-user/index.js';

// Deactivate User
export {
	type DeactivateUserCommand,
	createDeactivateUserUseCase,
	type DeactivateUserUseCaseDeps,
} from './deactivate-user/index.js';

// Delete User
export {
	type DeleteUserCommand,
	createDeleteUserUseCase,
	type DeleteUserUseCaseDeps,
} from './delete-user/index.js';

// Assign Roles
export {
	type AssignRolesCommand,
	createAssignRolesUseCase,
	type AssignRolesUseCaseDeps,
} from './assign-roles/index.js';

// Grant Client Access
export {
	type GrantClientAccessCommand,
	createGrantClientAccessUseCase,
	type GrantClientAccessUseCaseDeps,
} from './grant-client-access/index.js';

// Revoke Client Access
export {
	type RevokeClientAccessCommand,
	createRevokeClientAccessUseCase,
	type RevokeClientAccessUseCaseDeps,
} from './revoke-client-access/index.js';
