/**
 * Audit Logs Admin API
 *
 * REST endpoints for viewing audit logs.
 */

import type { FastifyInstance } from 'fastify';
import { Type, type Static } from '@sinclair/typebox';
import { jsonSuccess, notFound, ErrorResponseSchema } from '@flowcatalyst/http';

import type { AuditLog, AuditLogWithPrincipal } from '../../domain/index.js';
import type { AuditLogRepository, PrincipalRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { AUDIT_LOG_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// ─── Param / Query Schemas ──────────────────────────────────────────────────

const IdParam = Type.Object({ id: Type.String() });
const EntityParam = Type.Object({ entityType: Type.String(), entityId: Type.String() });

const ListAuditLogsQuery = Type.Object({
	entityType: Type.Optional(Type.String()),
	entityId: Type.Optional(Type.String()),
	principalId: Type.Optional(Type.String()),
	operation: Type.Optional(Type.String()),
	limit: Type.Optional(Type.String()),
	offset: Type.Optional(Type.String()),
});

const EntityLogsQuery = Type.Object({
	limit: Type.Optional(Type.String()),
	offset: Type.Optional(Type.String()),
});

// ─── Response Schemas ───────────────────────────────────────────────────────

const AuditLogResponseSchema = Type.Object({
	id: Type.String(),
	entityType: Type.String(),
	entityId: Type.String(),
	operation: Type.String(),
	operationJson: Type.Union([Type.Unknown(), Type.Null()]),
	principalId: Type.Union([Type.String(), Type.Null()]),
	principalName: Type.Union([Type.String(), Type.Null()]),
	performedAt: Type.String({ format: 'date-time' }),
});

const AuditLogListResponseSchema = Type.Object({
	logs: Type.Array(AuditLogResponseSchema),
	total: Type.Integer(),
	limit: Type.Integer(),
	offset: Type.Integer(),
});

const EntityTypesResponseSchema = Type.Object({
	entityTypes: Type.Array(Type.String()),
});

const OperationsResponseSchema = Type.Object({
	operations: Type.Array(Type.String()),
});

type AuditLogResponse = Static<typeof AuditLogResponseSchema>;

/**
 * Dependencies for the audit logs API.
 */
export interface AuditLogsRoutesDeps {
	readonly auditLogRepository: AuditLogRepository;
	readonly principalRepository: PrincipalRepository;
}

/**
 * Convert AuditLog to response with principal name.
 */
function toResponse(log: AuditLogWithPrincipal): AuditLogResponse {
	return {
		id: log.id,
		entityType: log.entityType,
		entityId: log.entityId,
		operation: log.operation,
		operationJson: log.operationJson,
		principalId: log.principalId,
		principalName: log.principalName,
		performedAt: log.performedAt.toISOString(),
	};
}

/**
 * Resolve principal names for a list of audit logs.
 */
async function resolvePrincipalNames(
	logs: AuditLog[],
	principalRepository: PrincipalRepository,
): Promise<AuditLogWithPrincipal[]> {
	// Collect unique principal IDs
	const principalIds = new Set<string>();
	for (const log of logs) {
		if (log.principalId) {
			principalIds.add(log.principalId);
		}
	}

	// Fetch all principals in parallel
	const principalMap = new Map<string, string>();
	const fetchPromises = Array.from(principalIds).map(async (id) => {
		const principal = await principalRepository.findById(id);
		if (principal) {
			principalMap.set(id, principal.name);
		}
	});
	await Promise.all(fetchPromises);

	// Map logs with principal names
	return logs.map((log) => ({
		...log,
		principalName: log.principalId ? principalMap.get(log.principalId) ?? null : null,
	}));
}

/**
 * Register audit log admin API routes.
 */
export async function registerAuditLogsRoutes(
	fastify: FastifyInstance,
	deps: AuditLogsRoutesDeps,
): Promise<void> {
	const { auditLogRepository, principalRepository } = deps;

	const DEFAULT_LIMIT = 50;
	const MAX_LIMIT = 100;

	// GET /api/admin/audit-logs - List audit logs with filters
	fastify.get(
		'/audit-logs',
		{
			preHandler: requirePermission(AUDIT_LOG_PERMISSIONS.READ),
			schema: {
				querystring: ListAuditLogsQuery,
				response: {
					200: AuditLogListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const query = request.query as Static<typeof ListAuditLogsQuery>;

			const limit = Math.min(
				Math.max(parseInt(query.limit ?? String(DEFAULT_LIMIT), 10) || DEFAULT_LIMIT, 1),
				MAX_LIMIT,
			);
			const offset = Math.max(parseInt(query.offset ?? '0', 10) || 0, 0);

			const result = await auditLogRepository.findPaged(
				{
					entityType: query.entityType,
					entityId: query.entityId,
					principalId: query.principalId,
					operation: query.operation,
				},
				{ limit, offset },
			);

			const logsWithPrincipals = await resolvePrincipalNames(result.logs, principalRepository);

			return jsonSuccess(reply, {
				logs: logsWithPrincipals.map(toResponse),
				total: result.total,
				limit: result.limit,
				offset: result.offset,
			});
		},
	);

	// GET /api/admin/audit-logs/:id - Get single audit log
	fastify.get(
		'/audit-logs/:id',
		{
			preHandler: requirePermission(AUDIT_LOG_PERMISSIONS.READ),
			schema: {
				params: IdParam,
				response: {
					200: AuditLogResponseSchema,
					404: ErrorResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { id } = request.params as Static<typeof IdParam>;
			const log = await auditLogRepository.findById(id);

			if (!log) {
				return notFound(reply, `Audit log not found: ${id}`);
			}

			const [logWithPrincipal] = await resolvePrincipalNames([log], principalRepository);

			return jsonSuccess(reply, toResponse(logWithPrincipal!));
		},
	);

	// GET /api/admin/audit-logs/entity/:entityType/:entityId - Get logs for specific entity
	fastify.get(
		'/audit-logs/entity/:entityType/:entityId',
		{
			preHandler: requirePermission(AUDIT_LOG_PERMISSIONS.READ),
			schema: {
				params: EntityParam,
				querystring: EntityLogsQuery,
				response: {
					200: AuditLogListResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const { entityType, entityId } = request.params as Static<typeof EntityParam>;
			const query = request.query as Static<typeof EntityLogsQuery>;

			const limit = Math.min(
				Math.max(parseInt(query.limit ?? String(DEFAULT_LIMIT), 10) || DEFAULT_LIMIT, 1),
				MAX_LIMIT,
			);
			const offset = Math.max(parseInt(query.offset ?? '0', 10) || 0, 0);

			const result = await auditLogRepository.findByEntity(entityType, entityId, { limit, offset });

			const logsWithPrincipals = await resolvePrincipalNames(result.logs, principalRepository);

			return jsonSuccess(reply, {
				logs: logsWithPrincipals.map(toResponse),
				total: result.total,
				limit: result.limit,
				offset: result.offset,
			});
		},
	);

	// GET /api/admin/audit-logs/entity-types - Get distinct entity types
	fastify.get(
		'/audit-logs/entity-types',
		{
			preHandler: requirePermission(AUDIT_LOG_PERMISSIONS.READ),
			schema: {
				response: {
					200: EntityTypesResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const entityTypes = await auditLogRepository.findDistinctEntityTypes();

			return jsonSuccess(reply, {
				entityTypes,
			});
		},
	);

	// GET /api/admin/audit-logs/operations - Get distinct operations
	fastify.get(
		'/audit-logs/operations',
		{
			preHandler: requirePermission(AUDIT_LOG_PERMISSIONS.READ),
			schema: {
				response: {
					200: OperationsResponseSchema,
				},
			},
		},
		async (request, reply) => {
			const operations = await auditLogRepository.findDistinctOperations();

			return jsonSuccess(reply, {
				operations,
			});
		},
	);
}
