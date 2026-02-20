/**
 * IDP Role Mapping Repository
 */

import { eq } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { TransactionContext } from "@flowcatalyst/persistence";

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import { idpRoleMappings } from "../schema/idp-role-mappings.js";
import type { IdpRoleMapping } from "../../../domain/idp-role-mapping/idp-role-mapping.js";

export interface IdpRoleMappingRepository {
	findById(
		id: string,
		tx?: TransactionContext,
	): Promise<IdpRoleMapping | undefined>;
	findByIdpRoleName(
		idpRoleName: string,
		tx?: TransactionContext,
	): Promise<IdpRoleMapping | undefined>;
	findAll(tx?: TransactionContext): Promise<IdpRoleMapping[]>;
	insert(
		entity: Omit<IdpRoleMapping, "createdAt" | "updatedAt">,
		tx?: TransactionContext,
	): Promise<void>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
}

export function createIdpRoleMappingRepository(
	defaultDb: AnyDb,
): IdpRoleMappingRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	function hydrate(
		record: typeof idpRoleMappings.$inferSelect,
	): IdpRoleMapping {
		return {
			id: record.id,
			idpRoleName: record.idpRoleName,
			internalRoleName: record.internalRoleName,
			createdAt: record.createdAt,
			updatedAt: record.updatedAt,
		};
	}

	return {
		async findById(
			id: string,
			tx?: TransactionContext,
		): Promise<IdpRoleMapping | undefined> {
			const [record] = await db(tx)
				.select()
				.from(idpRoleMappings)
				.where(eq(idpRoleMappings.id, id))
				.limit(1);

			if (!record) return undefined;
			return hydrate(record);
		},

		async findByIdpRoleName(
			idpRoleName: string,
			tx?: TransactionContext,
		): Promise<IdpRoleMapping | undefined> {
			const [record] = await db(tx)
				.select()
				.from(idpRoleMappings)
				.where(eq(idpRoleMappings.idpRoleName, idpRoleName))
				.limit(1);

			if (!record) return undefined;
			return hydrate(record);
		},

		async findAll(tx?: TransactionContext): Promise<IdpRoleMapping[]> {
			const records = await db(tx)
				.select()
				.from(idpRoleMappings)
				.orderBy(idpRoleMappings.idpRoleName);

			return records.map(hydrate);
		},

		async insert(
			entity: Omit<IdpRoleMapping, "createdAt" | "updatedAt">,
			tx?: TransactionContext,
		): Promise<void> {
			const now = new Date();
			await db(tx).insert(idpRoleMappings).values({
				id: entity.id,
				idpRoleName: entity.idpRoleName,
				internalRoleName: entity.internalRoleName,
				createdAt: now,
				updatedAt: now,
			});
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const result = await db(tx)
				.delete(idpRoleMappings)
				.where(eq(idpRoleMappings.id, id));
			return (result?.length ?? 0) > 0;
		},
	};
}
