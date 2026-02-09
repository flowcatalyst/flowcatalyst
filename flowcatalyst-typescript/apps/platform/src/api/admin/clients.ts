/**
 * Clients Admin API
 *
 * REST endpoints for client management.
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
  CreateClientCommand,
  UpdateClientCommand,
  ChangeClientStatusCommand,
  DeleteClientCommand,
  AddClientNoteCommand,
  EnableApplicationForClientCommand,
  DisableApplicationForClientCommand,
} from '../../application/index.js';
import type {
  ClientCreated,
  ClientUpdated,
  ClientStatusChanged,
  ClientDeleted,
  ClientNoteAdded,
  ClientStatus,
  ApplicationEnabledForClient,
  ApplicationDisabledForClient,
} from '../../domain/index.js';
import type {
  ClientRepository,
  ApplicationClientConfigRepository,
  ApplicationRepository,
} from '../../infrastructure/persistence/index.js';
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

const ChangeStatusSchema = Type.Object({
  reason: Type.Optional(Type.Union([Type.String({ maxLength: 255 }), Type.Null()])),
  note: Type.Optional(Type.Union([Type.String({ maxLength: 1000 }), Type.Null()])),
});

const AddNoteSchema = Type.Object({
  category: Type.String({ minLength: 1, maxLength: 50 }),
  text: Type.String({ minLength: 1, maxLength: 1000 }),
});

const IdParam = Type.Object({ id: Type.String() });
const IdentifierParam = Type.Object({ identifier: Type.String() });

const ListClientsQuery = Type.Object({
  status: Type.Optional(Type.String()),
  page: Type.Optional(Type.String()),
  pageSize: Type.Optional(Type.String()),
});

const SearchClientsQuery = Type.Object({
  q: Type.Optional(Type.String()),
  status: Type.Optional(Type.String()),
  limit: Type.Optional(Type.String()),
});

type CreateClientBody = Static<typeof CreateClientSchema>;
type UpdateClientBody = Static<typeof UpdateClientSchema>;
type ChangeStatusBody = Static<typeof ChangeStatusSchema>;
type AddNoteBody = Static<typeof AddNoteSchema>;

// ─── Response Schemas ───────────────────────────────────────────────────────

const ClientNoteResponseSchema = Type.Object({
  category: Type.String(),
  text: Type.String(),
  addedBy: Type.String(),
  addedAt: Type.String({ format: 'date-time' }),
});

const ClientResponseSchema = Type.Object({
  id: Type.String(),
  name: Type.String(),
  identifier: Type.String(),
  status: Type.String(),
  statusReason: Type.Union([Type.String(), Type.Null()]),
  statusChangedAt: Type.Union([Type.String({ format: 'date-time' }), Type.Null()]),
  notes: Type.Array(ClientNoteResponseSchema),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const ClientsListResponseSchema = Type.Object({
  clients: Type.Array(ClientResponseSchema),
  total: Type.Integer(),
  page: Type.Integer(),
  pageSize: Type.Integer(),
});

type ClientResponse = Static<typeof ClientResponseSchema>;

/**
 * Dependencies for the clients API.
 */
export interface ClientsRoutesDeps {
  readonly clientRepository: ClientRepository;
  readonly applicationRepository: ApplicationRepository;
  readonly applicationClientConfigRepository: ApplicationClientConfigRepository;
  readonly createClientUseCase: UseCase<CreateClientCommand, ClientCreated>;
  readonly updateClientUseCase: UseCase<UpdateClientCommand, ClientUpdated>;
  readonly changeClientStatusUseCase: UseCase<ChangeClientStatusCommand, ClientStatusChanged>;
  readonly deleteClientUseCase: UseCase<DeleteClientCommand, ClientDeleted>;
  readonly addClientNoteUseCase: UseCase<AddClientNoteCommand, ClientNoteAdded>;
  readonly enableApplicationForClientUseCase: UseCase<
    EnableApplicationForClientCommand,
    ApplicationEnabledForClient
  >;
  readonly disableApplicationForClientUseCase: UseCase<
    DisableApplicationForClientCommand,
    ApplicationDisabledForClient
  >;
}

/**
 * Register client admin API routes.
 */
export async function registerClientsRoutes(
  fastify: FastifyInstance,
  deps: ClientsRoutesDeps,
): Promise<void> {
  const {
    clientRepository,
    applicationRepository,
    applicationClientConfigRepository,
    createClientUseCase,
    updateClientUseCase,
    changeClientStatusUseCase,
    deleteClientUseCase,
    addClientNoteUseCase,
    enableApplicationForClientUseCase,
    disableApplicationForClientUseCase,
  } = deps;

  // POST /api/admin/clients - Create client
  fastify.post(
    '/clients',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.CREATE),
      schema: {
        body: CreateClientSchema,
        response: {
          201: ClientResponseSchema,
          400: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as CreateClientBody;
      const ctx = request.executionContext;

      const command: CreateClientCommand = {
        name: body.name,
        identifier: body.identifier,
      };

      const result = await createClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        // Fetch the created client for full response
        const client = await clientRepository.findById(result.value.getData().clientId);
        if (client) {
          return jsonCreated(reply, toClientResponse(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/clients - List clients (with optional status filter)
  fastify.get(
    '/clients',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
      schema: {
        querystring: ListClientsQuery,
        response: {
          200: ClientsListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof ListClientsQuery>;
      const page = parseInt(query.page ?? '0', 10);
      const pageSize = Math.min(parseInt(query.pageSize ?? '20', 10), 100);
      const accessibleClientIds = getAccessibleClientIds(request.audit?.principal);

      if (query.status) {
        // Filter by status
        const filtered = await clientRepository.findByStatus(query.status, accessibleClientIds);
        const start = page * pageSize;
        const pageItems = filtered.slice(start, start + pageSize);
        return jsonSuccess(reply, {
          clients: pageItems.map(toClientResponse),
          total: filtered.length,
          page,
          pageSize,
        });
      }

      const pagedResult = await clientRepository.findPagedScoped(
        page,
        pageSize,
        accessibleClientIds,
      );

      return jsonSuccess(reply, {
        clients: pagedResult.items.map(toClientResponse),
        total: pagedResult.totalItems,
        page: pagedResult.page,
        pageSize: pagedResult.pageSize,
      });
    },
  );

  // GET /api/admin/clients/search - Search clients
  fastify.get(
    '/clients/search',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
      schema: {
        querystring: SearchClientsQuery,
        response: {
          200: ClientsListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof SearchClientsQuery>;
      const q = query.q ?? '';
      const limit = Math.min(parseInt(query.limit ?? '20', 10), 100);
      const accessibleClientIds = getAccessibleClientIds(request.audit?.principal);

      if (!q) {
        return jsonSuccess(reply, { clients: [], total: 0, page: 0, pageSize: limit });
      }

      const results = await clientRepository.search(q, query.status, limit, accessibleClientIds);

      return jsonSuccess(reply, {
        clients: results.map(toClientResponse),
        total: results.length,
        page: 0,
        pageSize: limit,
      });
    },
  );

  // GET /api/admin/clients/:id - Get client by ID
  fastify.get(
    '/clients/:id',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: ClientResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const client = await clientRepository.findById(id);

      if (!client || !canAccessResourceByClient(client.id, request.audit?.principal)) {
        return notFound(reply, `Client not found: ${id}`);
      }

      return jsonSuccess(reply, toClientResponse(client));
    },
  );

  // GET /api/admin/clients/by-identifier/:identifier - Get client by identifier
  fastify.get(
    '/clients/by-identifier/:identifier',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
      schema: {
        params: IdentifierParam,
        response: {
          200: ClientResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { identifier } = request.params as Static<typeof IdentifierParam>;
      const client = await clientRepository.findByIdentifier(identifier);

      if (!client || !canAccessResourceByClient(client.id, request.audit?.principal)) {
        return notFound(reply, `Client not found with identifier: ${identifier}`);
      }

      return jsonSuccess(reply, toClientResponse(client));
    },
  );

  // PUT /api/admin/clients/:id - Update client
  fastify.put(
    '/clients/:id',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: UpdateClientSchema,
        response: {
          200: ClientResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as UpdateClientBody;
      const ctx = request.executionContext;

      const command: UpdateClientCommand = {
        clientId: id,
        name: body.name,
      };

      const result = await updateClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await clientRepository.findById(id);
        if (client) {
          return jsonSuccess(reply, toClientResponse(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/clients/:id/activate - Activate client
  fastify.post(
    '/clients/:id/activate',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.ACTIVATE),
      schema: {
        params: IdParam,
        body: ChangeStatusSchema,
        response: {
          200: ClientResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = (request.body ?? {}) as ChangeStatusBody;
      const ctx = request.executionContext;

      const command: ChangeClientStatusCommand = {
        clientId: id,
        newStatus: 'ACTIVE' as ClientStatus,
        reason: body.reason ?? null,
        note: body.note ?? null,
      };

      const result = await changeClientStatusUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await clientRepository.findById(id);
        if (client) {
          return jsonSuccess(reply, toClientResponse(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/clients/:id/suspend - Suspend client
  fastify.post(
    '/clients/:id/suspend',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.SUSPEND),
      schema: {
        params: IdParam,
        body: ChangeStatusSchema,
        response: {
          200: ClientResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = (request.body ?? {}) as ChangeStatusBody;
      const ctx = request.executionContext;

      const command: ChangeClientStatusCommand = {
        clientId: id,
        newStatus: 'SUSPENDED' as ClientStatus,
        reason: body.reason ?? null,
        note: body.note ?? null,
      };

      const result = await changeClientStatusUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await clientRepository.findById(id);
        if (client) {
          return jsonSuccess(reply, toClientResponse(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/clients/:id/deactivate - Deactivate client
  fastify.post(
    '/clients/:id/deactivate',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.DEACTIVATE),
      schema: {
        params: IdParam,
        body: ChangeStatusSchema,
        response: {
          200: ClientResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = (request.body ?? {}) as ChangeStatusBody;
      const ctx = request.executionContext;

      const command: ChangeClientStatusCommand = {
        clientId: id,
        newStatus: 'INACTIVE' as ClientStatus,
        reason: body.reason ?? null,
        note: body.note ?? null,
      };

      const result = await changeClientStatusUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await clientRepository.findById(id);
        if (client) {
          return jsonSuccess(reply, toClientResponse(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/clients/:id/notes - Add note to client
  fastify.post(
    '/clients/:id/notes',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: AddNoteSchema,
        response: {
          200: ClientResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as AddNoteBody;
      const ctx = request.executionContext;

      const command: AddClientNoteCommand = {
        clientId: id,
        category: body.category,
        text: body.text,
      };

      const result = await addClientNoteUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const client = await clientRepository.findById(id);
        if (client) {
          return jsonSuccess(reply, toClientResponse(client));
        }
      }

      return sendResult(reply, result);
    },
  );

  // GET /api/admin/clients/:id/applications - List applications for client
  fastify.get(
    '/clients/:id/applications',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: Type.Object({
            applications: Type.Array(
              Type.Object({
                id: Type.String(),
                code: Type.String(),
                name: Type.String(),
                description: Type.Union([Type.String(), Type.Null()]),
                iconUrl: Type.Union([Type.String(), Type.Null()]),
                website: Type.Union([Type.String(), Type.Null()]),
                logoMimeType: Type.Union([Type.String(), Type.Null()]),
                active: Type.Boolean(),
                enabledForClient: Type.Boolean(),
              }),
            ),
            total: Type.Integer(),
          }),
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const client = await clientRepository.findById(id);

      if (!client || !canAccessResourceByClient(client.id, request.audit?.principal)) {
        return notFound(reply, `Client not found: ${id}`);
      }

      const [allApps, configs] = await Promise.all([
        applicationRepository.findAll(),
        applicationClientConfigRepository.findByClient(id),
      ]);

      // Build set of enabled application IDs
      const enabledSet = new Set(configs.filter((c) => c.enabled).map((c) => c.applicationId));

      const applications = allApps.map((app) => ({
        id: app.id,
        code: app.code,
        name: app.name,
        description: app.description ?? null,
        iconUrl: app.iconUrl ?? null,
        website: app.website ?? null,
        logoMimeType: app.logoMimeType ?? null,
        active: app.active,
        enabledForClient: enabledSet.has(app.id),
      }));

      return jsonSuccess(reply, {
        applications,
        total: applications.length,
      });
    },
  );

  // PUT /api/admin/clients/:id/applications - Bulk update enabled applications
  fastify.put(
    '/clients/:id/applications',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: Type.Object({
          enabledApplicationIds: Type.Array(Type.String()),
        }),
        response: {
          200: Type.Object({ message: Type.String() }),
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const { enabledApplicationIds } = request.body as { enabledApplicationIds: string[] };
      const ctx = request.executionContext;

      const client = await clientRepository.findById(id);
      if (!client || !canAccessResourceByClient(client.id, request.audit?.principal)) {
        return notFound(reply, `Client not found: ${id}`);
      }

      // Get current configs
      const currentConfigs = await applicationClientConfigRepository.findByClient(id);
      const currentlyEnabled = new Set(currentConfigs.filter((c) => c.enabled).map((c) => c.applicationId));
      const desiredEnabled = new Set(enabledApplicationIds);

      // Enable apps that should be enabled but aren't
      for (const appId of enabledApplicationIds) {
        if (!currentlyEnabled.has(appId)) {
          await enableApplicationForClientUseCase.execute(
            { applicationId: appId, clientId: id },
            ctx,
          );
        }
      }

      // Disable apps that are currently enabled but shouldn't be
      for (const appId of currentlyEnabled) {
        if (!desiredEnabled.has(appId)) {
          await disableApplicationForClientUseCase.execute(
            { applicationId: appId, clientId: id },
            ctx,
          );
        }
      }

      return jsonSuccess(reply, { message: 'Applications updated' });
    },
  );

  // POST /api/admin/clients/:id/applications/:appId/enable - Enable application for client
  fastify.post(
    '/clients/:id/applications/:appId/enable',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
      schema: {
        params: Type.Object({ id: Type.String(), appId: Type.String() }),
        response: {
          200: Type.Object({ message: Type.String() }),
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id, appId } = request.params as { id: string; appId: string };
      const ctx = request.executionContext;

      const command: EnableApplicationForClientCommand = {
        applicationId: appId,
        clientId: id,
      };

      const result = await enableApplicationForClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return jsonSuccess(reply, { message: 'Application enabled for client' });
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/clients/:id/applications/:appId/disable - Disable application for client
  fastify.post(
    '/clients/:id/applications/:appId/disable',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.UPDATE),
      schema: {
        params: Type.Object({ id: Type.String(), appId: Type.String() }),
        response: {
          200: Type.Object({ message: Type.String() }),
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id, appId } = request.params as { id: string; appId: string };
      const ctx = request.executionContext;

      const command: DisableApplicationForClientCommand = {
        applicationId: appId,
        clientId: id,
      };

      const result = await disableApplicationForClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return jsonSuccess(reply, { message: 'Application disabled for client' });
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /api/admin/clients/:id - Delete client
  fastify.delete(
    '/clients/:id',
    {
      preHandler: requirePermission(CLIENT_PERMISSIONS.DELETE),
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

      const command: DeleteClientCommand = {
        clientId: id,
      };

      const result = await deleteClientUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );
}

/**
 * Convert a Client entity to a ClientResponse.
 */
function toClientResponse(client: {
  id: string;
  name: string;
  identifier: string;
  status: string;
  statusReason: string | null;
  statusChangedAt: Date | null;
  notes: readonly { category: string; text: string; addedBy: string; addedAt: Date }[];
  createdAt: Date;
  updatedAt: Date;
}): ClientResponse {
  return {
    id: client.id,
    name: client.name,
    identifier: client.identifier,
    status: client.status,
    statusReason: client.statusReason,
    statusChangedAt: client.statusChangedAt?.toISOString() ?? null,
    notes: client.notes.map((n) => ({
      category: n.category,
      text: n.text,
      addedBy: n.addedBy,
      addedAt: n.addedAt.toISOString(),
    })),
    createdAt: client.createdAt.toISOString(),
    updatedAt: client.updatedAt.toISOString(),
  };
}
