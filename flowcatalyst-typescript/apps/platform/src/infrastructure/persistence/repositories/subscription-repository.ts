/**
 * Subscription Repository
 *
 * Data access for Subscription entities with event type binding and config sub-queries.
 */

import { eq, sql, and, or, isNull, inArray } from 'drizzle-orm';
import type { PostgresJsDatabase } from 'drizzle-orm/postgres-js';
import type { Repository, TransactionContext } from '@flowcatalyst/persistence';

// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnyDb = PostgresJsDatabase<any>;

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
export function createSubscriptionRepository(defaultDb: AnyDb): SubscriptionRepository {
  const db = (tx?: TransactionContext): AnyDb => (tx?.db as AnyDb) ?? defaultDb;

  /**
   * Load event type bindings for a subscription.
   */
  async function loadEventTypes(
    subscriptionId: string,
    txCtx?: TransactionContext,
  ): Promise<EventTypeBinding[]> {
    const records = await db(txCtx)
      .select()
      .from(subscriptionEventTypes)
      .where(eq(subscriptionEventTypes.subscriptionId, subscriptionId));

    return records.map((r) => ({
      eventTypeId: r.eventTypeId,
      eventTypeCode: r.eventTypeCode,
      specVersion: r.specVersion,
    }));
  }

  /**
   * Load custom config entries for a subscription.
   */
  async function loadCustomConfig(
    subscriptionId: string,
    txCtx?: TransactionContext,
  ): Promise<ConfigEntry[]> {
    const records = await db(txCtx)
      .select()
      .from(subscriptionCustomConfigs)
      .where(eq(subscriptionCustomConfigs.subscriptionId, subscriptionId));

    return records.map((r) => ({
      key: r.configKey,
      value: r.configValue,
    }));
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

  /**
   * Hydrate a subscription record with its relations.
   */
  async function hydrate(
    record: SubscriptionRecord,
    txCtx?: TransactionContext,
  ): Promise<Subscription> {
    const [eventTypes, customConfig] = await Promise.all([
      loadEventTypes(record.id, txCtx),
      loadCustomConfig(record.id, txCtx),
    ]);
    return recordToSubscription(record, eventTypes, customConfig);
  }

  return {
    async findById(id: string, tx?: TransactionContext): Promise<Subscription | undefined> {
      const [record] = await db(tx)
        .select()
        .from(subscriptions)
        .where(eq(subscriptions.id, id))
        .limit(1);

      if (!record) return undefined;
      return hydrate(record, tx);
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

      const [record] = await db(tx).select().from(subscriptions).where(condition).limit(1);

      if (!record) return undefined;
      return hydrate(record, tx);
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
      const records = await db(tx).select().from(subscriptions).orderBy(subscriptions.code);
      return Promise.all(records.map((r) => hydrate(r, tx)));
    },

    async findByClientId(clientId: string, tx?: TransactionContext): Promise<Subscription[]> {
      const records = await db(tx)
        .select()
        .from(subscriptions)
        .where(eq(subscriptions.clientId, clientId))
        .orderBy(subscriptions.code);
      return Promise.all(records.map((r) => hydrate(r, tx)));
    },

    async findAnchorLevel(tx?: TransactionContext): Promise<Subscription[]> {
      const records = await db(tx)
        .select()
        .from(subscriptions)
        .where(isNull(subscriptions.clientId))
        .orderBy(subscriptions.code);
      return Promise.all(records.map((r) => hydrate(r, tx)));
    },

    async findActive(tx?: TransactionContext): Promise<Subscription[]> {
      const records = await db(tx)
        .select()
        .from(subscriptions)
        .where(eq(subscriptions.status, 'ACTIVE'))
        .orderBy(subscriptions.code);
      return Promise.all(records.map((r) => hydrate(r, tx)));
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

      return Promise.all(records.map((r) => hydrate(r, tx)));
    },

    async findByDispatchPoolId(
      dispatchPoolId: string,
      tx?: TransactionContext,
    ): Promise<Subscription[]> {
      const records = await db(tx)
        .select()
        .from(subscriptions)
        .where(eq(subscriptions.dispatchPoolId, dispatchPoolId))
        .orderBy(subscriptions.code);
      return Promise.all(records.map((r) => hydrate(r, tx)));
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
      return Promise.all(records.map((r) => hydrate(r, tx)));
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
 * Convert a database record to a Subscription domain entity.
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
