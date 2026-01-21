/**
 * Update Auth Config Use Cases
 */

export type {
	UpdateOidcSettingsCommand,
	UpdateConfigTypeCommand,
	UpdateAdditionalClientsCommand,
	UpdateGrantedClientsCommand,
} from './command.js';
export {
	createUpdateOidcSettingsUseCase,
	createUpdateConfigTypeUseCase,
	createUpdateAdditionalClientsUseCase,
	createUpdateGrantedClientsUseCase,
	type UpdateAuthConfigUseCaseDeps,
} from './use-case.js';
