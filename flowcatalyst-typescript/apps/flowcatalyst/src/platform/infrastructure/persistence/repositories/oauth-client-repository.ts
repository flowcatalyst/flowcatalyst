/**
 * OAuth Client Repository
 *
 * Data access for OAuthClient entities.
 * Array fields (redirectUris, allowedOrigins, grantTypes, applicationIds)
 * are stored in separate collection tables.
 *
 * Read paths use Drizzle relational queries (db.query) for efficient loading.
 * Write paths use standard insert/update with collection sync.
 */

import { eq, sql } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { TransactionContext } from '@flowcatalyst/persistence';

import type * as platformSchema from '../schema/drizzle-schema.js';

type PlatformDb = PostgresJsDatabase<typeof platformSchema>;

import {
  oauthClients,
  oauthClientRedirectUris,
  oauthClientAllowedOrigins,
  oauthClientGrantTypes,
  oauthClientApplicationIds,
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
export function createOAuthClientRepository(defaultDb: PlatformDb): OAuthClientRepository {
  const db = (tx?: TransactionContext): PlatformDb => (tx?.db as unknown as PlatformDb) ?? defaultDb;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const rq = (tx?: TransactionContext): any => db(tx).query;

  /** Relational query includes for loading all child collections. */
  const withCollections = {
    redirectUris: true,
    allowedOrigins: true,
    grantTypes: true,
    applicationIds: true,
  } as const;

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
      db(tx)
        .delete(oauthClientRedirectUris)
        .where(eq(oauthClientRedirectUris.oauthClientId, oauthClientId)),
      db(tx)
        .delete(oauthClientAllowedOrigins)
        .where(eq(oauthClientAllowedOrigins.oauthClientId, oauthClientId)),
      db(tx)
        .delete(oauthClientGrantTypes)
        .where(eq(oauthClientGrantTypes.oauthClientId, oauthClientId)),
      db(tx)
        .delete(oauthClientApplicationIds)
        .where(eq(oauthClientApplicationIds.oauthClientId, oauthClientId)),
    ]);

    // Insert new entries
    const insertPromises: Promise<unknown>[] = [];

    if (collections.redirectUris.length > 0) {
      insertPromises.push(
        db(tx)
          .insert(oauthClientRedirectUris)
          .values(collections.redirectUris.map((uri) => ({ oauthClientId, redirectUri: uri }))),
      );
    }

    if (collections.allowedOrigins.length > 0) {
      insertPromises.push(
        db(tx)
          .insert(oauthClientAllowedOrigins)
          .values(
            collections.allowedOrigins.map((origin) => ({ oauthClientId, allowedOrigin: origin })),
          ),
      );
    }

    if (collections.grantTypes.length > 0) {
      insertPromises.push(
        db(tx)
          .insert(oauthClientGrantTypes)
          .values(collections.grantTypes.map((grantType) => ({ oauthClientId, grantType }))),
      );
    }

    if (collections.applicationIds.length > 0) {
      insertPromises.push(
        db(tx)
          .insert(oauthClientApplicationIds)
          .values(
            collections.applicationIds.map((applicationId) => ({ oauthClientId, applicationId })),
          ),
      );
    }

    await Promise.all(insertPromises);
  }

  return {
    async findById(id: string, tx?: TransactionContext): Promise<OAuthClient | undefined> {
      const result = await rq(tx).oauthClients.findFirst({
        where: { id },
        with: withCollections,
      });
      if (!result) return undefined;
      return resultToOAuthClient(result as OAuthClientRelationalResult);
    },

    async findByClientId(
      clientId: string,
      tx?: TransactionContext,
    ): Promise<OAuthClient | undefined> {
      const result = await rq(tx).oauthClients.findFirst({
        where: { clientId },
        with: withCollections,
      });
      if (!result) return undefined;
      return resultToOAuthClient(result as OAuthClientRelationalResult);
    },

    async findAll(tx?: TransactionContext): Promise<OAuthClient[]> {
      const results = await rq(tx).oauthClients.findMany({
        with: withCollections,
      });
      return (results as OAuthClientRelationalResult[]).map(resultToOAuthClient);
    },

    async findActive(tx?: TransactionContext): Promise<OAuthClient[]> {
      const results = await rq(tx).oauthClients.findMany({
        where: { active: true },
        with: withCollections,
      });
      return (results as OAuthClientRelationalResult[]).map(resultToOAuthClient);
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
      await db(tx)
        .insert(oauthClients)
        .values({
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
      await syncCollections(
        entity.id,
        {
          redirectUris: [...entity.redirectUris],
          allowedOrigins: [...entity.allowedOrigins],
          grantTypes: [...entity.grantTypes],
          applicationIds: [...entity.applicationIds],
        },
        tx,
      );

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
      await syncCollections(
        entity.id,
        {
          redirectUris: [...entity.redirectUris],
          allowedOrigins: [...entity.allowedOrigins],
          grantTypes: [...entity.grantTypes],
          applicationIds: [...entity.applicationIds],
        },
        tx,
      );

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
 * Shape returned by Drizzle relational query with collections.
 */
interface OAuthClientRelationalResult {
  id: string;
  clientId: string;
  clientName: string;
  clientType: string;
  clientSecretRef: string | null;
  defaultScopes: string | null;
  pkceRequired: boolean;
  serviceAccountPrincipalId: string | null;
  active: boolean;
  createdAt: Date;
  updatedAt: Date;
  redirectUris: { oauthClientId: string; redirectUri: string }[];
  allowedOrigins: { oauthClientId: string; allowedOrigin: string }[];
  grantTypes: { oauthClientId: string; grantType: string }[];
  applicationIds: { oauthClientId: string; applicationId: string }[];
}

/**
 * Convert a relational query result to an OAuthClient domain entity.
 */
function resultToOAuthClient(result: OAuthClientRelationalResult): OAuthClient {
  return {
    id: result.id,
    clientId: result.clientId,
    clientName: result.clientName,
    clientType: result.clientType as OAuthClientType,
    clientSecretRef: result.clientSecretRef,
    redirectUris: result.redirectUris.map((r) => r.redirectUri),
    allowedOrigins: result.allowedOrigins.map((r) => r.allowedOrigin),
    grantTypes: result.grantTypes.map((r) => r.grantType) as OAuthGrantType[],
    defaultScopes: result.defaultScopes,
    pkceRequired: result.pkceRequired,
    applicationIds: result.applicationIds.map((r) => r.applicationId),
    serviceAccountPrincipalId: result.serviceAccountPrincipalId,
    active: result.active,
    createdAt: result.createdAt,
    updatedAt: result.updatedAt,
  };
}
