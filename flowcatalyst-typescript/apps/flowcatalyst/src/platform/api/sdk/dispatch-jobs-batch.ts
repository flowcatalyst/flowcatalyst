/**
 * Dispatch Jobs Batch API
 *
 * Batch ingestion endpoint for dispatch jobs from the outbox processor.
 * Accepts an array of dispatch job payloads and bulk-inserts into the
 * dispatch_jobs table and dispatch_job_projection_feed in a single transaction.
 * After commit, publishes MessagePointers to the queue via PostCommitDispatcher.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import { jsonSuccess, badRequest, BatchResponseSchema } from '@flowcatalyst/http';
import { generateRaw } from '@flowcatalyst/tsid';
import {
  dispatchJobs,
  dispatchJobProjectionFeed,
  type NewDispatchJobRecord,
  type NewDispatchJobProjectionFeedRecord,
  type DispatchJobMetadata,
  type PostCommitDispatcher,
} from '@flowcatalyst/persistence';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import { requirePermission } from '../../authorization/index.js';
import { BATCH_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Constants ──────────────────────────────────────────────────────────────

const MAX_BATCH_SIZE = 100;

// ─── Request Schemas ────────────────────────────────────────────────────────

const MetadataEntrySchema = Type.Object({
  key: Type.String(),
  value: Type.String(),
});

const DispatchJobItemSchema = Type.Object({
  source: Type.String({ minLength: 1 }),
  code: Type.String({ minLength: 1 }),
  targetUrl: Type.String({ minLength: 1 }),
  payload: Type.Optional(Type.String()),
  dispatchPoolId: Type.Optional(Type.String()),
  subject: Type.Optional(Type.String()),
  correlationId: Type.Optional(Type.String()),
  eventId: Type.Optional(Type.String()),
  metadata: Type.Optional(
    Type.Union([Type.Array(MetadataEntrySchema), Type.Record(Type.String(), Type.String())]),
  ),
  payloadContentType: Type.Optional(Type.String()),
  dataOnly: Type.Optional(Type.Boolean()),
  messageGroup: Type.Optional(Type.String()),
  sequence: Type.Optional(Type.Number()),
  timeoutSeconds: Type.Optional(Type.Number()),
  maxRetries: Type.Optional(Type.Number()),
  retryStrategy: Type.Optional(Type.String()),
  scheduledFor: Type.Optional(Type.String()),
  expiresAt: Type.Optional(Type.String()),
  idempotencyKey: Type.Optional(Type.String()),
  externalId: Type.Optional(Type.String()),
  clientId: Type.Optional(Type.String()),
});

const BatchDispatchJobsRequestSchema = Type.Object({
  items: Type.Array(DispatchJobItemSchema, { minItems: 1, maxItems: MAX_BATCH_SIZE }),
});

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface DispatchJobsBatchDeps {
  readonly db: PostgresJsDatabase;
  readonly getPostCommitDispatcher: () => PostCommitDispatcher | undefined;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerDispatchJobsBatchRoutes(
  fastify: FastifyInstance,
  deps: DispatchJobsBatchDeps,
): Promise<void> {
  const { db, getPostCommitDispatcher } = deps;

  fastify.post(
    '/dispatch/jobs/batch',
    {
      preHandler: requirePermission(BATCH_PERMISSIONS.DISPATCH_JOBS_WRITE),
      schema: {
        body: BatchDispatchJobsRequestSchema,
        response: { 200: BatchResponseSchema },
      },
    },
    async (request, reply) => {
      const { items } = request.body as Static<typeof BatchDispatchJobsRequestSchema>;

      if (items.length > MAX_BATCH_SIZE) {
        return badRequest(reply, `Batch size exceeds maximum of ${MAX_BATCH_SIZE}`);
      }

      const now = new Date();

      // Build all records in memory
      const jobRows: NewDispatchJobRecord[] = [];
      const feedRows: NewDispatchJobProjectionFeedRecord[] = [];
      const ids: string[] = [];

      for (const item of items) {
        const id = generateRaw();
        ids.push(id);

        const metadata = normalizeMetadata(item.metadata);

        const jobRecord: NewDispatchJobRecord = {
          id,
          externalId: item.externalId ?? null,
          source: item.source,
          kind: 'EVENT',
          code: item.code,
          subject: item.subject ?? null,
          eventId: item.eventId ?? null,
          correlationId: item.correlationId ?? null,
          metadata: metadata.length > 0 ? metadata : [],
          targetUrl: item.targetUrl,
          protocol: 'HTTP_WEBHOOK',
          payload: item.payload ?? null,
          payloadContentType: item.payloadContentType ?? 'application/json',
          dataOnly: item.dataOnly ?? true,
          clientId: item.clientId ?? null,
          mode: 'IMMEDIATE',
          dispatchPoolId: item.dispatchPoolId ?? null,
          messageGroup: item.messageGroup ?? null,
          sequence: item.sequence ?? 99,
          timeoutSeconds: item.timeoutSeconds ?? 30,
          status: 'QUEUED',
          maxRetries: item.maxRetries ?? 3,
          retryStrategy: item.retryStrategy ?? 'exponential',
          scheduledFor: item.scheduledFor ? new Date(item.scheduledFor) : null,
          expiresAt: item.expiresAt ? new Date(item.expiresAt) : null,
          attemptCount: 0,
          idempotencyKey: item.idempotencyKey ?? null,
          createdAt: now,
          updatedAt: now,
        };

        jobRows.push(jobRecord);

        feedRows.push({
          dispatchJobId: id,
          operation: 'INSERT',
          payload: jobRecord,
        });
      }

      // Bulk insert in a single transaction — two multi-row INSERTs
      await db.transaction(async (tx) => {
        await tx.insert(dispatchJobs).values(jobRows);
        await tx.insert(dispatchJobProjectionFeed).values(feedRows);
      });

      // Post-commit: publish MessagePointers to the queue
      const dispatcher = getPostCommitDispatcher();
      if (dispatcher) {
        const notifications = jobRows.map((job) => ({
          id: job.id,
          dispatchPoolId: job.dispatchPoolId ?? null,
          messageGroup: job.messageGroup ?? 'default',
        }));
        await dispatcher.dispatch(notifications);
      }

      return jsonSuccess(reply, {
        results: ids.map((id) => ({ id, status: 'SUCCESS' as const })),
      });
    },
  );
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function normalizeMetadata(metadata: unknown): DispatchJobMetadata[] {
  if (!metadata) return [];

  // Array format: [{ key, value }]
  if (Array.isArray(metadata)) {
    return metadata as DispatchJobMetadata[];
  }

  // Object format: { key: value }
  if (typeof metadata === 'object') {
    return Object.entries(metadata as Record<string, string>).map(([key, value]) => ({
      key,
      value,
    }));
  }

  return [];
}
