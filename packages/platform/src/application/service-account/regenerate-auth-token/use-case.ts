/**
 * Regenerate Auth Token Use Case
 *
 * Generates a new webhook auth token for a service account.
 * The new plaintext token is only available at regeneration time.
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	Result,
	type ExecutionContext,
	UseCaseError,
} from "@flowcatalyst/application";
import type { UnitOfWork } from "@flowcatalyst/domain";
import type { EncryptionService } from "@flowcatalyst/platform-crypto";

import type { PrincipalRepository } from "../../../infrastructure/persistence/index.js";
import {
	updatePrincipal,
	generateAuthToken,
	AuthTokenRegenerated,
} from "../../../domain/index.js";

import type { RegenerateAuthTokenCommand } from "./command.js";

/**
 * Dependencies for RegenerateAuthTokenUseCase.
 */
export interface RegenerateAuthTokenUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly encryptionService: EncryptionService;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the RegenerateAuthTokenUseCase.
 */
export function createRegenerateAuthTokenUseCase(
	deps: RegenerateAuthTokenUseCaseDeps,
): UseCase<RegenerateAuthTokenCommand, AuthTokenRegenerated> {
	const { principalRepository, encryptionService, unitOfWork } = deps;

	return {
		async execute(
			command: RegenerateAuthTokenCommand,
			context: ExecutionContext,
		): Promise<Result<AuthTokenRegenerated>> {
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

			// Generate new auth token
			const newToken = generateAuthToken();

			// Encrypt for storage
			const encryptResult = encryptionService.encrypt(newToken);
			if (encryptResult.isErr()) {
				return Result.failure(
					UseCaseError.businessRule(
						"ENCRYPTION_FAILED",
						"Failed to encrypt auth token",
					),
				);
			}

			// Update service account data with new token ref
			const updatedPrincipal = updatePrincipal(principal, {
				serviceAccount: {
					...principal.serviceAccount,
					whAuthTokenRef: encryptResult.value,
					whCredentialsRegeneratedAt: new Date(),
				},
			});

			// Create domain event
			const event = new AuthTokenRegenerated(context, {
				serviceAccountId: principal.id,
				code: principal.serviceAccount.code,
			});

			// Commit
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
