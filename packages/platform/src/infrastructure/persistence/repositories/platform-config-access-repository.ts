/**
 * Platform Config Access Repository
 */

import { eq, and } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { TransactionContext } from "@flowcatalyst/persistence";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	platformConfigAccess,
	type PlatformConfigAccessRecord,
	type NewPlatformConfigAccessRecord,
} from "../schema/index.js";
import type {
	PlatformConfigAccess,
	NewPlatformConfigAccess,
} from "../../../domain/index.js";

/**
 * Platform config access repository interface.
 */
export interface PlatformConfigAccessRepository {
	findByApplication(
		applicationCode: string,
		tx?: TransactionContext,
	): Promise<PlatformConfigAccess[]>;
	findByApplicationAndRole(
		applicationCode: string,
		roleCode: string,
		tx?: TransactionContext,
	): Promise<PlatformConfigAccess | undefined>;
	findByRoleCodes(
		applicationCode: string,
		roleCodes: readonly string[],
		tx?: TransactionContext,
	): Promise<PlatformConfigAccess[]>;
	insert(
		entity: NewPlatformConfigAccess,
		tx?: TransactionContext,
	): Promise<PlatformConfigAccess>;
	update(
		entity: PlatformConfigAccess,
		tx?: TransactionContext,
	): Promise<PlatformConfigAccess>;
	deleteByApplicationAndRole(
		applicationCode: string,
		roleCode: string,
		tx?: TransactionContext,
	): Promise<boolean>;
}

/**
 * Create a platform config access repository.
 */
export function createPlatformConfigAccessRepository(
	defaultDb: AnyDb,
): PlatformConfigAccessRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findByApplication(
			applicationCode: string,
			tx?: TransactionContext,
		): Promise<PlatformConfigAccess[]> {
			const records = await db(tx)
				.select()
				.from(platformConfigAccess)
				.where(eq(platformConfigAccess.applicationCode, applicationCode));

			return records.map(recordToEntity);
		},

		async findByApplicationAndRole(
			applicationCode: string,
			roleCode: string,
			tx?: TransactionContext,
		): Promise<PlatformConfigAccess | undefined> {
			const [record] = await db(tx)
				.select()
				.from(platformConfigAccess)
				.where(
					and(
						eq(platformConfigAccess.applicationCode, applicationCode),
						eq(platformConfigAccess.roleCode, roleCode),
					),
				)
				.limit(1);

			if (!record) return undefined;
			return recordToEntity(record);
		},

		async findByRoleCodes(
			applicationCode: string,
			roleCodes: readonly string[],
			tx?: TransactionContext,
		): Promise<PlatformConfigAccess[]> {
			if (roleCodes.length === 0) return [];

			const records = await db(tx)
				.select()
				.from(platformConfigAccess)
				.where(eq(platformConfigAccess.applicationCode, applicationCode));

			// Filter in-memory since drizzle's inArray doesn't accept readonly arrays cleanly
			const roleSet = new Set(roleCodes);
			return records.filter((r) => roleSet.has(r.roleCode)).map(recordToEntity);
		},

		async insert(
			entity: NewPlatformConfigAccess,
			tx?: TransactionContext,
		): Promise<PlatformConfigAccess> {
			const now = new Date();
			const record: NewPlatformConfigAccessRecord = {
				id: entity.id,
				applicationCode: entity.applicationCode,
				roleCode: entity.roleCode,
				canRead: entity.canRead,
				canWrite: entity.canWrite,
				createdAt: entity.createdAt ?? now,
			};

			await db(tx).insert(platformConfigAccess).values(record);

			const inserted = await this.findByApplicationAndRole(
				entity.applicationCode,
				entity.roleCode,
				tx,
			);
			return inserted!;
		},

		async update(
			entity: PlatformConfigAccess,
			tx?: TransactionContext,
		): Promise<PlatformConfigAccess> {
			await db(tx)
				.update(platformConfigAccess)
				.set({
					canRead: entity.canRead,
					canWrite: entity.canWrite,
				})
				.where(
					and(
						eq(platformConfigAccess.applicationCode, entity.applicationCode),
						eq(platformConfigAccess.roleCode, entity.roleCode),
					),
				);

			const updated = await this.findByApplicationAndRole(
				entity.applicationCode,
				entity.roleCode,
				tx,
			);
			return updated!;
		},

		async deleteByApplicationAndRole(
			applicationCode: string,
			roleCode: string,
			tx?: TransactionContext,
		): Promise<boolean> {
			const existing = await this.findByApplicationAndRole(
				applicationCode,
				roleCode,
				tx,
			);
			if (!existing) return false;
			await db(tx)
				.delete(platformConfigAccess)
				.where(
					and(
						eq(platformConfigAccess.applicationCode, applicationCode),
						eq(platformConfigAccess.roleCode, roleCode),
					),
				);
			return true;
		},
	};
}

function recordToEntity(
	record: PlatformConfigAccessRecord,
): PlatformConfigAccess {
	return {
		id: record.id,
		applicationCode: record.applicationCode,
		roleCode: record.roleCode,
		canRead: record.canRead,
		canWrite: record.canWrite,
		createdAt: record.createdAt,
	};
}
