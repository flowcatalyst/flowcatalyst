/**
 * Audit Logs Batch API
 *
 * Batch ingestion endpoint for audit logs from the outbox processor.
 * Accepts an array of audit log payloads and bulk-inserts into the
 * audit_logs table in a single transaction.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import { jsonSuccess, badRequest, BatchResponseSchema } from '@flowcatalyst/http';
import { generate } from '@flowcatalyst/tsid';
import { auditLogs, type NewAuditLog } from '@flowcatalyst/persistence';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import { requirePermission } from '../../authorization/index.js';
import { BATCH_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Constants ──────────────────────────────────────────────────────────────

const MAX_BATCH_SIZE = 100;

// ─── Request Schemas ────────────────────────────────────────────────────────

const AuditLogItemSchema = Type.Object({
  entityType: Type.String({ minLength: 1 }),
  entityId: Type.String({ minLength: 1 }),
  operation: Type.String({ minLength: 1 }),
  operationData: Type.Optional(Type.Any()),
  principalId: Type.Optional(Type.String()),
  performedAt: Type.Optional(Type.String()),
});

const BatchAuditLogsRequestSchema = Type.Object({
  items: Type.Array(AuditLogItemSchema, { minItems: 1, maxItems: MAX_BATCH_SIZE }),
});

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface AuditLogsBatchDeps {
  readonly db: PostgresJsDatabase;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerAuditLogsBatchRoutes(
  fastify: FastifyInstance,
  deps: AuditLogsBatchDeps,
): Promise<void> {
  const { db } = deps;

  fastify.post(
    '/audit-logs/batch',
    {
      preHandler: requirePermission(BATCH_PERMISSIONS.AUDIT_LOGS_WRITE),
      schema: {
        body: BatchAuditLogsRequestSchema,
        response: { 200: BatchResponseSchema },
      },
    },
    async (request, reply) => {
      const { items } = request.body as Static<typeof BatchAuditLogsRequestSchema>;

      if (items.length > MAX_BATCH_SIZE) {
        return badRequest(reply, `Batch size exceeds maximum of ${MAX_BATCH_SIZE}`);
      }

      // Build all records in memory
      const auditLogRows: NewAuditLog[] = [];
      const ids: string[] = [];

      for (const item of items) {
        const id = generate('AUDIT_LOG');
        ids.push(id);

        auditLogRows.push({
          id,
          entityType: item.entityType,
          entityId: item.entityId,
          operation: item.operation,
          operationJson: parseOperationData(item.operationData),
          principalId: item.principalId ?? null,
          performedAt: item.performedAt ? new Date(item.performedAt) : new Date(),
        });
      }

      // Bulk insert in a single transaction — one multi-row INSERT
      await db.transaction(async (tx) => {
        await tx.insert(auditLogs).values(auditLogRows);
      });

      return jsonSuccess(reply, {
        results: ids.map((id) => ({ id, status: 'SUCCESS' as const })),
      });
    },
  );
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function parseOperationData(data: unknown): unknown {
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
