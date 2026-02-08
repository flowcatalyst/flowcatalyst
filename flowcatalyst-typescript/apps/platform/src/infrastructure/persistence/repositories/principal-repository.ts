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
import { serviceAccounts } from '../schema/service-accounts.js';
import { principalApplicationAccess } from '../schema/principal-application-access.js';

import {
	type Principal,
	type NewPrincipal,
	type PrincipalType,
	type UserIdentity,
	type RoleAssignment,
	type UserScope,
	type IdpType,
	type ServiceAccountData,
	type WebhookAuthType,
	type SignatureAlgorithm,
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
	/** Find a SERVICE principal by its embedded service account code. */
	findByServiceAccountCode(code: string, tx?: TransactionContext): Promise<Principal | undefined>;
	/** Check if a service account code already exists. */
	existsByServiceAccountCode(code: string, tx?: TransactionContext): Promise<boolean>;
	/** Set application access for a principal (declarative replace). */
	setApplicationAccess(principalId: string, applicationIds: string[], tx?: TransactionContext): Promise<void>;
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
	 * Fetch service account data for multiple principals with service_account_id set.
	 */
	async function fetchServiceAccountsForPrincipals(
		records: PrincipalRecord[],
		tx?: TransactionContext,
	): Promise<Map<string, ServiceAccountData>> {
		const saIds = records
			.filter((r) => r.serviceAccountId !== null)
			.map((r) => r.serviceAccountId!);

		if (saIds.length === 0) return new Map();

		const saRecords = await db(tx)
			.select()
			.from(serviceAccounts)
			.where(sql`${serviceAccounts.id} = ANY(${saIds})`);

		const saMap = new Map<string, ServiceAccountData>();
		for (const sa of saRecords) {
			saMap.set(sa.id, recordToServiceAccountData(sa));
		}
		return saMap;
	}

	/**
	 * Fetch accessible application IDs for a principal.
	 */
	async function fetchApplicationAccess(principalId: string, tx?: TransactionContext): Promise<string[]> {
		const records = await db(tx)
			.select({ applicationId: principalApplicationAccess.applicationId })
			.from(principalApplicationAccess)
			.where(eq(principalApplicationAccess.principalId, principalId));
		return records.map((r) => r.applicationId);
	}

	/**
	 * Fetch application access for multiple principals in a single query.
	 */
	async function fetchApplicationAccessForPrincipals(
		principalIds: string[],
		tx?: TransactionContext,
	): Promise<Map<string, string[]>> {
		if (principalIds.length === 0) return new Map();

		const records = await db(tx)
			.select()
			.from(principalApplicationAccess)
			.where(sql`${principalApplicationAccess.principalId} = ANY(${principalIds})`);

		const accessMap = new Map<string, string[]>();
		for (const record of records) {
			const existing = accessMap.get(record.principalId) ?? [];
			existing.push(record.applicationId);
			accessMap.set(record.principalId, existing);
		}
		return accessMap;
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
			const appAccess = await fetchApplicationAccess(id, tx);
			const saRecord = record.serviceAccountId
				? (await db(tx).select().from(serviceAccounts).where(eq(serviceAccounts.id, record.serviceAccountId)).limit(1))[0]
				: undefined;
			return recordToPrincipal(record, roles, saRecord, undefined, appAccess);
		},

		async findByEmail(email: string, tx?: TransactionContext): Promise<Principal | undefined> {
			const [record] = await db(tx)
				.select()
				.from(principals)
				.where(eq(principals.email, email.toLowerCase()))
				.limit(1);

			if (!record) return undefined;

			const roles = await fetchRoles(record.id, tx);
			const appAccess = await fetchApplicationAccess(record.id, tx);
			return recordToPrincipal(record, roles, undefined, undefined, appAccess);
		},

		async findByClientId(clientId: string, tx?: TransactionContext): Promise<Principal[]> {
			const records = await db(tx)
				.select()
				.from(principals)
				.where(eq(principals.clientId, clientId));

			const ids = records.map((r) => r.id);
			const rolesMap = await fetchRolesForPrincipals(ids, tx);
			const saMap = await fetchServiceAccountsForPrincipals(records, tx);
			const appAccessMap = await fetchApplicationAccessForPrincipals(ids, tx);

			return records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? [], undefined, saMap.get(r.serviceAccountId!), appAccessMap.get(r.id)));
		},

		async findByType(type: PrincipalType, tx?: TransactionContext): Promise<Principal[]> {
			const records = await db(tx)
				.select()
				.from(principals)
				.where(eq(principals.type, type));

			const ids = records.map((r) => r.id);
			const rolesMap = await fetchRolesForPrincipals(ids, tx);
			const saMap = await fetchServiceAccountsForPrincipals(records, tx);
			const appAccessMap = await fetchApplicationAccessForPrincipals(ids, tx);

			return records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? [], undefined, saMap.get(r.serviceAccountId!), appAccessMap.get(r.id)));
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

			const ids = records.map((r) => r.id);
			const rolesMap = await fetchRolesForPrincipals(ids, tx);
			const appAccessMap = await fetchApplicationAccessForPrincipals(ids, tx);

			return records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? [], undefined, undefined, appAccessMap.get(r.id)));
		},

		async findByServiceAccountCode(code: string, tx?: TransactionContext): Promise<Principal | undefined> {
			// Query via service_accounts table (index scan on code)
			const [saRecord] = await db(tx)
				.select()
				.from(serviceAccounts)
				.where(eq(serviceAccounts.code, code))
				.limit(1);

			if (!saRecord) return undefined;

			const [record] = await db(tx)
				.select()
				.from(principals)
				.where(eq(principals.id, saRecord.id))
				.limit(1);

			if (!record) return undefined;

			const roles = await fetchRoles(record.id, tx);
			const appAccess = await fetchApplicationAccess(record.id, tx);
			return recordToPrincipal(record, roles, saRecord, undefined, appAccess);
		},

		async existsByServiceAccountCode(code: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(serviceAccounts)
				.where(eq(serviceAccounts.code, code));
			return Number(result?.count ?? 0) > 0;
		},

		async findAll(tx?: TransactionContext): Promise<Principal[]> {
			const records = await db(tx).select().from(principals);

			const ids = records.map((r) => r.id);
			const rolesMap = await fetchRolesForPrincipals(ids, tx);
			const saMap = await fetchServiceAccountsForPrincipals(records, tx);
			const appAccessMap = await fetchApplicationAccessForPrincipals(ids, tx);

			return records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? [], undefined, saMap.get(r.serviceAccountId!), appAccessMap.get(r.id)));
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

			const ids = records.map((r) => r.id);
			const rolesMap = await fetchRolesForPrincipals(ids, tx);
			const saMap = await fetchServiceAccountsForPrincipals(records, tx);
			const appAccessMap = await fetchApplicationAccessForPrincipals(ids, tx);

			const items = records.map((r) => recordToPrincipal(r, rolesMap.get(r.id) ?? [], undefined, saMap.get(r.serviceAccountId!), appAccessMap.get(r.id)));
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

			// If SERVICE type, insert service account record first
			if (entity.type === 'SERVICE' && entity.serviceAccount) {
				await db(tx).insert(serviceAccounts).values({
					id: entity.id,
					code: entity.serviceAccount.code,
					name: entity.name,
					description: entity.serviceAccount.description,
					applicationId: entity.applicationId,
					active: entity.active,
					whAuthType: entity.serviceAccount.whAuthType,
					whAuthTokenRef: entity.serviceAccount.whAuthTokenRef,
					whSigningSecretRef: entity.serviceAccount.whSigningSecretRef,
					whSigningAlgorithm: entity.serviceAccount.whSigningAlgorithm,
					whCredentialsCreatedAt: entity.serviceAccount.whCredentialsCreatedAt,
					whCredentialsRegeneratedAt: entity.serviceAccount.whCredentialsRegeneratedAt,
					lastUsedAt: entity.serviceAccount.lastUsedAt,
					createdAt: entity.createdAt ?? now,
					updatedAt: entity.updatedAt ?? now,
				});
			}

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
				// FK to service_accounts
				serviceAccountId: entity.type === 'SERVICE' ? entity.id : null,
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

			// If SERVICE type with service account data, update the service_accounts table
			if (entity.type === 'SERVICE' && entity.serviceAccount) {
				await db(tx)
					.update(serviceAccounts)
					.set({
						code: entity.serviceAccount.code,
						name: entity.name,
						description: entity.serviceAccount.description,
						applicationId: entity.applicationId,
						active: entity.active,
						whAuthType: entity.serviceAccount.whAuthType,
						whAuthTokenRef: entity.serviceAccount.whAuthTokenRef,
						whSigningSecretRef: entity.serviceAccount.whSigningSecretRef,
						whSigningAlgorithm: entity.serviceAccount.whSigningAlgorithm,
						whCredentialsCreatedAt: entity.serviceAccount.whCredentialsCreatedAt,
						whCredentialsRegeneratedAt: entity.serviceAccount.whCredentialsRegeneratedAt,
						lastUsedAt: entity.serviceAccount.lastUsedAt,
						updatedAt: now,
					})
					.where(eq(serviceAccounts.id, entity.id));
			}

			// Sync roles in junction table
			await syncRoles(entity.id, entity.roles, tx);

			// Sync application access junction table
			if (entity.accessibleApplicationIds.length > 0 || entity.type === 'USER') {
				await this.setApplicationAccess(entity.id, [...entity.accessibleApplicationIds], tx);
			}

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

			// Delete application access
			await db(tx).delete(principalApplicationAccess).where(eq(principalApplicationAccess.principalId, id));
			// Delete service account if exists
			await db(tx).delete(serviceAccounts).where(eq(serviceAccounts.id, id));
			// Roles are deleted automatically via CASCADE
			await db(tx).delete(principals).where(eq(principals.id, id));
			return true;
		},

		async delete(entity: Principal, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},

		async setApplicationAccess(principalId: string, applicationIds: string[], tx?: TransactionContext): Promise<void> {
			// Delete all existing access
			await db(tx).delete(principalApplicationAccess).where(eq(principalApplicationAccess.principalId, principalId));

			// Insert new access rows
			if (applicationIds.length > 0) {
				await db(tx).insert(principalApplicationAccess).values(
					applicationIds.map((appId) => ({
						principalId,
						applicationId: appId,
						grantedAt: new Date(),
					})),
				);
			}
		},
	};
}

/**
 * Convert a database record to a Principal domain object.
 * Accepts an optional service account record from the separate table,
 * or a pre-loaded ServiceAccountData from batch loading.
 */
function recordToPrincipal(
	record: PrincipalRecord,
	roles: RoleAssignment[],
	saRecord?: import('../schema/service-accounts.js').ServiceAccountRecord | undefined,
	preloadedSa?: ServiceAccountData | undefined,
	accessibleApplicationIds?: string[] | undefined,
): Principal {
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

	// Build service account data from the separate table record or pre-loaded data
	let serviceAccount: ServiceAccountData | null = null;
	if (preloadedSa) {
		serviceAccount = preloadedSa;
	} else if (saRecord) {
		serviceAccount = recordToServiceAccountData(saRecord);
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
		serviceAccount,
		roles,
		accessibleApplicationIds: accessibleApplicationIds ?? [],
	};
}

/**
 * Convert a ServiceAccountRecord to domain ServiceAccountData.
 */
function recordToServiceAccountData(sa: import('../schema/service-accounts.js').ServiceAccountRecord): ServiceAccountData {
	return {
		code: sa.code,
		description: sa.description,
		whAuthType: (sa.whAuthType ?? 'BEARER_TOKEN') as WebhookAuthType,
		whAuthTokenRef: sa.whAuthTokenRef,
		whSigningSecretRef: sa.whSigningSecretRef,
		whSigningAlgorithm: (sa.whSigningAlgorithm ?? 'HMAC_SHA256') as SignatureAlgorithm,
		whCredentialsCreatedAt: sa.whCredentialsCreatedAt,
		whCredentialsRegeneratedAt: sa.whCredentialsRegeneratedAt,
		lastUsedAt: sa.lastUsedAt,
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
