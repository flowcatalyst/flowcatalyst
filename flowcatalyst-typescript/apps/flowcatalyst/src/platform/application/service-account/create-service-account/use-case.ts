/**
 * Create Service Account Use Case
 *
 * Atomically creates a SERVICE-type Principal with embedded service account data
 * and a CONFIDENTIAL OAuthClient for client_credentials authentication.
 */

import type { UseCase } from '@flowcatalyst/application';
import {
  validateRequired,
  Result,
  ExecutionContext,
  UseCaseError,
} from '@flowcatalyst/application';
import type { UnitOfWork } from '@flowcatalyst/domain-core';
import type { EncryptionService } from '@flowcatalyst/platform-crypto';

import type {
  PrincipalRepository,
  OAuthClientRepository,
} from '../../../infrastructure/persistence/index.js';
import {
  createServicePrincipal,
  createServiceAccountData,
  createOAuthClient,
  generateAuthToken,
  generateSigningSecret,
  generateClientSecret,
  ServiceAccountCreated,
} from '../../../domain/index.js';

import type { CreateServiceAccountCommand } from './command.js';

/**
 * Dependencies for CreateServiceAccountUseCase.
 */
export interface CreateServiceAccountUseCaseDeps {
  readonly principalRepository: PrincipalRepository;
  readonly oauthClientRepository: OAuthClientRepository;
  readonly encryptionService: EncryptionService;
  readonly unitOfWork: UnitOfWork;
}

/**
 * Create the CreateServiceAccountUseCase.
 */
export function createCreateServiceAccountUseCase(
  deps: CreateServiceAccountUseCaseDeps,
): UseCase<CreateServiceAccountCommand, ServiceAccountCreated> {
  const { principalRepository, oauthClientRepository, encryptionService, unitOfWork } = deps;

  return {
    async execute(
      command: CreateServiceAccountCommand,
      context: ExecutionContext,
    ): Promise<Result<ServiceAccountCreated>> {
      // Validate code
      const codeResult = validateRequired(command.code, 'code', 'CODE_REQUIRED');
      if (Result.isFailure(codeResult)) {
        return codeResult;
      }

      // Validate name
      const nameResult = validateRequired(command.name, 'name', 'NAME_REQUIRED');
      if (Result.isFailure(nameResult)) {
        return nameResult;
      }

      // Check code uniqueness
      const codeExists = await principalRepository.existsByServiceAccountCode(command.code);
      if (codeExists) {
        return Result.failure(
          UseCaseError.businessRule('CODE_EXISTS', 'Service account code already exists', {
            code: command.code,
          }),
        );
      }

      // Generate credentials
      const authToken = generateAuthToken();
      const signingSecret = generateSigningSecret();
      const clientSecret = generateClientSecret();

      // Encrypt credentials for storage
      const authTokenRefResult = encryptionService.encrypt(authToken);
      if (authTokenRefResult.isErr()) {
        return Result.failure(
          UseCaseError.businessRule('ENCRYPTION_FAILED', 'Failed to encrypt auth token'),
        );
      }

      const signingSecretRefResult = encryptionService.encrypt(signingSecret);
      if (signingSecretRefResult.isErr()) {
        return Result.failure(
          UseCaseError.businessRule('ENCRYPTION_FAILED', 'Failed to encrypt signing secret'),
        );
      }

      const clientSecretRefResult = encryptionService.encrypt(clientSecret);
      if (clientSecretRefResult.isErr()) {
        return Result.failure(
          UseCaseError.businessRule('ENCRYPTION_FAILED', 'Failed to encrypt client secret'),
        );
      }

      // Create service account data (embedded in principal)
      const serviceAccountData = createServiceAccountData({
        code: command.code,
        description: command.description,
        whAuthType: command.webhookAuthType,
        whAuthTokenRef: authTokenRefResult.value,
        whSigningSecretRef: signingSecretRefResult.value,
      });

      // Create SERVICE principal
      const principal = createServicePrincipal({
        name: command.name,
        applicationId: command.applicationId,
        clientId: command.clientId,
        serviceAccount: serviceAccountData,
        ...(command.scope !== undefined ? { scope: command.scope } : {}),
      });

      // Create CONFIDENTIAL OAuthClient for client_credentials
      const oauthClient = createOAuthClient({
        clientName: `Service Account: ${command.name}`,
        clientType: 'CONFIDENTIAL',
        clientSecretRef: clientSecretRefResult.value,
        grantTypes: ['client_credentials'],
        pkceRequired: false,
        applicationIds: command.applicationId ? [command.applicationId] : [],
      });

      // Link OAuth client to principal
      const linkedOAuthClient = {
        ...oauthClient,
        serviceAccountPrincipalId: principal.id,
      };

      // Create domain event
      const event = new ServiceAccountCreated(context, {
        serviceAccountId: principal.id,
        principalId: principal.id,
        oauthClientId: linkedOAuthClient.id,
        code: command.code,
        name: command.name,
        applicationId: command.applicationId,
      });

      // Commit both aggregates atomically
      return unitOfWork.commitAll([principal, linkedOAuthClient], event, command);
    },
  };
}
