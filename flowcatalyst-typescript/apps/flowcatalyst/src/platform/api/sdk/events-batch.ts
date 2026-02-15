/**
 * Events Batch API
 *
 * Batch ingestion endpoint for events from the outbox processor.
 * Accepts an array of event payloads and bulk-inserts into the events table
 * and event projection feed in a single transaction (two multi-row INSERTs).
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import { jsonSuccess, badRequest } from '@flowcatalyst/http';
import { generateRaw } from '@flowcatalyst/tsid';
import {
  events,
  eventProjectionFeed,
  type NewEvent,
  type NewEventProjectionFeedRecord,
  type EventContextData,
} from '@flowcatalyst/persistence';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import { requirePermission } from '../../authorization/index.js';
import { BATCH_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Constants ──────────────────────────────────────────────────────────────

const MAX_BATCH_SIZE = 100;

// ─── Request Schemas ────────────────────────────────────────────────────────

const EventItemSchema = Type.Object({
  specVersion: Type.Optional(Type.String()),
  type: Type.String({ minLength: 1 }),
  source: Type.Optional(Type.String()),
  subject: Type.Optional(Type.String()),
  data: Type.Optional(Type.Any()),
  correlationId: Type.Optional(Type.String()),
  causationId: Type.Optional(Type.String()),
  deduplicationId: Type.Optional(Type.String()),
  messageGroup: Type.Optional(Type.String()),
  clientId: Type.Optional(Type.String()),
  contextData: Type.Optional(
    Type.Array(Type.Object({ key: Type.String(), value: Type.String() })),
  ),
});

const BatchEventsRequestSchema = Type.Object({
  items: Type.Array(EventItemSchema, { minItems: 1, maxItems: MAX_BATCH_SIZE }),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const BatchResponseSchema = Type.Object({
  results: Type.Array(
    Type.Object({
      id: Type.String(),
      status: Type.Union([Type.Literal('SUCCESS'), Type.Literal('ERROR')]),
      error: Type.Optional(Type.String()),
    }),
  ),
});

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface EventsBatchDeps {
  readonly db: PostgresJsDatabase;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerEventsBatchRoutes(
  fastify: FastifyInstance,
  deps: EventsBatchDeps,
): Promise<void> {
  const { db } = deps;

  fastify.post(
    '/events/batch',
    {
      preHandler: requirePermission(BATCH_PERMISSIONS.EVENTS_WRITE),
      schema: {
        body: BatchEventsRequestSchema,
        response: { 200: BatchResponseSchema },
      },
    },
    async (request, reply) => {
      const { items } = request.body as Static<typeof BatchEventsRequestSchema>;

      if (items.length > MAX_BATCH_SIZE) {
        return badRequest(reply, `Batch size exceeds maximum of ${MAX_BATCH_SIZE}`);
      }

      const now = new Date();
      const nowIso = now.toISOString();

      // Build all records in memory
      const eventRows: NewEvent[] = [];
      const feedRows: NewEventProjectionFeedRecord[] = [];
      const ids: string[] = [];

      for (const item of items) {
        const id = generateRaw();
        ids.push(id);

        const contextData = (item.contextData as EventContextData[] | undefined) ?? null;

        eventRows.push({
          id,
          specVersion: item.specVersion ?? '1.0',
          type: item.type,
          source: item.source ?? 'sdk',
          subject: item.subject ?? null,
          time: now,
          data: parseEventData(item.data),
          correlationId: item.correlationId ?? null,
          causationId: item.causationId ?? null,
          deduplicationId: item.deduplicationId ?? null,
          messageGroup: item.messageGroup ?? null,
          clientId: item.clientId ?? null,
          contextData: contextData && contextData.length > 0 ? contextData : null,
          createdAt: now,
        });

        feedRows.push({
          eventId: id,
          payload: {
            specVersion: item.specVersion ?? '1.0',
            type: item.type,
            source: item.source ?? 'sdk',
            subject: item.subject ?? null,
            time: nowIso,
            data: typeof item.data === 'string' ? item.data : JSON.stringify(item.data ?? null),
            correlationId: item.correlationId ?? null,
            causationId: item.causationId ?? null,
            deduplicationId: item.deduplicationId ?? null,
            messageGroup: item.messageGroup ?? null,
            clientId: item.clientId ?? null,
          },
        });
      }

      // Bulk insert in a single transaction — two multi-row INSERTs
      await db.transaction(async (tx) => {
        await tx.insert(events).values(eventRows);
        await tx.insert(eventProjectionFeed).values(feedRows);
      });

      return jsonSuccess(reply, {
        results: ids.map((id) => ({ id, status: 'SUCCESS' as const })),
      });
    },
  );
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function parseEventData(data: unknown): unknown {
  if (data === null || data === undefined) return null;
  if (typeof data === 'string') {
    try {
      return JSON.parse(data);
    } catch {
      return data;
    }
  }
  return data;
}
