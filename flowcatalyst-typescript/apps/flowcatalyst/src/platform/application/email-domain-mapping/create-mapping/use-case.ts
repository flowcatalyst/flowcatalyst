/**
 * Create Email Domain Mapping Use Case
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type {
  IdentityProviderRepository,
  EmailDomainMappingRepository,
} from '../../../infrastructure/persistence/index.js';
import { createEmailDomainMapping, EmailDomainMappingCreated } from '../../../domain/index.js';

import type { CreateEmailDomainMappingCommand } from './command.js';

export interface CreateEmailDomainMappingUseCaseDeps {
  readonly emailDomainMappingRepository: EmailDomainMappingRepository;
  readonly identityProviderRepository: IdentityProviderRepository;
  readonly unitOfWork: UnitOfWork;
}

export function createCreateEmailDomainMappingUseCase(
  deps: CreateEmailDomainMappingUseCaseDeps,
): UseCase<CreateEmailDomainMappingCommand, EmailDomainMappingCreated> {
  const { emailDomainMappingRepository, identityProviderRepository, unitOfWork } = deps;

  return {
    async execute(
      command: CreateEmailDomainMappingCommand,
      context: ExecutionContext,
    ): Promise<Result<EmailDomainMappingCreated>> {
      const domainResult = validateRequired(
        command.emailDomain,
        'emailDomain',
        'EMAIL_DOMAIN_REQUIRED',
      );
      if (Result.isFailure(domainResult)) return domainResult;

      const idpResult = validateRequired(
        command.identityProviderId,
        'identityProviderId',
        'IDENTITY_PROVIDER_REQUIRED',
      );
      if (Result.isFailure(idpResult)) return idpResult;

      // Verify IDP exists
      const idpExists = await identityProviderRepository.exists(command.identityProviderId);
      if (!idpExists) {
        return Result.failure(
          UseCaseError.notFound('IDP_NOT_FOUND', 'Identity provider not found', {
            identityProviderId: command.identityProviderId,
          }),
        );
      }

      // Check for duplicate email domain
      const domainExists = await emailDomainMappingRepository.existsByEmailDomain(
        command.emailDomain,
      );
      if (domainExists) {
        return Result.failure(
          UseCaseError.businessRule('DOMAIN_EXISTS', 'Email domain mapping already exists', {
            emailDomain: command.emailDomain,
          }),
        );
      }

      const mapping = createEmailDomainMapping({
        emailDomain: command.emailDomain,
        identityProviderId: command.identityProviderId,
        scopeType: command.scopeType,
        primaryClientId: command.primaryClientId ?? null,
        additionalClientIds: command.additionalClientIds ?? [],
        grantedClientIds: command.grantedClientIds ?? [],
        requiredOidcTenantId: command.requiredOidcTenantId ?? null,
        allowedRoleIds: command.allowedRoleIds ?? [],
        syncRolesFromIdp: command.syncRolesFromIdp ?? false,
      });

      const event = new EmailDomainMappingCreated(context, {
        emailDomainMappingId: mapping.id,
        emailDomain: mapping.emailDomain,
        identityProviderId: mapping.identityProviderId,
        scopeType: mapping.scopeType,
        primaryClientId: mapping.primaryClientId,
        additionalClientIds: mapping.additionalClientIds,
        grantedClientIds: mapping.grantedClientIds,
      });

      return unitOfWork.commit(mapping, event, command);
    },
  };
}
