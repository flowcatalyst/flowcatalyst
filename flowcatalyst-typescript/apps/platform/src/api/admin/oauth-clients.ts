/**
 * OAuth Clients Admin API
 *
 * REST endpoints for OAuth client management.
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
  CreateOAuthClientCommand,
  UpdateOAuthClientCommand,
  RegenerateOAuthClientSecretCommand,
  DeleteOAuthClientCommand,
} from '../../application/index.js';
import type {
  OAuthClientCreated,
  OAuthClientUpdated,
  OAuthClientSecretRegenerated,
  OAuthClientDeleted,
  OAuthClient,
  OAuthClientType,
  OAuthGrantType,
} from '../../domain/index.js';
import type { OAuthClientRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { OAUTH_CLIENT_PERMISSIONS } from '../../authorization/permissions/platform-auth.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const OAuthClientTypeSchema = Type.Union([Type.Literal('PUBLIC'), Type.Literal('CONFIDENTIAL')]);

const OAuthGrantTypeSchema = Type.Union([
  Type.Literal('authorization_code'),
  Type.Literal('client_credentials'),
  Type.Literal('refresh_token'),
  Type.Literal('password'),
]);

const CreateOAuthClientSchema = Type.Object({
  clientId: Type.String({ minLength: 1 }),
  clientName: Type.String({ minLength: 1 }),
  clientType: OAuthClientTypeSchema,
  clientSecretRef: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  redirectUris: Type.Optional(Type.Array(Type.String())),
  allowedOrigins: Type.Optional(Type.Array(Type.String())),
  grantTypes: Type.Optional(Type.Array(OAuthGrantTypeSchema)),
  defaultScopes: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  pkceRequired: Type.Optional(Type.Boolean()),
  applicationIds: Type.Optional(Type.Array(Type.String())),
});

const UpdateOAuthClientSchema = Type.Object({
  clientName: Type.Optional(Type.String({ minLength: 1 })),
  redirectUris: Type.Optional(Type.Array(Type.String())),
  allowedOrigins: Type.Optional(Type.Array(Type.String())),
  grantTypes: Type.Optional(Type.Array(OAuthGrantTypeSchema)),
  defaultScopes: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  pkceRequired: Type.Optional(Type.Boolean()),
  applicationIds: Type.Optional(Type.Array(Type.String())),
  active: Type.Optional(Type.Boolean()),
});

const RegenerateSecretSchema = Type.Object({
  newSecretRef: Type.String({ minLength: 1 }),
});

const IdParam = Type.Object({ id: Type.String() });
const ClientIdParam = Type.Object({ clientId: Type.String() });

const ListOAuthClientsQuery = Type.Object({
  active: Type.Optional(Type.String()),
});

type CreateOAuthClientBody = Static<typeof CreateOAuthClientSchema>;
type UpdateOAuthClientBody = Static<typeof UpdateOAuthClientSchema>;
type RegenerateSecretBody = Static<typeof RegenerateSecretSchema>;

// ─── Response Schemas ───────────────────────────────────────────────────────

const OAuthClientResponseSchema = Type.Object({
  id: Type.String(),
  clientId: Type.String(),
  clientName: Type.String(),
  clientType: Type.String(),
  hasClientSecret: Type.Boolean(),
  redirectUris: Type.Array(Type.String()),
  allowedOrigins: Type.Array(Type.String()),
  grantTypes: Type.Array(Type.String()),
  defaultScopes: Type.Union([Type.String(), Type.Null()]),
  pkceRequired: Type.Boolean(),
  applicationIds: Type.Array(Type.String()),
  serviceAccountPrincipalId: Type.Union([Type.String(), Type.Null()]),
  active: Type.Boolean(),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const OAuthClientListResponseSchema = Type.Object({
  clients: Type.Array(OAuthClientResponseSchema),
  total: Type.Integer(),
});

type OAuthClientResponse = Static<typeof OAuthClientResponseSchema>;

/**
 * Dependencies for the OAuth clients API.
 */
export interface OAuthClientsRoutesDeps {
  readonly oauthClientRepository: OAuthClientRepository;
  readonly createOAuthClientUseCase: UseCase<CreateOAuthClientCommand, OAuthClientCreated>;
  readonly updateOAuthClientUseCase: UseCase<UpdateOAuthClientCommand, OAuthClientUpdated>;
  readonly regenerateOAuthClientSecretUseCase: UseCase<
    RegenerateOAuthClientSecretCommand,
    OAuthClientSecretRegenerated
  >;
  readonly deleteOAuthClientUseCase: UseCase<DeleteOAuthClientCommand, OAuthClientDeleted>;
}

/**
 * Convert OAuthClient to response.
 */
function toResponse(client: OAuthClient): OAuthClientResponse {
  return {
    id: client.id,
    clientId: client.clientId,
    clientName: client.clientName,
    clientType: client.clientType,
    hasClientSecret: Boolean(client.clientSecretRef),
    redirectUris: [...client.redirectUris],
    allowedOrigins: [...client.allowedOrigins],
    grantTypes: [...client.grantTypes],
    defaultScopes: client.defaultScopes,
    pkceRequired: client.pkceRequired,
    applicationIds: [...client.applicationIds],
    serviceAccountPrincipalId: client.serviceAccountPrincipalId,
    active: client.active,
    createdAt: client.createdAt.toISOString(),
    updatedAt: client.updatedAt.toISOString(),
  };
}

/**
 * Register OAuth client admin API routes.
 */
export async function registerOAuthClientsRoutes(
  fastify: FastifyInstance,
  deps: OAuthClientsRoutesDeps,
): Promise<void> {
  const {
    oauthClientRepository,
    createOAuthClientUseCase,
    updateOAuthClientUseCase,
    regenerateOAuthClientSecretUseCase,
    deleteOAuthClientUseCase,
  } = deps;

  // GET /api/admin/oauth-clients - List all OAuth clients
  fastify.get(
    '/oauth-clients',
    {
      preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.READ),
      schema: {
        querystring: ListOAuthClientsQuery,
        response: {
          200: OAuthClientListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof ListOAuthClientsQuery>;

      let clients: OAuthClient[];
      if (query.active === 'true') {
        clients = await oauthClientRepository.findActive();
      } else {
        clients = await oauthClientRepository.findAll();
      }

      return jsonSuccess(reply, {
        clients: clients.map(toResponse),
        total: clients.length,
      });
    },
  );

  // GET /api/admin/oauth-clients/:id - Get OAuth client by ID
  fastify.get(
    '/oauth-clients/:id',
    {
      preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: OAuthClientResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const client = await oauthClientRepository.findById(id);

      if (!client) {
        return notFound(reply, `OAuth client not found: ${id}`);
      }

      return jsonSuccess(reply, toResponse(client));
    },
  );

  // GET /api/admin/oauth-clients/by-client-id/:clientId - Get OAuth client by clientId
  fastify.get(
    '/oauth-clients/by-client-id/:clientId',
    {
      preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.READ),
      schema: {
        params: ClientIdParam,
        response: {
          200: OAuthClientResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { clientId } = request.params as Static<typeof ClientIdParam>;
      const client = await oauthClientRepository.findByClientId(clientId);

      if (!client) {
        return notFound(reply, `OAuth client not found: ${clientId}`);
      }

      return jsonSuccess(reply, toResponse(client));
    },
  );

  // POST /api/admin/oauth-clients - Create OAuth client
  fastify.post(
    '/oauth-clients',
    {
      preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.CREATE),
      schema: {
        body: CreateOAuthClientSchema,
        response: {
          201: OAuthClientResponseSchema,
          400: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as CreateOAuthClientBody;
      const ctx = request.executionContext;

      const command: CreateOAuthClientCommand = {
        clientId: body.clientId,
        clientName: body.clientName,
        clientType: body.clientType,
        clientSecretRef: body.clientSecretRef,
        redirectUris: body.redirectUris,
        allowedOrigins: body.allowedOrigins,
        grantTypes: body.grantTypes,
        defaultScopes: body.defaultScopes,
        pkceRequired: body.pkceRequired,
        applicationIds: body.applicationIds,
      };

      const result = await createOAuthClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await oauthClientRepository.findById(result.value.getData().oauthClientId);
        if (client) {
          return jsonCreated(reply, toResponse(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // PUT /api/admin/oauth-clients/:id - Update OAuth client
  fastify.put(
    '/oauth-clients/:id',
    {
      preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: UpdateOAuthClientSchema,
        response: {
          200: OAuthClientResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as UpdateOAuthClientBody;
      const ctx = request.executionContext;

      const command: UpdateOAuthClientCommand = {
        oauthClientId: id,
        clientName: body.clientName,
        redirectUris: body.redirectUris,
        allowedOrigins: body.allowedOrigins,
        grantTypes: body.grantTypes,
        defaultScopes: body.defaultScopes,
        pkceRequired: body.pkceRequired,
        applicationIds: body.applicationIds,
        active: body.active,
      };

      const result = await updateOAuthClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await oauthClientRepository.findById(id);
        if (client) {
          return jsonSuccess(reply, toResponse(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/oauth-clients/:id/regenerate-secret - Regenerate client secret
  fastify.post(
    '/oauth-clients/:id/regenerate-secret',
    {
      preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.REGENERATE_SECRET),
      schema: {
        params: IdParam,
        body: RegenerateSecretSchema,
        response: {
          200: OAuthClientResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as RegenerateSecretBody;
      const ctx = request.executionContext;

      const command: RegenerateOAuthClientSecretCommand = {
        oauthClientId: id,
        newSecretRef: body.newSecretRef,
      };

      const result = await regenerateOAuthClientSecretUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await oauthClientRepository.findById(id);
        if (client) {
          return jsonSuccess(reply, toResponse(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /api/admin/oauth-clients/:id - Delete OAuth client
  fastify.delete(
    '/oauth-clients/:id',
    {
      preHandler: requirePermission(OAUTH_CLIENT_PERMISSIONS.DELETE),
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

      const command: DeleteOAuthClientCommand = {
        oauthClientId: id,
      };

      const result = await deleteOAuthClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );
}
