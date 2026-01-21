/**
 * Role Repository
 *
 * Data access for AuthRole and AuthPermission entities.
 */

import { eq, sql, ilike, or, and } from 'drizzle-orm';
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
	authRoles,
	authPermissions,
	type AuthRoleRecord,
	type NewAuthRoleRecord,
	type AuthPermissionRecord,
	type NewAuthPermissionRecord,
} from '../schema/index.js';
import {
	type AuthRole,
	type RoleSource,
	type AuthPermission,
} from '../../../domain/index.js';

/**
 * Type for creating a new AuthRole.
 */
export type NewAuthRole = Omit<AuthRole, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Type for creating a new AuthPermission.
 */
export type NewAuthPermission = Omit<AuthPermission, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Role repository interface.
 */
export interface RoleRepository extends PaginatedRepository<AuthRole> {
	findByName(name: string, tx?: TransactionContext): Promise<AuthRole | undefined>;
	findBySource(source: RoleSource, tx?: TransactionContext): Promise<AuthRole[]>;
	findByApplicationId(applicationId: string, tx?: TransactionContext): Promise<AuthRole[]>;
	findCodeDefinedRoles(tx?: TransactionContext): Promise<AuthRole[]>;
	existsByName(name: string, tx?: TransactionContext): Promise<boolean>;
	search(query: string, page: number, pageSize: number, tx?: TransactionContext): Promise<PagedResult<AuthRole>>;
}

/**
 * Permission repository interface.
 */
export interface PermissionRepository {
	findById(id: string, tx?: TransactionContext): Promise<AuthPermission | undefined>;
	findByCode(code: string, tx?: TransactionContext): Promise<AuthPermission | undefined>;
	findAll(tx?: TransactionContext): Promise<AuthPermission[]>;
	findBySubdomain(subdomain: string, tx?: TransactionContext): Promise<AuthPermission[]>;
	findByContext(subdomain: string, context: string, tx?: TransactionContext): Promise<AuthPermission[]>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByCode(code: string, tx?: TransactionContext): Promise<boolean>;
	insert(entity: NewAuthPermission, tx?: TransactionContext): Promise<AuthPermission>;
	update(entity: AuthPermission, tx?: TransactionContext): Promise<AuthPermission>;
	persist(entity: NewAuthPermission, tx?: TransactionContext): Promise<AuthPermission>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Create a Role repository.
 */
export function createRoleRepository(defaultDb: AnyDb): RoleRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(id: string, tx?: TransactionContext): Promise<AuthRole | undefined> {
			const [record] = await db(tx)
				.select()
				.from(authRoles)
				.where(eq(authRoles.id, id))
				.limit(1);

			if (!record) return undefined;

			return recordToAuthRole(record);
		},

		async findByName(name: string, tx?: TransactionContext): Promise<AuthRole | undefined> {
			const [record] = await db(tx)
				.select()
				.from(authRoles)
				.where(eq(authRoles.name, name.toLowerCase()))
				.limit(1);

			if (!record) return undefined;

			return recordToAuthRole(record);
		},

		async findBySource(source: RoleSource, tx?: TransactionContext): Promise<AuthRole[]> {
			const records = await db(tx)
				.select()
				.from(authRoles)
				.where(eq(authRoles.source, source));
			return records.map(recordToAuthRole);
		},

		async findByApplicationId(applicationId: string, tx?: TransactionContext): Promise<AuthRole[]> {
			const records = await db(tx)
				.select()
				.from(authRoles)
				.where(eq(authRoles.applicationId, applicationId));
			return records.map(recordToAuthRole);
		},

		async findCodeDefinedRoles(tx?: TransactionContext): Promise<AuthRole[]> {
			const records = await db(tx)
				.select()
				.from(authRoles)
				.where(eq(authRoles.source, 'CODE'));
			return records.map(recordToAuthRole);
		},

		async findAll(tx?: TransactionContext): Promise<AuthRole[]> {
			const records = await db(tx).select().from(authRoles);
			return records.map(recordToAuthRole);
		},

		async findPaged(page: number, pageSize: number, tx?: TransactionContext): Promise<PagedResult<AuthRole>> {
			const [countResult] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(authRoles);
			const totalItems = Number(countResult?.count ?? 0);

			const records = await db(tx)
				.select()
				.from(authRoles)
				.limit(pageSize)
				.offset(page * pageSize)
				.orderBy(authRoles.name);

			const items = records.map(recordToAuthRole);
			return createPagedResult(items, page, pageSize, totalItems);
		},

		async search(
			query: string,
			page: number,
			pageSize: number,
			tx?: TransactionContext,
		): Promise<PagedResult<AuthRole>> {
			const searchPattern = `%${query}%`;

			const [countResult] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(authRoles)
				.where(or(ilike(authRoles.name, searchPattern), ilike(authRoles.displayName, searchPattern)));
			const totalItems = Number(countResult?.count ?? 0);

			const records = await db(tx)
				.select()
				.from(authRoles)
				.where(or(ilike(authRoles.name, searchPattern), ilike(authRoles.displayName, searchPattern)))
				.limit(pageSize)
				.offset(page * pageSize)
				.orderBy(authRoles.name);

			const items = records.map(recordToAuthRole);
			return createPagedResult(items, page, pageSize, totalItems);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(authRoles);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(authRoles)
				.where(eq(authRoles.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByName(name: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(authRoles)
				.where(eq(authRoles.name, name.toLowerCase()));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewAuthRole, tx?: TransactionContext): Promise<AuthRole> {
			const now = new Date();
			const record: NewAuthRoleRecord = {
				id: entity.id,
				applicationId: entity.applicationId,
				applicationCode: entity.applicationCode,
				name: entity.name,
				displayName: entity.displayName,
				description: entity.description,
				permissions: entity.permissions as string[],
				source: entity.source,
				clientManaged: entity.clientManaged,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(authRoles).values(record);

			return this.findById(entity.id, tx) as Promise<AuthRole>;
		},

		async update(entity: AuthRole, tx?: TransactionContext): Promise<AuthRole> {
			const now = new Date();

			await db(tx)
				.update(authRoles)
				.set({
					displayName: entity.displayName,
					description: entity.description,
					permissions: entity.permissions as string[],
					clientManaged: entity.clientManaged,
					updatedAt: now,
				})
				.where(eq(authRoles.id, entity.id));

			return this.findById(entity.id, tx) as Promise<AuthRole>;
		},

		async persist(entity: NewAuthRole, tx?: TransactionContext): Promise<AuthRole> {
			const existing = await this.findById(entity.id, tx);
			if (existing) {
				// Merge with existing entity to preserve timestamps
				const merged: AuthRole = {
					...existing,
					...entity,
					createdAt: existing.createdAt,
					updatedAt: entity.updatedAt ?? existing.updatedAt,
				};
				return this.update(merged, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx).delete(authRoles).where(eq(authRoles.id, id));
			return true;
		},

		async delete(entity: AuthRole, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Create a Permission repository.
 */
export function createPermissionRepository(defaultDb: AnyDb): PermissionRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	return {
		async findById(id: string, tx?: TransactionContext): Promise<AuthPermission | undefined> {
			const [record] = await db(tx)
				.select()
				.from(authPermissions)
				.where(eq(authPermissions.id, id))
				.limit(1);

			if (!record) return undefined;

			return recordToAuthPermission(record);
		},

		async findByCode(code: string, tx?: TransactionContext): Promise<AuthPermission | undefined> {
			const [record] = await db(tx)
				.select()
				.from(authPermissions)
				.where(eq(authPermissions.code, code))
				.limit(1);

			if (!record) return undefined;

			return recordToAuthPermission(record);
		},

		async findAll(tx?: TransactionContext): Promise<AuthPermission[]> {
			const records = await db(tx).select().from(authPermissions);
			return records.map(recordToAuthPermission);
		},

		async findBySubdomain(subdomain: string, tx?: TransactionContext): Promise<AuthPermission[]> {
			const records = await db(tx)
				.select()
				.from(authPermissions)
				.where(eq(authPermissions.subdomain, subdomain));
			return records.map(recordToAuthPermission);
		},

		async findByContext(
			subdomain: string,
			context: string,
			tx?: TransactionContext,
		): Promise<AuthPermission[]> {
			const records = await db(tx)
				.select()
				.from(authPermissions)
				.where(
					and(eq(authPermissions.subdomain, subdomain), eq(authPermissions.context, context)),
				);
			return records.map(recordToAuthPermission);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(authPermissions)
				.where(eq(authPermissions.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByCode(code: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(authPermissions)
				.where(eq(authPermissions.code, code));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewAuthPermission, tx?: TransactionContext): Promise<AuthPermission> {
			const now = new Date();
			const record: NewAuthPermissionRecord = {
				id: entity.id,
				code: entity.code,
				subdomain: entity.subdomain,
				context: entity.context,
				aggregate: entity.aggregate,
				action: entity.action,
				description: entity.description,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			};

			await db(tx).insert(authPermissions).values(record);

			return this.findById(entity.id, tx) as Promise<AuthPermission>;
		},

		async update(entity: AuthPermission, tx?: TransactionContext): Promise<AuthPermission> {
			const now = new Date();

			await db(tx)
				.update(authPermissions)
				.set({
					description: entity.description,
					updatedAt: now,
				})
				.where(eq(authPermissions.id, entity.id));

			return this.findById(entity.id, tx) as Promise<AuthPermission>;
		},

		async persist(entity: NewAuthPermission, tx?: TransactionContext): Promise<AuthPermission> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as AuthPermission, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			await db(tx).delete(authPermissions).where(eq(authPermissions.id, id));
			return true;
		},
	};
}

/**
 * Convert a database record to an AuthRole.
 */
function recordToAuthRole(record: AuthRoleRecord): AuthRole {
	return {
		id: record.id,
		applicationId: record.applicationId,
		applicationCode: record.applicationCode,
		name: record.name,
		displayName: record.displayName,
		description: record.description,
		permissions: record.permissions as readonly string[],
		source: record.source as RoleSource,
		clientManaged: record.clientManaged,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}

/**
 * Convert a database record to an AuthPermission.
 */
function recordToAuthPermission(record: AuthPermissionRecord): AuthPermission {
	return {
		id: record.id,
		code: record.code,
		subdomain: record.subdomain,
		context: record.context,
		aggregate: record.aggregate,
		action: record.action,
		description: record.description,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
