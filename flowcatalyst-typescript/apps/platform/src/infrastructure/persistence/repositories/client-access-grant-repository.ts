/**
 * Client Access Grant Repository
 *
 * Data access for ClientAccessGrant entities.
 */

import { eq, sql, and } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	clientAccessGrants,
	type ClientAccessGrantRecord,
	type NewClientAccessGrantRecord,
} from '../schema/index.js';
import { type ClientAccessGrant, type NewClientAccessGrant } from '../../../domain/index.js';

/**
 * Client access grant repository interface.
 */
export interface ClientAccessGrantRepository {
	findById(id: string, tx?: TransactionContext): Promise<ClientAccessGrant | undefined>;
	findByPrincipal(principalId: string, tx?: TransactionContext): Promise<ClientAccessGrant[]>;
	findByClient(clientId: string, tx?: TransactionContext): Promise<ClientAccessGrant[]>;
	findByPrincipalAndClient(
		principalId: string,
		clientId: string,
		tx?: TransactionContext,
	): Promise<ClientAccessGrant | undefined>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByPrincipalAndClient(
		principalId: string,
		clientId: string,
		tx?: TransactionContext,
	): Promise<boolean>;
	insert(entity: NewClientAccessGrant, tx?: TransactionContext): Promise<ClientAccessGrant>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
	deleteByPrincipalAndClient(
		principalId: string,
		clientId: string,
		tx?: TransactionContext,
	): Promise<boolean>;
	persist(entity: NewClientAccessGrant, tx?: TransactionContext): Promise<ClientAccessGrant>;
	delete(entity: ClientAccessGrant, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Create a ClientAccessGrant repository.
 */
export function createClientAccessGrantRepository(defaultDb: AnyDb): ClientAccessGrantRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(id: string, tx?: TransactionContext): Promise<ClientAccessGrant | undefined> {
			const [record] = await db(tx)
				.select()
				.from(clientAccessGrants)
				.where(eq(clientAccessGrants.id, id))
				.limit(1);

			if (!record) return undefined;

			return recordToClientAccessGrant(record);
		},

		async findByPrincipal(
			principalId: string,
			tx?: TransactionContext,
		): Promise<ClientAccessGrant[]> {
			const records = await db(tx)
				.select()
				.from(clientAccessGrants)
				.where(eq(clientAccessGrants.principalId, principalId));
			return records.map(recordToClientAccessGrant);
		},

		async findByClient(clientId: string, tx?: TransactionContext): Promise<ClientAccessGrant[]> {
			const records = await db(tx)
				.select()
				.from(clientAccessGrants)
				.where(eq(clientAccessGrants.clientId, clientId));
			return records.map(recordToClientAccessGrant);
		},

		async findByPrincipalAndClient(
			principalId: string,
			clientId: string,
			tx?: TransactionContext,
		): Promise<ClientAccessGrant | undefined> {
			const [record] = await db(tx)
				.select()
				.from(clientAccessGrants)
				.where(
					and(
						eq(clientAccessGrants.principalId, principalId),
						eq(clientAccessGrants.clientId, clientId),
					),
				)
				.limit(1);

			if (!record) return undefined;

			return recordToClientAccessGrant(record);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(clientAccessGrants)
				.where(eq(clientAccessGrants.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByPrincipalAndClient(
			principalId: string,
			clientId: string,
			tx?: TransactionContext,
		): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(clientAccessGrants)
				.where(
					and(
						eq(clientAccessGrants.principalId, principalId),
						eq(clientAccessGrants.clientId, clientId),
					),
				);
			return Number(result?.count ?? 0) > 0;
		},

		async insert(
			entity: NewClientAccessGrant,
			tx?: TransactionContext,
		): Promise<ClientAccessGrant> {
			const now = new Date();
			const record: NewClientAccessGrantRecord = {
				id: entity.id,
				principalId: entity.principalId,
				clientId: entity.clientId,
				grantedBy: entity.grantedBy,
				grantedAt: entity.grantedAt,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(clientAccessGrants).values(record);

			return this.findById(entity.id, tx) as Promise<ClientAccessGrant>;
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx).delete(clientAccessGrants).where(eq(clientAccessGrants.id, id));
			return true;
		},

		async deleteByPrincipalAndClient(
			principalId: string,
			clientId: string,
			tx?: TransactionContext,
		): Promise<boolean> {
			const exists = await this.existsByPrincipalAndClient(principalId, clientId, tx);
			if (!exists) return false;
			await db(tx)
				.delete(clientAccessGrants)
				.where(
					and(
						eq(clientAccessGrants.principalId, principalId),
						eq(clientAccessGrants.clientId, clientId),
					),
				);
			return true;
		},

		async persist(
			entity: NewClientAccessGrant,
			tx?: TransactionContext,
		): Promise<ClientAccessGrant> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.findById(entity.id, tx) as Promise<ClientAccessGrant>;
			}
			return this.insert(entity, tx);
		},

		async delete(entity: ClientAccessGrant, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Convert a database record to a ClientAccessGrant.
 */
function recordToClientAccessGrant(record: ClientAccessGrantRecord): ClientAccessGrant {
	return {
		id: record.id,
		principalId: record.principalId,
		clientId: record.clientId,
		grantedBy: record.grantedBy,
		grantedAt: record.grantedAt,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
