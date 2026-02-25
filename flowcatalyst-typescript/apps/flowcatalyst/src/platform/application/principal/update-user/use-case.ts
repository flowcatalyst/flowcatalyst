/**
 * Update User Use Case
 *
 * Updates an existing user's information.
 * If the user's email domain has a configured EmailDomainMapping or AnchorDomain,
 * the scope/clientId from that configuration takes precedence over API-provided values.
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	Result,
	type ExecutionContext,
	UseCaseError,
} from "@flowcatalyst/application";
import type { UnitOfWork } from "@flowcatalyst/domain-core";

import type {
	PrincipalRepository,
	EmailDomainMappingRepository,
	AnchorDomainRepository,
} from "../../../infrastructure/persistence/index.js";
import {
	updatePrincipal,
	UserUpdated,
	PrincipalType,
	PrincipalScope,
	resolveScopeForEmail,
} from "../../../domain/index.js";

import type { UpdateUserCommand } from "./command.js";

/**
 * Dependencies for UpdateUserUseCase.
 */
export interface UpdateUserUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly emailDomainMappingRepository: EmailDomainMappingRepository;
	readonly anchorDomainRepository: AnchorDomainRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the UpdateUserUseCase.
 */
export function createUpdateUserUseCase(
	deps: UpdateUserUseCaseDeps,
): UseCase<UpdateUserCommand, UserUpdated> {
	const {
		principalRepository,
		emailDomainMappingRepository,
		anchorDomainRepository,
		unitOfWork,
	} = deps;

	return {
		async execute(
			command: UpdateUserCommand,
			context: ExecutionContext,
		): Promise<Result<UserUpdated>> {
			// Validate userId
			const userIdResult = validateRequired(
				command.userId,
				"userId",
				"USER_ID_REQUIRED",
			);
			if (Result.isFailure(userIdResult)) {
				return userIdResult;
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

			// Find the user
			const principal = await principalRepository.findById(command.userId);
			if (!principal) {
				return Result.failure(
					UseCaseError.notFound(
						"USER_NOT_FOUND",
						`User not found: ${command.userId}`,
					),
				);
			}

			// Verify it's a USER type
			if (principal.type !== PrincipalType.USER) {
				return Result.failure(
					UseCaseError.businessRule("NOT_A_USER", "Principal is not a user", {
						type: principal.type,
					}),
				);
			}

			// Validate scope if provided
			const commandScope =
				command.scope !== undefined
					? (command.scope as PrincipalScope)
					: undefined;
			if (
				commandScope !== undefined &&
				!Object.values(PrincipalScope).includes(commandScope)
			) {
				return Result.failure(
					UseCaseError.validation(
						"INVALID_SCOPE",
						`Invalid scope: ${command.scope}. Must be one of: ${Object.values(PrincipalScope).join(", ")}`,
						{ field: "scope" },
					),
				);
			}

			// Resolve effective scope: email domain config overrides API-provided values
			const emailDomain = principal.userIdentity?.emailDomain;
			let effectiveScope = commandScope;
			let effectiveClientId = command.clientId;

			if (emailDomain) {
				const mapping =
					await emailDomainMappingRepository.findByEmailDomain(emailDomain);
				const isAnchorDomain =
					!mapping &&
					(await anchorDomainRepository.existsByDomain(emailDomain));

				if (mapping || isAnchorDomain) {
					const resolved = resolveScopeForEmail({
						mapping,
						isAnchorDomain,
						fallbackScope: commandScope,
						fallbackClientId: command.clientId ?? null,
					});
					effectiveScope = resolved.scope;
					effectiveClientId = resolved.clientId;
				}
			}

			// Build updates object — only include fields that were provided or overridden
			const updates: Parameters<typeof updatePrincipal>[1] = {
				name: command.name,
				...(effectiveScope !== undefined && { scope: effectiveScope }),
				...(effectiveClientId !== undefined && {
					clientId: effectiveClientId ?? null,
				}),
			};

			// Update the principal
			const updatedPrincipal = updatePrincipal(principal, updates);

			// Create domain event
			const event = new UserUpdated(context, {
				userId: principal.id,
				name: command.name,
				previousName: principal.name,
				scope: updatedPrincipal.scope,
				previousScope: principal.scope,
				clientId: updatedPrincipal.clientId,
				previousClientId: principal.clientId,
			});

			// Commit atomically
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
