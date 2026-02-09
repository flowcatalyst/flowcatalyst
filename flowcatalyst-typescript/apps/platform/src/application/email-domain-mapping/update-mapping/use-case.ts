/**
 * Update Email Domain Mapping Use Case
 */

import type { UseCase } from '@flowcatalyst/application';
import { Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type {
  IdentityProviderRepository,
  EmailDomainMappingRepository,
} from '../../../infrastructure/persistence/index.js';
import { updateEmailDomainMapping, EmailDomainMappingUpdated } from '../../../domain/index.js';

import type { UpdateEmailDomainMappingCommand } from './command.js';

export interface UpdateEmailDomainMappingUseCaseDeps {
  readonly emailDomainMappingRepository: EmailDomainMappingRepository;
  readonly identityProviderRepository: IdentityProviderRepository;
  readonly unitOfWork: UnitOfWork;
}

export function createUpdateEmailDomainMappingUseCase(
  deps: UpdateEmailDomainMappingUseCaseDeps,
): UseCase<UpdateEmailDomainMappingCommand, EmailDomainMappingUpdated> {
  const { emailDomainMappingRepository, identityProviderRepository, unitOfWork } = deps;

  return {
    async execute(
      command: UpdateEmailDomainMappingCommand,
      context: ExecutionContext,
    ): Promise<Result<EmailDomainMappingUpdated>> {
      const mapping = await emailDomainMappingRepository.findById(command.emailDomainMappingId);
      if (!mapping) {
        return Result.failure(
          UseCaseError.notFound('MAPPING_NOT_FOUND', 'Email domain mapping not found', {
            emailDomainMappingId: command.emailDomainMappingId,
          }),
        );
      }

      // If changing IDP, verify it exists
      if (command.identityProviderId !== undefined) {
        const idpExists = await identityProviderRepository.exists(command.identityProviderId);
        if (!idpExists) {
          return Result.failure(
            UseCaseError.notFound('IDP_NOT_FOUND', 'Identity provider not found', {
              identityProviderId: command.identityProviderId,
            }),
          );
        }
      }

      const updated = updateEmailDomainMapping(mapping, {
        ...(command.identityProviderId !== undefined
          ? { identityProviderId: command.identityProviderId }
          : {}),
        ...(command.scopeType !== undefined ? { scopeType: command.scopeType } : {}),
        ...(command.primaryClientId !== undefined
          ? { primaryClientId: command.primaryClientId }
          : {}),
        ...(command.additionalClientIds !== undefined
          ? { additionalClientIds: command.additionalClientIds }
          : {}),
        ...(command.grantedClientIds !== undefined
          ? { grantedClientIds: command.grantedClientIds }
          : {}),
        ...(command.requiredOidcTenantId !== undefined
          ? { requiredOidcTenantId: command.requiredOidcTenantId }
          : {}),
        ...(command.allowedRoleIds !== undefined ? { allowedRoleIds: command.allowedRoleIds } : {}),
        ...(command.syncRolesFromIdp !== undefined
          ? { syncRolesFromIdp: command.syncRolesFromIdp }
          : {}),
      });

      const event = new EmailDomainMappingUpdated(context, {
        emailDomainMappingId: updated.id,
        emailDomain: updated.emailDomain,
        identityProviderId: updated.identityProviderId,
        scopeType: updated.scopeType,
        primaryClientId: updated.primaryClientId,
        additionalClientIds: updated.additionalClientIds,
        grantedClientIds: updated.grantedClientIds,
      });

      return unitOfWork.commit(updated, event, command);
    },
  };
}
