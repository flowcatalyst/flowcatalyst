/**
 * Identity Provider Repository
 *
 * Read paths use Drizzle relational queries (db.query) for efficient loading.
 * Write paths use standard insert/update with collection sync.
 */

import { eq, sql } from "drizzle-orm";
import type { PostgresJsDatabase } from "drizzle-orm/postgres-js";
import type { TransactionContext } from "@flowcatalyst/persistence";

import type * as platformSchema from "../schema/drizzle-schema.js";

type PlatformDb = PostgresJsDatabase<typeof platformSchema>;

import {
	identityProviders,
	identityProviderAllowedDomains,
} from "../schema/index.js";
import type {
	IdentityProvider,
	NewIdentityProvider,
} from "../../../domain/index.js";

export interface IdentityProviderRepository {
	findById(
		id: string,
		tx?: TransactionContext,
	): Promise<IdentityProvider | undefined>;
	findByCode(
		code: string,
		tx?: TransactionContext,
	): Promise<IdentityProvider | undefined>;
	findAll(tx?: TransactionContext): Promise<IdentityProvider[]>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByCode(code: string, tx?: TransactionContext): Promise<boolean>;
	persist(
		entity: NewIdentityProvider,
		tx?: TransactionContext,
	): Promise<IdentityProvider>;
	insert(entity: NewIdentityProvider, tx?: TransactionContext): Promise<void>;
	update(entity: IdentityProvider, tx?: TransactionContext): Promise<void>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
	delete(entity: IdentityProvider, tx?: TransactionContext): Promise<boolean>;
}

export function createIdentityProviderRepository(
	defaultDb: PlatformDb,
): IdentityProviderRepository {
	const db = (tx?: TransactionContext): PlatformDb =>
		(tx?.db as unknown as PlatformDb) ?? defaultDb;
	// eslint-disable-next-line @typescript-eslint/no-explicit-any
	const rq = (tx?: TransactionContext): any => db(tx).query;

	/** Relational query includes for loading allowed domains. */
	const withChildren = {
		allowedDomains: true,
	} as const;

	async function saveAllowedDomains(
		identityProviderId: string,
		domains: readonly string[],
		txCtx?: TransactionContext,
	): Promise<void> {
		await db(txCtx)
			.delete(identityProviderAllowedDomains)
			.where(
				eq(
					identityProviderAllowedDomains.identityProviderId,
					identityProviderId,
				),
			);

		if (domains.length > 0) {
			await db(txCtx)
				.insert(identityProviderAllowedDomains)
				.values(
					domains.map((emailDomain) => ({
						identityProviderId,
						emailDomain,
					})),
				);
		}
	}

	return {
		async persist(
			entity: NewIdentityProvider,
			tx?: TransactionContext,
		): Promise<IdentityProvider> {
			const existing = await this.findById(entity.id, tx);
			if (existing) {
				const updated: IdentityProvider = {
					...entity,
					createdAt: existing.createdAt,
					updatedAt: existing.updatedAt,
				};
				await this.update(updated, tx);
				return (await this.findById(entity.id, tx))!;
			}
			await this.insert(entity, tx);
			return (await this.findById(entity.id, tx))!;
		},

		async findById(
			id: string,
			tx?: TransactionContext,
		): Promise<IdentityProvider | undefined> {
			const result = await rq(tx).identityProviders.findFirst({
				where: { id },
				with: withChildren,
			});
			if (!result) return undefined;
			return resultToIdentityProvider(
				result as IdentityProviderRelationalResult,
			);
		},

		async findByCode(
			code: string,
			tx?: TransactionContext,
		): Promise<IdentityProvider | undefined> {
			const result = await rq(tx).identityProviders.findFirst({
				where: { code },
				with: withChildren,
			});
			if (!result) return undefined;
			return resultToIdentityProvider(
				result as IdentityProviderRelationalResult,
			);
		},

		async findAll(tx?: TransactionContext): Promise<IdentityProvider[]> {
			const results = await rq(tx).identityProviders.findMany({
				orderBy: { name: "asc" },
				with: withChildren,
			});
			return (results as IdentityProviderRelationalResult[]).map(
				resultToIdentityProvider,
			);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(identityProviders)
				.where(eq(identityProviders.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByCode(
			code: string,
			tx?: TransactionContext,
		): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(identityProviders)
				.where(eq(identityProviders.code, code));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(
			entity: NewIdentityProvider,
			tx?: TransactionContext,
		): Promise<void> {
			const now = new Date();
			await db(tx).insert(identityProviders).values({
				id: entity.id,
				code: entity.code,
				name: entity.name,
				type: entity.type,
				oidcIssuerUrl: entity.oidcIssuerUrl,
				oidcClientId: entity.oidcClientId,
				oidcClientSecretRef: entity.oidcClientSecretRef,
				oidcMultiTenant: entity.oidcMultiTenant,
				oidcIssuerPattern: entity.oidcIssuerPattern,
				createdAt: now,
				updatedAt: now,
			});

			await saveAllowedDomains(entity.id, entity.allowedEmailDomains, tx);
		},

		async update(
			entity: IdentityProvider,
			tx?: TransactionContext,
		): Promise<void> {
			await db(tx)
				.update(identityProviders)
				.set({
					name: entity.name,
					type: entity.type,
					oidcIssuerUrl: entity.oidcIssuerUrl,
					oidcClientId: entity.oidcClientId,
					oidcClientSecretRef: entity.oidcClientSecretRef,
					oidcMultiTenant: entity.oidcMultiTenant,
					oidcIssuerPattern: entity.oidcIssuerPattern,
					updatedAt: new Date(),
				})
				.where(eq(identityProviders.id, entity.id));

			await saveAllowedDomains(entity.id, entity.allowedEmailDomains, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			await db(tx)
				.delete(identityProviderAllowedDomains)
				.where(eq(identityProviderAllowedDomains.identityProviderId, id));

			const result = await db(tx)
				.delete(identityProviders)
				.where(eq(identityProviders.id, id));
			return (result?.length ?? 0) > 0;
		},

		async delete(
			entity: IdentityProvider,
			tx?: TransactionContext,
		): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Shape returned by Drizzle relational query with allowed domains.
 */
interface IdentityProviderRelationalResult {
	id: string;
	code: string;
	name: string;
	type: string;
	oidcIssuerUrl: string | null;
	oidcClientId: string | null;
	oidcClientSecretRef: string | null;
	oidcMultiTenant: boolean;
	oidcIssuerPattern: string | null;
	createdAt: Date;
	updatedAt: Date;
	allowedDomains: { identityProviderId: string; emailDomain: string }[];
}

/**
 * Convert a relational query result to an IdentityProvider domain entity.
 */
function resultToIdentityProvider(
	result: IdentityProviderRelationalResult,
): IdentityProvider {
	return {
		id: result.id,
		code: result.code,
		name: result.name,
		type: result.type as IdentityProvider["type"],
		oidcIssuerUrl: result.oidcIssuerUrl,
		oidcClientId: result.oidcClientId,
		oidcClientSecretRef: result.oidcClientSecretRef,
		oidcMultiTenant: result.oidcMultiTenant,
		oidcIssuerPattern: result.oidcIssuerPattern,
		allowedEmailDomains: result.allowedDomains.map((d) => d.emailDomain),
		createdAt: result.createdAt,
		updatedAt: result.updatedAt,
	};
}
