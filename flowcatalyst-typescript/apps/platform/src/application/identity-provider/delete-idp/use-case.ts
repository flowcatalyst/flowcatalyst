/**
 * Delete Identity Provider Use Case
 */

import type { UseCase } from '@flowcatalyst/application';
import { Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type { IdentityProviderRepository } from '../../../infrastructure/persistence/index.js';
import { IdentityProviderDeleted } from '../../../domain/index.js';

import type { DeleteIdentityProviderCommand } from './command.js';

export interface DeleteIdentityProviderUseCaseDeps {
  readonly identityProviderRepository: IdentityProviderRepository;
  readonly unitOfWork: UnitOfWork;
}

export function createDeleteIdentityProviderUseCase(
  deps: DeleteIdentityProviderUseCaseDeps,
): UseCase<DeleteIdentityProviderCommand, IdentityProviderDeleted> {
  const { identityProviderRepository, unitOfWork } = deps;

  return {
    async execute(
      command: DeleteIdentityProviderCommand,
      context: ExecutionContext,
    ): Promise<Result<IdentityProviderDeleted>> {
      const idp = await identityProviderRepository.findById(command.identityProviderId);
      if (!idp) {
        return Result.failure(
          UseCaseError.notFound('IDP_NOT_FOUND', 'Identity provider not found', {
            identityProviderId: command.identityProviderId,
          }),
        );
      }

      const event = new IdentityProviderDeleted(context, {
        identityProviderId: idp.id,
        code: idp.code,
      });

      return unitOfWork.commitDelete(idp, event, command);
    },
  };
}
