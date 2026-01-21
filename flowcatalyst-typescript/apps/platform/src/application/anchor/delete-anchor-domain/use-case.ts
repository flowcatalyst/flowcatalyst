/**
 * Delete Anchor Domain Use Case
 *
 * Deletes an anchor domain from the system.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { AnchorDomainRepository } from '../../../infrastructure/persistence/index.js';
import { AnchorDomainDeleted } from '../../../domain/index.js';

import type { DeleteAnchorDomainCommand } from './command.js';

/**
 * Dependencies for DeleteAnchorDomainUseCase.
 */
export interface DeleteAnchorDomainUseCaseDeps {
	readonly anchorDomainRepository: AnchorDomainRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the DeleteAnchorDomainUseCase.
 */
export function createDeleteAnchorDomainUseCase(
	deps: DeleteAnchorDomainUseCaseDeps,
): UseCase<DeleteAnchorDomainCommand, AnchorDomainDeleted> {
	const { anchorDomainRepository, unitOfWork } = deps;

	return {
		async execute(
			command: DeleteAnchorDomainCommand,
			context: ExecutionContext,
		): Promise<Result<AnchorDomainDeleted>> {
			// Validate anchor domain ID
			const idResult = validateRequired(command.anchorDomainId, 'anchorDomainId', 'ANCHOR_DOMAIN_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Find anchor domain
			const anchorDomain = await anchorDomainRepository.findById(command.anchorDomainId);
			if (!anchorDomain) {
				return Result.failure(UseCaseError.notFound('ANCHOR_DOMAIN_NOT_FOUND', 'Anchor domain not found'));
			}

			// Create domain event
			const event = new AnchorDomainDeleted(context, {
				anchorDomainId: anchorDomain.id,
				domain: anchorDomain.domain,
			});

			// Delete atomically
			return unitOfWork.commitDelete(anchorDomain, event, command);
		},
	};
}
