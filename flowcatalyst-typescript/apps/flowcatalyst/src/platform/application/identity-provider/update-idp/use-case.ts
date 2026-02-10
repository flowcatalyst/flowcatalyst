/**
 * Update Identity Provider Use Case
 */

import type { UseCase } from '@flowcatalyst/application';
import { Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type { IdentityProviderRepository } from '../../../infrastructure/persistence/index.js';
import { updateIdentityProvider, IdentityProviderUpdated } from '../../../domain/index.js';

import type { UpdateIdentityProviderCommand } from './command.js';

export interface UpdateIdentityProviderUseCaseDeps {
  readonly identityProviderRepository: IdentityProviderRepository;
  readonly unitOfWork: UnitOfWork;
}

export function createUpdateIdentityProviderUseCase(
  deps: UpdateIdentityProviderUseCaseDeps,
): UseCase<UpdateIdentityProviderCommand, IdentityProviderUpdated> {
  const { identityProviderRepository, unitOfWork } = deps;

  return {
    async execute(
      command: UpdateIdentityProviderCommand,
      context: ExecutionContext,
    ): Promise<Result<IdentityProviderUpdated>> {
      const idp = await identityProviderRepository.findById(command.identityProviderId);
      if (!idp) {
        return Result.failure(
          UseCaseError.notFound('IDP_NOT_FOUND', 'Identity provider not found', {
            identityProviderId: command.identityProviderId,
          }),
        );
      }

      const updated = updateIdentityProvider(idp, {
        ...(command.name !== undefined ? { name: command.name } : {}),
        ...(command.type !== undefined ? { type: command.type } : {}),
        ...(command.oidcIssuerUrl !== undefined ? { oidcIssuerUrl: command.oidcIssuerUrl } : {}),
        ...(command.oidcClientId !== undefined ? { oidcClientId: command.oidcClientId } : {}),
        ...(command.oidcClientSecretRef !== undefined
          ? { oidcClientSecretRef: command.oidcClientSecretRef }
          : {}),
        ...(command.oidcMultiTenant !== undefined
          ? { oidcMultiTenant: command.oidcMultiTenant }
          : {}),
        ...(command.oidcIssuerPattern !== undefined
          ? { oidcIssuerPattern: command.oidcIssuerPattern }
          : {}),
        ...(command.allowedEmailDomains !== undefined
          ? { allowedEmailDomains: command.allowedEmailDomains }
          : {}),
      });

      const event = new IdentityProviderUpdated(context, {
        identityProviderId: updated.id,
        name: updated.name,
        type: updated.type,
      });

      return unitOfWork.commit(updated, event, command);
    },
  };
}
