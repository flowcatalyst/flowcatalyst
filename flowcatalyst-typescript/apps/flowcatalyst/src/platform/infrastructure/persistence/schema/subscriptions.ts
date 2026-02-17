/**
 * Subscription Schema
 *
 * Table definitions for subscription management.
 * Subscriptions define how events are dispatched to target endpoints.
 */

import {
  pgTable,
  varchar,
  integer,
  boolean,
  text,
  index,
  uniqueIndex,
  serial,
} from 'drizzle-orm/pg-core';
import { baseEntityColumns, tsidColumn } from '@flowcatalyst/persistence';

/**
 * Subscriptions - main table defining event-to-endpoint bindings.
 */
export const subscriptions = pgTable(
  'msg_subscriptions',
  {
    ...baseEntityColumns,
    code: varchar('code', { length: 100 }).notNull(),
    applicationCode: varchar('application_code', { length: 100 }),
    name: varchar('name', { length: 255 }).notNull(),
    description: text('description'),
    clientId: tsidColumn('client_id'),
    clientIdentifier: varchar('client_identifier', { length: 100 }),
    clientScoped: boolean('client_scoped').notNull().default(false),
    target: varchar('target', { length: 500 }).notNull(),
    queue: varchar('queue', { length: 255 }),
    source: varchar('source', { length: 20 }).notNull().default('UI'),
    status: varchar('status', { length: 20 }).notNull().default('ACTIVE'),
    maxAgeSeconds: integer('max_age_seconds').notNull().default(86400),
    dispatchPoolId: tsidColumn('dispatch_pool_id'),
    dispatchPoolCode: varchar('dispatch_pool_code', { length: 100 }),
    delaySeconds: integer('delay_seconds').notNull().default(0),
    sequence: integer('sequence').notNull().default(99),
    mode: varchar('mode', { length: 20 }).notNull().default('IMMEDIATE'),
    timeoutSeconds: integer('timeout_seconds').notNull().default(30),
    maxRetries: integer('max_retries').notNull().default(3),
    serviceAccountId: tsidColumn('service_account_id'),
    dataOnly: boolean('data_only').notNull().default(true),
  },
  (table) => ({
    codeClientIdx: uniqueIndex('idx_msg_subscriptions_code_client').on(table.code, table.clientId),
    statusIdx: index('idx_msg_subscriptions_status').on(table.status),
    clientIdIdx: index('idx_msg_subscriptions_client_id').on(table.clientId),
    sourceIdx: index('idx_msg_subscriptions_source').on(table.source),
    dispatchPoolIdx: index('idx_msg_subscriptions_dispatch_pool').on(table.dispatchPoolId),
  }),
);

export type SubscriptionRecord = typeof subscriptions.$inferSelect;
export type NewSubscriptionRecord = typeof subscriptions.$inferInsert;

/**
 * Subscription event type bindings - junction table.
 */
export const subscriptionEventTypes = pgTable(
  'msg_subscription_event_types',
  {
    id: serial('id').primaryKey(),
    subscriptionId: varchar('subscription_id', { length: 17 }).notNull(),
    eventTypeId: varchar('event_type_id', { length: 17 }),
    eventTypeCode: varchar('event_type_code', { length: 255 }).notNull(),
    specVersion: varchar('spec_version', { length: 50 }),
  },
  (table) => ({
    subscriptionIdx: index('idx_msg_sub_event_types_subscription').on(table.subscriptionId),
    eventTypeIdx: index('idx_msg_sub_event_types_event_type').on(table.eventTypeId),
  }),
);

export type SubscriptionEventTypeRecord = typeof subscriptionEventTypes.$inferSelect;
export type NewSubscriptionEventTypeRecord = typeof subscriptionEventTypes.$inferInsert;

/**
 * Subscription custom configuration key-value pairs.
 */
export const subscriptionCustomConfigs = pgTable(
  'msg_subscription_custom_configs',
  {
    id: serial('id').primaryKey(),
    subscriptionId: varchar('subscription_id', { length: 17 }).notNull(),
    configKey: varchar('config_key', { length: 100 }).notNull(),
    configValue: varchar('config_value', { length: 1000 }).notNull(),
  },
  (table) => ({
    subscriptionIdx: index('idx_msg_sub_configs_subscription').on(table.subscriptionId),
  }),
);

export type SubscriptionCustomConfigRecord = typeof subscriptionCustomConfigs.$inferSelect;
export type NewSubscriptionCustomConfigRecord = typeof subscriptionCustomConfigs.$inferInsert;
