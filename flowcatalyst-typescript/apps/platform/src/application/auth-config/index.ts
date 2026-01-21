/**
 * Auth Config Application Layer
 *
 * Use cases for managing client authentication configurations.
 */

// Create Auth Config
export {
	type CreateInternalAuthConfigCommand,
	type CreateOidcAuthConfigCommand,
	createCreateInternalAuthConfigUseCase,
	createCreateOidcAuthConfigUseCase,
	type CreateAuthConfigUseCaseDeps,
} from './create-auth-config/index.js';

// Update Auth Config
export {
	type UpdateOidcSettingsCommand,
	type UpdateConfigTypeCommand,
	type UpdateAdditionalClientsCommand,
	type UpdateGrantedClientsCommand,
	createUpdateOidcSettingsUseCase,
	createUpdateConfigTypeUseCase,
	createUpdateAdditionalClientsUseCase,
	createUpdateGrantedClientsUseCase,
	type UpdateAuthConfigUseCaseDeps,
} from './update-auth-config/index.js';

// Delete Auth Config
export {
	type DeleteAuthConfigCommand,
	createDeleteAuthConfigUseCase,
	type DeleteAuthConfigUseCaseDeps,
} from './delete-auth-config/index.js';
