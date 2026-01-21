/**
 * Create Anchor Domain Use Case
 *
 * Creates a new anchor domain in the system.
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, ExecutionContext, UseCaseError } from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';

import type { AnchorDomainRepository } from '../../../infrastructure/persistence/index.js';
import { createAnchorDomain, AnchorDomainCreated } from '../../../domain/index.js';

import type { CreateAnchorDomainCommand } from './command.js';

/**
 * Dependencies for CreateAnchorDomainUseCase.
 */
export interface CreateAnchorDomainUseCaseDeps {
	readonly anchorDomainRepository: AnchorDomainRepository;
	readonly unitOfWork: UnitOfWork;
}

/**
 * Create the CreateAnchorDomainUseCase.
 */
export function createCreateAnchorDomainUseCase(
	deps: CreateAnchorDomainUseCaseDeps,
): UseCase<CreateAnchorDomainCommand, AnchorDomainCreated> {
	const { anchorDomainRepository, unitOfWork } = deps;

	return {
		async execute(
			command: CreateAnchorDomainCommand,
			context: ExecutionContext,
		): Promise<Result<AnchorDomainCreated>> {
			// Validate domain
			const domainResult = validateRequired(command.domain, 'domain', 'DOMAIN_REQUIRED');
			if (Result.isFailure(domainResult)) {
				return domainResult;
			}

			// Validate domain format
			const domainPattern = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*$/i;
			if (!domainPattern.test(command.domain)) {
				return Result.failure(
					UseCaseError.validation('INVALID_DOMAIN', 'Invalid domain format'),
				);
			}

			// Check if domain already exists
			const domainExists = await anchorDomainRepository.existsByDomain(command.domain);
			if (domainExists) {
				return Result.failure(
					UseCaseError.businessRule('DOMAIN_EXISTS', 'Anchor domain already exists', {
						domain: command.domain,
					}),
				);
			}

			// Create anchor domain
			const anchorDomain = createAnchorDomain(command.domain);

			// Create domain event
			const event = new AnchorDomainCreated(context, {
				anchorDomainId: anchorDomain.id,
				domain: anchorDomain.domain,
			});

			// Commit atomically
			return unitOfWork.commit(anchorDomain, event, command);
		},
	};
}
