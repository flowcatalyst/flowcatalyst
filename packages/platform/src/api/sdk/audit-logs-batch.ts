/**
 * Audit Logs Batch API
 *
 * Batch ingestion endpoint for audit logs from the outbox processor.
 * Accepts an array of audit log payloads and bulk-inserts into the
 * aud_logs table.
 *
 * Items may carry optional `applicationCode` and `clientCode` fields.
 * The endpoint resolves codes to IDs via a 30-minute TTL cache and
 * performs per-item client-access checks against the calling principal.
 * Items the principal cannot access are silently skipped (status: "SKIPPED").
 */

import type { FastifyInstance } from "fastify";
import { Type, type Static } from "@sinclair/typebox";
import {
	jsonSuccess,
	badRequest,
	BatchResponseSchema,
} from "@flowcatalyst/http";
import { generate } from "@flowcatalyst/tsid";
import { auditLogs, type NewAuditLog } from "@flowcatalyst/persistence";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import { requirePermission } from "../../authorization/index.js";
import { BATCH_PERMISSIONS } from "../../authorization/permissions/platform-admin.js";
import { canAccessClient } from "../../authorization/authorization-service.js";
import type {
	ApplicationRepository,
	ClientRepository,
} from "../../infrastructure/persistence/index.js";

// ─── Constants ──────────────────────────────────────────────────────────────

const MAX_BATCH_SIZE = 100;
const CACHE_TTL_MS = 30 * 60 * 1000; // 30 minutes

// ─── Request Schemas ────────────────────────────────────────────────────────

const AuditLogItemSchema = Type.Object({
	entityType: Type.String({ minLength: 1 }),
	entityId: Type.String({ minLength: 1 }),
	operation: Type.String({ minLength: 1 }),
	operationData: Type.Optional(Type.Any()),
	principalId: Type.Optional(Type.String()),
	performedAt: Type.Optional(Type.String()),
	applicationCode: Type.Optional(Type.String()),
	clientCode: Type.Optional(Type.String()),
});

const BatchAuditLogsRequestSchema = Type.Object({
	items: Type.Array(AuditLogItemSchema, {
		minItems: 1,
		maxItems: MAX_BATCH_SIZE,
	}),
});

// ─── Cache ───────────────────────────────────────────────────────────────────

interface CacheEntry {
	id: string | null; // null = code looked up but not found
	expiresAt: number;
}

function createCodeCache(
	applicationRepository: ApplicationRepository,
	clientRepository: ClientRepository,
) {
	const appCache = new Map<string, CacheEntry>();
	const clientCache = new Map<string, CacheEntry>();

	async function resolveApplicationId(code: string): Promise<string | null> {
		const now = Date.now();
		const cached = appCache.get(code);
		if (cached && cached.expiresAt > now) return cached.id;

		const app = await applicationRepository.findByCode(code);
		const entry: CacheEntry = {
			id: app?.id ?? null,
			expiresAt: now + CACHE_TTL_MS,
		};
		appCache.set(code, entry);
		return entry.id;
	}

	async function resolveClientId(identifier: string): Promise<string | null> {
		const now = Date.now();
		const cached = clientCache.get(identifier);
		if (cached && cached.expiresAt > now) return cached.id;

		const client = await clientRepository.findByIdentifier(identifier);
		const entry: CacheEntry = {
			id: client?.id ?? null,
			expiresAt: now + CACHE_TTL_MS,
		};
		clientCache.set(identifier, entry);
		return entry.id;
	}

	return { resolveApplicationId, resolveClientId };
}

// ─── Dependencies ───────────────────────────────────────────────────────────

export interface AuditLogsBatchDeps {
	readonly db: PostgresJsDatabase;
	readonly applicationRepository: ApplicationRepository;
	readonly clientRepository: ClientRepository;
}

// ─── Route Registration ─────────────────────────────────────────────────────

export async function registerAuditLogsBatchRoutes(
	fastify: FastifyInstance,
	deps: AuditLogsBatchDeps,
): Promise<void> {
	const { db, applicationRepository, clientRepository } = deps;

	// Code→ID cache shared across requests (module-level per registered route)
	const cache = createCodeCache(applicationRepository, clientRepository);

	fastify.post(
		"/audit-logs/batch",
		{
			preHandler: requirePermission(BATCH_PERMISSIONS.AUDIT_LOGS_WRITE),
			schema: {
				body: BatchAuditLogsRequestSchema,
				response: { 200: BatchResponseSchema },
			},
		},
		async (request, reply) => {
			const { items } = request.body as Static<
				typeof BatchAuditLogsRequestSchema
			>;

			if (items.length > MAX_BATCH_SIZE) {
				return badRequest(
					reply,
					`Batch size exceeds maximum of ${MAX_BATCH_SIZE}`,
				);
			}

			const principal = request.audit?.principal;

			// Build records, resolving codes and checking access per-item
			const auditLogRows: NewAuditLog[] = [];
			const results: { id: string; status: "SUCCESS" | "SKIPPED" }[] = [];

			for (const item of items) {
				const id = generate("AUDIT_LOG");

				// Resolve application and client IDs from codes
				const applicationId = item.applicationCode
					? await cache.resolveApplicationId(item.applicationCode)
					: null;

				const clientId = item.clientCode
					? await cache.resolveClientId(item.clientCode)
					: null;

				// Per-item access check: if the item is client-scoped, verify the
				// calling principal can access that client. Skip unauthorized items.
				if (clientId && principal && !canAccessClient(principal, clientId)) {
					results.push({ id, status: "SKIPPED" });
					continue;
				}

				auditLogRows.push({
					id,
					entityType: item.entityType,
					entityId: item.entityId,
					operation: item.operation,
					operationJson: parseOperationData(item.operationData),
					principalId: item.principalId ?? null,
					performedAt: item.performedAt
						? new Date(item.performedAt)
						: new Date(),
					applicationId,
					clientId,
				});

				results.push({ id, status: "SUCCESS" });
			}

			// Bulk insert all accepted rows in a single transaction
			if (auditLogRows.length > 0) {
				await db.transaction(async (tx) => {
					await tx.insert(auditLogs).values(auditLogRows);
				});
			}

			return jsonSuccess(reply, { results });
		},
	);
}

// ─── Helpers ────────────────────────────────────────────────────────────────

function parseOperationData(data: unknown): unknown {
	if (data === null || data === undefined) return null;
	if (typeof data === "string") {
		try {
			return JSON.parse(data);
		} catch {
			return data;
		}
	}
	return data;
}
