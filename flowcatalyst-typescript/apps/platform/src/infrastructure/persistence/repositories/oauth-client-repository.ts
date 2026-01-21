/**
 * OAuth Client Repository
 *
 * Data access for OAuthClient entities.
 * Array fields (redirectUris, allowedOrigins, grantTypes, applicationIds)
 * are stored in separate collection tables.
 */

import { eq, sql } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

import {
	oauthClients,
	oauthClientRedirectUris,
	oauthClientAllowedOrigins,
	oauthClientGrantTypes,
	oauthClientApplicationIds,
	type OAuthClientRecord,
} from '../schema/index.js';
import {
	type OAuthClient,
	type NewOAuthClient,
	type OAuthClientType,
	type OAuthGrantType,
} from '../../../domain/index.js';

/**
 * Collection data for an OAuth client.
 */
interface OAuthClientCollections {
	redirectUris: string[];
	allowedOrigins: string[];
	grantTypes: string[];
	applicationIds: string[];
}

/**
 * OAuth client repository interface.
 */
export interface OAuthClientRepository {
	findById(id: string, tx?: TransactionContext): Promise<OAuthClient | undefined>;
	findByClientId(clientId: string, tx?: TransactionContext): Promise<OAuthClient | undefined>;
	findAll(tx?: TransactionContext): Promise<OAuthClient[]>;
	findActive(tx?: TransactionContext): Promise<OAuthClient[]>;
	count(tx?: TransactionContext): Promise<number>;
	exists(id: string, tx?: TransactionContext): Promise<boolean>;
	existsByClientId(clientId: string, tx?: TransactionContext): Promise<boolean>;
	insert(entity: NewOAuthClient, tx?: TransactionContext): Promise<OAuthClient>;
	update(entity: OAuthClient, tx?: TransactionContext): Promise<OAuthClient>;
	persist(entity: NewOAuthClient, tx?: TransactionContext): Promise<OAuthClient>;
	deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
	delete(entity: OAuthClient, tx?: TransactionContext): Promise<boolean>;
}

/**
 * Create an OAuthClient repository.
 */
export function createOAuthClientRepository(defaultDb: AnyDb): OAuthClientRepository {
	const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

	/**
	 * Fetch all collection data for an OAuth client.
	 */
	async function fetchCollections(oauthClientId: string, tx?: TransactionContext): Promise<OAuthClientCollections> {
		const [redirectUriRecords, allowedOriginRecords, grantTypeRecords, applicationIdRecords] = await Promise.all([
			db(tx).select().from(oauthClientRedirectUris).where(eq(oauthClientRedirectUris.oauthClientId, oauthClientId)),
			db(tx).select().from(oauthClientAllowedOrigins).where(eq(oauthClientAllowedOrigins.oauthClientId, oauthClientId)),
			db(tx).select().from(oauthClientGrantTypes).where(eq(oauthClientGrantTypes.oauthClientId, oauthClientId)),
			db(tx).select().from(oauthClientApplicationIds).where(eq(oauthClientApplicationIds.oauthClientId, oauthClientId)),
		]);

		return {
			redirectUris: redirectUriRecords.map((r) => r.redirectUri),
			allowedOrigins: allowedOriginRecords.map((r) => r.allowedOrigin),
			grantTypes: grantTypeRecords.map((r) => r.grantType),
			applicationIds: applicationIdRecords.map((r) => r.applicationId),
		};
	}

	/**
	 * Fetch collection data for multiple OAuth clients in batch.
	 */
	async function fetchCollectionsForClients(
		clientIds: string[],
		tx?: TransactionContext,
	): Promise<Map<string, OAuthClientCollections>> {
		if (clientIds.length === 0) return new Map();

		const [redirectUriRecords, allowedOriginRecords, grantTypeRecords, applicationIdRecords] = await Promise.all([
			db(tx).select().from(oauthClientRedirectUris).where(sql`${oauthClientRedirectUris.oauthClientId} = ANY(${clientIds})`),
			db(tx).select().from(oauthClientAllowedOrigins).where(sql`${oauthClientAllowedOrigins.oauthClientId} = ANY(${clientIds})`),
			db(tx).select().from(oauthClientGrantTypes).where(sql`${oauthClientGrantTypes.oauthClientId} = ANY(${clientIds})`),
			db(tx).select().from(oauthClientApplicationIds).where(sql`${oauthClientApplicationIds.oauthClientId} = ANY(${clientIds})`),
		]);

		const collectionsMap = new Map<string, OAuthClientCollections>();

		// Initialize empty collections for all client IDs
		for (const id of clientIds) {
			collectionsMap.set(id, {
				redirectUris: [],
				allowedOrigins: [],
				grantTypes: [],
				applicationIds: [],
			});
		}

		// Populate redirect URIs
		for (const record of redirectUriRecords) {
			const collections = collectionsMap.get(record.oauthClientId);
			if (collections) collections.redirectUris.push(record.redirectUri);
		}

		// Populate allowed origins
		for (const record of allowedOriginRecords) {
			const collections = collectionsMap.get(record.oauthClientId);
			if (collections) collections.allowedOrigins.push(record.allowedOrigin);
		}

		// Populate grant types
		for (const record of grantTypeRecords) {
			const collections = collectionsMap.get(record.oauthClientId);
			if (collections) collections.grantTypes.push(record.grantType);
		}

		// Populate application IDs
		for (const record of applicationIdRecords) {
			const collections = collectionsMap.get(record.oauthClientId);
			if (collections) collections.applicationIds.push(record.applicationId);
		}

		return collectionsMap;
	}

	/**
	 * Sync all collection tables for an OAuth client.
	 */
	async function syncCollections(
		oauthClientId: string,
		collections: OAuthClientCollections,
		tx?: TransactionContext,
	): Promise<void> {
		// Delete existing entries
		await Promise.all([
			db(tx).delete(oauthClientRedirectUris).where(eq(oauthClientRedirectUris.oauthClientId, oauthClientId)),
			db(tx).delete(oauthClientAllowedOrigins).where(eq(oauthClientAllowedOrigins.oauthClientId, oauthClientId)),
			db(tx).delete(oauthClientGrantTypes).where(eq(oauthClientGrantTypes.oauthClientId, oauthClientId)),
			db(tx).delete(oauthClientApplicationIds).where(eq(oauthClientApplicationIds.oauthClientId, oauthClientId)),
		]);

		// Insert new entries
		const insertPromises: Promise<unknown>[] = [];

		if (collections.redirectUris.length > 0) {
			insertPromises.push(
				db(tx).insert(oauthClientRedirectUris).values(
					collections.redirectUris.map((uri) => ({ oauthClientId, redirectUri: uri })),
				),
			);
		}

		if (collections.allowedOrigins.length > 0) {
			insertPromises.push(
				db(tx).insert(oauthClientAllowedOrigins).values(
					collections.allowedOrigins.map((origin) => ({ oauthClientId, allowedOrigin: origin })),
				),
			);
		}

		if (collections.grantTypes.length > 0) {
			insertPromises.push(
				db(tx).insert(oauthClientGrantTypes).values(
					collections.grantTypes.map((grantType) => ({ oauthClientId, grantType })),
				),
			);
		}

		if (collections.applicationIds.length > 0) {
			insertPromises.push(
				db(tx).insert(oauthClientApplicationIds).values(
					collections.applicationIds.map((applicationId) => ({ oauthClientId, applicationId })),
				),
			);
		}

		await Promise.all(insertPromises);
	}

	return {
		async findById(id: string, tx?: TransactionContext): Promise<OAuthClient | undefined> {
			const [record] = await db(tx)
				.select()
				.from(oauthClients)
				.where(eq(oauthClients.id, id))
				.limit(1);

			if (!record) return undefined;

			const collections = await fetchCollections(id, tx);
			return recordToOAuthClient(record, collections);
		},

		async findByClientId(clientId: string, tx?: TransactionContext): Promise<OAuthClient | undefined> {
			const [record] = await db(tx)
				.select()
				.from(oauthClients)
				.where(eq(oauthClients.clientId, clientId))
				.limit(1);

			if (!record) return undefined;

			const collections = await fetchCollections(record.id, tx);
			return recordToOAuthClient(record, collections);
		},

		async findAll(tx?: TransactionContext): Promise<OAuthClient[]> {
			const records = await db(tx).select().from(oauthClients);

			const collectionsMap = await fetchCollectionsForClients(
				records.map((r) => r.id),
				tx,
			);

			return records.map((r) =>
				recordToOAuthClient(r, collectionsMap.get(r.id) ?? emptyCollections()),
			);
		},

		async findActive(tx?: TransactionContext): Promise<OAuthClient[]> {
			const records = await db(tx)
				.select()
				.from(oauthClients)
				.where(eq(oauthClients.active, true));

			const collectionsMap = await fetchCollectionsForClients(
				records.map((r) => r.id),
				tx,
			);

			return records.map((r) =>
				recordToOAuthClient(r, collectionsMap.get(r.id) ?? emptyCollections()),
			);
		},

		async count(tx?: TransactionContext): Promise<number> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(oauthClients);
			return Number(result?.count ?? 0);
		},

		async exists(id: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(oauthClients)
				.where(eq(oauthClients.id, id));
			return Number(result?.count ?? 0) > 0;
		},

		async existsByClientId(clientId: string, tx?: TransactionContext): Promise<boolean> {
			const [result] = await db(tx)
				.select({ count: sql<number>`count(*)` })
				.from(oauthClients)
				.where(eq(oauthClients.clientId, clientId));
			return Number(result?.count ?? 0) > 0;
		},

		async insert(entity: NewOAuthClient, tx?: TransactionContext): Promise<OAuthClient> {
			const now = new Date();

			// Insert main record
			await db(tx).insert(oauthClients).values({
				id: entity.id,
				clientId: entity.clientId,
				clientName: entity.clientName,
				clientType: entity.clientType,
				clientSecretRef: entity.clientSecretRef,
				defaultScopes: entity.defaultScopes,
				pkceRequired: entity.pkceRequired,
				serviceAccountPrincipalId: entity.serviceAccountPrincipalId,
				active: entity.active,
				createdAt: entity.createdAt ?? now,
				updatedAt: entity.updatedAt ?? now,
			});

			// Insert collection data
			await syncCollections(entity.id, {
				redirectUris: [...entity.redirectUris],
				allowedOrigins: [...entity.allowedOrigins],
				grantTypes: [...entity.grantTypes],
				applicationIds: [...entity.applicationIds],
			}, tx);

			return this.findById(entity.id, tx) as Promise<OAuthClient>;
		},

		async update(entity: OAuthClient, tx?: TransactionContext): Promise<OAuthClient> {
			const now = new Date();

			// Update main record
			await db(tx)
				.update(oauthClients)
				.set({
					clientName: entity.clientName,
					clientType: entity.clientType,
					clientSecretRef: entity.clientSecretRef,
					defaultScopes: entity.defaultScopes,
					pkceRequired: entity.pkceRequired,
					serviceAccountPrincipalId: entity.serviceAccountPrincipalId,
					active: entity.active,
					updatedAt: now,
				})
				.where(eq(oauthClients.id, entity.id));

			// Sync collection data
			await syncCollections(entity.id, {
				redirectUris: [...entity.redirectUris],
				allowedOrigins: [...entity.allowedOrigins],
				grantTypes: [...entity.grantTypes],
				applicationIds: [...entity.applicationIds],
			}, tx);

			return this.findById(entity.id, tx) as Promise<OAuthClient>;
		},

		async persist(entity: NewOAuthClient, tx?: TransactionContext): Promise<OAuthClient> {
			const existing = await this.exists(entity.id, tx);
			if (existing) {
				return this.update(entity as OAuthClient, tx);
			}
			return this.insert(entity, tx);
		},

		async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
			const exists = await this.exists(id, tx);
			if (!exists) return false;
			// Collection tables are deleted automatically via CASCADE
			await db(tx).delete(oauthClients).where(eq(oauthClients.id, id));
			return true;
		},

		async delete(entity: OAuthClient, tx?: TransactionContext): Promise<boolean> {
			return this.deleteById(entity.id, tx);
		},
	};
}

/**
 * Create empty collections object.
 */
function emptyCollections(): OAuthClientCollections {
	return {
		redirectUris: [],
		allowedOrigins: [],
		grantTypes: [],
		applicationIds: [],
	};
}

/**
 * Convert a database record to an OAuthClient.
 */
function recordToOAuthClient(record: OAuthClientRecord, collections: OAuthClientCollections): OAuthClient {
	return {
		id: record.id,
		clientId: record.clientId,
		clientName: record.clientName,
		clientType: record.clientType as OAuthClientType,
		clientSecretRef: record.clientSecretRef,
		redirectUris: collections.redirectUris,
		allowedOrigins: collections.allowedOrigins,
		grantTypes: collections.grantTypes as OAuthGrantType[],
		defaultScopes: record.defaultScopes,
		pkceRequired: record.pkceRequired,
		applicationIds: collections.applicationIds,
		serviceAccountPrincipalId: record.serviceAccountPrincipalId,
		active: record.active,
		createdAt: record.createdAt,
		updatedAt: record.updatedAt,
	};
}
