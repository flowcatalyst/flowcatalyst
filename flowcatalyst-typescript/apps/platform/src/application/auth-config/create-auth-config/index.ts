/**
 * Create Auth Config Use Case
 */

export type { CreateInternalAuthConfigCommand, CreateOidcAuthConfigCommand } from './command.js';
export {
	createCreateInternalAuthConfigUseCase,
	createCreateOidcAuthConfigUseCase,
	type CreateAuthConfigUseCaseDeps,
} from './use-case.js';
