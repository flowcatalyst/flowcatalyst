/**
 * Event Types BFF API
 *
 * Backend-for-Frontend endpoints for event type management.
 * Response shapes match what the Vue frontend expects.
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
  CreateEventTypeCommand,
  UpdateEventTypeCommand,
  DeleteEventTypeCommand,
  ArchiveEventTypeCommand,
  AddSchemaCommand,
  FinaliseSchemaCommand,
  DeprecateSchemaCommand,
} from '../../application/index.js';
import {
  parseCodeSegments,
  type EventTypeCreated,
  type EventTypeUpdated,
  type EventTypeDeleted,
  type EventTypeArchived,
  type SchemaAdded,
  type SchemaFinalised,
  type SchemaDeprecated,
  type EventType,
  type SpecVersion,
  type SchemaType,
  type EventTypeStatus,
} from '../../domain/index.js';
import type {
  EventTypeRepository,
  EventTypeFilters,
} from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { EVENT_TYPE_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Request Schemas ────────────────────────────────────────────────────────

const CreateEventTypeSchema = Type.Object({
  code: Type.String({ minLength: 3 }),
  name: Type.String({ minLength: 1, maxLength: 100 }),
  description: Type.Optional(Type.String({ maxLength: 255 })),
  clientScoped: Type.Boolean(),
});

const UpdateEventTypeSchema = Type.Object({
  name: Type.Optional(Type.String({ minLength: 1, maxLength: 100 })),
  description: Type.Optional(Type.String({ maxLength: 255 })),
});

const AddSchemaSchema = Type.Object({
  version: Type.String({ pattern: '^\\d+\\.\\d+$' }),
  mimeType: Type.String({ minLength: 1, maxLength: 100 }),
  schema: Type.String(),
  schemaType: Type.Union([Type.Literal('JSON_SCHEMA'), Type.Literal('PROTO'), Type.Literal('XSD')]),
});

const IdParam = Type.Object({ id: Type.String() });
const IdVersionParam = Type.Object({ id: Type.String(), version: Type.String() });

const EventTypeListQuerySchema = Type.Object({
  status: Type.Optional(Type.String()),
  application: Type.Optional(Type.Union([Type.String(), Type.Array(Type.String())])),
  subdomain: Type.Optional(Type.Union([Type.String(), Type.Array(Type.String())])),
  aggregate: Type.Optional(Type.Union([Type.String(), Type.Array(Type.String())])),
});

const FilterApplicationsQuerySchema = Type.Object({
  application: Type.Optional(Type.Union([Type.String(), Type.Array(Type.String())])),
});

const FilterAggregatesQuerySchema = Type.Object({
  application: Type.Optional(Type.Union([Type.String(), Type.Array(Type.String())])),
  subdomain: Type.Optional(Type.Union([Type.String(), Type.Array(Type.String())])),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const BffSpecVersionSchema = Type.Object({
  version: Type.String(),
  mimeType: Type.String(),
  schema: Type.Optional(Type.String()),
  schemaType: Type.String(),
  status: Type.String(),
});

const BffEventTypeSchema = Type.Object({
  id: Type.String(),
  code: Type.String(),
  application: Type.String(),
  subdomain: Type.String(),
  aggregate: Type.String(),
  event: Type.String(),
  name: Type.String(),
  description: Type.Optional(Type.String()),
  status: Type.String(),
  clientScoped: Type.Boolean(),
  specVersions: Type.Array(BffSpecVersionSchema),
  createdAt: Type.String({ format: 'date-time' }),
  updatedAt: Type.String({ format: 'date-time' }),
});

const BffEventTypeListResponseSchema = Type.Object({
  items: Type.Array(BffEventTypeSchema),
  total: Type.Integer(),
});

const BffFilterOptionsResponseSchema = Type.Object({
  options: Type.Array(Type.String()),
});

type BffEventType = Static<typeof BffEventTypeSchema>;
type BffSpecVersion = Static<typeof BffSpecVersionSchema>;

/**
 * Dependencies for the event types BFF API.
 */
export interface EventTypesBffDeps {
  readonly eventTypeRepository: EventTypeRepository;
  readonly createEventTypeUseCase: UseCase<CreateEventTypeCommand, EventTypeCreated>;
  readonly updateEventTypeUseCase: UseCase<UpdateEventTypeCommand, EventTypeUpdated>;
  readonly deleteEventTypeUseCase: UseCase<DeleteEventTypeCommand, EventTypeDeleted>;
  readonly archiveEventTypeUseCase: UseCase<ArchiveEventTypeCommand, EventTypeArchived>;
  readonly addSchemaUseCase: UseCase<AddSchemaCommand, SchemaAdded>;
  readonly finaliseSchemaUseCase: UseCase<FinaliseSchemaCommand, SchemaFinalised>;
  readonly deprecateSchemaUseCase: UseCase<DeprecateSchemaCommand, SchemaDeprecated>;
}

/**
 * Register event type BFF routes.
 */
export async function registerEventTypesBffRoutes(
  fastify: FastifyInstance,
  deps: EventTypesBffDeps,
): Promise<void> {
  const {
    eventTypeRepository,
    createEventTypeUseCase,
    updateEventTypeUseCase,
    deleteEventTypeUseCase,
    archiveEventTypeUseCase,
    addSchemaUseCase,
    finaliseSchemaUseCase,
    deprecateSchemaUseCase,
  } = deps;

  // GET /bff/event-types - List with filters
  fastify.get(
    '/event-types',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
      schema: {
        querystring: EventTypeListQuerySchema,
        response: {
          200: BffEventTypeListResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof EventTypeListQuerySchema>;

      const applications = toArray(query.application);
      const subdomains = toArray(query.subdomain);
      const aggregates = toArray(query.aggregate);

      const filters: EventTypeFilters = {
        ...(query.status ? { status: query.status as EventTypeStatus } : {}),
        ...(applications.length > 0 ? { applications } : {}),
        ...(subdomains.length > 0 ? { subdomains } : {}),
        ...(aggregates.length > 0 ? { aggregates } : {}),
      };

      const eventTypes = await eventTypeRepository.findWithFilters(filters);

      return jsonSuccess(reply, {
        items: eventTypes.map(toBffEventType),
        total: eventTypes.length,
      });
    },
  );

  // GET /bff/event-types/:id - Get by ID
  fastify.get(
    '/event-types/:id',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
      schema: {
        params: IdParam,
        response: {
          200: BffEventTypeSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const eventType = await eventTypeRepository.findById(id);

      if (!eventType) {
        return notFound(reply, `Event type not found: ${id}`);
      }

      return jsonSuccess(reply, toBffEventType(eventType));
    },
  );

  // POST /bff/event-types - Create
  fastify.post(
    '/event-types',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.CREATE),
      schema: {
        body: CreateEventTypeSchema,
        response: {
          201: BffEventTypeSchema,
          400: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const body = request.body as Static<typeof CreateEventTypeSchema>;
      const ctx = request.executionContext;

      // Parse code to extract segments
      const segments = parseCodeSegments(body.code);

      const command: CreateEventTypeCommand = {
        application: segments?.application ?? body.code,
        subdomain: segments?.subdomain ?? '',
        aggregate: segments?.aggregate ?? '',
        event: segments?.event ?? '',
        name: body.name,
        description: body.description ?? null,
        clientScoped: body.clientScoped,
      };

      const result = await createEventTypeUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const eventType = await eventTypeRepository.findById(result.value.getData().eventTypeId);
        if (eventType) {
          return jsonCreated(reply, toBffEventType(eventType));
        }
      }

      return sendResult(reply, result);
    },
  );

  // PATCH /bff/event-types/:id - Update
  fastify.patch(
    '/event-types/:id',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        body: UpdateEventTypeSchema,
        response: {
          200: BffEventTypeSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof UpdateEventTypeSchema>;
      const ctx = request.executionContext;

      const command: UpdateEventTypeCommand = {
        eventTypeId: id,
        ...(body.name !== undefined ? { name: body.name } : {}),
        ...(body.description !== undefined ? { description: body.description } : {}),
      };

      const result = await updateEventTypeUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const eventType = await eventTypeRepository.findById(id);
        if (eventType) {
          return jsonSuccess(reply, toBffEventType(eventType));
        }
      }

      return sendResult(reply, result);
    },
  );

  // DELETE /bff/event-types/:id
  fastify.delete(
    '/event-types/:id',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.DELETE),
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

      const result = await deleteEventTypeUseCase.execute({ eventTypeId: id }, ctx);

      if (Result.isSuccess(result)) {
        return noContent(reply);
      }

      return sendResult(reply, result);
    },
  );

  // POST /bff/event-types/:id/archive
  fastify.post(
    '/event-types/:id/archive',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.UPDATE),
      schema: {
        params: IdParam,
        response: {
          200: BffEventTypeSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const ctx = request.executionContext;

      const result = await archiveEventTypeUseCase.execute({ eventTypeId: id }, ctx);

      if (Result.isSuccess(result)) {
        const eventType = await eventTypeRepository.findById(id);
        if (eventType) {
          return jsonSuccess(reply, toBffEventType(eventType));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /bff/event-types/:id/schemas - Add schema
  fastify.post(
    '/event-types/:id/schemas',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.MANAGE_SCHEMA),
      schema: {
        params: IdParam,
        body: AddSchemaSchema,
        response: {
          201: BffEventTypeSchema,
          400: ErrorResponseSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id } = request.params as Static<typeof IdParam>;
      const body = request.body as Static<typeof AddSchemaSchema>;
      const ctx = request.executionContext;

      const command: AddSchemaCommand = {
        eventTypeId: id,
        version: body.version,
        mimeType: body.mimeType,
        schemaContent: body.schema,
        schemaType: body.schemaType as SchemaType,
      };

      const result = await addSchemaUseCase.execute(command, ctx);

      if (Result.isSuccess(result)) {
        const eventType = await eventTypeRepository.findById(id);
        if (eventType) {
          return jsonCreated(reply, toBffEventType(eventType));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /bff/event-types/:id/schemas/:version/finalise
  fastify.post(
    '/event-types/:id/schemas/:version/finalise',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.MANAGE_SCHEMA),
      schema: {
        params: IdVersionParam,
        response: {
          200: BffEventTypeSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id, version } = request.params as Static<typeof IdVersionParam>;
      const ctx = request.executionContext;

      const result = await finaliseSchemaUseCase.execute({ eventTypeId: id, version }, ctx);

      if (Result.isSuccess(result)) {
        const eventType = await eventTypeRepository.findById(id);
        if (eventType) {
          return jsonSuccess(reply, toBffEventType(eventType));
        }
      }

      return sendResult(reply, result);
    },
  );

  // POST /bff/event-types/:id/schemas/:version/deprecate
  fastify.post(
    '/event-types/:id/schemas/:version/deprecate',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.MANAGE_SCHEMA),
      schema: {
        params: IdVersionParam,
        response: {
          200: BffEventTypeSchema,
          404: ErrorResponseSchema,
          409: ErrorResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const { id, version } = request.params as Static<typeof IdVersionParam>;
      const ctx = request.executionContext;

      const result = await deprecateSchemaUseCase.execute({ eventTypeId: id, version }, ctx);

      if (Result.isSuccess(result)) {
        const eventType = await eventTypeRepository.findById(id);
        if (eventType) {
          return jsonSuccess(reply, toBffEventType(eventType));
        }
      }

      return sendResult(reply, result);
    },
  );

  // GET /bff/event-types/filters/applications
  fastify.get(
    '/event-types/filters/applications',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
      schema: {
        response: {
          200: BffFilterOptionsResponseSchema,
        },
      },
    },
    async (_request, reply) => {
      const applications = await eventTypeRepository.findDistinctApplications();
      return jsonSuccess(reply, { options: applications });
    },
  );

  // GET /bff/event-types/filters/subdomains
  fastify.get(
    '/event-types/filters/subdomains',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
      schema: {
        querystring: FilterApplicationsQuerySchema,
        response: {
          200: BffFilterOptionsResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof FilterApplicationsQuerySchema>;
      const applications = toArray(query.application);
      const subdomains = await eventTypeRepository.findDistinctSubdomains(
        applications.length > 0 ? applications : undefined,
      );
      return jsonSuccess(reply, { options: subdomains });
    },
  );

  // GET /bff/event-types/filters/aggregates
  fastify.get(
    '/event-types/filters/aggregates',
    {
      preHandler: requirePermission(EVENT_TYPE_PERMISSIONS.READ),
      schema: {
        querystring: FilterAggregatesQuerySchema,
        response: {
          200: BffFilterOptionsResponseSchema,
        },
      },
    },
    async (request, reply) => {
      const query = request.query as Static<typeof FilterAggregatesQuerySchema>;
      const applications = toArray(query.application);
      const subdomains = toArray(query.subdomain);
      const aggregates = await eventTypeRepository.findDistinctAggregates(
        applications.length > 0 ? applications : undefined,
        subdomains.length > 0 ? subdomains : undefined,
      );
      return jsonSuccess(reply, { options: aggregates });
    },
  );
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function toArray(value: string | string[] | undefined): string[] {
  if (!value) return [];
  return Array.isArray(value) ? value : [value];
}

function toBffEventType(eventType: EventType): BffEventType {
  const parts = eventType.code.split(':');
  return {
    id: eventType.id,
    code: eventType.code,
    application: eventType.application,
    subdomain: eventType.subdomain,
    aggregate: eventType.aggregate,
    event: parts[3] ?? '',
    name: eventType.name,
    ...(eventType.description ? { description: eventType.description } : {}),
    status: eventType.status,
    clientScoped: eventType.clientScoped,
    specVersions: eventType.specVersions.map(toBffSpecVersion),
    createdAt: eventType.createdAt.toISOString(),
    updatedAt: eventType.updatedAt.toISOString(),
  };
}

function toBffSpecVersion(sv: SpecVersion): BffSpecVersion {
  return {
    version: sv.version,
    mimeType: sv.mimeType,
    ...(sv.schemaContent !== null
      ? {
          schema:
            typeof sv.schemaContent === 'string'
              ? sv.schemaContent
              : JSON.stringify(sv.schemaContent),
        }
      : {}),
    schemaType: sv.schemaType,
    status: sv.status,
  };
}
