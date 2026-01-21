/**
 * Audit Logs Admin API
 *
 * REST endpoints for viewing audit logs.
 */

import type { FastifyInstance } from 'fastify';
import { jsonSuccess, notFound } from '@flowcatalyst/http';

import type { AuditLog, AuditLogWithPrincipal } from '../../domain/index.js';
import type { AuditLogRepository, PrincipalRepository } from '../../infrastructure/persistence/index.js';
import { requirePermission } from '../../authorization/index.js';
import { AUDIT_LOG_PERMISSIONS } from '../../authorization/permissions/platform-admin.js';

// Response schemas
interface AuditLogResponse {
	id: string;
	entityType: string;
	entityId: string;
	operation: string;
	operationJson: unknown | null;
	principalId: string | null;
	principalName: string | null;
	performedAt: string;
}

interface AuditLogListResponse {
	logs: AuditLogResponse[];
	total: number;
	limit: number;
	offset: number;
}

interface EntityTypesResponse {
	entityTypes: string[];
}

interface OperationsResponse {
	operations: string[];
}

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
		},
		async (request, reply) => {
			const query = request.query as {
				entityType?: string;
				entityId?: string;
				principalId?: string;
				operation?: string;
				limit?: string;
				offset?: string;
			};

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

			const response: AuditLogListResponse = {
				logs: logsWithPrincipals.map(toResponse),
				total: result.total,
				limit: result.limit,
				offset: result.offset,
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/audit-logs/:id - Get single audit log
	fastify.get(
		'/audit-logs/:id',
		{
			preHandler: requirePermission(AUDIT_LOG_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const { id } = request.params as { id: string };
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
		},
		async (request, reply) => {
			const { entityType, entityId } = request.params as { entityType: string; entityId: string };
			const query = request.query as { limit?: string; offset?: string };

			const limit = Math.min(
				Math.max(parseInt(query.limit ?? String(DEFAULT_LIMIT), 10) || DEFAULT_LIMIT, 1),
				MAX_LIMIT,
			);
			const offset = Math.max(parseInt(query.offset ?? '0', 10) || 0, 0);

			const result = await auditLogRepository.findByEntity(entityType, entityId, { limit, offset });

			const logsWithPrincipals = await resolvePrincipalNames(result.logs, principalRepository);

			const response: AuditLogListResponse = {
				logs: logsWithPrincipals.map(toResponse),
				total: result.total,
				limit: result.limit,
				offset: result.offset,
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/audit-logs/entity-types - Get distinct entity types
	fastify.get(
		'/audit-logs/entity-types',
		{
			preHandler: requirePermission(AUDIT_LOG_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const entityTypes = await auditLogRepository.findDistinctEntityTypes();

			const response: EntityTypesResponse = {
				entityTypes,
			};

			return jsonSuccess(reply, response);
		},
	);

	// GET /api/admin/audit-logs/operations - Get distinct operations
	fastify.get(
		'/audit-logs/operations',
		{
			preHandler: requirePermission(AUDIT_LOG_PERMISSIONS.READ),
		},
		async (request, reply) => {
			const operations = await auditLogRepository.findDistinctOperations();

			const response: OperationsResponse = {
				operations,
			};

			return jsonSuccess(reply, response);
		},
	);
}
