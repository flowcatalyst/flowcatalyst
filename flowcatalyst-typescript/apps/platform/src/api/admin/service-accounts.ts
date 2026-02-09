/**
 * Service Accounts Admin API
 *
 * REST endpoints for service account management.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import {
  sendResult,
  jsonCreated,
  jsonSuccess,
  noContent,
  notFound,
  ErrorResponseSchema,
} from '@flowcatalyst/http';
import { Result } from '@flowcatalyst/application';
import type { UseCase } from '@flowcatalyst/application';

import type {
  CreateServiceAccountCommand,
  UpdateServiceAccountCommand,
  DeleteServiceAccountCommand,
  RegenerateAuthTokenCommand,
  RegenerateSigningSecretCommand,
  AssignServiceAccountRolesCommand,
} from '../../application/index.js';
import type {
  ServiceAccountCreated,
  ServiceAccountUpdated,
  ServiceAccountDeleted,
  AuthTokenRegenerated,
  SigningSecretRegenerated,
  RolesAssigned,
  WebhookAuthType,
} from '../../domain/index.js';
import type { PrincipalRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { SERVICE_ACCOUNT_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateServiceAccountSchema = Type.Object({
  code: Type.String({ minLength: 1, maxLength: 100 }),
  name: Type.String({ minLength: 1, maxLength: 200 }),
  description: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
  applicationId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  clientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  webhookAuthType: Type.Optional(
    Type.Union([
      Type.Literal('NONE'),
      Type.Literal('BEARER_TOKEN'),
      Type.Literal('BASIC_AUTH'),
      Type.Literal('API_KEY'),
      Type.Literal('HMAC_SIGNATURE'),
    ]),
  ),
});

const UpdateServiceAccountSchema = Type.Object({
  name: Type.Optional(Type.String({ minLength: 1, maxLength: 200 })),
  description: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
});

const AssignRolesSchema = Type.Object({
  roles: Type.Array(Type.String({ minLength: 1 })),
});

const IdParam = Type.Object({ id: Type.String() });

// ─── Response Schemas ───────────────────────────────────────────────────────

const ServiceAccountResponseSchema = Type.Object({
  id: Type.String(),
  code: Type.String(),
  name: Type.String(),
  description: Type.Union([Type.String(), Type.Null()]),
  applicationId: Type.Union([Type.String(), Type.Null()]),
  clientId: Type.Union([Type.String(), Type.Null()]),
  active: Type.Boolean(),
  webhookAuthType: Type.String(),
  signingAlgorithm: Type.String(),
  credentialsCreatedAt: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
  credentialsRegeneratedAt: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
  lastUsedAt: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
  roles: Type.Array(Type.String()),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const ServiceAccountListResponseSchema = Type.Object({
  serviceAccounts: Type.Array(ServiceAccountResponseSchema),
});

const MessageResponseSchema = Type.Object({
  message: Type.String(),
});

type ServiceAccountResponse = Static<typeof ServiceAccountResponseSchema>;

/**
 * Dependencies for the service accounts API.
 */
export interface ServiceAccountsRoutesDeps {
  readonly principalRepository: PrincipalRepository;
  readonly createServiceAccountUseCase: UseCase<CreateServiceAccountCommand, ServiceAccountCreated>;
  readonly updateServiceAccountUseCase: UseCase<UpdateServiceAccountCommand, ServiceAccountUpdated>;
  readonly deleteServiceAccountUseCase: UseCase<DeleteServiceAccountCommand, ServiceAccountDeleted>;
  readonly regenerateAuthTokenUseCase: UseCase<RegenerateAuthTokenCommand, AuthTokenRegenerated>;
  readonly regenerateSigningSecretUseCase: UseCase<
    RegenerateSigningSecretCommand,
    SigningSecretRegenerated
  >;
  readonly assignServiceAccountRolesUseCase: UseCase<
    AssignServiceAccountRolesCommand,
    RolesAssigned
  >;
}

/**
 * Register service account admin API routes.
 */
export async function registerServiceAccountsRoutes(
  fastify: FastifyInstance,
  deps: ServiceAccountsRoutesDeps,
): Promise<void> {
  const {
    principalRepository,
    createServiceAccountUseCase,
    updateServiceAccountUseCase,
    deleteServiceAccountUseCase,
    regenerateAuthTokenUseCase,
    regenerateSigningSecretUseCase,
    assignServiceAccountRolesUseCase,
  } = deps;

  // POST /api/admin/service-accounts - Create service account
  fastify.post(
    '/service-accounts',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.CREATE),
      schema: {
        body: CreateServiceAccountSchema,
        response: {
          201: ServiceAccountResponseSchema,
          400: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as Static<typeof CreateServiceAccountSchema>;
      const ctx = request.executionContext;

      const command: CreateServiceAccountCommand = {
        code: body.code,
        name: body.name,
        description: body.description ?? null,
        applicationId: body.applicationId ?? null,
        clientId: body.clientId ?? null,
        webhookAuthType: body.webhookAuthType as WebhookAuthType | undefined,
      };

      const result = await createServiceAccountUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(result.value.getData().principalId);
        if (principal && principal.serviceAccount) {
          return jsonCreated(reply, toServiceAccountResponse(principal));
        }
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/service-accounts - List service accounts
  fastify.get(
    '/service-accounts',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.READ),
      schema: {
        response: {
          200: ServiceAccountListResponseSchema,
        },
      },
    },
    async (_request, reply) => {
      const principals = await principalRepository.findByType('SERVICE');

      return jsonSuccess(reply, {
        serviceAccounts: principals
          .filter((p) => p.serviceAccount !== null)
          .map(toServiceAccountResponse),
      });
    },
  );

  // GET /api/admin/service-accounts/:id - Get service account
  fastify.get(
    '/service-accounts/:id',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: ServiceAccountResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const principal = await principalRepository.findById(id);

      if (!principal || principal.type !== 'SERVICE' || !principal.serviceAccount) {
        return notFound(reply, `Service account not found: ${id}`);
      }

      return jsonSuccess(reply, toServiceAccountResponse(principal));
    },
  );

  // PATCH /api/admin/service-accounts/:id - Update service account
  fastify.patch(
    '/service-accounts/:id',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: UpdateServiceAccountSchema,
        response: {
          200: ServiceAccountResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof UpdateServiceAccountSchema>;
      const ctx = request.executionContext;

      const command: UpdateServiceAccountCommand = {
        serviceAccountId: id,
        name: body.name,
        description: body.description,
      };

      const result = await updateServiceAccountUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(id);
        if (principal && principal.serviceAccount) {
          return jsonSuccess(reply, toServiceAccountResponse(principal));
        }
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /api/admin/service-accounts/:id - Delete service account
  fastify.delete(
    '/service-accounts/:id',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.DELETE),
      schema: {
        params: IdParam,
        response: {
          204: Type.Null(),
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: DeleteServiceAccountCommand = {
        serviceAccountId: id,
      };

      const result = await deleteServiceAccountUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/service-accounts/:id/regenerate-auth-token
  fastify.post(
    '/service-accounts/:id/regenerate-auth-token',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.MANAGE),
      schema: {
        params: IdParam,
        response: {
          200: MessageResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: RegenerateAuthTokenCommand = {
        serviceAccountId: id,
      };

      const result = await regenerateAuthTokenUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return jsonSuccess(reply, { message: 'Auth token regenerated' });
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/service-accounts/:id/regenerate-signing-secret
  fastify.post(
    '/service-accounts/:id/regenerate-signing-secret',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.MANAGE),
      schema: {
        params: IdParam,
        response: {
          200: MessageResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: RegenerateSigningSecretCommand = {
        serviceAccountId: id,
      };

      const result = await regenerateSigningSecretUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return jsonSuccess(reply, { message: 'Signing secret regenerated' });
      }

      return sendResult(reply, result);
    },
  );

  // PUT /api/admin/service-accounts/:id/roles - Assign roles
  fastify.put(
    '/service-accounts/:id/roles',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.MANAGE),
      schema: {
        params: IdParam,
        body: AssignRolesSchema,
        response: {
          200: ServiceAccountResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof AssignRolesSchema>;
      const ctx = request.executionContext;

      const command: AssignServiceAccountRolesCommand = {
        serviceAccountId: id,
        roles: body.roles,
      };

      const result = await assignServiceAccountRolesUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(id);
        if (principal && principal.serviceAccount) {
          return jsonSuccess(reply, toServiceAccountResponse(principal));
        }
      }

      return sendResult(reply, result);
    },
  );
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

import type { Principal } from '../../domain/index.js';

function toServiceAccountResponse(principal: Principal): ServiceAccountResponse {
  const sa = principal.serviceAccount!;
  return {
    id: principal.id,
    code: sa.code,
    name: principal.name,
    description: sa.description,
    applicationId: principal.applicationId,
    clientId: principal.clientId,
    active: principal.active,
    webhookAuthType: sa.whAuthType,
    signingAlgorithm: sa.whSigningAlgorithm,
    credentialsCreatedAt: sa.whCredentialsCreatedAt?.toISOString() ?? null,
    credentialsRegeneratedAt: sa.whCredentialsRegeneratedAt?.toISOString() ?? null,
    lastUsedAt: sa.lastUsedAt?.toISOString() ?? null,
    roles: principal.roles.map((r) => r.roleName),
    createdAt: principal.createdAt.toISOString(),
    updatedAt: principal.updatedAt.toISOString(),
  };
}
