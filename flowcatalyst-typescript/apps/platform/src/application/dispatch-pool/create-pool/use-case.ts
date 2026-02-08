/**
 * Create Dispatch Pool Use Case
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type { DispatchPoolRepository } from '../../../infrastructure/persistence/index.js';
import type { ClientRepository } from '../../../infrastructure/persistence/index.js';
import { createDispatchPool, DispatchPoolCreated } from '../../../domain/index.js';

import type { CreateDispatchPoolCommand } from './command.js';

export interface CreateDispatchPoolUseCaseDeps {
	readonly dispatchPoolRepository: DispatchPoolRepository;
	readonly clientRepository: ClientRepository;
	readonly unitOfWork: UnitOfWork;
}

const CODE_PATTERN = /^[a-z][a-z0-9-]*$/;

export function createCreateDispatchPoolUseCase(
	deps: CreateDispatchPoolUseCaseDeps,
): UseCase<CreateDispatchPoolCommand, DispatchPoolCreated> {
	const { dispatchPoolRepository, clientRepository, unitOfWork } = deps;

	return {
		async execute(
			command: CreateDispatchPoolCommand,
			context: ExecutionContext,
		): Promise<Result<DispatchPoolCreated>> {
			// Validate required fields
			const codeResult = validateRequired(command.code, 'code', 'CODE_REQUIRED');
			if (Result.isFailure(codeResult)) return codeResult;

			const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
			if (Result.isFailure(nameResult)) return nameResult;

			// Validate code format
			if (!CODE_PATTERN.test(command.code)) {
				return Result.failure(
					UseCaseError.validation(
						'INVALID_CODE_FORMAT',
						'Code must start with a lowercase letter and contain only lowercase letters, numbers, and hyphens',
					),
				);
			}

			// Validate rate limit and concurrency
			const rateLimit = command.rateLimit ?? 100;
			const concurrency = command.concurrency ?? 10;

			if (rateLimit < 1) {
				return Result.failure(
					UseCaseError.validation('INVALID_RATE_LIMIT', 'Rate limit must be at least 1'),
				);
			}

			if (concurrency < 1) {
				return Result.failure(
					UseCaseError.validation('INVALID_CONCURRENCY', 'Concurrency must be at least 1'),
				);
			}

			// Validate client exists if provided
			const clientId = command.clientId ?? null;
			if (clientId !== null) {
				const clientExists = await clientRepository.exists(clientId);
				if (!clientExists) {
					return Result.failure(
						UseCaseError.notFound('CLIENT_NOT_FOUND', 'Client not found', { clientId }),
					);
				}
			}

			// Check code uniqueness per client scope
			const codeExists = await dispatchPoolRepository.existsByCodeAndClientId(command.code, clientId);
			if (codeExists) {
				return Result.failure(
					UseCaseError.businessRule('CODE_EXISTS', 'Dispatch pool code already exists in this scope', {
						code: command.code,
					}),
				);
			}

			const pool = createDispatchPool({
				code: command.code,
				name: command.name,
				description: command.description ?? null,
				rateLimit,
				concurrency,
				clientId,
			});

			const event = new DispatchPoolCreated(context, {
				poolId: pool.id,
				code: pool.code,
				name: pool.name,
				description: pool.description,
				rateLimit: pool.rateLimit,
				concurrency: pool.concurrency,
				clientId: pool.clientId,
				clientIdentifier: pool.clientIdentifier,
				status: pool.status,
			});

			return unitOfWork.commit(pool, event, command);
		},
	};
}
