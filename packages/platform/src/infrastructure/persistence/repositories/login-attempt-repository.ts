/**
 * Login Attempt Repository
 *
 * Data access for LoginAttempt entities.
 */

import { eq, asc, desc, sql, and, gte, lte } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { TransactionContext } from "@flowcatalyst/persistence";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import { loginAttempts, type LoginAttemptRecord } from "../schema/index.js";
import type {
	LoginAttempt,
	LoginAttemptType,
	LoginOutcome,
	LoginFailureReason,
} from "../../../domain/index.js";
import { generate } from "@flowcatalyst/tsid";

/**
 * Pagination options.
 */
export interface LoginAttemptPaginationOptions {
	readonly page: number;
	readonly pageSize: number;
	readonly sortField?: string | undefined;
	readonly sortOrder?: string | undefined;
}

/**
 * Filter options for login attempt queries.
 */
export interface LoginAttemptFilters {
	readonly attemptType?: LoginAttemptType | undefined;
	readonly outcome?: LoginOutcome | undefined;
	readonly identifier?: string | undefined;
	readonly principalId?: string | undefined;
	readonly dateFrom?: Date | undefined;
	readonly dateTo?: Date | undefined;
}

/**
 * Paginated login attempt result.
 */
export interface PaginatedLoginAttempts {
	readonly items: LoginAttempt[];
	readonly total: number;
	readonly page: number;
	readonly pageSize: number;
}

/**
 * Login attempt repository interface.
 */
export interface LoginAttemptRepository {
	create(
		attempt: Omit<LoginAttempt, "id">,
		tx?: TransactionContext,
	): Promise<LoginAttempt>;
	findPaged(
		filters: LoginAttemptFilters,
		pagination: LoginAttemptPaginationOptions,
		tx?: TransactionContext,
	): Promise<PaginatedLoginAttempts>;
}

/**
 * Create a LoginAttempt repository.
 */
export function createLoginAttemptRepository(
	defaultDb: AnyDb,
): LoginAttemptRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async create(
			attempt: Omit<LoginAttempt, "id">,
			tx?: TransactionContext,
		): Promise<LoginAttempt> {
			const id = generate("LOGIN_ATTEMPT");

			const [record] = await db(tx)
				.insert(loginAttempts)
				.values({
					id,
					attemptType: attempt.attemptType,
					outcome: attempt.outcome,
					failureReason: attempt.failureReason ?? undefined,
					identifier: attempt.identifier,
					principalId: attempt.principalId ?? undefined,
					ipAddress: attempt.ipAddress ?? undefined,
					userAgent: attempt.userAgent ?? undefined,
					attemptedAt: attempt.attemptedAt,
				})
				.returning();

			return recordToLoginAttempt(record!);
		},

		async findPaged(
			filters: LoginAttemptFilters,
			pagination: LoginAttemptPaginationOptions,
			tx?: TransactionContext,
		): Promise<PaginatedLoginAttempts> {
			const conditions = [];

			if (filters.attemptType) {
				conditions.push(eq(loginAttempts.attemptType, filters.attemptType));
			}
			if (filters.outcome) {
				conditions.push(eq(loginAttempts.outcome, filters.outcome));
			}
			if (filters.identifier) {
				conditions.push(eq(loginAttempts.identifier, filters.identifier));
			}
			if (filters.principalId) {
				conditions.push(eq(loginAttempts.principalId, filters.principalId));
			}
			if (filters.dateFrom) {
				conditions.push(gte(loginAttempts.attemptedAt, filters.dateFrom));
			}
			if (filters.dateTo) {
				conditions.push(lte(loginAttempts.attemptedAt, filters.dateTo));
			}

			const whereClause =
				conditions.length > 0 ? and(...conditions) : undefined;

			const limit = pagination.pageSize;
			const offset = pagination.page * pagination.pageSize;

			const sortFn = pagination.sortOrder === "asc" ? asc : desc;
			const sortCol =
				pagination.sortField === "attemptType"
					? loginAttempts.attemptType
					: pagination.sortField === "outcome"
						? loginAttempts.outcome
						: pagination.sortField === "identifier"
							? loginAttempts.identifier
							: loginAttempts.attemptedAt;

			const [records, countResult] = await Promise.all([
				whereClause
					? db(tx)
							.select()
							.from(loginAttempts)
							.where(whereClause)
							.orderBy(sortFn(sortCol))
							.limit(limit)
							.offset(offset)
					: db(tx)
							.select()
							.from(loginAttempts)
							.orderBy(sortFn(sortCol))
							.limit(limit)
							.offset(offset),
				whereClause
					? db(tx)
							.select({ count: sql<number>`count(*)` })
							.from(loginAttempts)
							.where(whereClause)
					: db(tx)
							.select({ count: sql<number>`count(*)` })
							.from(loginAttempts),
			]);

			return {
				items: records.map(recordToLoginAttempt),
				total: Number(countResult[0]?.count ?? 0),
				page: pagination.page,
				pageSize: pagination.pageSize,
			};
		},
	};
}

/**
 * Convert a database record to a LoginAttempt domain object.
 */
function recordToLoginAttempt(record: LoginAttemptRecord): LoginAttempt {
	return {
		id: record.id,
		attemptType: record.attemptType as LoginAttemptType,
		outcome: record.outcome as LoginOutcome,
		failureReason: (record.failureReason as LoginFailureReason) ?? null,
		identifier: record.identifier ?? "",
		principalId: record.principalId ?? null,
		ipAddress: record.ipAddress ?? null,
		userAgent: record.userAgent ?? null,
		attemptedAt: record.attemptedAt,
	};
}
