/**
 * Anchor Domain Repository
 *
 * Data access for AnchorDomain entities.
 */

import { eq, sql } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import { anchorDomains, type AnchorDomainRecord, type NewAnchorDomainRecord } from '../schema/index.js';
import { type AnchorDomain, type NewAnchorDomain } from '../../../domain/index.js';

/**
 * Anchor domain repository interface.
 * Note: AnchorDomain doesn't have updatedAt so we define a custom interface.
 */
export interface AnchorDomainRepository {
	findById(id: string, tx?: TransactionContext): Promise<AnchorDomain | undefined>;
	findByDomain(domain: string, tx?: TransactionContext): Promise<AnchorDomain | undefined>;
	findAll(tx?: TransactionContext): Promise<AnchorDomain[]>;
	count(tx?: TransactionContext): Promise<number>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByDomain(domain: string, tx?: TransactionContext): Promise<boolean>;
	insert(entity: NewAnchorDomain, tx?: TransactionContext): Promise<AnchorDomain>;
	update(entity: AnchorDomain, tx?: TransactionContext): Promise<AnchorDomain>;
	persist(entity: NewAnchorDomain, tx?: TransactionContext): Promise<AnchorDomain>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
	delete(entity: AnchorDomain, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Create an AnchorDomain repository.
 */
export function createAnchorDomainRepository(defaultDb: AnyDb): AnchorDomainRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(id: string, tx?: TransactionContext): Promise<AnchorDomain | undefined> {
			const [record] = await db(tx)
				.select()
				.from(anchorDomains)
				.where(eq(anchorDomains.id, id))
				.limit(1);

			if (!record) return undefined;

			return recordToAnchorDomain(record);
		},

		async findByDomain(domain: string, tx?: TransactionContext): Promise<AnchorDomain | undefined> {
			const [record] = await db(tx)
				.select()
				.from(anchorDomains)
				.where(eq(anchorDomains.domain, domain.toLowerCase()))
				.limit(1);

			if (!record) return undefined;

			return recordToAnchorDomain(record);
		},

		async findAll(tx?: TransactionContext): Promise<AnchorDomain[]> {
			const records = await db(tx).select().from(anchorDomains);
			return records.map(recordToAnchorDomain);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(anchorDomains);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(anchorDomains)
				.where(eq(anchorDomains.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByDomain(domain: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(anchorDomains)
				.where(eq(anchorDomains.domain, domain.toLowerCase()));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewAnchorDomain, tx?: TransactionContext): Promise<AnchorDomain> {
			const now = new Date();
			const record: NewAnchorDomainRecord = {
				id: entity.id,
				domain: entity.domain,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(anchorDomains).values(record);

			return this.findById(entity.id, tx) as Promise<AnchorDomain>;
		},

		async update(entity: AnchorDomain, tx?: TransactionContext): Promise<AnchorDomain> {
			const now = new Date();
			await db(tx)
				.update(anchorDomains)
				.set({
					domain: entity.domain,
					updatedAt: now,
				})
				.where(eq(anchorDomains.id, entity.id));

			return this.findById(entity.id, tx) as Promise<AnchorDomain>;
		},

		async persist(entity: NewAnchorDomain, tx?: TransactionContext): Promise<AnchorDomain> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as AnchorDomain, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx).delete(anchorDomains).where(eq(anchorDomains.id, id));
			return true;
		},

		async delete(entity: AnchorDomain, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Convert a database record to an AnchorDomain.
 */
function recordToAnchorDomain(record: AnchorDomainRecord): AnchorDomain {
	return {
		id: record.id,
		domain: record.domain,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
