/**
 * Delete Service Account Use Case
 *
 * Deletes a service account and its linked OAuth client.
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
	OAuthClientRepository,
} from "../../../infrastructure/persistence/index.js";
import { ServiceAccountDeleted } from "../../../domain/index.js";

import type { DeleteServiceAccountCommand } from "./command.js";

/**
 * Dependencies for DeleteServiceAccountUseCase.
 */
export interface DeleteServiceAccountUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly oauthClientRepository: OAuthClientRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeleteServiceAccountUseCase.
 */
export function createDeleteServiceAccountUseCase(
	deps: DeleteServiceAccountUseCaseDeps,
): UseCase<DeleteServiceAccountCommand, ServiceAccountDeleted> {
	const { principalRepository, oauthClientRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeleteServiceAccountCommand,
			context: ExecutionContext,
		): Promise<Result<ServiceAccountDeleted>> {
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

			// Find and delete linked OAuth client(s)
			const allOAuthClients = await oauthClientRepository.findAll();
			for (const oauthClient of allOAuthClients) {
				if (oauthClient.serviceAccountPrincipalId === principal.id) {
					await oauthClientRepository.deleteById(oauthClient.id);
				}
			}

			// Create domain event
			const event = new ServiceAccountDeleted(context, {
				serviceAccountId: principal.id,
				code: principal.serviceAccount.code,
			});

			// Delete principal atomically
			return unitOfWork.commitDelete(principal, event, command);
		},
	};
}
