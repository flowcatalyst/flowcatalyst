/**
 * Regenerate Signing Secret Use Case
 *
 * Generates a new webhook signing secret for a service account.
 * The new plaintext secret is only available at regeneration time.
 */

import type { UseCase } from "@flowcatalyst/application";
import {
	validateRequired,
	Result,
	type ExecutionContext,
	UseCaseError,
} from "@flowcatalyst/application";
import type { UnitOfWork } from "@flowcatalyst/domain-core";
import type { EncryptionService } from "@flowcatalyst/platform-crypto";

import type { PrincipalRepository } from "../../../infrastructure/persistence/index.js";
import {
	updatePrincipal,
	generateSigningSecret,
	SigningSecretRegenerated,
} from "../../../domain/index.js";

import type { RegenerateSigningSecretCommand } from "./command.js";

/**
 * Dependencies for RegenerateSigningSecretUseCase.
 */
export interface RegenerateSigningSecretUseCaseDeps {
	readonly principalRepository: PrincipalRepository;
	readonly encryptionService: EncryptionService;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the RegenerateSigningSecretUseCase.
 */
export function createRegenerateSigningSecretUseCase(
	deps: RegenerateSigningSecretUseCaseDeps,
): UseCase<RegenerateSigningSecretCommand, SigningSecretRegenerated> {
	const { principalRepository, encryptionService, unitOfWork } = deps;

	return {
		async execute(
			command: RegenerateSigningSecretCommand,
			context: ExecutionContext,
		): Promise<Result<SigningSecretRegenerated>> {
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

			// Generate new signing secret
			const newSecret = generateSigningSecret();

			// Encrypt for storage
			const encryptResult = encryptionService.encrypt(newSecret);
			if (encryptResult.isErr()) {
				return Result.failure(
					UseCaseError.businessRule(
						"ENCRYPTION_FAILED",
						"Failed to encrypt signing secret",
					),
				);
			}

			// Update service account data with new secret ref
			const updatedPrincipal = updatePrincipal(principal, {
				serviceAccount: {
					...principal.serviceAccount,
					whSigningSecretRef: encryptResult.value,
					whCredentialsRegeneratedAt: new Date(),
				},
			});

			// Create domain event
			const event = new SigningSecretRegenerated(context, {
				serviceAccountId: principal.id,
				code: principal.serviceAccount.code,
			});

			// Commit
			return unitOfWork.commit(updatedPrincipal, event, command);
		},
	};
}
