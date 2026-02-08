/**
 * Identity Provider Repository
 */

import { eq, sql } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	identityProviders,
	identityProviderAllowedDomains,
	type IdentityProviderRecord,
} from '../schema/index.js';
import type { IdentityProvider, NewIdentityProvider } from '../../../domain/index.js';

export interface IdentityProviderRepository {
	findById(id: string, tx?: TransactionContext): Promise<IdentityProvider | undefined>;
	findByCode(code: string, tx?: TransactionContext): Promise<IdentityProvider | undefined>;
	findAll(tx?: TransactionContext): Promise<IdentityProvider[]>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByCode(code: string, tx?: TransactionContext): Promise<boolean>;
	persist(entity: NewIdentityProvider, tx?: TransactionContext): Promise<IdentityProvider>;
	insert(entity: NewIdentityProvider, tx?: TransactionContext): Promise<void>;
	update(entity: IdentityProvider, tx?: TransactionContext): Promise<void>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
	delete(entity: IdentityProvider, tx?: TransactionContext): Promise<boolean>;
}

export function createIdentityProviderRepository(defaultDb: AnyDb): IdentityProviderRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	async function loadAllowedDomains(identityProviderId: string, database: AnyDb): Promise<string[]> {
		const rows = await database
			.select({ emailDomain: identityProviderAllowedDomains.emailDomain })
			.from(identityProviderAllowedDomains)
			.where(eq(identityProviderAllowedDomains.identityProviderId, identityProviderId));
		return rows.map((r) => r.emailDomain);
	}

	async function saveAllowedDomains(identityProviderId: string, domains: readonly string[], database: AnyDb): Promise<void> {
		await database
			.delete(identityProviderAllowedDomains)
			.where(eq(identityProviderAllowedDomains.identityProviderId, identityProviderId));

		if (domains.length > 0) {
			await database.insert(identityProviderAllowedDomains).values(
				domains.map((emailDomain) => ({
					identityProviderId,
					emailDomain,
				})),
			);
		}
	}

	async function hydrate(record: IdentityProviderRecord, database: AnyDb): Promise<IdentityProvider> {
		const allowedEmailDomains = await loadAllowedDomains(record.id, database);
		return {
			id: record.id,
			code: record.code,
			name: record.name,
			type: record.type as IdentityProvider['type'],
			oidcIssuerUrl: record.oidcIssuerUrl,
			oidcClientId: record.oidcClientId,
			oidcClientSecretRef: record.oidcClientSecretRef,
			oidcMultiTenant: record.oidcMultiTenant,
			oidcIssuerPattern: record.oidcIssuerPattern,
			allowedEmailDomains,
			createdAt: record.createdAt,
			updatedAt: record.updatedAt,
		};
	}

	return {
		async persist(entity: NewIdentityProvider, tx?: TransactionContext): Promise<IdentityProvider> {
			const existing = await this.findById(entity.id, tx);
			if (existing) {
				const updated: IdentityProvider = { ...entity, createdAt: existing.createdAt, updatedAt: existing.updatedAt };
				await this.update(updated, tx);
				return (await this.findById(entity.id, tx))!;
			}
			await this.insert(entity, tx);
			return (await this.findById(entity.id, tx))!;
		},

		async findById(id: string, tx?: TransactionContext): Promise<IdentityProvider | undefined> {
			const database = db(tx);
			const [record] = await database
				.select()
				.from(identityProviders)
				.where(eq(identityProviders.id, id))
				.limit(1);

			if (!record) return undefined;
			return hydrate(record, database);
		},

		async findByCode(code: string, tx?: TransactionContext): Promise<IdentityProvider | undefined> {
			const database = db(tx);
			const [record] = await database
				.select()
				.from(identityProviders)
				.where(eq(identityProviders.code, code))
				.limit(1);

			if (!record) return undefined;
			return hydrate(record, database);
		},

		async findAll(tx?: TransactionContext): Promise<IdentityProvider[]> {
			const database = db(tx);
			const records = await database
				.select()
				.from(identityProviders)
				.orderBy(identityProviders.name);

			return Promise.all(records.map((r) => hydrate(r, database)));
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(identityProviders)
				.where(eq(identityProviders.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByCode(code: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(identityProviders)
				.where(eq(identityProviders.code, code));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewIdentityProvider, tx?: TransactionContext): Promise<void> {
			const database = db(tx);
			const now = new Date();
			await database.insert(identityProviders).values({
				id: entity.id,
				code: entity.code,
				name: entity.name,
				type: entity.type,
				oidcIssuerUrl: entity.oidcIssuerUrl,
				oidcClientId: entity.oidcClientId,
				oidcClientSecretRef: entity.oidcClientSecretRef,
				oidcMultiTenant: entity.oidcMultiTenant,
				oidcIssuerPattern: entity.oidcIssuerPattern,
				createdAt: now,
				updatedAt: now,
			});

			await saveAllowedDomains(entity.id, entity.allowedEmailDomains, database);
		},

		async update(entity: IdentityProvider, tx?: TransactionContext): Promise<void> {
			const database = db(tx);
			await database
				.update(identityProviders)
				.set({
					name: entity.name,
					type: entity.type,
					oidcIssuerUrl: entity.oidcIssuerUrl,
					oidcClientId: entity.oidcClientId,
					oidcClientSecretRef: entity.oidcClientSecretRef,
					oidcMultiTenant: entity.oidcMultiTenant,
					oidcIssuerPattern: entity.oidcIssuerPattern,
					updatedAt: new Date(),
				})
				.where(eq(identityProviders.id, entity.id));

			await saveAllowedDomains(entity.id, entity.allowedEmailDomains, database);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const database = db(tx);
			await database
				.delete(identityProviderAllowedDomains)
				.where(eq(identityProviderAllowedDomains.identityProviderId, id));

			const result = await database
				.delete(identityProviders)
				.where(eq(identityProviders.id, id));
			return (result?.length ?? 0) > 0;
		},

		async delete(entity: IdentityProvider, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}
