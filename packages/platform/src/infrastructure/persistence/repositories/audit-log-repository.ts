/**
 * Audit Log Repository
 *
 * Data access for AuditLog entities.
 */

import { eq, asc, desc, sql, and, inArray, isNotNull } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { TransactionContext } from "@flowcatalyst/persistence";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import { auditLogs, type AuditLogRecord } from "../schema/index.js";
import type { AuditLog } from "../../../domain/index.js";

/**
 * Pagination options.
 */
export interface PaginationOptions {
	readonly limit: number;
	readonly offset: number;
	readonly sortField?: string | undefined;
	readonly sortOrder?: string | undefined;
}

/**
 * Filter options for audit log queries.
 */
export interface AuditLogFilters {
	readonly entityType?: string | undefined;
	readonly entityId?: string | undefined;
	readonly principalId?: string | undefined;
	readonly operation?: string | undefined;
	readonly applicationIds?: string[] | undefined;
	readonly clientIds?: string[] | undefined;
}

/**
 * Paginated audit log result.
 */
export interface PaginatedAuditLogs {
	readonly logs: AuditLog[];
	readonly total: number;
	readonly limit: number;
	readonly offset: number;
}

/**
 * Audit log repository interface.
 */
export interface AuditLogRepository {
	findById(id: string, tx?: TransactionContext): Promise<AuditLog | undefined>;
	findByEntity(
		entityType: string,
		entityId: string,
		pagination: PaginationOptions,
		tx?: TransactionContext,
	): Promise<PaginatedAuditLogs>;
	findByPrincipal(
		principalId: string,
		pagination: PaginationOptions,
		tx?: TransactionContext,
	): Promise<PaginatedAuditLogs>;
	findByOperation(
		operation: string,
		pagination: PaginationOptions,
		tx?: TransactionContext,
	): Promise<PaginatedAuditLogs>;
	findPaged(
		filters: AuditLogFilters,
		pagination: PaginationOptions,
		tx?: TransactionContext,
	): Promise<PaginatedAuditLogs>;
	findDistinctEntityTypes(tx?: TransactionContext): Promise<string[]>;
	findDistinctOperations(tx?: TransactionContext): Promise<string[]>;
	findDistinctApplicationIds(tx?: TransactionContext): Promise<string[]>;
	findDistinctClientIds(tx?: TransactionContext): Promise<string[]>;
	count(tx?: TransactionContext): Promise<number>;
	countByEntityType(
		entityType: string,
		tx?: TransactionContext,
	): Promise<number>;
}

/**
 * Create an AuditLog repository.
 */
export function createAuditLogRepository(defaultDb: AnyDb): AuditLogRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(
			id: string,
			tx?: TransactionContext,
		): Promise<AuditLog | undefined> {
			const [record] = await db(tx)
				.select()
				.from(auditLogs)
				.where(eq(auditLogs.id, id))
				.limit(1);

			if (!record) return undefined;

			return recordToAuditLog(record);
		},

		async findByEntity(
			entityType: string,
			entityId: string,
			pagination: PaginationOptions,
			tx?: TransactionContext,
		): Promise<PaginatedAuditLogs> {
			const conditions = and(
				eq(auditLogs.entityType, entityType),
				eq(auditLogs.entityId, entityId),
			);

			const [records, countResult] = await Promise.all([
				db(tx)
					.select()
					.from(auditLogs)
					.where(conditions)
					.orderBy(desc(auditLogs.performedAt))
					.limit(pagination.limit)
					.offset(pagination.offset),
				db(tx)
					.select({ count: sql<number>`count(*)` })
					.from(auditLogs)
					.where(conditions),
			]);

			return {
				logs: records.map(recordToAuditLog),
				total: Number(countResult[0]?.count ?? 0),
				limit: pagination.limit,
				offset: pagination.offset,
			};
		},

		async findByPrincipal(
			principalId: string,
			pagination: PaginationOptions,
			tx?: TransactionContext,
		): Promise<PaginatedAuditLogs> {
			const conditions = eq(auditLogs.principalId, principalId);

			const [records, countResult] = await Promise.all([
				db(tx)
					.select()
					.from(auditLogs)
					.where(conditions)
					.orderBy(desc(auditLogs.performedAt))
					.limit(pagination.limit)
					.offset(pagination.offset),
				db(tx)
					.select({ count: sql<number>`count(*)` })
					.from(auditLogs)
					.where(conditions),
			]);

			return {
				logs: records.map(recordToAuditLog),
				total: Number(countResult[0]?.count ?? 0),
				limit: pagination.limit,
				offset: pagination.offset,
			};
		},

		async findByOperation(
			operation: string,
			pagination: PaginationOptions,
			tx?: TransactionContext,
		): Promise<PaginatedAuditLogs> {
			const conditions = eq(auditLogs.operation, operation);

			const [records, countResult] = await Promise.all([
				db(tx)
					.select()
					.from(auditLogs)
					.where(conditions)
					.orderBy(desc(auditLogs.performedAt))
					.limit(pagination.limit)
					.offset(pagination.offset),
				db(tx)
					.select({ count: sql<number>`count(*)` })
					.from(auditLogs)
					.where(conditions),
			]);

			return {
				logs: records.map(recordToAuditLog),
				total: Number(countResult[0]?.count ?? 0),
				limit: pagination.limit,
				offset: pagination.offset,
			};
		},

		async findPaged(
			filters: AuditLogFilters,
			pagination: PaginationOptions,
			tx?: TransactionContext,
		): Promise<PaginatedAuditLogs> {
			// Build conditions dynamically
			const conditions = [];

			if (filters.entityType) {
				conditions.push(eq(auditLogs.entityType, filters.entityType));
			}
			if (filters.entityId) {
				conditions.push(eq(auditLogs.entityId, filters.entityId));
			}
			if (filters.principalId) {
				conditions.push(eq(auditLogs.principalId, filters.principalId));
			}
			if (filters.operation) {
				conditions.push(eq(auditLogs.operation, filters.operation));
			}
			if (filters.applicationIds && filters.applicationIds.length > 0) {
				conditions.push(inArray(auditLogs.applicationId, filters.applicationIds));
			}
			if (filters.clientIds && filters.clientIds.length > 0) {
				conditions.push(inArray(auditLogs.clientId, filters.clientIds));
			}

			const whereClause =
				conditions.length > 0 ? and(...conditions) : undefined;

			const sortFn = pagination.sortOrder === "asc" ? asc : desc;
			const sortCol =
				pagination.sortField === "entityType"
					? auditLogs.entityType
					: pagination.sortField === "operation"
						? auditLogs.operation
						: auditLogs.performedAt;

			const [records, countResult] = await Promise.all([
				whereClause
					? db(tx)
							.select()
							.from(auditLogs)
							.where(whereClause)
							.orderBy(sortFn(sortCol))
							.limit(pagination.limit)
							.offset(pagination.offset)
					: db(tx)
							.select()
							.from(auditLogs)
							.orderBy(sortFn(sortCol))
							.limit(pagination.limit)
							.offset(pagination.offset),
				whereClause
					? db(tx)
							.select({ count: sql<number>`count(*)` })
							.from(auditLogs)
							.where(whereClause)
					: db(tx).select({ count: sql<number>`count(*)` }).from(auditLogs),
			]);

			return {
				logs: records.map(recordToAuditLog),
				total: Number(countResult[0]?.count ?? 0),
				limit: pagination.limit,
				offset: pagination.offset,
			};
		},

		async findDistinctEntityTypes(tx?: TransactionContext): Promise<string[]> {
			const results = await db(tx)
				.selectDistinct({ entityType: auditLogs.entityType })
				.from(auditLogs)
				.orderBy(auditLogs.entityType);

			return results.map((r) => r.entityType);
		},

		async findDistinctOperations(tx?: TransactionContext): Promise<string[]> {
			const results = await db(tx)
				.selectDistinct({ operation: auditLogs.operation })
				.from(auditLogs)
				.orderBy(auditLogs.operation);

			return results.map((r) => r.operation);
		},

		async findDistinctApplicationIds(tx?: TransactionContext): Promise<string[]> {
			const results = await db(tx)
				.selectDistinct({ applicationId: auditLogs.applicationId })
				.from(auditLogs)
				.where(isNotNull(auditLogs.applicationId))
				.orderBy(auditLogs.applicationId);

			return results.map((r) => r.applicationId!);
		},

		async findDistinctClientIds(tx?: TransactionContext): Promise<string[]> {
			const results = await db(tx)
				.selectDistinct({ clientId: auditLogs.clientId })
				.from(auditLogs)
				.where(isNotNull(auditLogs.clientId))
				.orderBy(auditLogs.clientId);

			return results.map((r) => r.clientId!);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(auditLogs);
			return Number(result?.count ?? 0);
		},

		async countByEntityType(
			entityType: string,
			tx?: TransactionContext,
		): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(auditLogs)
				.where(eq(auditLogs.entityType, entityType));
			return Number(result?.count ?? 0);
		},
	};
}

/**
 * Convert a database record to an AuditLog.
 */
function recordToAuditLog(record: AuditLogRecord): AuditLog {
	return {
		id: record.id,
		entityType: record.entityType,
		entityId: record.entityId,
		operation: record.operation,
		operationJson: record.operationJson,
		principalId: record.principalId,
		applicationId: record.applicationId ?? null,
		clientId: record.clientId ?? null,
		performedAt: record.performedAt,
	};
}
