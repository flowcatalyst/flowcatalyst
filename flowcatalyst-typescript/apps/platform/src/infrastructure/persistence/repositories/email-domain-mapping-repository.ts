/**
 * Email Domain Mapping Repository
 */

import { eq, sql } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	emailDomainMappings,
	emailDomainMappingAdditionalClients,
	emailDomainMappingGrantedClients,
	emailDomainMappingAllowedRoles,
	type EmailDomainMappingRecord,
} from '../schema/index.js';
import type { EmailDomainMapping, NewEmailDomainMapping, ScopeType } from '../../../domain/index.js';

export interface EmailDomainMappingRepository {
	findById(id: string, tx?: TransactionContext): Promise<EmailDomainMapping | undefined>;
	findByEmailDomain(emailDomain: string, tx?: TransactionContext): Promise<EmailDomainMapping | undefined>;
	findAll(tx?: TransactionContext): Promise<EmailDomainMapping[]>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByEmailDomain(emailDomain: string, tx?: TransactionContext): Promise<boolean>;
	persist(entity: NewEmailDomainMapping, tx?: TransactionContext): Promise<EmailDomainMapping>;
	insert(entity: NewEmailDomainMapping, tx?: TransactionContext): Promise<void>;
	update(entity: EmailDomainMapping, tx?: TransactionContext): Promise<void>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
	delete(entity: EmailDomainMapping, tx?: TransactionContext): Promise<boolean>;
}

export function createEmailDomainMappingRepository(defaultDb: AnyDb): EmailDomainMappingRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	async function loadAdditionalClients(mappingId: string, database: AnyDb): Promise<string[]> {
		const rows = await database
			.select({ clientId: emailDomainMappingAdditionalClients.clientId })
			.from(emailDomainMappingAdditionalClients)
			.where(eq(emailDomainMappingAdditionalClients.emailDomainMappingId, mappingId));
		return rows.map((r) => r.clientId);
	}

	async function loadGrantedClients(mappingId: string, database: AnyDb): Promise<string[]> {
		const rows = await database
			.select({ clientId: emailDomainMappingGrantedClients.clientId })
			.from(emailDomainMappingGrantedClients)
			.where(eq(emailDomainMappingGrantedClients.emailDomainMappingId, mappingId));
		return rows.map((r) => r.clientId);
	}

	async function loadAllowedRoles(mappingId: string, database: AnyDb): Promise<string[]> {
		const rows = await database
			.select({ roleId: emailDomainMappingAllowedRoles.roleId })
			.from(emailDomainMappingAllowedRoles)
			.where(eq(emailDomainMappingAllowedRoles.emailDomainMappingId, mappingId));
		return rows.map((r) => r.roleId);
	}

	async function saveAdditionalClients(mappingId: string, clientIds: readonly string[], database: AnyDb): Promise<void> {
		await database
			.delete(emailDomainMappingAdditionalClients)
			.where(eq(emailDomainMappingAdditionalClients.emailDomainMappingId, mappingId));

		if (clientIds.length > 0) {
			await database.insert(emailDomainMappingAdditionalClients).values(
				clientIds.map((clientId) => ({
					emailDomainMappingId: mappingId,
					clientId,
				})),
			);
		}
	}

	async function saveGrantedClients(mappingId: string, clientIds: readonly string[], database: AnyDb): Promise<void> {
		await database
			.delete(emailDomainMappingGrantedClients)
			.where(eq(emailDomainMappingGrantedClients.emailDomainMappingId, mappingId));

		if (clientIds.length > 0) {
			await database.insert(emailDomainMappingGrantedClients).values(
				clientIds.map((clientId) => ({
					emailDomainMappingId: mappingId,
					clientId,
				})),
			);
		}
	}

	async function saveAllowedRoles(mappingId: string, roleIds: readonly string[], database: AnyDb): Promise<void> {
		await database
			.delete(emailDomainMappingAllowedRoles)
			.where(eq(emailDomainMappingAllowedRoles.emailDomainMappingId, mappingId));

		if (roleIds.length > 0) {
			await database.insert(emailDomainMappingAllowedRoles).values(
				roleIds.map((roleId) => ({
					emailDomainMappingId: mappingId,
					roleId,
				})),
			);
		}
	}

	async function hydrate(record: EmailDomainMappingRecord, database: AnyDb): Promise<EmailDomainMapping> {
		const [additionalClientIds, grantedClientIds, allowedRoleIds] = await Promise.all([
			loadAdditionalClients(record.id, database),
			loadGrantedClients(record.id, database),
			loadAllowedRoles(record.id, database),
		]);

		return {
			id: record.id,
			emailDomain: record.emailDomain,
			identityProviderId: record.identityProviderId,
			scopeType: record.scopeType as ScopeType,
			primaryClientId: record.primaryClientId,
			additionalClientIds,
			grantedClientIds,
			requiredOidcTenantId: record.requiredOidcTenantId,
			allowedRoleIds,
			syncRolesFromIdp: record.syncRolesFromIdp,
			createdAt: record.createdAt,
			updatedAt: record.updatedAt,
		};
	}

	return {
		async persist(entity: NewEmailDomainMapping, tx?: TransactionContext): Promise<EmailDomainMapping> {
			const existing = await this.findById(entity.id, tx);
			if (existing) {
				const updated: EmailDomainMapping = { ...entity, createdAt: existing.createdAt, updatedAt: existing.updatedAt };
				await this.update(updated, tx);
				return (await this.findById(entity.id, tx))!;
			}
			await this.insert(entity, tx);
			return (await this.findById(entity.id, tx))!;
		},

		async findById(id: string, tx?: TransactionContext): Promise<EmailDomainMapping | undefined> {
			const database = db(tx);
			const [record] = await database
				.select()
				.from(emailDomainMappings)
				.where(eq(emailDomainMappings.id, id))
				.limit(1);

			if (!record) return undefined;
			return hydrate(record, database);
		},

		async findByEmailDomain(emailDomain: string, tx?: TransactionContext): Promise<EmailDomainMapping | undefined> {
			const database = db(tx);
			const [record] = await database
				.select()
				.from(emailDomainMappings)
				.where(eq(emailDomainMappings.emailDomain, emailDomain.toLowerCase()))
				.limit(1);

			if (!record) return undefined;
			return hydrate(record, database);
		},

		async findAll(tx?: TransactionContext): Promise<EmailDomainMapping[]> {
			const database = db(tx);
			const records = await database
				.select()
				.from(emailDomainMappings)
				.orderBy(emailDomainMappings.emailDomain);

			return Promise.all(records.map((r) => hydrate(r, database)));
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(emailDomainMappings)
				.where(eq(emailDomainMappings.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByEmailDomain(emailDomain: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(emailDomainMappings)
				.where(eq(emailDomainMappings.emailDomain, emailDomain.toLowerCase()));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewEmailDomainMapping, tx?: TransactionContext): Promise<void> {
			const database = db(tx);
			const now = new Date();
			await database.insert(emailDomainMappings).values({
				id: entity.id,
				emailDomain: entity.emailDomain,
				identityProviderId: entity.identityProviderId,
				scopeType: entity.scopeType,
				primaryClientId: entity.primaryClientId,
				requiredOidcTenantId: entity.requiredOidcTenantId,
				syncRolesFromIdp: entity.syncRolesFromIdp,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			});

			await Promise.all([
				saveAdditionalClients(entity.id, entity.additionalClientIds, database),
				saveGrantedClients(entity.id, entity.grantedClientIds, database),
				saveAllowedRoles(entity.id, entity.allowedRoleIds, database),
			]);
		},

		async update(entity: EmailDomainMapping, tx?: TransactionContext): Promise<void> {
			const database = db(tx);
			await database
				.update(emailDomainMappings)
				.set({
					identityProviderId: entity.identityProviderId,
					scopeType: entity.scopeType,
					primaryClientId: entity.primaryClientId,
					requiredOidcTenantId: entity.requiredOidcTenantId,
					syncRolesFromIdp: entity.syncRolesFromIdp,
					updatedAt: new Date(),
				})
				.where(eq(emailDomainMappings.id, entity.id));

			await Promise.all([
				saveAdditionalClients(entity.id, entity.additionalClientIds, database),
				saveGrantedClients(entity.id, entity.grantedClientIds, database),
				saveAllowedRoles(entity.id, entity.allowedRoleIds, database),
			]);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const database = db(tx);
			await Promise.all([
				database.delete(emailDomainMappingAdditionalClients).where(eq(emailDomainMappingAdditionalClients.emailDomainMappingId, id)),
				database.delete(emailDomainMappingGrantedClients).where(eq(emailDomainMappingGrantedClients.emailDomainMappingId, id)),
				database.delete(emailDomainMappingAllowedRoles).where(eq(emailDomainMappingAllowedRoles.emailDomainMappingId, id)),
			]);

			const result = await database
				.delete(emailDomainMappings)
				.where(eq(emailDomainMappings.id, id));
			return (result?.length ?? 0) > 0;
		},

		async delete(entity: EmailDomainMapping, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}
