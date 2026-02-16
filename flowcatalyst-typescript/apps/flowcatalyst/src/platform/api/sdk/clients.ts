/**
 * SDK Clients API
 *
 * REST endpoints for client management via external SDKs.
 * Uses Bearer token authentication (not BFF session).
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import {
  sendResult,
  jsonCreated,
  jsonSuccess,
  notFound,
  ErrorResponseSchema,
} from '@flowcatalyst/http';
import { Result } from '@flowcatalyst/application';
import type { UseCase } from '@flowcatalyst/application';

import type {
  CreateClientCommand,
  UpdateClientCommand,
  ChangeClientStatusCommand,
  DeleteClientCommand,
} from '../../application/index.js';
import type {
  ClientCreated,
  ClientUpdated,
  ClientStatusChanged,
  ClientDeleted,
  ClientStatus,
} from '../../domain/index.js';
import type { ClientRepository } from '../../infrastructure/persistence/index.js';
import {
  requirePermission,
  getAccessibleClientIds,
  canAccessResourceByClient,
} from '../../authorization/index.js';
import { CLIENT_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateClientSchema = Type.Object({
  name: Type.String({ minLength: 1, maxLength: 255 }),
  identifier: Type.String({ minLength: 1, maxLength: 60 }),
});

const UpdateClientSchema = Type.Object({
  name: Type.String({ minLength: 1, maxLength: 255 }),
});

const StatusChangeSchema = Type.Object({
  reason: Type.Optional(Type.Union([Type.String({ maxLength: 255 }), Type.Null()])),
});

const IdParam = Type.Object({ id: Type.String() });

// ─── Response Schemas ───────────────────────────────────────────────────────

const SdkClientResponseSchema = Type.Object({
  id: Type.String(),
  name: Type.String(),
  identifier: Type.String(),
  status: Type.String(),
  statusReason: Type.Union([Type.String(), Type.Null()]),
  statusChangedAt: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const SdkClientListResponseSchema = Type.Object({
  clients: Type.Array(SdkClientResponseSchema),
  total: Type.Integer(),
});

const MessageResponseSchema = Type.Object({
  message: Type.String(),
});

type SdkClientResponse = Static<typeof SdkClientResponseSchema>;

/**
 * Dependencies for the SDK clients API.
 */
export interface SdkClientsDeps {
  readonly clientRepository: ClientRepository;
  readonly createClientUseCase: UseCase<CreateClientCommand, ClientCreated>;
  readonly updateClientUseCase: UseCase<UpdateClientCommand, ClientUpdated>;
  readonly changeClientStatusUseCase: UseCase<ChangeClientStatusCommand, ClientStatusChanged>;
  readonly deleteClientUseCase: UseCase<DeleteClientCommand, ClientDeleted>;
}

/**
 * Register SDK client routes.
 */
export async function registerSdkClientsRoutes(
  fastify: FastifyInstance,
  deps: SdkClientsDeps,
): Promise<void> {
  const {
    clientRepository,
    createClientUseCase,
    updateClientUseCase,
    changeClientStatusUseCase,
    deleteClientUseCase,
  } = deps;

  // GET /api/sdk/clients - List clients
  fastify.get(
    '/clients',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
      schema: {
        response: {
          200: SdkClientListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const principal = request.audit?.principal ?? null;
      const accessibleClientIds = getAccessibleClientIds(principal);
      const pagedResult = await clientRepository.findPagedScoped(0, 1000, accessibleClientIds);

      return jsonSuccess(reply, {
        clients: pagedResult.items.map(toSdkClient),
        total: pagedResult.totalItems,
      });
    },
  );

  // GET /api/sdk/clients/:id - Get client by ID
  fastify.get(
    '/clients/:id',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: SdkClientResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const client = await clientRepository.findById(id);
      const principal = request.audit?.principal ?? null;

      if (!client || !canAccessResourceByClient(client.id, principal)) {
        return notFound(reply, `Client not found: ${id}`);
      }

      return jsonSuccess(reply, toSdkClient(client));
    },
  );

  // POST /api/sdk/clients - Create client
  fastify.post(
    '/clients',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.CREATE),
      schema: {
        body: CreateClientSchema,
        response: {
          201: SdkClientResponseSchema,
          400: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as Static<typeof CreateClientSchema>;
      const ctx = request.executionContext;

      const command: CreateClientCommand = {
        name: body.name,
        identifier: body.identifier,
      };

      const result = await createClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await clientRepository.findById(result.value.getData().clientId);
        if (client) {
          return jsonCreated(reply, toSdkClient(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // PUT /api/sdk/clients/:id - Update client
  fastify.put(
    '/clients/:id',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: UpdateClientSchema,
        response: {
          200: SdkClientResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof UpdateClientSchema>;
      const ctx = request.executionContext;

      const command: UpdateClientCommand = {
        clientId: id,
        name: body.name,
      };

      const result = await updateClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await clientRepository.findById(id);
        if (client) {
          return jsonSuccess(reply, toSdkClient(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/sdk/clients/:id/activate - Activate client
  fastify.post(
    '/clients/:id/activate',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: StatusChangeSchema,
        response: {
          200: MessageResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof StatusChangeSchema>;
      const ctx = request.executionContext;

      const command: ChangeClientStatusCommand = {
        clientId: id,
        newStatus: 'ACTIVE' as ClientStatus,
        reason: body.reason ?? null,
        note: null,
      };

      const result = await changeClientStatusUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return jsonSuccess(reply, { message: 'Client activated' });
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/sdk/clients/:id/suspend - Suspend client
  fastify.post(
    '/clients/:id/suspend',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: StatusChangeSchema,
        response: {
          200: MessageResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof StatusChangeSchema>;
      const ctx = request.executionContext;

      const command: ChangeClientStatusCommand = {
        clientId: id,
        newStatus: 'SUSPENDED' as ClientStatus,
        reason: body.reason ?? null,
        note: null,
      };

      const result = await changeClientStatusUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return jsonSuccess(reply, { message: 'Client suspended' });
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/sdk/clients/:id/deactivate - Deactivate client
  fastify.post(
    '/clients/:id/deactivate',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.DELETE),
      schema: {
        params: IdParam,
        body: StatusChangeSchema,
        response: {
          200: MessageResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof StatusChangeSchema>;
      const ctx = request.executionContext;

      const command: ChangeClientStatusCommand = {
        clientId: id,
        newStatus: 'INACTIVE' as ClientStatus,
        reason: body.reason ?? null,
        note: null,
      };

      const result = await changeClientStatusUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return jsonSuccess(reply, { message: 'Client deactivated' });
      }

      return sendResult(reply, result);
    },
  );
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toSdkClient(client: {
  id: string;
  name: string;
  identifier: string;
  status: string;
  statusReason: string | null;
  statusChangedAt: Date | null;
  createdAt: Date;
  updatedAt: Date;
}): SdkClientResponse {
  return {
    id: client.id,
    name: client.name,
    identifier: client.identifier,
    status: client.status,
    statusReason: client.statusReason,
    statusChangedAt: client.statusChangedAt?.toISOString() ?? null,
    createdAt: client.createdAt.toISOString(),
    updatedAt: client.updatedAt.toISOString(),
  };
}
