/**
 * Dispatch Job Read Repository
 *
 * Read-only data access for the dispatch_jobs_read CQRS projection table.
 * Supports pagination, filtering, cascading filter options, and attempt loading.
 */

import { eq, desc, sql, and, inArray, gte, lte } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { TransactionContext } from "@flowcatalyst/persistence";
import {
	dispatchJobsRead,
	dispatchJobAttempts,
	type DispatchJobReadRecord,
	type DispatchJobAttemptRecord,
} from "@flowcatalyst/persistence";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

/**
 * Filter options for dispatch job read queries.
 */
export interface DispatchJobReadFilters {
	readonly clientIds?: string[] | undefined;
	readonly statuses?: string[] | undefined;
	readonly applications?: string[] | undefined;
	readonly subdomains?: string[] | undefined;
	readonly aggregates?: string[] | undefined;
	readonly codes?: string[] | undefined;
	readonly source?: string | undefined;
	readonly kind?: string | undefined;
	readonly subscriptionId?: string | undefined;
	readonly dispatchPoolId?: string | undefined;
	readonly messageGroup?: string | undefined;
	readonly createdAfter?: Date | undefined;
	readonly createdBefore?: Date | undefined;
}

/**
 * Pagination options.
 */
export interface DispatchJobReadPagination {
	readonly page: number;
	readonly size: number;
}

/**
 * Paged result.
 */
export interface PagedDispatchJobReadResult {
	readonly items: DispatchJobReadRecord[];
	readonly page: number;
	readonly size: number;
	readonly totalItems: number;
	readonly totalPages: number;
}

/**
 * Cascading filter option.
 */
export interface FilterOption {
	readonly value: string;
	readonly label: string;
}

/**
 * Filter options request.
 */
export interface DispatchJobFilterOptionsRequest {
	readonly clientIds?: string[] | undefined;
	readonly applications?: string[] | undefined;
	readonly subdomains?: string[] | undefined;
	readonly aggregates?: string[] | undefined;
}

/**
 * Available filter options.
 */
export interface DispatchJobFilterOptions {
	readonly applications: FilterOption[];
	readonly subdomains: FilterOption[];
	readonly aggregates: FilterOption[];
	readonly codes: FilterOption[];
	readonly statuses: FilterOption[];
}

function toFilterOptions(results: { value: string | null }[]): FilterOption[] {
	return results
		.filter(
			(r): r is { value: string } =>
				r.value !== null && r.value.trim() !== "",
		)
		.map((r) => ({ value: r.value, label: r.value }))
		.sort((a, b) => a.label.localeCompare(b.label));
}

/**
 * Dispatch job read repository interface.
 */
export interface DispatchJobReadRepository {
	findById(
		id: string,
		tx?: TransactionContext,
	): Promise<DispatchJobReadRecord | undefined>;
	findPaged(
		filters: DispatchJobReadFilters,
		pagination: DispatchJobReadPagination,
		tx?: TransactionContext,
	): Promise<PagedDispatchJobReadResult>;
	getFilterOptions(
		request: DispatchJobFilterOptionsRequest,
		tx?: TransactionContext,
	): Promise<DispatchJobFilterOptions>;
	findAttempts(
		dispatchJobId: string,
		tx?: TransactionContext,
	): Promise<DispatchJobAttemptRecord[]>;
	count(tx?: TransactionContext): Promise<number>;
}

/**
 * Create a DispatchJobRead repository.
 */
export function createDispatchJobReadRepository(
	defaultDb: AnyDb,
): DispatchJobReadRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	function buildConditions(
		filters: DispatchJobReadFilters | DispatchJobFilterOptionsRequest,
	) {
		const conditions = [];

		if (
			"clientIds" in filters &&
			filters.clientIds &&
			filters.clientIds.length > 0
		) {
			conditions.push(inArray(dispatchJobsRead.clientId, filters.clientIds));
		}
		if (
			"statuses" in filters &&
			filters.statuses &&
			filters.statuses.length > 0
		) {
			conditions.push(
				sql`${dispatchJobsRead.status} IN (${sql.join(
					filters.statuses.map((s) => sql`${s}`),
					sql`, `,
				)})`,
			);
		}
		if (filters.applications && filters.applications.length > 0) {
			conditions.push(
				inArray(dispatchJobsRead.application, filters.applications),
			);
		}
		if (filters.subdomains && filters.subdomains.length > 0) {
			conditions.push(inArray(dispatchJobsRead.subdomain, filters.subdomains));
		}
		if (filters.aggregates && filters.aggregates.length > 0) {
			conditions.push(inArray(dispatchJobsRead.aggregate, filters.aggregates));
		}
		if ("codes" in filters && filters.codes && filters.codes.length > 0) {
			conditions.push(inArray(dispatchJobsRead.code, filters.codes));
		}
		if ("source" in filters && filters.source) {
			conditions.push(eq(dispatchJobsRead.source, filters.source));
		}
		if ("kind" in filters && filters.kind) {
			conditions.push(sql`${dispatchJobsRead.kind} = ${filters.kind}`);
		}
		if ("subscriptionId" in filters && filters.subscriptionId) {
			conditions.push(
				eq(dispatchJobsRead.subscriptionId, filters.subscriptionId),
			);
		}
		if ("dispatchPoolId" in filters && filters.dispatchPoolId) {
			conditions.push(
				eq(dispatchJobsRead.dispatchPoolId, filters.dispatchPoolId),
			);
		}
		if ("messageGroup" in filters && filters.messageGroup) {
			conditions.push(eq(dispatchJobsRead.messageGroup, filters.messageGroup));
		}
		if ("createdAfter" in filters && filters.createdAfter) {
			conditions.push(gte(dispatchJobsRead.createdAt, filters.createdAfter));
		}
		if ("createdBefore" in filters && filters.createdBefore) {
			conditions.push(lte(dispatchJobsRead.createdAt, filters.createdBefore));
		}

		return conditions.length > 0 ? and(...conditions) : undefined;
	}

	return {
		async findById(
			id: string,
			tx?: TransactionContext,
		): Promise<DispatchJobReadRecord | undefined> {
			const [record] = await db(tx)
				.select()
				.from(dispatchJobsRead)
				.where(eq(dispatchJobsRead.id, id))
				.limit(1);

			return record;
		},

		async findPaged(
			filters: DispatchJobReadFilters,
			pagination: DispatchJobReadPagination,
			tx?: TransactionContext,
		): Promise<PagedDispatchJobReadResult> {
			const page = Math.max(pagination.page, 0);
			const size = Math.min(Math.max(pagination.size, 1), 100);
			const offset = page * size;
			const whereClause = buildConditions(filters);

			const baseSelect = db(tx).select().from(dispatchJobsRead);
			const baseCount = db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(dispatchJobsRead);

			const [records, countResult] = await Promise.all([
				whereClause
					? baseSelect
							.where(whereClause)
							.orderBy(desc(dispatchJobsRead.createdAt))
							.limit(size)
							.offset(offset)
					: baseSelect
							.orderBy(desc(dispatchJobsRead.createdAt))
							.limit(size)
							.offset(offset),
				whereClause ? baseCount.where(whereClause) : baseCount,
			]);

			const totalItems = Number(countResult[0]?.count ?? 0);

			return {
				items: records,
				page,
				size,
				totalItems,
				totalPages: Math.ceil(totalItems / size),
			};
		},

		async getFilterOptions(
			request: DispatchJobFilterOptionsRequest,
			tx?: TransactionContext,
		): Promise<DispatchJobFilterOptions> {
			const whereClause = buildConditions(request);
			const queryDb = db(tx);

			const [appResults, subResults, aggResults, codeResults, statusResults] =
				await Promise.all([
					whereClause
						? queryDb
								.selectDistinct({ value: dispatchJobsRead.application })
								.from(dispatchJobsRead)
								.where(whereClause)
						: queryDb
								.selectDistinct({ value: dispatchJobsRead.application })
								.from(dispatchJobsRead),
					whereClause
						? queryDb
								.selectDistinct({ value: dispatchJobsRead.subdomain })
								.from(dispatchJobsRead)
								.where(whereClause)
						: queryDb
								.selectDistinct({ value: dispatchJobsRead.subdomain })
								.from(dispatchJobsRead),
					whereClause
						? queryDb
								.selectDistinct({ value: dispatchJobsRead.aggregate })
								.from(dispatchJobsRead)
								.where(whereClause)
						: queryDb
								.selectDistinct({ value: dispatchJobsRead.aggregate })
								.from(dispatchJobsRead),
					whereClause
						? queryDb
								.selectDistinct({ value: dispatchJobsRead.code })
								.from(dispatchJobsRead)
								.where(whereClause)
						: queryDb
								.selectDistinct({ value: dispatchJobsRead.code })
								.from(dispatchJobsRead),
					whereClause
						? queryDb
								.selectDistinct({ value: dispatchJobsRead.status })
								.from(dispatchJobsRead)
								.where(whereClause)
						: queryDb
								.selectDistinct({ value: dispatchJobsRead.status })
								.from(dispatchJobsRead),
				]);

			return {
				applications: toFilterOptions(appResults),
				subdomains: toFilterOptions(subResults),
				aggregates: toFilterOptions(aggResults),
				codes: toFilterOptions(codeResults),
				statuses: toFilterOptions(statusResults),
			};
		},

		async findAttempts(
			dispatchJobId: string,
			tx?: TransactionContext,
		): Promise<DispatchJobAttemptRecord[]> {
			return db(tx)
				.select()
				.from(dispatchJobAttempts)
				.where(eq(dispatchJobAttempts.dispatchJobId, dispatchJobId))
				.orderBy(dispatchJobAttempts.attemptNumber);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(dispatchJobsRead);
			return Number(result?.count ?? 0);
		},
	};
}
