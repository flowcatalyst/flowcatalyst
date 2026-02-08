/**
 * Platform Config Repository
 */

import { eq, and, sql } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	platformConfigs,
	type PlatformConfigRecord,
	type NewPlatformConfigRecord,
} from '../schema/index.js';
import {
	type PlatformConfig,
	type NewPlatformConfig,
	type ConfigScope,
} from '../../../domain/index.js';

/**
 * Platform config repository interface.
 */
export interface PlatformConfigRepository {
	findById(id: string, tx?: TransactionContext): Promise<PlatformConfig | undefined>;
	findByKey(
		applicationCode: string,
		section: string,
		property: string,
		scope: ConfigScope,
		clientId: string | null,
		tx?: TransactionContext,
	): Promise<PlatformConfig | undefined>;
	findBySection(
		applicationCode: string,
		section: string,
		scope: ConfigScope,
		clientId: string | null,
		tx?: TransactionContext,
	): Promise<PlatformConfig[]>;
	findByApplication(
		applicationCode: string,
		scope: ConfigScope,
		clientId: string | null,
		tx?: TransactionContext,
	): Promise<PlatformConfig[]>;
	insert(entity: NewPlatformConfig, tx?: TransactionContext): Promise<PlatformConfig>;
	update(entity: PlatformConfig, tx?: TransactionContext): Promise<PlatformConfig>;
	deleteByKey(
		applicationCode: string,
		section: string,
		property: string,
		scope: ConfigScope,
		clientId: string | null,
		tx?: TransactionContext,
	): Promise<boolean>;
}

/**
 * Create a platform config repository.
 */
export function createPlatformConfigRepository(defaultDb: AnyDb): PlatformConfigRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	function buildKeyCondition(
		applicationCode: string,
		section: string,
		property: string,
		scope: ConfigScope,
		clientId: string | null,
	) {
		const conditions = [
			eq(platformConfigs.applicationCode, applicationCode),
			eq(platformConfigs.section, section),
			eq(platformConfigs.property, property),
			eq(platformConfigs.scope, scope),
		];

		if (clientId !== null) {
			conditions.push(eq(platformConfigs.clientId, clientId));
		} else {
			conditions.push(sql`${platformConfigs.clientId} IS NULL`);
		}

		return and(...conditions)!;
	}

	return {
		async findById(id: string, tx?: TransactionContext): Promise<PlatformConfig | undefined> {
			const [record] = await db(tx)
				.select()
				.from(platformConfigs)
				.where(eq(platformConfigs.id, id))
				.limit(1);

			if (!record) return undefined;
			return recordToEntity(record);
		},

		async findByKey(
			applicationCode: string,
			section: string,
			property: string,
			scope: ConfigScope,
			clientId: string | null,
			tx?: TransactionContext,
		): Promise<PlatformConfig | undefined> {
			const condition = buildKeyCondition(applicationCode, section, property, scope, clientId);
			const [record] = await db(tx).select().from(platformConfigs).where(condition).limit(1);

			if (!record) return undefined;
			return recordToEntity(record);
		},

		async findBySection(
			applicationCode: string,
			section: string,
			scope: ConfigScope,
			clientId: string | null,
			tx?: TransactionContext,
		): Promise<PlatformConfig[]> {
			const conditions = [
				eq(platformConfigs.applicationCode, applicationCode),
				eq(platformConfigs.section, section),
				eq(platformConfigs.scope, scope),
			];

			if (clientId !== null) {
				conditions.push(eq(platformConfigs.clientId, clientId));
			} else {
				conditions.push(sql`${platformConfigs.clientId} IS NULL`);
			}

			const records = await db(tx)
				.select()
				.from(platformConfigs)
				.where(and(...conditions));

			return records.map(recordToEntity);
		},

		async findByApplication(
			applicationCode: string,
			scope: ConfigScope,
			clientId: string | null,
			tx?: TransactionContext,
		): Promise<PlatformConfig[]> {
			const conditions = [
				eq(platformConfigs.applicationCode, applicationCode),
				eq(platformConfigs.scope, scope),
			];

			if (clientId !== null) {
				conditions.push(eq(platformConfigs.clientId, clientId));
			} else {
				conditions.push(sql`${platformConfigs.clientId} IS NULL`);
			}

			const records = await db(tx)
				.select()
				.from(platformConfigs)
				.where(and(...conditions));

			return records.map(recordToEntity);
		},

		async insert(entity: NewPlatformConfig, tx?: TransactionContext): Promise<PlatformConfig> {
			const now = new Date();
			const record: NewPlatformConfigRecord = {
				id: entity.id,
				applicationCode: entity.applicationCode,
				section: entity.section,
				property: entity.property,
				scope: entity.scope,
				clientId: entity.clientId,
				valueType: entity.valueType,
				value: entity.value,
				description: entity.description,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(platformConfigs).values(record);
			return this.findById(entity.id, tx) as Promise<PlatformConfig>;
		},

		async update(entity: PlatformConfig, tx?: TransactionContext): Promise<PlatformConfig> {
			const now = new Date();
			await db(tx)
				.update(platformConfigs)
				.set({
					value: entity.value,
					valueType: entity.valueType,
					description: entity.description,
					updatedAt: now,
				})
				.where(eq(platformConfigs.id, entity.id));

			return this.findById(entity.id, tx) as Promise<PlatformConfig>;
		},

		async deleteByKey(
			applicationCode: string,
			section: string,
			property: string,
			scope: ConfigScope,
			clientId: string | null,
			tx?: TransactionContext,
		): Promise<boolean> {
			const existing = await this.findByKey(applicationCode, section, property, scope, clientId, tx);
			if (!existing) return false;
			const condition = buildKeyCondition(applicationCode, section, property, scope, clientId);
			await db(tx).delete(platformConfigs).where(condition);
			return true;
		},
	};
}

function recordToEntity(record: PlatformConfigRecord): PlatformConfig {
	return {
		id: record.id,
		applicationCode: record.applicationCode,
		section: record.section,
		property: record.property,
		scope: record.scope as ConfigScope,
		clientId: record.clientId,
		valueType: record.valueType as PlatformConfig['valueType'],
		value: record.value,
		description: record.description,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
