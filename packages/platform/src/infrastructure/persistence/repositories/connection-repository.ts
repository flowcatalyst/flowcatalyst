/**
 * Connection Repository
 *
 * Data access for Connection entities.
 */

import { eq, sql, and, or, isNull, inArray } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { Repository, TransactionContext } from "@flowcatalyst/persistence";

import {
	connections,
	type ConnectionRecord,
	type NewConnectionRecord,
} from "../schema/index.js";
import type {
	Connection,
	NewConnection,
	ConnectionStatus,
} from "../../../domain/index.js";

/**
 * Filters for connection listing.
 */
export interface ConnectionFilters {
	readonly clientId?: string | null | undefined;
	readonly status?: ConnectionStatus | undefined;
	readonly serviceAccountId?: string | undefined;
	/** Scope filter: restrict results to these client IDs (+ anchor-level). Null = unrestricted. */
	readonly accessibleClientIds?: readonly string[] | null | undefined;
}

/**
 * Connection repository interface.
 */
export interface ConnectionRepository extends Repository<Connection> {
	findByCodeAndClientId(
		code: string,
		clientId: string | null,
		tx?: TransactionContext,
	): Promise<Connection | undefined>;
	existsByCodeAndClientId(
		code: string,
		clientId: string | null,
		tx?: TransactionContext,
	): Promise<boolean>;
	findByClientId(
		clientId: string,
		tx?: TransactionContext,
	): Promise<Connection[]>;
	findByServiceAccountId(
		serviceAccountId: string,
		tx?: TransactionContext,
	): Promise<Connection[]>;
	findByIds(
		ids: string[],
		tx?: TransactionContext,
	): Promise<Connection[]>;
	findActive(tx?: TransactionContext): Promise<Connection[]>;
	findWithFilters(
		filters: ConnectionFilters,
		tx?: TransactionContext,
	): Promise<Connection[]>;
}

/**
 * Create a Connection repository.
 */
export function createConnectionRepository(
	defaultDb: PostgresJsDatabase,
): ConnectionRepository {
	const db = (tx?: TransactionContext): PostgresJsDatabase =>
		(tx?.db as unknown as PostgresJsDatabase) ?? defaultDb;

	function recordToConnection(record: ConnectionRecord): Connection {
		return {
			id: record.id,
			code: record.code,
			name: record.name,
			description: record.description,
			endpoint: record.endpoint,
			externalId: record.externalId,
			status: record.status as ConnectionStatus,
			serviceAccountId: record.serviceAccountId,
			clientId: record.clientId,
			clientIdentifier: record.clientIdentifier,
			createdAt: record.createdAt,
			updatedAt: record.updatedAt,
		};
	}

	return {
		async findById(
			id: string,
			tx?: TransactionContext,
		): Promise<Connection | undefined> {
			const [record] = await db(tx)
				.select()
				.from(connections)
				.where(eq(connections.id, id))
				.limit(1);
			if (!record) return undefined;
			return recordToConnection(record);
		},

		async findByCodeAndClientId(
			code: string,
			clientId: string | null,
			tx?: TransactionContext,
		): Promise<Connection | undefined> {
			const condition =
				clientId === null
					? and(eq(connections.code, code), isNull(connections.clientId))
					: and(
							eq(connections.code, code),
							eq(connections.clientId, clientId),
						);

			const [record] = await db(tx)
				.select()
				.from(connections)
				.where(condition)
				.limit(1);
			if (!record) return undefined;
			return recordToConnection(record);
		},

		async existsByCodeAndClientId(
			code: string,
			clientId: string | null,
			tx?: TransactionContext,
		): Promise<boolean> {
			const condition =
				clientId === null
					? and(eq(connections.code, code), isNull(connections.clientId))
					: and(
							eq(connections.code, code),
							eq(connections.clientId, clientId),
						);

			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(connections)
				.where(condition);
			return Number(result?.count ?? 0) > 0;
		},

		async findByClientId(
			clientId: string,
			tx?: TransactionContext,
		): Promise<Connection[]> {
			const records = await db(tx)
				.select()
				.from(connections)
				.where(eq(connections.clientId, clientId))
				.orderBy(connections.code);
			return records.map(recordToConnection);
		},

		async findByServiceAccountId(
			serviceAccountId: string,
			tx?: TransactionContext,
		): Promise<Connection[]> {
			const records = await db(tx)
				.select()
				.from(connections)
				.where(eq(connections.serviceAccountId, serviceAccountId))
				.orderBy(connections.code);
			return records.map(recordToConnection);
		},

		async findByIds(
			ids: string[],
			tx?: TransactionContext,
		): Promise<Connection[]> {
			if (ids.length === 0) return [];
			const records = await db(tx)
				.select()
				.from(connections)
				.where(inArray(connections.id, ids));
			return records.map(recordToConnection);
		},

		async findAll(tx?: TransactionContext): Promise<Connection[]> {
			const records = await db(tx)
				.select()
				.from(connections)
				.orderBy(connections.code);
			return records.map(recordToConnection);
		},

		async findActive(tx?: TransactionContext): Promise<Connection[]> {
			const records = await db(tx)
				.select()
				.from(connections)
				.where(eq(connections.status, "ACTIVE"))
				.orderBy(connections.code);
			return records.map(recordToConnection);
		},

		async findWithFilters(
			filters: ConnectionFilters,
			tx?: TransactionContext,
		): Promise<Connection[]> {
			const conditions = [];

			if (filters.clientId !== undefined) {
				if (filters.clientId === null) {
					conditions.push(isNull(connections.clientId));
				} else {
					conditions.push(eq(connections.clientId, filters.clientId));
				}
			}

			if (filters.status) {
				conditions.push(eq(connections.status, filters.status));
			}

			if (filters.serviceAccountId) {
				conditions.push(
					eq(connections.serviceAccountId, filters.serviceAccountId),
				);
			}

			// Scope filter: show anchor-level (null clientId) + accessible client resources
			if (
				filters.accessibleClientIds !== undefined &&
				filters.accessibleClientIds !== null
			) {
				if (filters.accessibleClientIds.length === 0) {
					conditions.push(isNull(connections.clientId));
				} else {
					conditions.push(
						or(
							isNull(connections.clientId),
							inArray(connections.clientId, [...filters.accessibleClientIds]),
						)!,
					);
				}
			}

			if (conditions.length === 0) {
				return this.findAll(tx);
			}

			const records = await db(tx)
				.select()
				.from(connections)
				.where(conditions.length === 1 ? conditions[0]! : and(...conditions))
				.orderBy(connections.code);
			return records.map(recordToConnection);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(connections);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(connections)
				.where(eq(connections.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(
			entity: NewConnection,
			tx?: TransactionContext,
		): Promise<Connection> {
			const now = new Date();
			const record: NewConnectionRecord = {
				id: entity.id,
				code: entity.code,
				name: entity.name,
				description: entity.description,
				endpoint: entity.endpoint,
				externalId: entity.externalId,
				status: entity.status,
				serviceAccountId: entity.serviceAccountId,
				clientId: entity.clientId,
				clientIdentifier: entity.clientIdentifier,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(connections).values(record);
			return this.findById(entity.id, tx) as Promise<Connection>;
		},

		async update(
			entity: Connection,
			tx?: TransactionContext,
		): Promise<Connection> {
			const now = new Date();

			await db(tx)
				.update(connections)
				.set({
					name: entity.name,
					description: entity.description,
					endpoint: entity.endpoint,
					externalId: entity.externalId,
					status: entity.status,
					serviceAccountId: entity.serviceAccountId,
					updatedAt: now,
				})
				.where(eq(connections.id, entity.id));

			return this.findById(entity.id, tx) as Promise<Connection>;
		},

		async persist(
			entity: NewConnection,
			tx?: TransactionContext,
		): Promise<Connection> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as Connection, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx).delete(connections).where(eq(connections.id, id));
			return true;
		},

		async delete(
			entity: Connection,
			tx?: TransactionContext,
		): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}
