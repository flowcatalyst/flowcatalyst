/**
 * Update Anchor Domain Use Case
 *
 * Updates an existing anchor domain.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { AnchorDomainRepository } from '../../../infrastructure/persistence/index.js';
import { AnchorDomainUpdated } from '../../../domain/index.js';

import type { UpdateAnchorDomainCommand } from './command.js';

/**
 * Dependencies for UpdateAnchorDomainUseCase.
 */
export interface UpdateAnchorDomainUseCaseDeps {
	readonly anchorDomainRepository: AnchorDomainRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the UpdateAnchorDomainUseCase.
 */
export function createUpdateAnchorDomainUseCase(
	deps: UpdateAnchorDomainUseCaseDeps,
): UseCase<UpdateAnchorDomainCommand, AnchorDomainUpdated> {
	const { anchorDomainRepository, unitOfWork } = deps;

	return {
		async execute(
			command: UpdateAnchorDomainCommand,
			context: ExecutionContext,
		): Promise<Result<AnchorDomainUpdated>> {
			// Validate anchor domain ID
			const idResult = validateRequired(command.anchorDomainId, 'anchorDomainId', 'ANCHOR_DOMAIN_ID_REQUIRED');
			if (Result.isFailure(idResult)) {
				return idResult;
			}

			// Validate domain
			const domainResult = validateRequired(command.domain, 'domain', 'DOMAIN_REQUIRED');
			if (Result.isFailure(domainResult)) {
				return domainResult;
			}

			// Validate domain format
			const domainPattern = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/i;
			if (!domainPattern.test(command.domain)) {
				return Result.failure(UseCaseError.validation('INVALID_DOMAIN', 'Invalid domain format'));
			}

			// Find anchor domain
			const anchorDomain = await anchorDomainRepository.findById(command.anchorDomainId);
			if (!anchorDomain) {
				return Result.failure(UseCaseError.notFound('ANCHOR_DOMAIN_NOT_FOUND', 'Anchor domain not found'));
			}

			// Check if domain changed
			const normalizedDomain = command.domain.toLowerCase();
			if (anchorDomain.domain === normalizedDomain) {
				// No changes
				const event = new AnchorDomainUpdated(context, {
					anchorDomainId: anchorDomain.id,
					domain: anchorDomain.domain,
					previousDomain: anchorDomain.domain,
				});
				return unitOfWork.commit(anchorDomain, event, command);
			}

			// Check if new domain already exists
			const domainExists = await anchorDomainRepository.existsByDomain(normalizedDomain);
			if (domainExists) {
				return Result.failure(
					UseCaseError.businessRule('DOMAIN_EXISTS', 'Anchor domain already exists', {
						domain: normalizedDomain,
					}),
				);
			}

			// Update anchor domain
			const previousDomain = anchorDomain.domain;
			const updatedAnchorDomain = {
				...anchorDomain,
				domain: normalizedDomain,
			};

			// Create domain event
			const event = new AnchorDomainUpdated(context, {
				anchorDomainId: updatedAnchorDomain.id,
				domain: updatedAnchorDomain.domain,
				previousDomain,
			});

			// Commit atomically
			return unitOfWork.commit(updatedAnchorDomain, event, command);
		},
	};
}
