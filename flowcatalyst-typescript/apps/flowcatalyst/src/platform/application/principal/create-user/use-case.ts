/**
 * Create User Use Case
 *
 * Creates a new user principal in the system.
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	validateEmail,
	Result,
	type ExecutionContext,
	UseCaseError,
} from "@flowcatalyst/application";
import type { UnitOfWork } from "@flowcatalyst/domain-core";
import type { PasswordService } from "@flowcatalyst/platform-crypto";

import type {
	PrincipalRepository,
	AnchorDomainRepository,
	EmailDomainMappingRepository,
	IdentityProviderRepository,
} from "../../../infrastructure/persistence/index.js";
import {
	createUserPrincipal,
	createUserIdentity,
	extractEmailDomain,
	UserCreated,
	IdpType,
	PrincipalScope,
} from "../../../domain/index.js";

import type { CreateUserCommand } from "./command.js";

/**
 * Dependencies for CreateUserUseCase.
 */
export interface CreateUserUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly anchorDomainRepository: AnchorDomainRepository;
	readonly emailDomainMappingRepository: EmailDomainMappingRepository;
	readonly identityProviderRepository: IdentityProviderRepository;
	readonly passwordService: PasswordService;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the CreateUserUseCase.
 */
export function createCreateUserUseCase(
	deps: CreateUserUseCaseDeps,
): UseCase<CreateUserCommand, UserCreated> {
	const {
		principalRepository,
		anchorDomainRepository,
		emailDomainMappingRepository,
		identityProviderRepository,
		passwordService,
		unitOfWork,
	} = deps;

	return {
		async execute(
			command: CreateUserCommand,
			context: ExecutionContext,
		): Promise<Result<UserCreated>> {
			// Validate email
			const emailResult = validateRequired(
				command.email,
				"email",
				"EMAIL_REQUIRED",
			);
			if (Result.isFailure(emailResult)) {
				return emailResult;
			}

			const emailFormatResult = validateEmail(command.email);
			if (Result.isFailure(emailFormatResult)) {
				return emailFormatResult;
			}

			// Validate name
			const nameResult = validateRequired(
				command.name,
				"name",
				"NAME_REQUIRED",
			);
			if (Result.isFailure(nameResult)) {
				return nameResult;
			}

			// Check if email already exists
			const emailExists = await principalRepository.existsByEmail(
				command.email,
			);
			if (emailExists) {
				return Result.failure(
					UseCaseError.businessRule("EMAIL_EXISTS", "Email already exists", {
						email: command.email,
					}),
				);
			}

			// Extract domain from email
			const emailDomain = extractEmailDomain(command.email);

			// Determine scope and IDP type from email domain mapping
			let scope: PrincipalScope = PrincipalScope.CLIENT;
			let idpType: IdpType = IdpType.INTERNAL;
			const mapping =
				await emailDomainMappingRepository.findByEmailDomain(emailDomain);
			if (mapping) {
				// Use the scope configured on the email domain mapping
				scope = mapping.scopeType as PrincipalScope;

				const idp = await identityProviderRepository.findById(
					mapping.identityProviderId,
				);
				if (idp && idp.type === "OIDC") {
					idpType = IdpType.OIDC;
				}
			} else {
				// Fall back to legacy anchor domain check
				const isAnchorDomain =
					await anchorDomainRepository.existsByDomain(emailDomain);
				if (isAnchorDomain) {
					scope = PrincipalScope.ANCHOR;
				}
			}

			const isAnchorUser = scope === PrincipalScope.ANCHOR;

			// Validate and hash password for INTERNAL auth, or reject for OIDC
			let passwordHash: string | null = null;
			if (idpType === IdpType.OIDC) {
				// OIDC users should not have a password
				if (command.password) {
					return Result.failure(
						UseCaseError.validation(
							"PASSWORD_NOT_ALLOWED",
							"Password is not allowed for OIDC-authenticated users. Authentication is handled by the external identity provider.",
						),
					);
				}
			} else {
				// INTERNAL auth - require and validate password
				if (!command.password) {
					return Result.failure(
						UseCaseError.validation(
							"PASSWORD_REQUIRED",
							"Password is required for internal authentication",
						),
					);
				}

				// Validate password strength
				const passwordValidation = passwordService.validateComplexity(
					command.password,
				);
				if (passwordValidation.isErr()) {
					const err = passwordValidation.error;
					return Result.failure(
						UseCaseError.validation("INVALID_PASSWORD", err.message),
					);
				}

				// Hash the password
				const hashResult = await passwordService.hash(command.password);
				if (hashResult.isErr()) {
					return Result.failure(
						UseCaseError.businessRule("HASH_FAILED", hashResult.error.message),
					);
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
