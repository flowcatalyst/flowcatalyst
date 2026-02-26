/**
 * CORS Allowed Origin Repository
 */

import { eq, sql } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { TransactionContext } from "@flowcatalyst/persistence";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	corsAllowedOrigins,
	type CorsAllowedOriginRecord,
	type NewCorsAllowedOriginRecord,
} from "../schema/index.js";
import type {
	CorsAllowedOrigin,
	NewCorsAllowedOrigin,
} from "../../../domain/index.js";

/**
 * CORS allowed origin repository interface.
 */
export interface CorsAllowedOriginRepository {
	findById(
		id: string,
		tx?: TransactionContext,
	): Promise<CorsAllowedOrigin | undefined>;
	findByOrigin(
		origin: string,
		tx?: TransactionContext,
	): Promise<CorsAllowedOrigin | undefined>;
	findAll(tx?: TransactionContext): Promise<CorsAllowedOrigin[]>;
	count(tx?: TransactionContext): Promise<number>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByOrigin(origin: string, tx?: TransactionContext): Promise<boolean>;
	getAllowedOrigins(tx?: TransactionContext): Promise<Set<string>>;
	insert(
		entity: NewCorsAllowedOrigin,
		tx?: TransactionContext,
	): Promise<CorsAllowedOrigin>;
	update(
		entity: CorsAllowedOrigin,
		tx?: TransactionContext,
	): Promise<CorsAllowedOrigin>;
	persist(
		entity: NewCorsAllowedOrigin,
		tx?: TransactionContext,
	): Promise<CorsAllowedOrigin>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
	delete(entity: CorsAllowedOrigin, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Create a CORS allowed origin repository.
 */
export function createCorsAllowedOriginRepository(
	defaultDb: AnyDb,
): CorsAllowedOriginRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(
			id: string,
			tx?: TransactionContext,
		): Promise<CorsAllowedOrigin | undefined> {
			const [record] = await db(tx)
				.select()
				.from(corsAllowedOrigins)
				.where(eq(corsAllowedOrigins.id, id))
				.limit(1);

			if (!record) return undefined;
			return recordToEntity(record);
		},

		async findByOrigin(
			origin: string,
			tx?: TransactionContext,
		): Promise<CorsAllowedOrigin | undefined> {
			const [record] = await db(tx)
				.select()
				.from(corsAllowedOrigins)
				.where(eq(corsAllowedOrigins.origin, origin.toLowerCase()))
				.limit(1);

			if (!record) return undefined;
			return recordToEntity(record);
		},

		async findAll(tx?: TransactionContext): Promise<CorsAllowedOrigin[]> {
			const records = await db(tx).select().from(corsAllowedOrigins);
			return records.map(recordToEntity);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(corsAllowedOrigins);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(corsAllowedOrigins)
				.where(eq(corsAllowedOrigins.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByOrigin(
			origin: string,
			tx?: TransactionContext,
		): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(corsAllowedOrigins)
				.where(eq(corsAllowedOrigins.origin, origin.toLowerCase()));
			return Number(result?.count ?? 0) > 0;
		},

		async getAllowedOrigins(tx?: TransactionContext): Promise<Set<string>> {
			const records = await db(tx)
				.select({ origin: corsAllowedOrigins.origin })
				.from(corsAllowedOrigins);
			return new Set(records.map((r) => r.origin));
		},

		async insert(
			entity: NewCorsAllowedOrigin,
			tx?: TransactionContext,
		): Promise<CorsAllowedOrigin> {
			const now = new Date();
			const record: NewCorsAllowedOriginRecord = {
				id: entity.id,
				origin: entity.origin,
				description: entity.description,
				createdBy: entity.createdBy,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(corsAllowedOrigins).values(record);
			return this.findById(entity.id, tx) as Promise<CorsAllowedOrigin>;
		},

		async update(
			entity: CorsAllowedOrigin,
			tx?: TransactionContext,
		): Promise<CorsAllowedOrigin> {
			const now = new Date();
			await db(tx)
				.update(corsAllowedOrigins)
				.set({
					origin: entity.origin,
					description: entity.description,
					updatedAt: now,
				})
				.where(eq(corsAllowedOrigins.id, entity.id));

			return this.findById(entity.id, tx) as Promise<CorsAllowedOrigin>;
		},

		async persist(
			entity: NewCorsAllowedOrigin,
			tx?: TransactionContext,
		): Promise<CorsAllowedOrigin> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as CorsAllowedOrigin, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx)
				.delete(corsAllowedOrigins)
				.where(eq(corsAllowedOrigins.id, id));
			return true;
		},

		async delete(
			entity: CorsAllowedOrigin,
			tx?: TransactionContext,
		): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

function recordToEntity(record: CorsAllowedOriginRecord): CorsAllowedOrigin {
	return {
		id: record.id,
		origin: record.origin,
		description: record.description,
		createdBy: record.createdBy,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
