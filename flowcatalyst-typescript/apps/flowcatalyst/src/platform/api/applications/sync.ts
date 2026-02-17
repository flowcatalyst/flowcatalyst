/**
 * Application-Scoped Sync API
 *
 * REST endpoints called by the PHP/SDK `flowcatalyst:sync` command.
 * All routes live under /api/applications/:appCode/*.
 *
 * Schemas match the typed DTO classes in the PHP SDK:
 * - RoleDefinition::toArray()
 * - EventTypeDefinition::toArray()
 * - SubscriptionDefinition::toArray()
 * - DispatchPoolDefinition::toArray()
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import { sendResult, jsonSuccess, ErrorResponseSchema, SyncResponseSchema } from '@flowcatalyst/http';
import { Result } from '@flowcatalyst/application';
import type { UseCase } from '@flowcatalyst/application';

import type {
  SyncRolesCommand,
  SyncRoleItem,
  SyncEventTypesCommand,
  SyncSubscriptionsCommand,
  SyncDispatchPoolsCommand,
  SyncPrincipalsCommand,
  SyncPrincipalItem,
} from '../../application/index.js';
import type {
  RolesSynced,
  EventTypesSynced,
  SubscriptionsSynced,
  DispatchPoolsSynced,
  PrincipalsSynced,
} from '../../domain/index.js';
import { requirePermission } from '../../authorization/index.js';
import { ROLE_PERMISSIONS, USER_PERMISSIONS } from '../../authorization/permissions/platform-iam.js';
import {
  EVENT_TYPE_PERMISSIONS,
  SUBSCRIPTION_PERMISSIONS,
  DISPATCH_POOL_PERMISSIONS,
} from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────
// Each schema mirrors the corresponding PHP DTO's toArray() output.

const AppCodeParam = Type.Object({ appCode: Type.String() });
const RemoveUnlistedQuery = Type.Object({
  removeUnlisted: Type.Optional(Type.Union([Type.Boolean(), Type.String()])),
});

// RoleDefinition::toArray() → { name, displayName?, description?, permissions?: PermissionInput[], clientManaged? }
// PermissionInput::toArray() → { application, context, aggregate, action }
const PermissionInputSchema = Type.Object({
  application: Type.String(),
  context: Type.String(),
  aggregate: Type.String(),
  action: Type.String(),
});

const SyncRolesBodySchema = Type.Object({
  roles: Type.Array(
    Type.Object({
      name: Type.String({ minLength: 1 }),
      displayName: Type.Optional(Type.String()),
      description: Type.Optional(Type.Union([Type.String(), Type.Null()])),
      permissions: Type.Optional(Type.Array(PermissionInputSchema)),
      clientManaged: Type.Optional(Type.Boolean()),
    }),
  ),
});

// EventTypeDefinition::toArray() → { name, application, subdomain, aggregate, event, clientScoped, description? }
const SyncEventTypesBodySchema = Type.Object({
  eventTypes: Type.Array(
    Type.Object({
      name: Type.String({ minLength: 1 }),
      application: Type.Optional(Type.String()),
      subdomain: Type.String({ minLength: 1 }),
      aggregate: Type.String({ minLength: 1 }),
      event: Type.String({ minLength: 1 }),
      description: Type.Optional(Type.Union([Type.String(), Type.Null()])),
      clientScoped: Type.Optional(Type.Boolean()),
    }),
  ),
});

// SubscriptionDefinition::toArray() → { code, name, target, queue, dispatchPoolCode,
//   clientScoped, maxRetries, retryDelaySeconds, timeoutSeconds, active,
//   applicationCode?, description?, eventTypeCode? }
const SyncSubscriptionsBodySchema = Type.Object({
  subscriptions: Type.Array(
    Type.Object({
      code: Type.String({ minLength: 1 }),
      name: Type.String({ minLength: 1 }),
      target: Type.String({ minLength: 1 }),
      queue: Type.String(),
      dispatchPoolCode: Type.String(),
      clientScoped: Type.Optional(Type.Boolean()),
      maxRetries: Type.Optional(Type.Integer({ minimum: 0 })),
      retryDelaySeconds: Type.Optional(Type.Integer({ minimum: 0 })),
      timeoutSeconds: Type.Optional(Type.Integer({ minimum: 1 })),
      active: Type.Optional(Type.Boolean()),
      applicationCode: Type.Optional(Type.Union([Type.String(), Type.Null()])),
      description: Type.Optional(Type.Union([Type.String(), Type.Null()])),
      eventTypeCode: Type.Optional(Type.Union([Type.String(), Type.Null()])),
    }),
  ),
});

// DispatchPoolDefinition::toArray() → { code, name?, description?, rateLimit, concurrency }
const SyncDispatchPoolsBodySchema = Type.Object({
  pools: Type.Array(
    Type.Object({
      code: Type.String({ minLength: 1 }),
      name: Type.Optional(Type.String()),
      description: Type.Optional(Type.Union([Type.String(), Type.Null()])),
      rateLimit: Type.Optional(Type.Integer({ minimum: 1 })),
      concurrency: Type.Optional(Type.Integer({ minimum: 1 })),
    }),
  ),
});

// PrincipalDefinition → { email, name, roles?, active? }
const SyncPrincipalsBodySchema = Type.Object({
  principals: Type.Array(
    Type.Object({
      email: Type.String({ minLength: 1 }),
      name: Type.String({ minLength: 1 }),
      roles: Type.Optional(Type.Array(Type.String())),
      active: Type.Optional(Type.Boolean()),
    }),
  ),
});

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface ApplicationSyncRoutesDeps {
  readonly syncRolesUseCase: UseCase<SyncRolesCommand, RolesSynced>;
  readonly syncEventTypesUseCase: UseCase<SyncEventTypesCommand, EventTypesSynced>;
  readonly syncSubscriptionsUseCase: UseCase<SyncSubscriptionsCommand, SubscriptionsSynced>;
  readonly syncDispatchPoolsUseCase: UseCase<SyncDispatchPoolsCommand, DispatchPoolsSynced>;
  readonly syncPrincipalsUseCase: UseCase<SyncPrincipalsCommand, PrincipalsSynced>;
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function parseRemoveUnlisted(query: Static<typeof RemoveUnlistedQuery>): boolean {
  const val = query.removeUnlisted;
  if (typeof val === 'boolean') return val;
  if (typeof val === 'string') return val === 'true' || val === '1';
  return false;
}

/**
 * Convert a PermissionInput object to its colon-delimited string form.
 */
function permissionToString(p: Static<typeof PermissionInputSchema>): string {
  return `${p.application}:${p.context}:${p.aggregate}:${p.action}`.toLowerCase();
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerApplicationSyncRoutes(
  fastify: FastifyInstance,
  deps: ApplicationSyncRoutesDeps,
): Promise<void> {
  const {
    syncRolesUseCase,
    syncEventTypesUseCase,
    syncSubscriptionsUseCase,
    syncDispatchPoolsUseCase,
    syncPrincipalsUseCase,
  } = deps;

  // POST /api/applications/:appCode/roles/sync
  fastify.post(
    '/:appCode/roles/sync',
    {
      preHandler: requirePermission(ROLE_PERMISSIONS.MANAGE),
      schema: {
        params: AppCodeParam,
        querystring: RemoveUnlistedQuery,
        body: SyncRolesBodySchema,
        response: {
          200: SyncResponseSchema,
          400: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { appCode } = request.params as Static<typeof AppCodeParam>;
      const query = request.query as Static<typeof RemoveUnlistedQuery>;
      const body = request.body as Static<typeof SyncRolesBodySchema>;
      const ctx = request.executionContext;

      const roles: SyncRoleItem[] = body.roles.map((r) => {
        const item: SyncRoleItem = { name: r.name, description: r.description ?? null };
        if (r.displayName !== undefined) {
          (item as { displayName: string }).displayName = r.displayName;
        }
        if (r.permissions !== undefined) {
          (item as { permissions: string[] }).permissions = r.permissions.map(permissionToString);
        }
        if (r.clientManaged !== undefined) {
          (item as { clientManaged: boolean }).clientManaged = r.clientManaged;
        }
        return item;
      });

      const command: SyncRolesCommand = {
        applicationCode: appCode,
        roles,
        removeUnlisted: parseRemoveUnlisted(query),
      };

      const result = await syncRolesUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const data = result.value.getData();
        return jsonSuccess(reply, {
          applicationCode: data.applicationCode,
          created: data.rolesCreated,
          updated: data.rolesUpdated,
          deleted: data.rolesDeleted,
          syncedCodes: data.syncedRoleNames,
        });
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/applications/:appCode/event-types/sync
  fastify.post(
    '/:appCode/event-types/sync',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.SYNC),
      schema: {
        params: AppCodeParam,
        querystring: RemoveUnlistedQuery,
        body: SyncEventTypesBodySchema,
        response: {
          200: SyncResponseSchema,
          400: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { appCode } = request.params as Static<typeof AppCodeParam>;
      const query = request.query as Static<typeof RemoveUnlistedQuery>;
      const body = request.body as Static<typeof SyncEventTypesBodySchema>;
      const ctx = request.executionContext;

      const command: SyncEventTypesCommand = {
        applicationCode: appCode,
        eventTypes: body.eventTypes.map((et) => ({
          subdomain: et.subdomain,
          aggregate: et.aggregate,
          event: et.event,
          name: et.name,
          description: et.description ?? null,
          clientScoped: et.clientScoped ?? false,
        })),
        removeUnlisted: parseRemoveUnlisted(query),
      };

      const result = await syncEventTypesUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const data = result.value.getData();
        return jsonSuccess(reply, {
          applicationCode: data.applicationCode,
          created: data.eventTypesCreated,
          updated: data.eventTypesUpdated,
          deleted: data.eventTypesDeleted,
          syncedCodes: data.syncedEventTypeCodes,
        });
      }

      return sendResult(reply, result);
    },
  );

  // POST /api/applications/:appCode/subscriptions/sync
  fastify.post(
    '/:appCode/subscriptions/sync',
    {
      preHandler: requirePermission(SUBSCRIPTION_PERMISSIONS.SYNC),
      schema: {
        params: AppCodeParam,
        querystring: RemoveUnlistedQuery,
        body: SyncSubscriptionsBodySchema,
        response: {
          200: SyncResponseSchema,
          400: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { appCode } = request.params as Static<typeof AppCodeParam>;
      const query = request.query as Static<typeof RemoveUnlistedQuery>;
      const body = request.body as Static<typeof SyncSubscriptionsBodySchema>;
      const ctx = request.executionContext;

      const command: SyncSubscriptionsCommand = {
        applicationCode: appCode,
        subscriptions: body.subscriptions.map((s) => ({
          code: s.code,
          name: s.name,
          description: s.description ?? null,
          clientScoped: s.clientScoped,
          eventTypes: s.eventTypeCode
            ? [{ eventTypeId: null, eventTypeCode: s.eventTypeCode, specVersion: null }]
            : [],
          target: s.target,
          queue: s.queue ?? null,
          dispatchPoolCode: s.dispatchPoolCode ?? null,
          delaySeconds: s.retryDelaySeconds,
          timeoutSeconds: s.timeoutSeconds,
          maxRetries: s.maxRetries,
        })),
        removeUnlisted: parseRemoveUnlisted(query),
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

  // POST /api/applications/:appCode/dispatch-pools/sync
  fastify.post(
    '/:appCode/dispatch-pools/sync',
    {
      preHandler: requirePermission(DISPATCH_POOL_PERMISSIONS.SYNC),
      schema: {
        params: AppCodeParam,
        querystring: RemoveUnlistedQuery,
        body: SyncDispatchPoolsBodySchema,
        response: {
          200: SyncResponseSchema,
          400: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { appCode } = request.params as Static<typeof AppCodeParam>;
      const query = request.query as Static<typeof RemoveUnlistedQuery>;
      const body = request.body as Static<typeof SyncDispatchPoolsBodySchema>;
      const ctx = request.executionContext;

      const command: SyncDispatchPoolsCommand = {
        applicationCode: appCode,
        pools: body.pools.map((p) => ({
          code: p.code,
          name: p.name || p.code,
          description: p.description ?? null,
          rateLimit: p.rateLimit,
          concurrency: p.concurrency,
        })),
        removeUnlisted: parseRemoveUnlisted(query),
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

  // POST /api/applications/:appCode/principals/sync
  fastify.post(
    '/:appCode/principals/sync',
    {
      preHandler: requirePermission(USER_PERMISSIONS.MANAGE),
      schema: {
        params: AppCodeParam,
        querystring: RemoveUnlistedQuery,
        body: SyncPrincipalsBodySchema,
        response: {
          200: SyncResponseSchema,
          400: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { appCode } = request.params as Static<typeof AppCodeParam>;
      const query = request.query as Static<typeof RemoveUnlistedQuery>;
      const body = request.body as Static<typeof SyncPrincipalsBodySchema>;
      const ctx = request.executionContext;

      const command: SyncPrincipalsCommand = {
        applicationCode: appCode,
        principals: body.principals.map((p) => {
          const item: SyncPrincipalItem = { email: p.email, name: p.name };
          if (p.roles !== undefined) {
            (item as { roles: string[] }).roles = p.roles;
          }
          if (p.active !== undefined) {
            (item as { active: boolean }).active = p.active;
          }
          return item;
        }),
        removeUnlisted: parseRemoveUnlisted(query),
      };

      const result = await syncPrincipalsUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const data = result.value.getData();
        return jsonSuccess(reply, {
          applicationCode: data.applicationCode,
          created: data.principalsCreated,
          updated: data.principalsUpdated,
          deleted: data.principalsDeactivated,
          syncedCodes: data.syncedEmails,
        });
      }

      return sendResult(reply, result);
    },
  );
}
