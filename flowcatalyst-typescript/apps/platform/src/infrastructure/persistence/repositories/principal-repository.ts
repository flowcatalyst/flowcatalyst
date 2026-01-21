/**
 * Principal Repository
 *
 * Data access for Principal aggregates.
 * Principal data is stored in the principals table with flattened user identity columns.
 * Roles are stored in the separate principal_roles junction table.
 */

import { eq, and, sql } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import {
	type PaginatedRepository,
	type PagedResult,
	type TransactionContext,
	createPagedResult,
} from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import { principals, principalRoles, type PrincipalRecord, type PrincipalRoleRecord } from '../schema/index.js';

import {
	type Principal,
	type NewPrincipal,
	type PrincipalType,
	type UserIdentity,
	type RoleAssignment,
	type UserScope,
	type IdpType,
} from '../../../domain/index.js';

/**
 * Principal repository interface.
 */
export interface PrincipalRepository extends PaginatedRepository<Principal> {
	findByEmail(email: string, tx?: TransactionContext): Promise<Principal | undefined>;
	findByClientId(clientId: string, tx?: TransactionContext): Promise<Principal[]>;
	findByType(type: PrincipalType, tx?: TransactionContext): Promise<Principal[]>;
	findActiveUsersByClientId(clientId: string, tx?: TransactionContext): Promise<Principal[]>;
	existsByEmail(email: string, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Create a Principal repository.
 */
export function createPrincipalRepository(defaultDb: AnyDb): PrincipalRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	/**
	 * Fetch roles for a principal from the junction table.
	 */
	async function fetchRoles(principalId: string, tx?: TransactionContext): Promise<RoleAssignment[]> {
		const roleRecords = await db(tx)
			.select()
			.from(principalRoles)
			.where(eq(principalRoles.principalId, principalId));

		return roleRecords.map(roleRecordToAssignment);
	}

	/**
	 * Fetch roles for multiple principals in a single query.
	 */
	async function fetchRolesForPrincipals(
		principalIds: string[],
		tx?: TransactionContext,
	): Promise<Map<string, RoleAssignment[]>> {
		if (principalIds.length === 0) return new Map();

		const roleRecords = await db(tx)
			.select()
			.from(principalRoles)
			.where(sql`${principalRoles.principalId} = ANY(${principalIds})`);

		const rolesMap = new Map<string, RoleAssignment[]>();
		for (const record of roleRecords) {
			const existing = rolesMap.get(record.principalId) ?? [];
			existing.push(roleRecordToAssignment(record));
			rolesMap.set(record.principalId, existing);
		}

		return rolesMap;
	}

	/**
	 * Sync roles for a principal - delete existing and insert new.
	 */
	async function syncRoles(
		principalId: string,
		roles: readonly RoleAssignment[],
		tx?: TransactionContext,
	): Promise<void> {
		// Delete existing roles
		await db(tx).delete(principalRoles).where(eq(principalRoles.principalId, principalId));

		// Insert new roles
		if (roles.length > 0) {
			await db(tx).insert(principalRoles).values(
				roles.map((role) => ({
					principalId,
					roleName: role.roleName,
					assignmentSource: role.assignmentSource,
					assignedAt: role.assignedAt,
				})),
			);
		}
	}

	return {
		async findById(id: string, tx?: TransactionContext): Promise<Principal | undefined> {
			const [record] = await db(tx)
				.select()
				.from(principals)
				.where(eq(principals.id, id))
				.limit(1);

			if (!record) return undefined;

			const roles = await fetchRoles(id, tx);
			return recordToPrincipal(record, roles);
		},

		async findByEmail(email: string, tx?: TransactionContext): Promise<Principal | undefined> {
			const [record] = await db(tx)
				.select()
				.from(principals)
				.where(eq(principals.email, email.toLowerCase()))
				.limit(1);

			if (!record) return undefined;

			const roles = await fetchRoles(record.id, tx);
			return recordToPrincipal(record, roles);
		},

		async findByClientId(clientId: string, tx?: TransactionContext): Promise<Principal[]> {
			const records = await db(tx)
				.select()
				.from(principals)
				.where(eq(principals.clientId, clientId));

			const rolesMap = await fetchRolesForPrincipals(
				records.map((r) => r.id),
				tx,
			);

			return records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? []));
		},

		async findByType(type: PrincipalType, tx?: TransactionContext): Promise<Principal[]> {
			const records = await db(tx)
				.select()
				.from(principals)
				.where(eq(principals.type, type));

			const rolesMap = await fetchRolesForPrincipals(
				records.map((r) => r.id),
				tx,
			);

			return records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? []));
		},

		async findActiveUsersByClientId(clientId: string, tx?: TransactionContext): Promise<Principal[]> {
			const records = await db(tx)
				.select()
				.from(principals)
				.where(
					and(
						eq(principals.clientId, clientId),
						eq(principals.type, 'USER'),
						eq(principals.active, true),
					),
				);

			const rolesMap = await fetchRolesForPrincipals(
				records.map((r) => r.id),
				tx,
			);

			return records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? []));
		},

		async findAll(tx?: TransactionContext): Promise<Principal[]> {
			const records = await db(tx).select().from(principals);

			const rolesMap = await fetchRolesForPrincipals(
				records.map((r) => r.id),
				tx,
			);

			return records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? []));
		},

		async findPaged(page: number, pageSize: number, tx?: TransactionContext): Promise<PagedResult<Principal>> {
			const [countResult] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(principals);
			const totalItems = Number(countResult?.count ?? 0);

			const records = await db(tx)
				.select()
				.from(principals)
				.limit(pageSize)
				.offset(page * pageSize)
				.orderBy(principals.createdAt);

			const rolesMap = await fetchRolesForPrincipals(
				records.map((r) => r.id),
				tx,
			);

			const items = records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? []));
			return createPagedResult(items, page, pageSize, totalItems);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(principals);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(principals)
				.where(eq(principals.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByEmail(email: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(principals)
				.where(eq(principals.email, email.toLowerCase()));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewPrincipal, tx?: TransactionContext): Promise<Principal> {
			const now = new Date();

			// Insert principal
			await db(tx).insert(principals).values({
				id: entity.id,
				type: entity.type,
				scope: entity.scope,
				clientId: entity.clientId,
				applicationId: entity.applicationId,
				name: entity.name,
				active: entity.active,
				// Flattened user identity fields
				email: entity.userIdentity?.email ?? null,
				emailDomain: entity.userIdentity?.emailDomain ?? null,
				idpType: entity.userIdentity?.idpType ?? null,
				externalIdpId: entity.userIdentity?.externalIdpId ?? null,
				passwordHash: entity.userIdentity?.passwordHash ?? null,
				lastLoginAt: entity.userIdentity?.lastLoginAt ?? null,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			});

			// Insert roles into junction table
			await syncRoles(entity.id, entity.roles, tx);

			return this.findById(entity.id, tx) as Promise<Principal>;
		},

		async update(entity: Principal, tx?: TransactionContext): Promise<Principal> {
			const now = new Date();

			// Update principal
			await db(tx)
				.update(principals)
				.set({
					type: entity.type,
					scope: entity.scope,
					clientId: entity.clientId,
					applicationId: entity.applicationId,
					name: entity.name,
					active: entity.active,
					// Flattened user identity fields
					email: entity.userIdentity?.email ?? null,
					emailDomain: entity.userIdentity?.emailDomain ?? null,
					idpType: entity.userIdentity?.idpType ?? null,
					externalIdpId: entity.userIdentity?.externalIdpId ?? null,
					passwordHash: entity.userIdentity?.passwordHash ?? null,
					lastLoginAt: entity.userIdentity?.lastLoginAt ?? null,
					updatedAt: now,
				})
				.where(eq(principals.id, entity.id));

			// Sync roles in junction table
			await syncRoles(entity.id, entity.roles, tx);

			return this.findById(entity.id, tx) as Promise<Principal>;
		},

		async persist(entity: NewPrincipal, tx?: TransactionContext): Promise<Principal> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as Principal, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;

			// Roles are deleted automatically via CASCADE
			await db(tx).delete(principals).where(eq(principals.id, id));
			return true;
		},

		async delete(entity: Principal, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Convert a database record to a Principal domain object.
 */
function recordToPrincipal(record: PrincipalRecord, roles: RoleAssignment[]): Principal {
	// Build user identity from flat columns if email is present
	let userIdentity: UserIdentity | null = null;
	if (record.email) {
		userIdentity = {
			email: record.email,
			emailDomain: record.emailDomain!,
			idpType: record.idpType as IdpType,
			externalIdpId: record.externalIdpId,
			passwordHash: record.passwordHash,
			lastLoginAt: record.lastLoginAt,
		};
	}

	return {
		id: record.id,
		type: record.type as PrincipalType,
		scope: record.scope as UserScope | null,
		clientId: record.clientId,
		applicationId: record.applicationId,
		name: record.name,
		active: record.active,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
		userIdentity,
		roles,
	};
}

/**
 * Convert a role record from the junction table to a domain RoleAssignment.
 */
function roleRecordToAssignment(record: PrincipalRoleRecord): RoleAssignment {
	return {
		roleName: record.roleName,
		assignmentSource: record.assignmentSource ?? 'MANUAL',
		assignedAt: record.assignedAt,
	};
}
