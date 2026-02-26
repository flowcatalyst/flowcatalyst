/**
 * Update Service Account Use Case
 *
 * Updates a service account's name and/or description.
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	Result,
	type ExecutionContext,
	UseCaseError,
} from "@flowcatalyst/application";
import type { UnitOfWork } from "@flowcatalyst/domain";

import type { PrincipalRepository } from "../../../infrastructure/persistence/index.js";
import {
	updatePrincipal,
	ServiceAccountUpdated,
} from "../../../domain/index.js";

import type { UpdateServiceAccountCommand } from "./command.js";

/**
 * Dependencies for UpdateServiceAccountUseCase.
 */
export interface UpdateServiceAccountUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the UpdateServiceAccountUseCase.
 */
export function createUpdateServiceAccountUseCase(
	deps: UpdateServiceAccountUseCaseDeps,
): UseCase<UpdateServiceAccountCommand, ServiceAccountUpdated> {
	const { principalRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateServiceAccountCommand,
			context: ExecutionContext,
		): Promise<Result<ServiceAccountUpdated>> {
			// Validate serviceAccountId
			const idResult = validateRequired(
				command.serviceAccountId,
				"serviceAccountId",
				"SERVICE_ACCOUNT_ID_REQUIRED",
			);
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find the principal
			const principal = await principalRepository.findById(
				command.serviceAccountId,
			);
			if (!principal) {
				return Result.failure(
					UseCaseError.notFound(
						"SERVICE_ACCOUNT_NOT_FOUND",
						`Service account not found: ${command.serviceAccountId}`,
					),
				);
			}

			// Verify it's a SERVICE type
			if (principal.type !== "SERVICE" || !principal.serviceAccount) {
				return Result.failure(
					UseCaseError.businessRule(
						"NOT_A_SERVICE_ACCOUNT",
						"Principal is not a service account",
						{
							type: principal.type,
						},
					),
				);
			}

			// Build updates inline to satisfy readonly constraints
			const updatedPrincipal = updatePrincipal(principal, {
				...(command.name !== undefined ? { name: command.name } : {}),
				...(command.scope !== undefined ? { scope: command.scope } : {}),
				...(command.description !== undefined
					? {
							serviceAccount: {
								...principal.serviceAccount,
								description: command.description,
							},
						}
					: {}),
			});

			// Create domain event
			const event = new ServiceAccountUpdated(context, {
				serviceAccountId: principal.id,
				code: principal.serviceAccount.code,
			});

			// Commit
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
