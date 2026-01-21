/**
 * Application Repository
 *
 * Data access for Application entities.
 */

import { eq, sql, and } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import {
	type PaginatedRepository,
	type PagedResult,
	type TransactionContext,
	createPagedResult,
} from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	applications,
	applicationClientConfigs,
	type ApplicationRecord,
	type NewApplicationRecord,
	type ApplicationClientConfigRecord,
	type NewApplicationClientConfigRecord,
} from '../schema/index.js';
import {
	type Application,
	type NewApplication,
	type ApplicationType,
	type ApplicationClientConfig,
	type NewApplicationClientConfig,
} from '../../../domain/index.js';

/**
 * Application repository interface.
 */
export interface ApplicationRepository extends PaginatedRepository<Application> {
	findByCode(code: string, tx?: TransactionContext): Promise<Application | undefined>;
	existsByCode(code: string, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Application client config repository interface.
 */
export interface ApplicationClientConfigRepository {
	findById(id: string, tx?: TransactionContext): Promise<ApplicationClientConfig | undefined>;
	findByApplicationAndClient(
		applicationId: string,
		clientId: string,
		tx?: TransactionContext,
	): Promise<ApplicationClientConfig | undefined>;
	findByApplication(applicationId: string, tx?: TransactionContext): Promise<ApplicationClientConfig[]>;
	findByClient(clientId: string, tx?: TransactionContext): Promise<ApplicationClientConfig[]>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByApplicationAndClient(
		applicationId: string,
		clientId: string,
		tx?: TransactionContext,
	): Promise<boolean>;
	insert(entity: NewApplicationClientConfig, tx?: TransactionContext): Promise<ApplicationClientConfig>;
	update(entity: ApplicationClientConfig, tx?: TransactionContext): Promise<ApplicationClientConfig>;
	persist(entity: NewApplicationClientConfig, tx?: TransactionContext): Promise<ApplicationClientConfig>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
	delete(entity: ApplicationClientConfig, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Create an Application repository.
 */
export function createApplicationRepository(defaultDb: AnyDb): ApplicationRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(id: string, tx?: TransactionContext): Promise<Application | undefined> {
			const [record] = await db(tx)
				.select()
				.from(applications)
				.where(eq(applications.id, id))
				.limit(1);

			if (!record) return undefined;

			return recordToApplication(record);
		},

		async findByCode(code: string, tx?: TransactionContext): Promise<Application | undefined> {
			const [record] = await db(tx)
				.select()
				.from(applications)
				.where(eq(applications.code, code.toLowerCase()))
				.limit(1);

			if (!record) return undefined;

			return recordToApplication(record);
		},

		async findAll(tx?: TransactionContext): Promise<Application[]> {
			const records = await db(tx).select().from(applications);
			return records.map(recordToApplication);
		},

		async findPaged(page: number, pageSize: number, tx?: TransactionContext): Promise<PagedResult<Application>> {
			const [countResult] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(applications);
			const totalItems = Number(countResult?.count ?? 0);

			const records = await db(tx)
				.select()
				.from(applications)
				.limit(pageSize)
				.offset(page * pageSize)
				.orderBy(applications.createdAt);

			const items = records.map(recordToApplication);
			return createPagedResult(items, page, pageSize, totalItems);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(applications);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(applications)
				.where(eq(applications.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByCode(code: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(applications)
				.where(eq(applications.code, code.toLowerCase()));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewApplication, tx?: TransactionContext): Promise<Application> {
			const now = new Date();
			const record: NewApplicationRecord = {
				id: entity.id,
				type: entity.type,
				code: entity.code,
				name: entity.name,
				description: entity.description,
				iconUrl: entity.iconUrl,
				website: entity.website,
				logo: entity.logo,
				logoMimeType: entity.logoMimeType,
				defaultBaseUrl: entity.defaultBaseUrl,
				serviceAccountId: entity.serviceAccountId,
				active: entity.active,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(applications).values(record);

			return this.findById(entity.id, tx) as Promise<Application>;
		},

		async update(entity: Application, tx?: TransactionContext): Promise<Application> {
			const now = new Date();

			await db(tx)
				.update(applications)
				.set({
					name: entity.name,
					description: entity.description,
					iconUrl: entity.iconUrl,
					website: entity.website,
					logo: entity.logo,
					logoMimeType: entity.logoMimeType,
					defaultBaseUrl: entity.defaultBaseUrl,
					serviceAccountId: entity.serviceAccountId,
					active: entity.active,
					updatedAt: now,
				})
				.where(eq(applications.id, entity.id));

			return this.findById(entity.id, tx) as Promise<Application>;
		},

		async persist(entity: NewApplication, tx?: TransactionContext): Promise<Application> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as Application, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx).delete(applications).where(eq(applications.id, id));
			return true;
		},

		async delete(entity: Application, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Create an ApplicationClientConfig repository.
 */
export function createApplicationClientConfigRepository(defaultDb: AnyDb): ApplicationClientConfigRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(id: string, tx?: TransactionContext): Promise<ApplicationClientConfig | undefined> {
			const [record] = await db(tx)
				.select()
				.from(applicationClientConfigs)
				.where(eq(applicationClientConfigs.id, id))
				.limit(1);

			if (!record) return undefined;

			return recordToApplicationClientConfig(record);
		},

		async findByApplicationAndClient(
			applicationId: string,
			clientId: string,
			tx?: TransactionContext,
		): Promise<ApplicationClientConfig | undefined> {
			const [record] = await db(tx)
				.select()
				.from(applicationClientConfigs)
				.where(
					and(
						eq(applicationClientConfigs.applicationId, applicationId),
						eq(applicationClientConfigs.clientId, clientId),
					),
				)
				.limit(1);

			if (!record) return undefined;

			return recordToApplicationClientConfig(record);
		},

		async findByApplication(applicationId: string, tx?: TransactionContext): Promise<ApplicationClientConfig[]> {
			const records = await db(tx)
				.select()
				.from(applicationClientConfigs)
				.where(eq(applicationClientConfigs.applicationId, applicationId));
			return records.map(recordToApplicationClientConfig);
		},

		async findByClient(clientId: string, tx?: TransactionContext): Promise<ApplicationClientConfig[]> {
			const records = await db(tx)
				.select()
				.from(applicationClientConfigs)
				.where(eq(applicationClientConfigs.clientId, clientId));
			return records.map(recordToApplicationClientConfig);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(applicationClientConfigs)
				.where(eq(applicationClientConfigs.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByApplicationAndClient(
			applicationId: string,
			clientId: string,
			tx?: TransactionContext,
		): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(applicationClientConfigs)
				.where(
					and(
						eq(applicationClientConfigs.applicationId, applicationId),
						eq(applicationClientConfigs.clientId, clientId),
					),
				);
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewApplicationClientConfig, tx?: TransactionContext): Promise<ApplicationClientConfig> {
			const now = new Date();
			const record: NewApplicationClientConfigRecord = {
				id: entity.id,
				applicationId: entity.applicationId,
				clientId: entity.clientId,
				enabled: entity.enabled,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(applicationClientConfigs).values(record);

			return this.findById(entity.id, tx) as Promise<ApplicationClientConfig>;
		},

		async update(entity: ApplicationClientConfig, tx?: TransactionContext): Promise<ApplicationClientConfig> {
			const now = new Date();

			await db(tx)
				.update(applicationClientConfigs)
				.set({
					enabled: entity.enabled,
					updatedAt: now,
				})
				.where(eq(applicationClientConfigs.id, entity.id));

			return this.findById(entity.id, tx) as Promise<ApplicationClientConfig>;
		},

		async persist(
			entity: NewApplicationClientConfig,
			tx?: TransactionContext,
		): Promise<ApplicationClientConfig> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as ApplicationClientConfig, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx).delete(applicationClientConfigs).where(eq(applicationClientConfigs.id, id));
			return true;
		},

		async delete(entity: ApplicationClientConfig, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Convert a database record to an Application.
 */
function recordToApplication(record: ApplicationRecord): Application {
	return {
		id: record.id,
		type: record.type as ApplicationType,
		code: record.code,
		name: record.name,
		description: record.description,
		iconUrl: record.iconUrl,
		website: record.website,
		logo: record.logo,
		logoMimeType: record.logoMimeType,
		defaultBaseUrl: record.defaultBaseUrl,
		serviceAccountId: record.serviceAccountId,
		active: record.active,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}

/**
 * Convert a database record to an ApplicationClientConfig.
 */
function recordToApplicationClientConfig(record: ApplicationClientConfigRecord): ApplicationClientConfig {
	return {
		id: record.id,
		applicationId: record.applicationId,
		clientId: record.clientId,
		enabled: record.enabled,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
