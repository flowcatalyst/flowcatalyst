/**
 * Subscription Repository
 *
 * Data access for Subscription entities with event type binding and config sub-queries.
 *
 * Read paths use Drizzle relational queries (db.query) for efficient loading.
 * Complex queries (findActiveByEventTypeCode, findWithFilters) use query builder
 * with batch loading of children.
 * Write paths use standard insert/update with collection sync.
 */

import { eq, sql, and, or, isNull, inArray } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { Repository, TransactionContext } from '@flowcatalyst/persistence';

import type * as platformSchema from '../schema/drizzle-schema.js';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type PlatformDb = PostgresJsDatabase<typeof platformSchema, any>;

import {
  subscriptions,
  subscriptionEventTypes,
  subscriptionCustomConfigs,
  type SubscriptionRecord,
  type NewSubscriptionRecord,
} from '../schema/index.js';
import type {
  Subscription,
  NewSubscription,
  SubscriptionStatus,
  SubscriptionSource,
  DispatchMode,
  EventTypeBinding,
  ConfigEntry,
} from '../../../domain/index.js';

/**
 * Filters for subscription listing.
 */
export interface SubscriptionFilters {
  readonly clientId?: string | null | undefined;
  readonly status?: SubscriptionStatus | undefined;
  readonly source?: SubscriptionSource | undefined;
  readonly dispatchPoolId?: string | undefined;
  /** Scope filter: restrict results to these client IDs (+ anchor-level). Null = unrestricted. */
  readonly accessibleClientIds?: readonly string[] | null | undefined;
}

/**
 * Subscription repository interface.
 */
export interface SubscriptionRepository extends Repository<Subscription> {
  findByCodeAndClient(
    code: string,
    clientId: string | null,
    tx?: TransactionContext,
  ): Promise<Subscription | undefined>;
  existsByCodeAndClient(
    code: string,
    clientId: string | null,
    tx?: TransactionContext,
  ): Promise<boolean>;
  findByClientId(clientId: string, tx?: TransactionContext): Promise<Subscription[]>;
  findAnchorLevel(tx?: TransactionContext): Promise<Subscription[]>;
  findActive(tx?: TransactionContext): Promise<Subscription[]>;
  findActiveByEventTypeCode(
    eventTypeCode: string,
    clientId: string | null,
    tx?: TransactionContext,
  ): Promise<Subscription[]>;
  findByDispatchPoolId(dispatchPoolId: string, tx?: TransactionContext): Promise<Subscription[]>;
  existsByDispatchPoolId(dispatchPoolId: string, tx?: TransactionContext): Promise<boolean>;
  findWithFilters(filters: SubscriptionFilters, tx?: TransactionContext): Promise<Subscription[]>;
}

/**
 * Create a Subscription repository.
 */
export function createSubscriptionRepository(defaultDb: PlatformDb): SubscriptionRepository {
  const db = (tx?: TransactionContext): PlatformDb => (tx?.db as PlatformDb) ?? defaultDb;

  /** Relational query includes for loading all child collections. */
  const withChildren = {
    eventTypes: true,
    customConfigs: true,
  } as const;

  /**
   * Load event type bindings for multiple subscriptions (batch).
   */
  async function loadEventTypes(
    subscriptionIds: string[],
    txCtx?: TransactionContext,
  ): Promise<Map<string, EventTypeBinding[]>> {
    if (subscriptionIds.length === 0) return new Map();

    const records = await db(txCtx)
      .select()
      .from(subscriptionEventTypes)
      .where(inArray(subscriptionEventTypes.subscriptionId, subscriptionIds));

    const map = new Map<string, EventTypeBinding[]>();
    for (const r of records) {
      const list = map.get(r.subscriptionId) ?? [];
      list.push({
        eventTypeId: r.eventTypeId,
        eventTypeCode: r.eventTypeCode,
        specVersion: r.specVersion,
      });
      map.set(r.subscriptionId, list);
    }
    return map;
  }

  /**
   * Load custom config entries for multiple subscriptions (batch).
   */
  async function loadCustomConfig(
    subscriptionIds: string[],
    txCtx?: TransactionContext,
  ): Promise<Map<string, ConfigEntry[]>> {
    if (subscriptionIds.length === 0) return new Map();

    const records = await db(txCtx)
      .select()
      .from(subscriptionCustomConfigs)
      .where(inArray(subscriptionCustomConfigs.subscriptionId, subscriptionIds));

    const map = new Map<string, ConfigEntry[]>();
    for (const r of records) {
      const list = map.get(r.subscriptionId) ?? [];
      list.push({
        key: r.configKey,
        value: r.configValue,
      });
      map.set(r.subscriptionId, list);
    }
    return map;
  }

  /**
   * Batch-hydrate subscription records with their children.
   * Used by complex query-builder paths (findActiveByEventTypeCode, findWithFilters).
   */
  async function batchHydrate(
    records: SubscriptionRecord[],
    txCtx?: TransactionContext,
  ): Promise<Subscription[]> {
    if (records.length === 0) return [];

    const ids = records.map((r) => r.id);
    const [eventTypesMap, customConfigMap] = await Promise.all([
      loadEventTypes(ids, txCtx),
      loadCustomConfig(ids, txCtx),
    ]);

    return records.map((r) =>
      recordToSubscription(r, eventTypesMap.get(r.id) ?? [], customConfigMap.get(r.id) ?? []),
    );
  }

  /**
   * Save event type bindings (delete old, insert new).
   */
  async function saveEventTypes(
    subscriptionId: string,
    eventTypes: readonly EventTypeBinding[],
    txCtx?: TransactionContext,
  ): Promise<void> {
    await db(txCtx)
      .delete(subscriptionEventTypes)
      .where(eq(subscriptionEventTypes.subscriptionId, subscriptionId));

    for (const et of eventTypes) {
      await db(txCtx).insert(subscriptionEventTypes).values({
        subscriptionId,
        eventTypeId: et.eventTypeId,
        eventTypeCode: et.eventTypeCode,
        specVersion: et.specVersion,
      });
    }
  }

  /**
   * Save custom config entries (delete old, insert new).
   */
  async function saveCustomConfig(
    subscriptionId: string,
    config: readonly ConfigEntry[],
    txCtx?: TransactionContext,
  ): Promise<void> {
    await db(txCtx)
      .delete(subscriptionCustomConfigs)
      .where(eq(subscriptionCustomConfigs.subscriptionId, subscriptionId));

    for (const entry of config) {
      await db(txCtx).insert(subscriptionCustomConfigs).values({
        subscriptionId,
        configKey: entry.key,
        configValue: entry.value,
      });
    }
  }

  return {
    async findById(id: string, tx?: TransactionContext): Promise<Subscription | undefined> {
      const result = await db(tx).query.subscriptions.findFirst({
        where: { RAW: eq(subscriptions.id, id) },
        with: withChildren,
      });
      if (!result) return undefined;
      return resultToSubscription(result as SubscriptionRelationalResult);
    },

    async findByCodeAndClient(
      code: string,
      clientId: string | null,
      tx?: TransactionContext,
    ): Promise<Subscription | undefined> {
      const condition =
        clientId === null
          ? and(eq(subscriptions.code, code), isNull(subscriptions.clientId))
          : and(eq(subscriptions.code, code), eq(subscriptions.clientId, clientId));

      const result = await db(tx).query.subscriptions.findFirst({
        where: { RAW: condition },
        with: withChildren,
      });
      if (!result) return undefined;
      return resultToSubscription(result as SubscriptionRelationalResult);
    },

    async existsByCodeAndClient(
      code: string,
      clientId: string | null,
      tx?: TransactionContext,
    ): Promise<boolean> {
      const condition =
        clientId === null
          ? and(eq(subscriptions.code, code), isNull(subscriptions.clientId))
          : and(eq(subscriptions.code, code), eq(subscriptions.clientId, clientId));

      const [result] = await db(tx)
        .select({ count: sql<number>`count(*)` })
        .from(subscriptions)
        .where(condition);
      return Number(result?.count ?? 0) > 0;
    },

    async findAll(tx?: TransactionContext): Promise<Subscription[]> {
      const results = await db(tx).query.subscriptions.findMany({
        orderBy: { code: 'asc' },
        with: withChildren,
      });
      return (results as SubscriptionRelationalResult[]).map(resultToSubscription);
    },

    async findByClientId(clientId: string, tx?: TransactionContext): Promise<Subscription[]> {
      const results = await db(tx).query.subscriptions.findMany({
        where: { RAW: eq(subscriptions.clientId, clientId) },
        orderBy: { code: 'asc' },
        with: withChildren,
      });
      return (results as SubscriptionRelationalResult[]).map(resultToSubscription);
    },

    async findAnchorLevel(tx?: TransactionContext): Promise<Subscription[]> {
      const results = await db(tx).query.subscriptions.findMany({
        where: { RAW: isNull(subscriptions.clientId) },
        orderBy: { code: 'asc' },
        with: withChildren,
      });
      return (results as SubscriptionRelationalResult[]).map(resultToSubscription);
    },

    async findActive(tx?: TransactionContext): Promise<Subscription[]> {
      const results = await db(tx).query.subscriptions.findMany({
        where: { RAW: eq(subscriptions.status, 'ACTIVE') },
        orderBy: { code: 'asc' },
        with: withChildren,
      });
      return (results as SubscriptionRelationalResult[]).map(resultToSubscription);
    },

    async findActiveByEventTypeCode(
      eventTypeCode: string,
      clientId: string | null,
      tx?: TransactionContext,
    ): Promise<Subscription[]> {
      // Find active subscriptions that have a binding for this event type code
      // and whose clientId matches the event's clientId OR is null (anchor-level)
      const matchingSubIds = await db(tx)
        .select({ subscriptionId: subscriptionEventTypes.subscriptionId })
        .from(subscriptionEventTypes)
        .where(eq(subscriptionEventTypes.eventTypeCode, eventTypeCode));

      if (matchingSubIds.length === 0) return [];

      const subIds = matchingSubIds.map((r) => r.subscriptionId);

      const clientCondition =
        clientId === null
          ? isNull(subscriptions.clientId)
          : or(isNull(subscriptions.clientId), eq(subscriptions.clientId, clientId))!;

      const records = await db(tx)
        .select()
        .from(subscriptions)
        .where(
          and(
            eq(subscriptions.status, 'ACTIVE'),
            inArray(subscriptions.id, subIds),
            clientCondition,
          ),
        );

      return batchHydrate(records, tx);
    },

    async findByDispatchPoolId(
      dispatchPoolId: string,
      tx?: TransactionContext,
    ): Promise<Subscription[]> {
      const results = await db(tx).query.subscriptions.findMany({
        where: { RAW: eq(subscriptions.dispatchPoolId, dispatchPoolId) },
        orderBy: { code: 'asc' },
        with: withChildren,
      });
      return (results as SubscriptionRelationalResult[]).map(resultToSubscription);
    },

    async existsByDispatchPoolId(
      dispatchPoolId: string,
      tx?: TransactionContext,
    ): Promise<boolean> {
      const [result] = await db(tx)
        .select({ count: sql<number>`count(*)` })
        .from(subscriptions)
        .where(eq(subscriptions.dispatchPoolId, dispatchPoolId));
      return Number(result?.count ?? 0) > 0;
    },

    async findWithFilters(
      filters: SubscriptionFilters,
      tx?: TransactionContext,
    ): Promise<Subscription[]> {
      const conditions = [];

      if (filters.clientId !== undefined) {
        if (filters.clientId === null) {
          conditions.push(isNull(subscriptions.clientId));
        } else {
          conditions.push(eq(subscriptions.clientId, filters.clientId));
        }
      }

      if (filters.status) {
        conditions.push(eq(subscriptions.status, filters.status));
      }

      if (filters.source) {
        conditions.push(eq(subscriptions.source, filters.source));
      }

      if (filters.dispatchPoolId) {
        conditions.push(eq(subscriptions.dispatchPoolId, filters.dispatchPoolId));
      }

      // Scope filter: show anchor-level (null clientId) + accessible client resources
      if (filters.accessibleClientIds !== undefined && filters.accessibleClientIds !== null) {
        if (filters.accessibleClientIds.length === 0) {
          // No accessible clients - only show anchor-level
          conditions.push(isNull(subscriptions.clientId));
        } else {
          conditions.push(
            or(
              isNull(subscriptions.clientId),
              inArray(subscriptions.clientId, [...filters.accessibleClientIds]),
            )!,
          );
        }
      }

      if (conditions.length === 0) {
        return this.findAll(tx);
      }

      const records = await db(tx)
        .select()
        .from(subscriptions)
        .where(conditions.length === 1 ? conditions[0]! : and(...conditions))
        .orderBy(subscriptions.code);
      return batchHydrate(records, tx);
    },

    async count(tx?: TransactionContext): Promise<number> {
      const [result] = await db(tx)
        .select({ count: sql<number>`count(*)` })
        .from(subscriptions);
      return Number(result?.count ?? 0);
    },

    async exists(id: string, tx?: TransactionContext): Promise<boolean> {
      const [result] = await db(tx)
        .select({ count: sql<number>`count(*)` })
        .from(subscriptions)
        .where(eq(subscriptions.id, id));
      return Number(result?.count ?? 0) > 0;
    },

    async insert(entity: NewSubscription, tx?: TransactionContext): Promise<Subscription> {
      const now = new Date();
      const record: NewSubscriptionRecord = {
        id: entity.id,
        code: entity.code,
        applicationCode: entity.applicationCode,
        name: entity.name,
        description: entity.description,
        clientId: entity.clientId,
        clientIdentifier: entity.clientIdentifier,
        clientScoped: entity.clientScoped,
        target: entity.target,
        queue: entity.queue,
        source: entity.source,
        status: entity.status,
        maxAgeSeconds: entity.maxAgeSeconds,
        dispatchPoolId: entity.dispatchPoolId,
        dispatchPoolCode: entity.dispatchPoolCode,
        delaySeconds: entity.delaySeconds,
        sequence: entity.sequence,
        mode: entity.mode,
        timeoutSeconds: entity.timeoutSeconds,
        maxRetries: entity.maxRetries,
        serviceAccountId: entity.serviceAccountId,
        dataOnly: entity.dataOnly,
        createdAt: entity.createdAt ?? now,
        updatedAt: entity.updatedAt ?? now,
      };

      await db(tx).insert(subscriptions).values(record);

      // Save related entities
      await saveEventTypes(entity.id, entity.eventTypes, tx);
      await saveCustomConfig(entity.id, entity.customConfig, tx);

      return this.findById(entity.id, tx) as Promise<Subscription>;
    },

    async update(entity: Subscription, tx?: TransactionContext): Promise<Subscription> {
      const now = new Date();

      await db(tx)
        .update(subscriptions)
        .set({
          name: entity.name,
          description: entity.description,
          target: entity.target,
          queue: entity.queue,
          status: entity.status,
          maxAgeSeconds: entity.maxAgeSeconds,
          dispatchPoolId: entity.dispatchPoolId,
          dispatchPoolCode: entity.dispatchPoolCode,
          delaySeconds: entity.delaySeconds,
          sequence: entity.sequence,
          mode: entity.mode,
          timeoutSeconds: entity.timeoutSeconds,
          maxRetries: entity.maxRetries,
          serviceAccountId: entity.serviceAccountId,
          dataOnly: entity.dataOnly,
          updatedAt: now,
        })
        .where(eq(subscriptions.id, entity.id));

      // Replace related entities
      await saveEventTypes(entity.id, entity.eventTypes, tx);
      await saveCustomConfig(entity.id, entity.customConfig, tx);

      return this.findById(entity.id, tx) as Promise<Subscription>;
    },

    async persist(entity: NewSubscription, tx?: TransactionContext): Promise<Subscription> {
      const existing = await this.exists(entity.id, tx);
      if (existing) {
        return this.update(entity as Subscription, tx);
      }
      return this.insert(entity, tx);
    },

    async deleteById(id: string, tx?: TransactionContext): Promise<boolean> {
      const exists = await this.exists(id, tx);
      if (!exists) return false;

      // Delete related entities first
      await db(tx)
        .delete(subscriptionEventTypes)
        .where(eq(subscriptionEventTypes.subscriptionId, id));
      await db(tx)
        .delete(subscriptionCustomConfigs)
        .where(eq(subscriptionCustomConfigs.subscriptionId, id));
      await db(tx).delete(subscriptions).where(eq(subscriptions.id, id));
      return true;
    },

    async delete(entity: Subscription, tx?: TransactionContext): Promise<boolean> {
      return this.deleteById(entity.id, tx);
    },
  };
}

/**
 * Shape returned by Drizzle relational query with children.
 */
interface SubscriptionRelationalResult {
  id: string;
  code: string;
  applicationCode: string | null;
  name: string;
  description: string | null;
  clientId: string | null;
  clientIdentifier: string | null;
  clientScoped: boolean;
  target: string;
  queue: string | null;
  source: string;
  status: string;
  maxAgeSeconds: number;
  dispatchPoolId: string | null;
  dispatchPoolCode: string | null;
  delaySeconds: number;
  sequence: number;
  mode: string;
  timeoutSeconds: number;
  maxRetries: number;
  serviceAccountId: string | null;
  dataOnly: boolean;
  createdAt: Date;
  updatedAt: Date;
  eventTypes: {
    subscriptionId: string;
    eventTypeId: string | null;
    eventTypeCode: string;
    specVersion: string | null;
  }[];
  customConfigs: {
    subscriptionId: string;
    configKey: string;
    configValue: string;
  }[];
}

/**
 * Convert a relational query result to a Subscription domain entity.
 * Used by simple read methods that leverage db.query with { with: withChildren }.
 */
function resultToSubscription(result: SubscriptionRelationalResult): Subscription {
  return {
    id: result.id,
    code: result.code,
    applicationCode: result.applicationCode,
    name: result.name,
    description: result.description,
    clientId: result.clientId,
    clientIdentifier: result.clientIdentifier,
    clientScoped: result.clientScoped,
    eventTypes: result.eventTypes.map((et) => ({
      eventTypeId: et.eventTypeId,
      eventTypeCode: et.eventTypeCode,
      specVersion: et.specVersion,
    })),
    target: result.target,
    queue: result.queue,
    customConfig: result.customConfigs.map((c) => ({
      key: c.configKey,
      value: c.configValue,
    })),
    source: result.source as SubscriptionSource,
    status: result.status as SubscriptionStatus,
    maxAgeSeconds: result.maxAgeSeconds,
    dispatchPoolId: result.dispatchPoolId,
    dispatchPoolCode: result.dispatchPoolCode,
    delaySeconds: result.delaySeconds,
    sequence: result.sequence,
    mode: result.mode as DispatchMode,
    timeoutSeconds: result.timeoutSeconds,
    maxRetries: result.maxRetries,
    serviceAccountId: result.serviceAccountId,
    dataOnly: result.dataOnly,
    createdAt: result.createdAt,
    updatedAt: result.updatedAt,
  };
}

/**
 * Convert a database record to a Subscription domain entity.
 * Used by batch-loading paths (findActiveByEventTypeCode, findWithFilters).
 */
function recordToSubscription(
  record: SubscriptionRecord,
  eventTypes: EventTypeBinding[],
  customConfig: ConfigEntry[],
): Subscription {
  return {
    id: record.id,
    code: record.code,
    applicationCode: record.applicationCode,
    name: record.name,
    description: record.description,
    clientId: record.clientId,
    clientIdentifier: record.clientIdentifier,
    clientScoped: record.clientScoped,
    eventTypes,
    target: record.target,
    queue: record.queue,
    customConfig,
    source: record.source as SubscriptionSource,
    status: record.status as SubscriptionStatus,
    maxAgeSeconds: record.maxAgeSeconds,
    dispatchPoolId: record.dispatchPoolId,
    dispatchPoolCode: record.dispatchPoolCode,
    delaySeconds: record.delaySeconds,
    sequence: record.sequence,
    mode: record.mode as DispatchMode,
    timeoutSeconds: record.timeoutSeconds,
    maxRetries: record.maxRetries,
    serviceAccountId: record.serviceAccountId,
    dataOnly: record.dataOnly,
    createdAt: record.createdAt,
    updatedAt: record.updatedAt,
  };
}
