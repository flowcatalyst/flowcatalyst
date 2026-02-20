/**
 * Dispatch Pool Repository
 *
 * Data access for DispatchPool entities.
 */

import { eq, sql, and, or, isNull, inArray } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { Repository, TransactionContext } from "@flowcatalyst/persistence";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	dispatchPools,
	type DispatchPoolRecord,
	type NewDispatchPoolRecord,
} from "../schema/index.js";
import type {
	DispatchPool,
	NewDispatchPool,
	DispatchPoolStatus,
} from "../../../domain/index.js";

/**
 * Filters for dispatch pool listing.
 */
export interface DispatchPoolFilters {
	readonly clientId?: string | null | undefined;
	readonly status?: DispatchPoolStatus | undefined;
	readonly includeArchived?: boolean | undefined;
	/** Scope filter: restrict results to these client IDs (+ anchor-level). Null = unrestricted. */
	readonly accessibleClientIds?: readonly string[] | null | undefined;
}

/**
 * Dispatch Pool repository interface.
 */
export interface DispatchPoolRepository extends Repository<DispatchPool> {
	findByCodeAndClientId(
		code: string,
		clientId: string | null,
		tx?: TransactionContext,
	): Promise<DispatchPool | undefined>;
	existsByCodeAndClientId(
		code: string,
		clientId: string | null,
		tx?: TransactionContext,
	): Promise<boolean>;
	findAnchorLevel(tx?: TransactionContext): Promise<DispatchPool[]>;
	findByClientId(
		clientId: string,
		tx?: TransactionContext,
	): Promise<DispatchPool[]>;
	findActive(tx?: TransactionContext): Promise<DispatchPool[]>;
	findWithFilters(
		filters: DispatchPoolFilters,
		tx?: TransactionContext,
	): Promise<DispatchPool[]>;
}

/**
 * Create a Dispatch Pool repository.
 */
export function createDispatchPoolRepository(
	defaultDb: AnyDb,
): DispatchPoolRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(
			id: string,
			tx?: TransactionContext,
		): Promise<DispatchPool | undefined> {
			const [record] = await db(tx)
				.select()
				.from(dispatchPools)
				.where(eq(dispatchPools.id, id))
				.limit(1);

			if (!record) return undefined;
			return recordToDispatchPool(record);
		},

		async findByCodeAndClientId(
			code: string,
			clientId: string | null,
			tx?: TransactionContext,
		): Promise<DispatchPool | undefined> {
			const condition =
				clientId === null
					? and(eq(dispatchPools.code, code), isNull(dispatchPools.clientId))
					: and(
							eq(dispatchPools.code, code),
							eq(dispatchPools.clientId, clientId),
						);

			const [record] = await db(tx)
				.select()
				.from(dispatchPools)
				.where(condition)
				.limit(1);

			if (!record) return undefined;
			return recordToDispatchPool(record);
		},

		async existsByCodeAndClientId(
			code: string,
			clientId: string | null,
			tx?: TransactionContext,
		): Promise<boolean> {
			const condition =
				clientId === null
					? and(eq(dispatchPools.code, code), isNull(dispatchPools.clientId))
					: and(
							eq(dispatchPools.code, code),
							eq(dispatchPools.clientId, clientId),
						);

			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(dispatchPools)
				.where(condition);
			return Number(result?.count ?? 0) > 0;
		},

		async findAll(tx?: TransactionContext): Promise<DispatchPool[]> {
			const records = await db(tx)
				.select()
				.from(dispatchPools)
				.orderBy(dispatchPools.code);
			return records.map(recordToDispatchPool);
		},

		async findAnchorLevel(tx?: TransactionContext): Promise<DispatchPool[]> {
			const records = await db(tx)
				.select()
				.from(dispatchPools)
				.where(isNull(dispatchPools.clientId))
				.orderBy(dispatchPools.code);
			return records.map(recordToDispatchPool);
		},

		async findByClientId(
			clientId: string,
			tx?: TransactionContext,
		): Promise<DispatchPool[]> {
			const records = await db(tx)
				.select()
				.from(dispatchPools)
				.where(eq(dispatchPools.clientId, clientId))
				.orderBy(dispatchPools.code);
			return records.map(recordToDispatchPool);
		},

		async findActive(tx?: TransactionContext): Promise<DispatchPool[]> {
			const records = await db(tx)
				.select()
				.from(dispatchPools)
				.where(eq(dispatchPools.status, "ACTIVE"))
				.orderBy(dispatchPools.code);
			return records.map(recordToDispatchPool);
		},

		async findWithFilters(
			filters: DispatchPoolFilters,
			tx?: TransactionContext,
		): Promise<DispatchPool[]> {
			const conditions = [];

			if (filters.clientId !== undefined) {
				if (filters.clientId === null) {
					conditions.push(isNull(dispatchPools.clientId));
				} else {
					conditions.push(eq(dispatchPools.clientId, filters.clientId));
				}
			}

			if (filters.status) {
				conditions.push(eq(dispatchPools.status, filters.status));
			} else if (!filters.includeArchived) {
				// By default, exclude archived
				conditions.push(sql`${dispatchPools.status} != 'ARCHIVED'`);
			}

			// Scope filter: show anchor-level (null clientId) + accessible client resources
			if (
				filters.accessibleClientIds !== undefined &&
				filters.accessibleClientIds !== null
			) {
				if (filters.accessibleClientIds.length === 0) {
					// No accessible clients - only show anchor-level
					conditions.push(isNull(dispatchPools.clientId));
				} else {
					conditions.push(
						or(
							isNull(dispatchPools.clientId),
							inArray(dispatchPools.clientId, [...filters.accessibleClientIds]),
						)!,
					);
				}
			}

			if (conditions.length === 0) {
				return this.findAll(tx);
			}

			const records = await db(tx)
				.select()
				.from(dispatchPools)
				.where(conditions.length === 1 ? conditions[0]! : and(...conditions))
				.orderBy(dispatchPools.code);
			return records.map(recordToDispatchPool);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(dispatchPools);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(dispatchPools)
				.where(eq(dispatchPools.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(
			entity: NewDispatchPool,
			tx?: TransactionContext,
		): Promise<DispatchPool> {
			const now = new Date();
			const record: NewDispatchPoolRecord = {
				id: entity.id,
				code: entity.code,
				name: entity.name,
				description: entity.description,
				rateLimit: entity.rateLimit,
				concurrency: entity.concurrency,
				clientId: entity.clientId,
				clientIdentifier: entity.clientIdentifier,
				status: entity.status,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(dispatchPools).values(record);
			return this.findById(entity.id, tx) as Promise<DispatchPool>;
		},

		async update(
			entity: DispatchPool,
			tx?: TransactionContext,
		): Promise<DispatchPool> {
			const now = new Date();

			await db(tx)
				.update(dispatchPools)
				.set({
					name: entity.name,
					description: entity.description,
					rateLimit: entity.rateLimit,
					concurrency: entity.concurrency,
					status: entity.status,
					updatedAt: now,
				})
				.where(eq(dispatchPools.id, entity.id));

			return this.findById(entity.id, tx) as Promise<DispatchPool>;
		},

		async persist(
			entity: NewDispatchPool,
			tx?: TransactionContext,
		): Promise<DispatchPool> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as DispatchPool, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx).delete(dispatchPools).where(eq(dispatchPools.id, id));
			return true;
		},

		async delete(
			entity: DispatchPool,
			tx?: TransactionContext,
		): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Convert a database record to a DispatchPool domain entity.
 */
function recordToDispatchPool(record: DispatchPoolRecord): DispatchPool {
	return {
		id: record.id,
		code: record.code,
		name: record.name,
		description: record.description,
		rateLimit: record.rateLimit,
		concurrency: record.concurrency,
		clientId: record.clientId,
		clientIdentifier: record.clientIdentifier,
		status: record.status as DispatchPoolStatus,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
