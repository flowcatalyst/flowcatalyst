/**
 * Client Auth Config Repository
 *
 * Data access for ClientAuthConfig entities.
 */

import { eq, sql } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	clientAuthConfigs,
	type ClientAuthConfigRecord,
	type NewClientAuthConfigRecord,
} from '../schema/index.js';
import {
	type ClientAuthConfig,
	type NewClientAuthConfig,
	type AuthConfigType,
	type AuthProvider,
} from '../../../domain/index.js';

/**
 * Client auth config repository interface.
 */
export interface ClientAuthConfigRepository {
	findById(id: string, tx?: TransactionContext): Promise<ClientAuthConfig | undefined>;
	findByEmailDomain(emailDomain: string, tx?: TransactionContext): Promise<ClientAuthConfig | undefined>;
	findByConfigType(configType: AuthConfigType, tx?: TransactionContext): Promise<ClientAuthConfig[]>;
	findByPrimaryClientId(clientId: string, tx?: TransactionContext): Promise<ClientAuthConfig[]>;
	findAll(tx?: TransactionContext): Promise<ClientAuthConfig[]>;
	count(tx?: TransactionContext): Promise<number>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByEmailDomain(emailDomain: string, tx?: TransactionContext): Promise<boolean>;
	insert(entity: NewClientAuthConfig, tx?: TransactionContext): Promise<ClientAuthConfig>;
	update(entity: ClientAuthConfig, tx?: TransactionContext): Promise<ClientAuthConfig>;
	persist(entity: NewClientAuthConfig, tx?: TransactionContext): Promise<ClientAuthConfig>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
	delete(entity: ClientAuthConfig, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Create a ClientAuthConfig repository.
 */
export function createClientAuthConfigRepository(defaultDb: AnyDb): ClientAuthConfigRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(id: string, tx?: TransactionContext): Promise<ClientAuthConfig | undefined> {
			const [record] = await db(tx)
				.select()
				.from(clientAuthConfigs)
				.where(eq(clientAuthConfigs.id, id))
				.limit(1);

			if (!record) return undefined;

			return recordToClientAuthConfig(record);
		},

		async findByEmailDomain(
			emailDomain: string,
			tx?: TransactionContext,
		): Promise<ClientAuthConfig | undefined> {
			const [record] = await db(tx)
				.select()
				.from(clientAuthConfigs)
				.where(eq(clientAuthConfigs.emailDomain, emailDomain.toLowerCase()))
				.limit(1);

			if (!record) return undefined;

			return recordToClientAuthConfig(record);
		},

		async findByConfigType(
			configType: AuthConfigType,
			tx?: TransactionContext,
		): Promise<ClientAuthConfig[]> {
			const records = await db(tx)
				.select()
				.from(clientAuthConfigs)
				.where(eq(clientAuthConfigs.configType, configType));

			return records.map(recordToClientAuthConfig);
		},

		async findByPrimaryClientId(clientId: string, tx?: TransactionContext): Promise<ClientAuthConfig[]> {
			const records = await db(tx)
				.select()
				.from(clientAuthConfigs)
				.where(eq(clientAuthConfigs.primaryClientId, clientId));

			return records.map(recordToClientAuthConfig);
		},

		async findAll(tx?: TransactionContext): Promise<ClientAuthConfig[]> {
			const records = await db(tx).select().from(clientAuthConfigs);
			return records.map(recordToClientAuthConfig);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(clientAuthConfigs);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(clientAuthConfigs)
				.where(eq(clientAuthConfigs.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByEmailDomain(emailDomain: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(clientAuthConfigs)
				.where(eq(clientAuthConfigs.emailDomain, emailDomain.toLowerCase()));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewClientAuthConfig, tx?: TransactionContext): Promise<ClientAuthConfig> {
			const now = new Date();
			const record: NewClientAuthConfigRecord = {
				id: entity.id,
				emailDomain: entity.emailDomain,
				configType: entity.configType,
				primaryClientId: entity.primaryClientId,
				additionalClientIds: [...entity.additionalClientIds],
				grantedClientIds: [...entity.grantedClientIds],
				authProvider: entity.authProvider,
				oidcIssuerUrl: entity.oidcIssuerUrl,
				oidcClientId: entity.oidcClientId,
				oidcMultiTenant: entity.oidcMultiTenant,
				oidcIssuerPattern: entity.oidcIssuerPattern,
				oidcClientSecretRef: entity.oidcClientSecretRef,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(clientAuthConfigs).values(record);

			return this.findById(entity.id, tx) as Promise<ClientAuthConfig>;
		},

		async update(entity: ClientAuthConfig, tx?: TransactionContext): Promise<ClientAuthConfig> {
			const now = new Date();
			await db(tx)
				.update(clientAuthConfigs)
				.set({
					emailDomain: entity.emailDomain,
					configType: entity.configType,
					primaryClientId: entity.primaryClientId,
					additionalClientIds: [...entity.additionalClientIds],
					grantedClientIds: [...entity.grantedClientIds],
					authProvider: entity.authProvider,
					oidcIssuerUrl: entity.oidcIssuerUrl,
					oidcClientId: entity.oidcClientId,
					oidcMultiTenant: entity.oidcMultiTenant,
					oidcIssuerPattern: entity.oidcIssuerPattern,
					oidcClientSecretRef: entity.oidcClientSecretRef,
					updatedAt: now,
				})
				.where(eq(clientAuthConfigs.id, entity.id));

			return this.findById(entity.id, tx) as Promise<ClientAuthConfig>;
		},

		async persist(entity: NewClientAuthConfig, tx?: TransactionContext): Promise<ClientAuthConfig> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as ClientAuthConfig, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx).delete(clientAuthConfigs).where(eq(clientAuthConfigs.id, id));
			return true;
		},

		async delete(entity: ClientAuthConfig, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Convert a database record to a ClientAuthConfig.
 */
function recordToClientAuthConfig(record: ClientAuthConfigRecord): ClientAuthConfig {
	return {
		id: record.id,
		emailDomain: record.emailDomain,
		configType: record.configType as AuthConfigType,
		primaryClientId: record.primaryClientId,
		additionalClientIds: record.additionalClientIds ?? [],
		grantedClientIds: record.grantedClientIds ?? [],
		authProvider: record.authProvider as AuthProvider,
		oidcIssuerUrl: record.oidcIssuerUrl,
		oidcClientId: record.oidcClientId,
		oidcMultiTenant: record.oidcMultiTenant,
		oidcIssuerPattern: record.oidcIssuerPattern,
		oidcClientSecretRef: record.oidcClientSecretRef,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
