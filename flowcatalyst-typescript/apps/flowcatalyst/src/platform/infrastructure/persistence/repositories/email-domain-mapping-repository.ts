/**
 * Email Domain Mapping Repository
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
  emailDomainMappings,
  emailDomainMappingAdditionalClients,
  emailDomainMappingGrantedClients,
  emailDomainMappingAllowedRoles,
} from '../schema/index.js';
import type {
  EmailDomainMapping,
  NewEmailDomainMapping,
  ScopeType,
} from '../../../domain/index.js';

export interface EmailDomainMappingRepository {
  findById(id: string, tx?: TransactionContext): Promise<EmailDomainMapping | undefined>;
  findByEmailDomain(
    emailDomain: string,
    tx?: TransactionContext,
  ): Promise<EmailDomainMapping | undefined>;
  findAll(tx?: TransactionContext): Promise<EmailDomainMapping[]>;
  exists(id: string, tx?: TransactionContext): Promise<boolean>;
  existsByEmailDomain(emailDomain: string, tx?: TransactionContext): Promise<boolean>;
  persist(entity: NewEmailDomainMapping, tx?: TransactionContext): Promise<EmailDomainMapping>;
  insert(entity: NewEmailDomainMapping, tx?: TransactionContext): Promise<void>;
  update(entity: EmailDomainMapping, tx?: TransactionContext): Promise<void>;
  deleteById(id: string, tx?: TransactionContext): Promise<boolean>;
  delete(entity: EmailDomainMapping, tx?: TransactionContext): Promise<boolean>;
}

export function createEmailDomainMappingRepository(defaultDb: PlatformDb): EmailDomainMappingRepository {
  const db = (tx?: TransactionContext): PlatformDb => (tx?.db as unknown as PlatformDb) ?? defaultDb;
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const rq = (tx?: TransactionContext): any => db(tx).query;

  /** Relational query includes for loading all child collections. */
  const withChildren = {
    additionalClients: true,
    grantedClients: true,
    allowedRoles: true,
  } as const;

  async function saveAdditionalClients(
    mappingId: string,
    clientIds: readonly string[],
    txCtx?: TransactionContext,
  ): Promise<void> {
    await db(txCtx)
      .delete(emailDomainMappingAdditionalClients)
      .where(eq(emailDomainMappingAdditionalClients.emailDomainMappingId, mappingId));

    if (clientIds.length > 0) {
      await db(txCtx).insert(emailDomainMappingAdditionalClients).values(
        clientIds.map((clientId) => ({
          emailDomainMappingId: mappingId,
          clientId,
        })),
      );
    }
  }

  async function saveGrantedClients(
    mappingId: string,
    clientIds: readonly string[],
    txCtx?: TransactionContext,
  ): Promise<void> {
    await db(txCtx)
      .delete(emailDomainMappingGrantedClients)
      .where(eq(emailDomainMappingGrantedClients.emailDomainMappingId, mappingId));

    if (clientIds.length > 0) {
      await db(txCtx).insert(emailDomainMappingGrantedClients).values(
        clientIds.map((clientId) => ({
          emailDomainMappingId: mappingId,
          clientId,
        })),
      );
    }
  }

  async function saveAllowedRoles(
    mappingId: string,
    roleIds: readonly string[],
    txCtx?: TransactionContext,
  ): Promise<void> {
    await db(txCtx)
      .delete(emailDomainMappingAllowedRoles)
      .where(eq(emailDomainMappingAllowedRoles.emailDomainMappingId, mappingId));

    if (roleIds.length > 0) {
      await db(txCtx).insert(emailDomainMappingAllowedRoles).values(
        roleIds.map((roleId) => ({
          emailDomainMappingId: mappingId,
          roleId,
        })),
      );
    }
  }

  return {
    async persist(
      entity: NewEmailDomainMapping,
      tx?: TransactionContext,
    ): Promise<EmailDomainMapping> {
      const existing = await this.findById(entity.id, tx);
      if (existing) {
        const updated: EmailDomainMapping = {
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

    async findById(id: string, tx?: TransactionContext): Promise<EmailDomainMapping | undefined> {
      const result = await rq(tx).emailDomainMappings.findFirst({
        where: { RAW: eq(emailDomainMappings.id, id) },
        with: withChildren,
      });
      if (!result) return undefined;
      return resultToEmailDomainMapping(result as EmailDomainMappingRelationalResult);
    },

    async findByEmailDomain(
      emailDomain: string,
      tx?: TransactionContext,
    ): Promise<EmailDomainMapping | undefined> {
      const result = await rq(tx).emailDomainMappings.findFirst({
        where: { RAW: eq(emailDomainMappings.emailDomain, emailDomain.toLowerCase()) },
        with: withChildren,
      });
      if (!result) return undefined;
      return resultToEmailDomainMapping(result as EmailDomainMappingRelationalResult);
    },

    async findAll(tx?: TransactionContext): Promise<EmailDomainMapping[]> {
      const results = await rq(tx).emailDomainMappings.findMany({
        orderBy: { emailDomain: 'asc' },
        with: withChildren,
      });
      return (results as EmailDomainMappingRelationalResult[]).map(resultToEmailDomainMapping);
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
      const now = new Date();
      await db(tx).insert(emailDomainMappings).values({
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
        saveAdditionalClients(entity.id, entity.additionalClientIds, tx),
        saveGrantedClients(entity.id, entity.grantedClientIds, tx),
        saveAllowedRoles(entity.id, entity.allowedRoleIds, tx),
      ]);
    },

    async update(entity: EmailDomainMapping, tx?: TransactionContext): Promise<void> {
      await db(tx)
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
        saveAdditionalClients(entity.id, entity.additionalClientIds, tx),
        saveGrantedClients(entity.id, entity.grantedClientIds, tx),
        saveAllowedRoles(entity.id, entity.allowedRoleIds, tx),
      ]);
    },

    async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
      await Promise.all([
        db(tx)
          .delete(emailDomainMappingAdditionalClients)
          .where(eq(emailDomainMappingAdditionalClients.emailDomainMappingId, id)),
        db(tx)
          .delete(emailDomainMappingGrantedClients)
          .where(eq(emailDomainMappingGrantedClients.emailDomainMappingId, id)),
        db(tx)
          .delete(emailDomainMappingAllowedRoles)
          .where(eq(emailDomainMappingAllowedRoles.emailDomainMappingId, id)),
      ]);

      const result = await db(tx)
        .delete(emailDomainMappings)
        .where(eq(emailDomainMappings.id, id));
      return (result?.length ?? 0) > 0;
    },

    async delete(entity: EmailDomainMapping, tx?: TransactionContext): Promise<boolean> {
      return this.deleteById(entity.id, tx);
    },
  };
}

/**
 * Shape returned by Drizzle relational query with children.
 */
interface EmailDomainMappingRelationalResult {
  id: string;
  emailDomain: string;
  identityProviderId: string;
  scopeType: string;
  primaryClientId: string | null;
  requiredOidcTenantId: string | null;
  syncRolesFromIdp: boolean;
  createdAt: Date;
  updatedAt: Date;
  additionalClients: { emailDomainMappingId: string; clientId: string }[];
  grantedClients: { emailDomainMappingId: string; clientId: string }[];
  allowedRoles: { emailDomainMappingId: string; roleId: string }[];
}

/**
 * Convert a relational query result to an EmailDomainMapping domain entity.
 */
function resultToEmailDomainMapping(result: EmailDomainMappingRelationalResult): EmailDomainMapping {
  return {
    id: result.id,
    emailDomain: result.emailDomain,
    identityProviderId: result.identityProviderId,
    scopeType: result.scopeType as ScopeType,
    primaryClientId: result.primaryClientId,
    additionalClientIds: result.additionalClients.map((c) => c.clientId),
    grantedClientIds: result.grantedClients.map((c) => c.clientId),
    requiredOidcTenantId: result.requiredOidcTenantId,
    allowedRoleIds: result.allowedRoles.map((r) => r.roleId),
    syncRolesFromIdp: result.syncRolesFromIdp,
    createdAt: result.createdAt,
    updatedAt: result.updatedAt,
  };
}
