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
import type { EncryptionService } from '@flowcatalyst/platform-crypto';

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
  Principal,
} from '../../domain/index.js';
import { updatePrincipal } from '../../domain/index.js';
import type {
  PrincipalRepository,
  OAuthClientRepository,
} from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { SERVICE_ACCOUNT_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateServiceAccountSchema = Type.Object({
  code: Type.String({ minLength: 1, maxLength: 100 }),
  name: Type.String({ minLength: 1, maxLength: 200 }),
  description: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
  applicationId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  clientIds: Type.Optional(Type.Array(Type.String())),
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
  clientIds: Type.Optional(Type.Array(Type.String())),
});

const AssignRolesSchema = Type.Object({
  roles: Type.Array(Type.String({ minLength: 1 })),
});

const UpdateAuthTokenSchema = Type.Object({
  authToken: Type.String({ minLength: 1 }),
});

const IdParam = Type.Object({ id: Type.String() });
const CodeParam = Type.Object({ code: Type.String() });

const ListServiceAccountsQuery = Type.Object({
  clientId: Type.Optional(Type.String()),
  applicationId: Type.Optional(Type.String()),
  active: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const ServiceAccountResponseSchema = Type.Object({
  id: Type.String(),
  code: Type.String(),
  name: Type.String(),
  description: Type.Union([Type.String(), Type.Null()]),
  clientIds: Type.Array(Type.String()),
  applicationId: Type.Union([Type.String(), Type.Null()]),
  active: Type.Boolean(),
  authType: Type.Union([Type.String(), Type.Null()]),
  roles: Type.Array(Type.String()),
  lastUsedAt: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const ServiceAccountListResponseSchema = Type.Object({
  serviceAccounts: Type.Array(ServiceAccountResponseSchema),
  total: Type.Integer(),
});

const OAuthCredentialsSchema = Type.Object({
  clientId: Type.String(),
  clientSecret: Type.String(),
});

const WebhookCredentialsSchema = Type.Object({
  authToken: Type.String(),
  signingSecret: Type.String(),
});

const CreateServiceAccountResponseSchema = Type.Object({
  serviceAccount: ServiceAccountResponseSchema,
  principalId: Type.String(),
  oauth: OAuthCredentialsSchema,
  webhook: WebhookCredentialsSchema,
});

const RegenerateTokenResponseSchema = Type.Object({
  authToken: Type.String(),
});

const RegenerateSecretResponseSchema = Type.Object({
  signingSecret: Type.String(),
});

const RoleAssignmentSchema = Type.Object({
  roleName: Type.String(),
  assignmentSource: Type.String(),
  assignedAt: Type.String({ format: 'date-time' }),
});

const RolesResponseSchema = Type.Object({
  roles: Type.Array(RoleAssignmentSchema),
});

const RolesAssignedResponseSchema = Type.Object({
  roles: Type.Array(RoleAssignmentSchema),
  addedRoles: Type.Array(Type.String()),
  removedRoles: Type.Array(Type.String()),
});

type ServiceAccountResponse = Static<typeof ServiceAccountResponseSchema>;

/**
 * Dependencies for the service accounts API.
 */
export interface ServiceAccountsRoutesDeps {
  readonly principalRepository: PrincipalRepository;
  readonly oauthClientRepository: OAuthClientRepository;
  readonly encryptionService: EncryptionService;
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
    oauthClientRepository,
    encryptionService,
    createServiceAccountUseCase,
    updateServiceAccountUseCase,
    deleteServiceAccountUseCase,
    regenerateAuthTokenUseCase,
    regenerateSigningSecretUseCase,
    assignServiceAccountRolesUseCase,
  } = deps;

  /**
   * Decrypt an encrypted reference, returning empty string on failure.
   */
  function decryptRef(ref: string | null): string {
    if (!ref) return '';
    const result = encryptionService.decrypt(ref);
    return result.isOk() ? result.value : '';
  }

  // POST /api/admin/service-accounts - Create service account
  fastify.post(
    '/service-accounts',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.CREATE),
      schema: {
        body: CreateServiceAccountSchema,
        response: {
          201: CreateServiceAccountResponseSchema,
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
        clientId: body.clientIds?.[0] ?? null,
        webhookAuthType: body.webhookAuthType as WebhookAuthType | undefined,
      };

      const result = await createServiceAccountUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const eventData = result.value.getData();

        // Fetch created entities to extract credentials
        const [principal, oauthClient] = await Promise.all([
          principalRepository.findById(eventData.principalId),
          oauthClientRepository.findById(eventData.oauthClientId),
        ]);

        if (principal?.serviceAccount && oauthClient) {
          return jsonCreated(reply, {
            serviceAccount: toServiceAccountResponse(principal),
            principalId: principal.id,
            oauth: {
              clientId: oauthClient.clientId,
              clientSecret: decryptRef(oauthClient.clientSecretRef),
            },
            webhook: {
              authToken: decryptRef(principal.serviceAccount.whAuthTokenRef),
              signingSecret: decryptRef(principal.serviceAccount.whSigningSecretRef),
            },
          });
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
        querystring: ListServiceAccountsQuery,
        response: {
          200: ServiceAccountListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof ListServiceAccountsQuery>;
      let principals = await principalRepository.findByType('SERVICE');

      // Apply filters
      principals = principals.filter((p) => {
        if (!p.serviceAccount) return false;
        if (query.clientId && p.clientId !== query.clientId) return false;
        if (query.applicationId && p.applicationId !== query.applicationId) return false;
        if (query.active !== undefined) {
          const isActive = query.active === 'true';
          if (p.active !== isActive) return false;
        }
        return true;
      });

      const serviceAccounts = principals.map(toServiceAccountResponse);

      return jsonSuccess(reply, {
        serviceAccounts,
        total: serviceAccounts.length,
      });
    },
  );

  // GET /api/admin/service-accounts/:id - Get service account by ID
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

  // GET /api/admin/service-accounts/code/:code - Get service account by code
  fastify.get(
    '/service-accounts/code/:code',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.READ),
      schema: {
        params: CodeParam,
        response: {
          200: ServiceAccountResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { code } = request.params as Static<typeof CodeParam>;
      const principal = await principalRepository.findByServiceAccountCode(code);

      if (!principal || !principal.serviceAccount) {
        return notFound(reply, `Service account not found: ${code}`);
      }

      return jsonSuccess(reply, toServiceAccountResponse(principal));
    },
  );

  // PUT /api/admin/service-accounts/:id - Update service account
  fastify.put(
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

  // PUT /api/admin/service-accounts/:id/auth-token - Update auth token with custom value
  fastify.put(
    '/service-accounts/:id/auth-token',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.MANAGE),
      schema: {
        params: IdParam,
        body: UpdateAuthTokenSchema,
        response: {
          200: ServiceAccountResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof UpdateAuthTokenSchema>;

      const principal = await principalRepository.findById(id);
      if (!principal || principal.type !== 'SERVICE' || !principal.serviceAccount) {
        return notFound(reply, `Service account not found: ${id}`);
      }

      // Encrypt the provided auth token
      const encryptResult = encryptionService.encrypt(body.authToken);
      if (encryptResult.isErr()) {
        return reply.status(400).send({ code: 'ENCRYPTION_FAILED', message: 'Failed to encrypt auth token' });
      }

      // Update principal with new auth token ref
      const updatedPrincipal = updatePrincipal(principal, {
        serviceAccount: {
          ...principal.serviceAccount,
          whAuthTokenRef: encryptResult.value,
          whCredentialsRegeneratedAt: new Date(),
        },
      });

      await principalRepository.persist(updatedPrincipal);

      const refreshed = await principalRepository.findById(id);
      if (refreshed?.serviceAccount) {
        return jsonSuccess(reply, toServiceAccountResponse(refreshed));
      }

      return jsonSuccess(reply, toServiceAccountResponse(updatedPrincipal as Principal));
    },
  );

  // POST /api/admin/service-accounts/:id/regenerate-token
  fastify.post(
    '/service-accounts/:id/regenerate-token',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.MANAGE),
      schema: {
        params: IdParam,
        response: {
          200: RegenerateTokenResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: RegenerateAuthTokenCommand = { serviceAccountId: id };
      const result = await regenerateAuthTokenUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(id);
        if (principal?.serviceAccount?.whAuthTokenRef) {
          const authToken = decryptRef(principal.serviceAccount.whAuthTokenRef);
          return jsonSuccess(reply, { authToken });
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/service-accounts/:id/regenerate-auth-token (legacy URL)
  fastify.post(
    '/service-accounts/:id/regenerate-auth-token',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.MANAGE),
      schema: {
        params: IdParam,
        response: {
          200: RegenerateTokenResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: RegenerateAuthTokenCommand = { serviceAccountId: id };
      const result = await regenerateAuthTokenUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(id);
        if (principal?.serviceAccount?.whAuthTokenRef) {
          const authToken = decryptRef(principal.serviceAccount.whAuthTokenRef);
          return jsonSuccess(reply, { authToken });
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/service-accounts/:id/regenerate-secret
  fastify.post(
    '/service-accounts/:id/regenerate-secret',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.MANAGE),
      schema: {
        params: IdParam,
        response: {
          200: RegenerateSecretResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: RegenerateSigningSecretCommand = { serviceAccountId: id };
      const result = await regenerateSigningSecretUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(id);
        if (principal?.serviceAccount?.whSigningSecretRef) {
          const signingSecret = decryptRef(principal.serviceAccount.whSigningSecretRef);
          return jsonSuccess(reply, { signingSecret });
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/service-accounts/:id/regenerate-signing-secret (legacy URL)
  fastify.post(
    '/service-accounts/:id/regenerate-signing-secret',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.MANAGE),
      schema: {
        params: IdParam,
        response: {
          200: RegenerateSecretResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: RegenerateSigningSecretCommand = { serviceAccountId: id };
      const result = await regenerateSigningSecretUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(id);
        if (principal?.serviceAccount?.whSigningSecretRef) {
          const signingSecret = decryptRef(principal.serviceAccount.whSigningSecretRef);
          return jsonSuccess(reply, { signingSecret });
        }
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/service-accounts/:id/roles - Get assigned roles
  fastify.get(
    '/service-accounts/:id/roles',
    {
      preHandler: requirePermission(SERVICE_ACCOUNT_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: RolesResponseSchema,
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

      return jsonSuccess(reply, {
        roles: principal.roles.map((r) => ({
          roleName: r.roleName,
          assignmentSource: r.assignmentSource,
          assignedAt: r.assignedAt.toISOString(),
        })),
      });
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
          200: RolesAssignedResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof AssignRolesSchema>;
      const ctx = request.executionContext;

      // Get previous roles before assignment
      const existingPrincipal = await principalRepository.findById(id);
      const previousRoleNames = existingPrincipal?.roles.map((r) => r.roleName) ?? [];

      const command: AssignServiceAccountRolesCommand = {
        serviceAccountId: id,
        roles: body.roles,
      };

      const result = await assignServiceAccountRolesUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const principal = await principalRepository.findById(id);
        if (principal && principal.serviceAccount) {
          const currentRoleNames = new Set(body.roles);
          const previousRoleSet = new Set(previousRoleNames);

          const addedRoles = body.roles.filter((r) => !previousRoleSet.has(r));
          const removedRoles = previousRoleNames.filter((r) => !currentRoleNames.has(r));

          return jsonSuccess(reply, {
            roles: principal.roles.map((r) => ({
              roleName: r.roleName,
              assignmentSource: r.assignmentSource,
              assignedAt: r.assignedAt.toISOString(),
            })),
            addedRoles,
            removedRoles,
          });
        }
      }

      return sendResult(reply, result);
    },
  );
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

function toServiceAccountResponse(principal: Principal): ServiceAccountResponse {
  const sa = principal.serviceAccount!;
  return {
    id: principal.id,
    code: sa.code,
    name: principal.name,
    description: sa.description,
    clientIds: principal.clientId ? [principal.clientId] : [],
    applicationId: principal.applicationId,
    active: principal.active,
    authType: sa.whAuthType,
    roles: principal.roles.map((r) => r.roleName),
    lastUsedAt: sa.lastUsedAt?.toISOString() ?? null,
    createdAt: principal.createdAt.toISOString(),
    updatedAt: principal.updatedAt.toISOString(),
  };
}
