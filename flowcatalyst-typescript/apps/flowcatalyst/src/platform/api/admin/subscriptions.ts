/**
 * Subscriptions Admin API
 *
 * REST endpoints for subscription management.
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
  CreateSubscriptionCommand,
  UpdateSubscriptionCommand,
  DeleteSubscriptionCommand,
  SyncSubscriptionsCommand,
} from '../../application/index.js';
import type {
  SubscriptionCreated,
  SubscriptionUpdated,
  SubscriptionDeleted,
  SubscriptionsSynced,
  Subscription,
  SubscriptionStatus,
  SubscriptionSource,
  DispatchMode,
} from '../../domain/index.js';
import type {
  SubscriptionRepository,
  SubscriptionFilters,
} from '../../infrastructure/persistence/index.js';
import {
  requirePermission,
  getAccessibleClientIds,
  canAccessResourceByClient,
} from '../../authorization/index.js';
import { SUBSCRIPTION_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const EventTypeBindingSchema = Type.Object({
  eventTypeId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  eventTypeCode: Type.String({ minLength: 1 }),
  specVersion: Type.Optional(Type.Union([Type.String(), Type.Null()])),
});

const ConfigEntrySchema = Type.Object({
  key: Type.String({ minLength: 1, maxLength: 100 }),
  value: Type.String({ minLength: 1, maxLength: 1000 }),
});

const CreateSubscriptionSchema = Type.Object({
  code: Type.String({ minLength: 2, maxLength: 100, pattern: '^[a-z][a-z0-9-]*$' }),
  applicationCode: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  name: Type.String({ minLength: 1, maxLength: 255 }),
  description: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  clientId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  clientScoped: Type.Optional(Type.Boolean()),
  eventTypes: Type.Array(EventTypeBindingSchema, { minItems: 1 }),
  target: Type.String({ minLength: 1, maxLength: 500 }),
  queue: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  customConfig: Type.Optional(Type.Array(ConfigEntrySchema)),
  maxAgeSeconds: Type.Optional(Type.Integer({ minimum: 1 })),
  dispatchPoolId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  dispatchPoolCode: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  delaySeconds: Type.Optional(Type.Integer({ minimum: 0 })),
  sequence: Type.Optional(Type.Integer({ minimum: 0 })),
  mode: Type.Optional(Type.Union([Type.Literal('IMMEDIATE'), Type.Literal('BLOCK_ON_ERROR')])),
  timeoutSeconds: Type.Optional(Type.Integer({ minimum: 1 })),
  maxRetries: Type.Optional(Type.Integer({ minimum: 0 })),
  serviceAccountId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  dataOnly: Type.Optional(Type.Boolean()),
});

const UpdateSubscriptionSchema = Type.Object({
  name: Type.Optional(Type.String({ minLength: 1, maxLength: 255 })),
  description: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  eventTypes: Type.Optional(Type.Array(EventTypeBindingSchema, { minItems: 1 })),
  target: Type.Optional(Type.String({ minLength: 1, maxLength: 500 })),
  queue: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  customConfig: Type.Optional(Type.Array(ConfigEntrySchema)),
  status: Type.Optional(Type.Union([Type.Literal('ACTIVE'), Type.Literal('PAUSED')])),
  maxAgeSeconds: Type.Optional(Type.Integer({ minimum: 1 })),
  dispatchPoolId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  dispatchPoolCode: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  delaySeconds: Type.Optional(Type.Integer({ minimum: 0 })),
  sequence: Type.Optional(Type.Integer({ minimum: 0 })),
  mode: Type.Optional(Type.Union([Type.Literal('IMMEDIATE'), Type.Literal('BLOCK_ON_ERROR')])),
  timeoutSeconds: Type.Optional(Type.Integer({ minimum: 1 })),
  maxRetries: Type.Optional(Type.Integer({ minimum: 0 })),
  serviceAccountId: Type.Optional(Type.Union([Type.String(), Type.Null()])),
  dataOnly: Type.Optional(Type.Boolean()),
});

const SyncSubscriptionsSchema = Type.Object({
  applicationCode: Type.String({ minLength: 1 }),
  subscriptions: Type.Array(
    Type.Object({
      code: Type.String({ minLength: 1 }),
      name: Type.String({ minLength: 1 }),
      description: Type.Optional(Type.Union([Type.String(), Type.Null()])),
      clientScoped: Type.Optional(Type.Boolean()),
      eventTypes: Type.Array(EventTypeBindingSchema, { minItems: 1 }),
      target: Type.String({ minLength: 1 }),
      queue: Type.Optional(Type.Union([Type.String(), Type.Null()])),
      customConfig: Type.Optional(Type.Array(ConfigEntrySchema)),
      maxAgeSeconds: Type.Optional(Type.Integer({ minimum: 1 })),
      dispatchPoolCode: Type.Optional(Type.Union([Type.String(), Type.Null()])),
      delaySeconds: Type.Optional(Type.Integer({ minimum: 0 })),
      sequence: Type.Optional(Type.Integer({ minimum: 0 })),
      mode: Type.Optional(Type.Union([Type.Literal('IMMEDIATE'), Type.Literal('BLOCK_ON_ERROR')])),
      timeoutSeconds: Type.Optional(Type.Integer({ minimum: 1 })),
      maxRetries: Type.Optional(Type.Integer({ minimum: 0 })),
      dataOnly: Type.Optional(Type.Boolean()),
    }),
  ),
  removeUnlisted: Type.Optional(Type.Boolean()),
});

type CreateSubscriptionBody = Static<typeof CreateSubscriptionSchema>;
type UpdateSubscriptionBody = Static<typeof UpdateSubscriptionSchema>;
type SyncSubscriptionsBody = Static<typeof SyncSubscriptionsSchema>;

// ─── Param Schemas ──────────────────────────────────────────────────────────

const IdParam = Type.Object({ id: Type.String() });

// ─── Query Schemas ──────────────────────────────────────────────────────────

const SubscriptionListQuerySchema = Type.Object({
  clientId: Type.Optional(Type.String()),
  status: Type.Optional(Type.String()),
  source: Type.Optional(Type.String()),
  dispatchPoolId: Type.Optional(Type.String()),
  anchorLevel: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const EventTypeBindingResponseSchema = Type.Object({
  eventTypeId: Type.Union([Type.String(), Type.Null()]),
  eventTypeCode: Type.String(),
  specVersion: Type.Union([Type.String(), Type.Null()]),
});

const ConfigEntryResponseSchema = Type.Object({
  key: Type.String(),
  value: Type.String(),
});

const SubscriptionResponseSchema = Type.Object({
  id: Type.String(),
  code: Type.String(),
  applicationCode: Type.Union([Type.String(), Type.Null()]),
  name: Type.String(),
  description: Type.Union([Type.String(), Type.Null()]),
  clientId: Type.Union([Type.String(), Type.Null()]),
  clientIdentifier: Type.Union([Type.String(), Type.Null()]),
  clientScoped: Type.Boolean(),
  eventTypes: Type.Array(EventTypeBindingResponseSchema),
  target: Type.String(),
  queue: Type.Union([Type.String(), Type.Null()]),
  customConfig: Type.Array(ConfigEntryResponseSchema),
  source: Type.String(),
  status: Type.String(),
  maxAgeSeconds: Type.Integer(),
  dispatchPoolId: Type.Union([Type.String(), Type.Null()]),
  dispatchPoolCode: Type.Union([Type.String(), Type.Null()]),
  delaySeconds: Type.Integer(),
  sequence: Type.Integer(),
  mode: Type.String(),
  timeoutSeconds: Type.Integer(),
  maxRetries: Type.Integer(),
  serviceAccountId: Type.Union([Type.String(), Type.Null()]),
  dataOnly: Type.Boolean(),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const SubscriptionListResponseSchema = Type.Object({
  subscriptions: Type.Array(SubscriptionResponseSchema),
  total: Type.Integer(),
});

const SyncResponseSchema = Type.Object({
  applicationCode: Type.String(),
  created: Type.Integer(),
  updated: Type.Integer(),
  deleted: Type.Integer(),
  syncedCodes: Type.Array(Type.String()),
});

type SubscriptionResponse = Static<typeof SubscriptionResponseSchema>;

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface SubscriptionsRoutesDeps {
  readonly subscriptionRepository: SubscriptionRepository;
  readonly createSubscriptionUseCase: UseCase<CreateSubscriptionCommand, SubscriptionCreated>;
  readonly updateSubscriptionUseCase: UseCase<UpdateSubscriptionCommand, SubscriptionUpdated>;
  readonly deleteSubscriptionUseCase: UseCase<DeleteSubscriptionCommand, SubscriptionDeleted>;
  readonly syncSubscriptionsUseCase: UseCase<SyncSubscriptionsCommand, SubscriptionsSynced>;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerSubscriptionsRoutes(
  fastify: FastifyInstance,
  deps: SubscriptionsRoutesDeps,
): Promise<void> {
  const {
    subscriptionRepository,
    createSubscriptionUseCase,
    updateSubscriptionUseCase,
    deleteSubscriptionUseCase,
    syncSubscriptionsUseCase,
  } = deps;

  // GET /api/admin/subscriptions - List with filters
  fastify.get(
    '/subscriptions',
    {
      preHandler: requirePermission(SUBSCRIPTION_PERMISSIONS.READ),
      schema: {
        querystring: SubscriptionListQuerySchema,
        response: {
          200: SubscriptionListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof SubscriptionListQuerySchema>;

      const accessibleClientIds = getAccessibleClientIds();

      const filters: SubscriptionFilters = {
        ...(query.anchorLevel === 'true' ? { clientId: null } : {}),
        ...(query.clientId && query.anchorLevel !== 'true' ? { clientId: query.clientId } : {}),
        ...(query.status ? { status: query.status as SubscriptionStatus } : {}),
        ...(query.source ? { source: query.source as SubscriptionSource } : {}),
        ...(query.dispatchPoolId ? { dispatchPoolId: query.dispatchPoolId } : {}),
        accessibleClientIds,
      };

      const subs = await subscriptionRepository.findWithFilters(filters);

      return jsonSuccess(reply, {
        subscriptions: subs.map(toSubscriptionResponse),
        total: subs.length,
      });
    },
  );

  // GET /api/admin/subscriptions/:id - Get by ID
  fastify.get(
    '/subscriptions/:id',
    {
      preHandler: requirePermission(SUBSCRIPTION_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: SubscriptionResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const subscription = await subscriptionRepository.findById(id);

      if (!subscription || !canAccessResourceByClient(subscription.clientId)) {
        return notFound(reply, `Subscription not found: ${id}`);
      }

      return jsonSuccess(reply, toSubscriptionResponse(subscription));
    },
  );

  // POST /api/admin/subscriptions - Create
  fastify.post(
    '/subscriptions',
    {
      preHandler: requirePermission(SUBSCRIPTION_PERMISSIONS.CREATE),
      schema: {
        body: CreateSubscriptionSchema,
        response: {
          201: SubscriptionResponseSchema,
          400: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as CreateSubscriptionBody;
      const ctx = request.executionContext;

      const command: CreateSubscriptionCommand = {
        code: body.code,
        applicationCode: body.applicationCode ?? null,
        name: body.name,
        description: body.description ?? null,
        clientId: body.clientId ?? null,
        clientScoped: body.clientScoped ?? false,
        eventTypes: body.eventTypes.map((et) => ({
          eventTypeId: et.eventTypeId ?? null,
          eventTypeCode: et.eventTypeCode,
          specVersion: et.specVersion ?? null,
        })),
        target: body.target,
        queue: body.queue ?? null,
        customConfig: body.customConfig ?? [],
        maxAgeSeconds: body.maxAgeSeconds,
        dispatchPoolId: body.dispatchPoolId ?? null,
        dispatchPoolCode: body.dispatchPoolCode ?? null,
        delaySeconds: body.delaySeconds,
        sequence: body.sequence,
        mode: body.mode as DispatchMode | undefined,
        timeoutSeconds: body.timeoutSeconds,
        maxRetries: body.maxRetries,
        serviceAccountId: body.serviceAccountId ?? null,
        dataOnly: body.dataOnly,
      };

      const result = await createSubscriptionUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const sub = await subscriptionRepository.findById(result.value.getData().subscriptionId);
        if (sub) {
          return jsonCreated(reply, toSubscriptionResponse(sub));
        }
      }

      return sendResult(reply, result);
    },
  );

  // PUT /api/admin/subscriptions/:id - Update
  fastify.put(
    '/subscriptions/:id',
    {
      preHandler: requirePermission(SUBSCRIPTION_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: UpdateSubscriptionSchema,
        response: {
          200: SubscriptionResponseSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as UpdateSubscriptionBody;
      const ctx = request.executionContext;

      const command: UpdateSubscriptionCommand = {
        subscriptionId: id,
        ...(body.name !== undefined ? { name: body.name } : {}),
        ...(body.description !== undefined ? { description: body.description } : {}),
        ...(body.eventTypes !== undefined
          ? {
              eventTypes: body.eventTypes.map((et) => ({
                eventTypeId: et.eventTypeId ?? null,
                eventTypeCode: et.eventTypeCode,
                specVersion: et.specVersion ?? null,
              })),
            }
          : {}),
        ...(body.target !== undefined ? { target: body.target } : {}),
        ...(body.queue !== undefined ? { queue: body.queue } : {}),
        ...(body.customConfig !== undefined ? { customConfig: body.customConfig } : {}),
        ...(body.status !== undefined ? { status: body.status as SubscriptionStatus } : {}),
        ...(body.maxAgeSeconds !== undefined ? { maxAgeSeconds: body.maxAgeSeconds } : {}),
        ...(body.dispatchPoolId !== undefined ? { dispatchPoolId: body.dispatchPoolId } : {}),
        ...(body.dispatchPoolCode !== undefined ? { dispatchPoolCode: body.dispatchPoolCode } : {}),
        ...(body.delaySeconds !== undefined ? { delaySeconds: body.delaySeconds } : {}),
        ...(body.sequence !== undefined ? { sequence: body.sequence } : {}),
        ...(body.mode !== undefined ? { mode: body.mode as DispatchMode } : {}),
        ...(body.timeoutSeconds !== undefined ? { timeoutSeconds: body.timeoutSeconds } : {}),
        ...(body.maxRetries !== undefined ? { maxRetries: body.maxRetries } : {}),
        ...(body.serviceAccountId !== undefined ? { serviceAccountId: body.serviceAccountId } : {}),
        ...(body.dataOnly !== undefined ? { dataOnly: body.dataOnly } : {}),
      };

      const result = await updateSubscriptionUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const sub = await subscriptionRepository.findById(id);
        if (sub) {
          return jsonSuccess(reply, toSubscriptionResponse(sub));
        }
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /api/admin/subscriptions/:id - Delete
  fastify.delete(
    '/subscriptions/:id',
    {
      preHandler: requirePermission(SUBSCRIPTION_PERMISSIONS.DELETE),
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

      const command: DeleteSubscriptionCommand = { subscriptionId: id };
      const result = await deleteSubscriptionUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/subscriptions/:id/pause - Pause
  fastify.post(
    '/subscriptions/:id/pause',
    {
      preHandler: requirePermission(SUBSCRIPTION_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        response: {
          200: SubscriptionResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: UpdateSubscriptionCommand = { subscriptionId: id, status: 'PAUSED' };
      const result = await updateSubscriptionUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const sub = await subscriptionRepository.findById(id);
        if (sub) {
          return jsonSuccess(reply, toSubscriptionResponse(sub));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/subscriptions/:id/resume - Resume
  fastify.post(
    '/subscriptions/:id/resume',
    {
      preHandler: requirePermission(SUBSCRIPTION_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        response: {
          200: SubscriptionResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const command: UpdateSubscriptionCommand = { subscriptionId: id, status: 'ACTIVE' };
      const result = await updateSubscriptionUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const sub = await subscriptionRepository.findById(id);
        if (sub) {
          return jsonSuccess(reply, toSubscriptionResponse(sub));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/admin/subscriptions/sync - Sync from SDK
  fastify.post(
    '/subscriptions/sync',
    {
      preHandler: requirePermission(SUBSCRIPTION_PERMISSIONS.SYNC),
      schema: {
        body: SyncSubscriptionsSchema,
        response: {
          200: SyncResponseSchema,
          400: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as SyncSubscriptionsBody;
      const ctx = request.executionContext;

      const command: SyncSubscriptionsCommand = {
        applicationCode: body.applicationCode,
        subscriptions: body.subscriptions.map((s) => ({
          code: s.code,
          name: s.name,
          description: s.description ?? null,
          clientScoped: s.clientScoped,
          eventTypes: s.eventTypes.map((et) => ({
            eventTypeId: et.eventTypeId ?? null,
            eventTypeCode: et.eventTypeCode,
            specVersion: et.specVersion ?? null,
          })),
          target: s.target,
          queue: s.queue ?? null,
          customConfig: s.customConfig ?? [],
          maxAgeSeconds: s.maxAgeSeconds,
          dispatchPoolCode: s.dispatchPoolCode ?? null,
          delaySeconds: s.delaySeconds,
          sequence: s.sequence,
          mode: s.mode as DispatchMode | undefined,
          timeoutSeconds: s.timeoutSeconds,
          maxRetries: s.maxRetries,
          dataOnly: s.dataOnly,
        })),
        removeUnlisted: body.removeUnlisted ?? false,
      };

      const result = await syncSubscriptionsUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const data = result.value.getData();
        return jsonSuccess(reply, {
          applicationCode: data.applicationCode,
          created: data.subscriptionsCreated,
          updated: data.subscriptionsUpdated,
          deleted: data.subscriptionsDeleted,
          syncedCodes: data.syncedSubscriptionCodes,
        });
      }

      return sendResult(reply, result);
    },
  );
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toSubscriptionResponse(sub: Subscription): SubscriptionResponse {
  return {
    id: sub.id,
    code: sub.code,
    applicationCode: sub.applicationCode,
    name: sub.name,
    description: sub.description,
    clientId: sub.clientId,
    clientIdentifier: sub.clientIdentifier,
    clientScoped: sub.clientScoped,
    eventTypes: sub.eventTypes.map((et) => ({
      eventTypeId: et.eventTypeId,
      eventTypeCode: et.eventTypeCode,
      specVersion: et.specVersion,
    })),
    target: sub.target,
    queue: sub.queue,
    customConfig: sub.customConfig.map((c) => ({
      key: c.key,
      value: c.value,
    })),
    source: sub.source,
    status: sub.status,
    maxAgeSeconds: sub.maxAgeSeconds,
    dispatchPoolId: sub.dispatchPoolId,
    dispatchPoolCode: sub.dispatchPoolCode,
    delaySeconds: sub.delaySeconds,
    sequence: sub.sequence,
    mode: sub.mode,
    timeoutSeconds: sub.timeoutSeconds,
    maxRetries: sub.maxRetries,
    serviceAccountId: sub.serviceAccountId,
    dataOnly: sub.dataOnly,
    createdAt: sub.createdAt.toISOString(),
    updatedAt: sub.updatedAt.toISOString(),
  };
}
