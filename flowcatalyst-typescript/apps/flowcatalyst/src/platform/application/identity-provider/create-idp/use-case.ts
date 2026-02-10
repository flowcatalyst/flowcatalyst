/**
 * Create Identity Provider Use Case
 */

import type { UseCase } from '@flowcatalyst/application';
import { validateRequired, Result, UseCaseError } from '@flowcatalyst/application';
import type { ExecutionContext, UnitOfWork } from '@flowcatalyst/domain-core';

import type { IdentityProviderRepository } from '../../../infrastructure/persistence/index.js';
import { createIdentityProvider, IdentityProviderCreated } from '../../../domain/index.js';

import type { CreateIdentityProviderCommand } from './command.js';

export interface CreateIdentityProviderUseCaseDeps {
  readonly identityProviderRepository: IdentityProviderRepository;
  readonly unitOfWork: UnitOfWork;
}

const CODE_PATTERN = /^[a-z][a-z0-9-]*$/;

export function createCreateIdentityProviderUseCase(
  deps: CreateIdentityProviderUseCaseDeps,
): UseCase<CreateIdentityProviderCommand, IdentityProviderCreated> {
  const { identityProviderRepository, unitOfWork } = deps;

  return {
    async execute(
      command: CreateIdentityProviderCommand,
      context: ExecutionContext,
    ): Promise<Result<IdentityProviderCreated>> {
      const codeResult = validateRequired(command.code, 'code', 'CODE_REQUIRED');
      if (Result.isFailure(codeResult)) return codeResult;

      const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
      if (Result.isFailure(nameResult)) return nameResult;

      if (!CODE_PATTERN.test(command.code)) {
        return Result.failure(
          UseCaseError.validation(
            'INVALID_CODE_FORMAT',
            'Code must start with a lowercase letter and contain only lowercase letters, digits, and hyphens',
          ),
        );
      }

      const codeExists = await identityProviderRepository.existsByCode(command.code);
      if (codeExists) {
        return Result.failure(
          UseCaseError.businessRule('CODE_EXISTS', 'Identity provider code already exists', {
            code: command.code,
          }),
        );
      }

      const idp = createIdentityProvider({
        code: command.code,
        name: command.name,
        type: command.type,
        oidcIssuerUrl: command.oidcIssuerUrl ?? null,
        oidcClientId: command.oidcClientId ?? null,
        oidcClientSecretRef: command.oidcClientSecretRef ?? null,
        oidcMultiTenant: command.oidcMultiTenant ?? false,
        oidcIssuerPattern: command.oidcIssuerPattern ?? null,
        allowedEmailDomains: command.allowedEmailDomains ?? [],
      });

      const event = new IdentityProviderCreated(context, {
        identityProviderId: idp.id,
        code: idp.code,
        name: idp.name,
        type: idp.type,
      });

      return unitOfWork.commit(idp, event, command);
    },
  };
}
