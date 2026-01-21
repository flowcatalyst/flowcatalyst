/**
 * Create User Use Case
 *
 * Creates a new user principal in the system.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, validateEmail, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';
import type { PasswordService } from '@flowcatalyst/platform-crypto';

import type { PrincipalRepository, AnchorDomainRepository } from '../../../infrastructure/persistence/index.js';
import {
	createUserPrincipal,
	createUserIdentity,
	extractEmailDomain,
	UserCreated,
	IdpType,
	UserScope,
} from '../../../domain/index.js';

import type { CreateUserCommand } from './command.js';

/**
 * Dependencies for CreateUserUseCase.
 */
export interface CreateUserUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly anchorDomainRepository: AnchorDomainRepository;
	readonly passwordService: PasswordService;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the CreateUserUseCase.
 */
export function createCreateUserUseCase(deps: CreateUserUseCaseDeps): UseCase<CreateUserCommand, UserCreated> {
	const { principalRepository, anchorDomainRepository, passwordService, unitOfWork } = deps;

	return {
		async execute(command: CreateUserCommand, context: ExecutionContext): Promise<Result<UserCreated>> {
			// Validate email
			const emailResult = validateRequired(command.email, 'email', 'EMAIL_REQUIRED');
			if (Result.isFailure(emailResult)) {
				return emailResult;
			}

			const emailFormatResult = validateEmail(command.email);
			if (Result.isFailure(emailFormatResult)) {
				return emailFormatResult;
			}

			// Validate name
			const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
			if (Result.isFailure(nameResult)) {
				return nameResult;
			}

			// Check if email already exists
			const emailExists = await principalRepository.existsByEmail(command.email);
			if (emailExists) {
				return Result.failure(
					UseCaseError.businessRule('EMAIL_EXISTS', 'Email already exists', { email: command.email }),
				);
			}

			// Extract domain from email
			const emailDomain = extractEmailDomain(command.email);

			// Check if anchor domain user
			const isAnchorUser = await anchorDomainRepository.existsByDomain(emailDomain);

			// Determine scope based on anchor domain
			const scope: UserScope = isAnchorUser ? UserScope.ANCHOR : UserScope.CLIENT;

			// For Phase 1, we only support INTERNAL auth
			// OIDC support will be added in Phase 4
			const idpType: IdpType = IdpType.INTERNAL;

			// Validate and hash password for INTERNAL auth
			let passwordHash: string | null = null;
			if (idpType === IdpType.INTERNAL) {
				if (!command.password) {
					return Result.failure(
						UseCaseError.validation('PASSWORD_REQUIRED', 'Password is required for internal authentication'),
					);
				}

				// Validate password strength
				const passwordValidation = passwordService.validateComplexity(command.password);
				if (passwordValidation.isErr()) {
					const err = passwordValidation.error;
					return Result.failure(UseCaseError.validation('INVALID_PASSWORD', err.message));
				}

				// Hash the password
				const hashResult = await passwordService.hash(command.password);
				if (hashResult.isErr()) {
					return Result.failure(UseCaseError.businessRule('HASH_FAILED', hashResult.error.message));
				}
				passwordHash = hashResult.value;
			}

			// Create user identity
			const userIdentity = createUserIdentity({
				email: command.email,
				idpType,
				passwordHash,
			});

			// Create principal
			const principal = createUserPrincipal({
				name: command.name,
				scope,
				clientId: command.clientId,
				userIdentity,
			});

			// Create domain event
			const event = new UserCreated(context, {
				userId: principal.id,
				email: userIdentity.email,
				emailDomain: userIdentity.emailDomain,
				name: principal.name,
				scope,
				clientId: principal.clientId,
				idpType,
				isAnchorUser,
			});

			// Commit atomically
			return unitOfWork.commit(principal, event, command);
		},
	};
}
