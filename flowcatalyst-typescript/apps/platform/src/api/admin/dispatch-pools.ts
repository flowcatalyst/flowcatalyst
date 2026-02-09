/**
 * Dispatch Pools Admin API
 *
 * REST endpoints for dispatch pool management.
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
  CreateDispatchPoolCommand,
  UpdateDispatchPoolCommand,
  DeleteDispatchPoolCommand,
  SyncDispatchPoolsCommand,
} from '../../application/index.js';
import type {
  DispatchPoolCreated,
  DispatchPoolUpdated,
  DispatchPoolDeleted,
  DispatchPoolsSynced,
  DispatchPool,
  DispatchPoolStatus,
} from '../../domain/index.js';
import type {
  DispatchPoolRepository,
  DispatchPoolFilters,
} from '../../infrastructure/persistence/index.js';
import {
  requirePermission,
  getAccessibleClientIds,
  canAccessResourceByClient,
} from '../../authorization/index.js';
import { DISPATCH_POOL_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateDispatchPoolSchema = Type.Object({
  code: Type.String({ minLength: 2, maxLength: 100, pattern: '^[a-z][a-z0-9-]*$' }),
  name: Type.String({ minLength: 1, maxLength: 255 }),
  description: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
  rateLimit: Type.Optional(Type.Integer({ minimum: 1 })),
  concurrency: Type.Optional(Type.Integer({ minimum: 1 })),
  clientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
});

const UpdateDispatchPoolSchema = Type.Object({
  name: Type.Optional(Type.String({ minLength: 1, maxLength: 255 })),
  description: Type.Optional(Type.Union([Type.String({ maxLength: 500 }), Type.Null()])),
  rateLimit: Type.Optional(Type.Integer({ minimum: 1 })),
  concurrency: Type.Optional(Type.Integer({ minimum: 1 })),
  status: Type.Optional(Type.Union([Type.Literal('ACTIVE'), Type.Literal('SUSPENDED')])),
});

const SyncDispatchPoolsSchema = Type.Object({
  applicationCode: Type.String({ minLength: 1 }),
  pools: Type.Array(
    Type.Object({
      code: Type.String({ minLength: 1 }),
      name: Type.String({ minLength: 1 }),
      description: Type.Optional(Type.Union([Type.String(), Type.Null()])),
      rateLimit: Type.Optional(Type.Integer({ minimum: 1 })),
      concurrency: Type.Optional(Type.Integer({ minimum: 1 })),
    }),
  ),
  removeUnlisted: Type.Optional(Type.Boolean()),
});

type CreateDispatchPoolBody = Static<typeof CreateDispatchPoolSchema>;
type UpdateDispatchPoolBody = Static<typeof UpdateDispatchPoolSchema>;
type SyncDispatchPoolsBody = Static<typeof SyncDispatchPoolsSchema>;

// ─── Param Schemas ──────────────────────────────────────────────────────────

const IdParam = Type.Object({ id: Type.String() });

// ─── Query Schemas ──────────────────────────────────────────────────────────

const DispatchPoolListQuerySchema = Type.Object({
  clientId: Type.Optional(Type.String()),
  status: Type.Optional(Type.String()),
  anchorLevel: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const DispatchPoolResponseSchema = Type.Object({
  id: Type.String(),
  code: Type.String(),
  name: Type.String(),
  description: Type.Union([Type.String(), Type.Null()]),
  rateLimit: Type.Integer(),
  concurrency: Type.Integer(),
  clientId: Type.Union([Type.String(), Type.Null()]),
  clientIdentifier: Type.Union([Type.String(), Type.Null()]),
  status: Type.String(),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const DispatchPoolListResponseSchema = Type.Object({
  pools: Type.Array(DispatchPoolResponseSchema),
  total: Type.Integer(),
});

const SyncResponseSchema = Type.Object({
  applicationCode: Type.String(),
  created: Type.Integer(),
  updated: Type.Integer(),
  deleted: Type.Integer(),
  syncedCodes: Type.Array(Type.String()),
});

type DispatchPoolResponse = Static<typeof DispatchPoolResponseSchema>;

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface DispatchPoolsRoutesDeps {
  readonly dispatchPoolRepository: DispatchPoolRepository;
  readonly createDispatchPoolUseCase: UseCase<CreateDispatchPoolCommand, DispatchPoolCreated>;
  readonly updateDispatchPoolUseCase: UseCase<UpdateDispatchPoolCommand, DispatchPoolUpdated>;
  readonly deleteDispatchPoolUseCase: UseCase<DeleteDispatchPoolCommand, DispatchPoolDeleted>;
  readonly syncDispatchPoolsUseCase: UseCase<SyncDispatchPoolsCommand, DispatchPoolsSynced>;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerDispatchPoolsRoutes(
  fastify: FastifyInstance,
  deps: DispatchPoolsRoutesDeps,
): Promise<void> {
  const {
    dispatchPoolRepository,
    createDispatchPoolUseCase,
    updateDispatchPoolUseCase,
    deleteDispatchPoolUseCase,
    syncDispatchPoolsUseCase,
  } = deps;

  // GET /api/admin/dispatch-pools - List with filters
  fastify.get(
    '/dispatch-pools',
    {
      preHandler: requirePermission(DISPATCH_POOL_PERMISSIONS.READ),
      schema: {
        querystring: DispatchPoolListQuerySchema,
        response: {
          200: DispatchPoolListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof DispatchPoolListQuerySchema>;

      const accessibleClientIds = getAccessibleClientIds();

      const filters: DispatchPoolFilters = {
        ...(query.anchorLevel === 'true' ? { clientId: null } : {}),
        ...(query.clientId && query.anchorLevel !== 'true' ? { clientId: query.clientId } : {}),
        ...(query.status ? { status: query.status as DispatchPoolStatus } : {}),
        accessibleClientIds,
      };

      const pools = await dispatchPoolRepository.findWithFilters(filters);

      return jsonSuccess(reply, {
        pools: pools.map(toDispatchPoolResponse),
        total: pools.length,
      });
    },
  );

  // GET /api/admin/dispatch-pools/:id - Get by ID
  fastify.get(
    '/dispatch-pools/:id',
    {
      preHandler: requirePermission(DISPATCH_POOL_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: DispatchPoolResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const pool = await dispatchPoolRepository.findById(id);

      if (!pool || !canAccessResourceByClient(pool.clientId)) {
        return notFound(reply, `Dispatch pool not found: ${id}`);
      }

      return jsonSuccess(reply, toDispatchPoolResponse(pool));
    },
  );

  // POST /api/admin/dispatch-pools - Create
  fastify.post(
    '/dispatch-pools',
    {
      preHandler: requirePermission(DISPATCH_POOL_PERMISSIONS.CREATE),
      schema: {
        body: CreateDispatchPoolSchema,
        response: {
          201: DispatchPoolResponseSchema,
          400: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as CreateDispatchPoolBody;
      const ctx = request.executionContext;

      const command: CreateDispatchPoolCommand = {
        code: body.code,
        name: body.name,
        description: body.description ?? null,
        rateLimit: body.rateLimit,
        concurrency: body.concurrency,
        clientId: body.clientId ?? null,
      };

      const result = await createDispatchPoolUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const pool = await dispatchPoolRepository.findById(result.value.getData().poolId);
        if (pool) {
          return jsonCreated(reply, toDispatchPoolResponse(pool));
        }
      }

      return sendResult(reply, result);
    },
  );

  // PUT /api/admin/dispatch-pools/:id - Update
  fastify.put(
    '/dispatch-pools/:id',
    {
      preHandler: requirePermission(DISPATCH_POOL_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: UpdateDispatchPoolSchema,
        response: {
          200: DispatchPoolResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as UpdateDispatchPoolBody;
      const ctx = request.executionContext;

      const command: UpdateDispatchPoolCommand = {
        poolId: id,
        ...(body.name !== undefined ? { name: body.name } : {}),
        ...(body.description !== undefined ? { description: body.description } : {}),
        ...(body.rateLimit !== undefined ? { rateLimit: body.rateLimit } : {}),
        ...(body.concurrency !== undefined ? { concurrency: body.concurrency } : {}),
        ...(body.status !== undefined ? { status: body.status as DispatchPoolStatus } : {}),
      };

      const result = await updateDispatchPoolUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const pool = await dispatchPoolRepository.findById(id);
        if (pool) {
          return jsonSuccess(reply, toDispatchPoolResponse(pool));
        }
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /api/admin/dispatch-pools/:id - Delete (archive)
  fastify.delete(
    '/dispatch-pools/:id',
    {
      preHandler: requirePermission(DISPATCH_POOL_PERMISSIONS.DELETE),
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

      const command: DeleteDispatchPoolCommand = { poolId: id };
      const result = await deleteDispatchPoolUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/dispatch-pools/:id/suspend - Suspend
  fastify.post(
    '/dispatch-pools/:id/suspend',
    {
      preHandler: requirePermission(DISPATCH_POOL_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        response: {
          200: DispatchPoolResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: UpdateDispatchPoolCommand = { poolId: id, status: 'SUSPENDED' };
      const result = await updateDispatchPoolUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const pool = await dispatchPoolRepository.findById(id);
        if (pool) {
          return jsonSuccess(reply, toDispatchPoolResponse(pool));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/dispatch-pools/:id/activate - Activate
  fastify.post(
    '/dispatch-pools/:id/activate',
    {
      preHandler: requirePermission(DISPATCH_POOL_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        response: {
          200: DispatchPoolResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: UpdateDispatchPoolCommand = { poolId: id, status: 'ACTIVE' };
      const result = await updateDispatchPoolUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const pool = await dispatchPoolRepository.findById(id);
        if (pool) {
          return jsonSuccess(reply, toDispatchPoolResponse(pool));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/dispatch-pools/sync - Sync from SDK
  fastify.post(
    '/dispatch-pools/sync',
    {
      preHandler: requirePermission(DISPATCH_POOL_PERMISSIONS.SYNC),
      schema: {
        body: SyncDispatchPoolsSchema,
        response: {
          200: SyncResponseSchema,
          400: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as SyncDispatchPoolsBody;
      const ctx = request.executionContext;

      const command: SyncDispatchPoolsCommand = {
        applicationCode: body.applicationCode,
        pools: body.pools,
        removeUnlisted: body.removeUnlisted ?? false,
      };

      const result = await syncDispatchPoolsUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const data = result.value.getData();
        return jsonSuccess(reply, {
          applicationCode: data.applicationCode,
          created: data.poolsCreated,
          updated: data.poolsUpdated,
          deleted: data.poolsDeleted,
          syncedCodes: data.syncedPoolCodes,
        });
      }

      return sendResult(reply, result);
    },
  );
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toDispatchPoolResponse(pool: DispatchPool): DispatchPoolResponse {
  return {
    id: pool.id,
    code: pool.code,
    name: pool.name,
    description: pool.description,
    rateLimit: pool.rateLimit,
    concurrency: pool.concurrency,
    clientId: pool.clientId,
    clientIdentifier: pool.clientIdentifier,
    status: pool.status,
    createdAt: pool.createdAt.toISOString(),
    updatedAt: pool.updatedAt.toISOString(),
  };
}
