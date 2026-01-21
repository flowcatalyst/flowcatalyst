/**
 * Update OAuth Client Use Cases
 */

export type { UpdateOAuthClientCommand, RegenerateOAuthClientSecretCommand } from './command.js';
export {
	createUpdateOAuthClientUseCase,
	createRegenerateOAuthClientSecretUseCase,
	type UpdateOAuthClientUseCaseDeps,
} from './use-case.js';
