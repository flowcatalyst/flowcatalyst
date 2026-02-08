/**
 * Service Account Repository
 *
 * Data access for the normalized service_accounts table.
 */

import { eq, sql } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import { serviceAccounts, type ServiceAccountRecord } from '../schema/service-accounts.js';

import type {
	ServiceAccountData,
	WebhookAuthType,
	SignatureAlgorithm,
} from '../../../domain/index.js';

/**
 * Service account repository interface.
 */
export interface ServiceAccountRepository {
	findById(id: string, tx?: TransactionContext): Promise<ServiceAccountData | undefined>;
	findByCode(code: string, tx?: TransactionContext): Promise<ServiceAccountData | undefined>;
	existsByCode(code: string, tx?: TransactionContext): Promise<boolean>;
	persist(id: string, name: string, sa: ServiceAccountData, applicationId: string | null, active: boolean, tx?: TransactionContext): Promise<void>;
	delete(id: string, tx?: TransactionContext): Promise<void>;
}

/**
 * Create a Service Account repository.
 */
export function createServiceAccountRepository(defaultDb: AnyDb): ServiceAccountRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(id: string, tx?: TransactionContext): Promise<ServiceAccountData | undefined> {
			const [record] = await db(tx)
				.select()
				.from(serviceAccounts)
				.where(eq(serviceAccounts.id, id))
				.limit(1);

			if (!record) return undefined;
			return recordToServiceAccountData(record);
		},

		async findByCode(code: string, tx?: TransactionContext): Promise<ServiceAccountData | undefined> {
			const [record] = await db(tx)
				.select()
				.from(serviceAccounts)
				.where(eq(serviceAccounts.code, code))
				.limit(1);

			if (!record) return undefined;
			return recordToServiceAccountData(record);
		},

		async existsByCode(code: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(serviceAccounts)
				.where(eq(serviceAccounts.code, code));
			return Number(result?.count ?? 0) > 0;
		},

		async persist(
			id: string,
			name: string,
			sa: ServiceAccountData,
			applicationId: string | null,
			active: boolean,
			tx?: TransactionContext,
		): Promise<void> {
			const now = new Date();
			const values = {
				id,
				code: sa.code,
				name,
				description: sa.description,
				applicationId,
				active,
				whAuthType: sa.whAuthType,
				whAuthTokenRef: sa.whAuthTokenRef,
				whSigningSecretRef: sa.whSigningSecretRef,
				whSigningAlgorithm: sa.whSigningAlgorithm,
				whCredentialsCreatedAt: sa.whCredentialsCreatedAt,
				whCredentialsRegeneratedAt: sa.whCredentialsRegeneratedAt,
				lastUsedAt: sa.lastUsedAt,
				updatedAt: now,
			};

			await db(tx)
				.insert(serviceAccounts)
				.values({ ...values, createdAt: now })
				.onConflictDoUpdate({
					target: serviceAccounts.id,
					set: values,
				});
		},

		async delete(id: string, tx?: TransactionContext): Promise<void> {
			await db(tx).delete(serviceAccounts).where(eq(serviceAccounts.id, id));
		},
	};
}

/**
 * Convert a database record to ServiceAccountData domain object.
 */
function recordToServiceAccountData(record: ServiceAccountRecord): ServiceAccountData {
	return {
		code: record.code,
		description: record.description,
		whAuthType: (record.whAuthType ?? 'BEARER_TOKEN') as WebhookAuthType,
		whAuthTokenRef: record.whAuthTokenRef,
		whSigningSecretRef: record.whSigningSecretRef,
		whSigningAlgorithm: (record.whSigningAlgorithm ?? 'HMAC_SHA256') as SignatureAlgorithm,
		whCredentialsCreatedAt: record.whCredentialsCreatedAt,
		whCredentialsRegeneratedAt: record.whCredentialsRegeneratedAt,
		lastUsedAt: record.lastUsedAt,
	};
}
