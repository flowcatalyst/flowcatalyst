/**
 * Subscription Domain Aggregate
 *
 * Defines how events are dispatched to a target endpoint.
 */

import { generate } from '@flowcatalyst/tsid';
import type { SubscriptionStatus } from './subscription-status.js';
import type { SubscriptionSource } from './subscription-source.js';
import type { DispatchMode } from './dispatch-mode.js';
import type { EventTypeBinding } from './event-type-binding.js';
import type { ConfigEntry } from './config-entry.js';

export interface Subscription {
  readonly id: string;
  readonly code: string;
  readonly applicationCode: string | null;
  readonly name: string;
  readonly description: string | null;
  readonly clientId: string | null;
  readonly clientIdentifier: string | null;
  readonly clientScoped: boolean;
  readonly eventTypes: readonly EventTypeBinding[];
  readonly target: string;
  readonly queue: string | null;
  readonly customConfig: readonly ConfigEntry[];
  readonly source: SubscriptionSource;
  readonly status: SubscriptionStatus;
  readonly maxAgeSeconds: number;
  readonly dispatchPoolId: string | null;
  readonly dispatchPoolCode: string | null;
  readonly delaySeconds: number;
  readonly sequence: number;
  readonly mode: DispatchMode;
  readonly timeoutSeconds: number;
  readonly maxRetries: number;
  readonly serviceAccountId: string | null;
  readonly dataOnly: boolean;
  readonly createdAt: Date;
  readonly updatedAt: Date;
}

export type NewSubscription = Omit<Subscription, 'createdAt' | 'updatedAt'> & {
  createdAt?: Date;
  updatedAt?: Date;
};

/**
 * Create a new subscription with sensible defaults.
 */
export function createSubscription(params: {
  code: string;
  applicationCode?: string | null;
  name: string;
  description?: string | null;
  clientId?: string | null;
  clientIdentifier?: string | null;
  clientScoped?: boolean;
  eventTypes: EventTypeBinding[];
  target: string;
  queue?: string | null;
  customConfig?: ConfigEntry[];
  source?: SubscriptionSource;
  maxAgeSeconds?: number;
  dispatchPoolId?: string | null;
  dispatchPoolCode?: string | null;
  delaySeconds?: number;
  sequence?: number;
  mode?: DispatchMode;
  timeoutSeconds?: number;
  maxRetries?: number;
  serviceAccountId?: string | null;
  dataOnly?: boolean;
}): NewSubscription {
  return {
    id: generate('SUBSCRIPTION'),
    code: params.code,
    applicationCode: params.applicationCode ?? null,
    name: params.name,
    description: params.description ?? null,
    clientId: params.clientId ?? null,
    clientIdentifier: params.clientIdentifier ?? null,
    clientScoped: params.clientScoped ?? false,
    eventTypes: params.eventTypes,
    target: params.target,
    queue: params.queue ?? null,
    customConfig: params.customConfig ?? [],
    source: params.source ?? 'UI',
    status: 'ACTIVE',
    maxAgeSeconds: params.maxAgeSeconds ?? 86400,
    dispatchPoolId: params.dispatchPoolId ?? null,
    dispatchPoolCode: params.dispatchPoolCode ?? null,
    delaySeconds: params.delaySeconds ?? 0,
    sequence: params.sequence ?? 99,
    mode: params.mode ?? 'IMMEDIATE',
    timeoutSeconds: params.timeoutSeconds ?? 30,
    maxRetries: params.maxRetries ?? 3,
    serviceAccountId: params.serviceAccountId ?? null,
    dataOnly: params.dataOnly ?? true,
  };
}

/**
 * Update a subscription with partial updates.
 * Immutable fields (code, applicationCode, clientId, clientScoped, source) are preserved.
 */
export function updateSubscription(
  sub: Subscription,
  updates: {
    name?: string | undefined;
    description?: string | null | undefined;
    eventTypes?: EventTypeBinding[] | undefined;
    target?: string | undefined;
    queue?: string | null | undefined;
    customConfig?: ConfigEntry[] | undefined;
    status?: SubscriptionStatus | undefined;
    maxAgeSeconds?: number | undefined;
    dispatchPoolId?: string | null | undefined;
    dispatchPoolCode?: string | null | undefined;
    delaySeconds?: number | undefined;
    sequence?: number | undefined;
    mode?: DispatchMode | undefined;
    timeoutSeconds?: number | undefined;
    maxRetries?: number | undefined;
    serviceAccountId?: string | null | undefined;
    dataOnly?: boolean | undefined;
  },
): Subscription {
  return {
    ...sub,
    ...(updates.name !== undefined ? { name: updates.name } : {}),
    ...(updates.description !== undefined ? { description: updates.description } : {}),
    ...(updates.eventTypes !== undefined ? { eventTypes: updates.eventTypes } : {}),
    ...(updates.target !== undefined ? { target: updates.target } : {}),
    ...(updates.queue !== undefined ? { queue: updates.queue } : {}),
    ...(updates.customConfig !== undefined ? { customConfig: updates.customConfig } : {}),
    ...(updates.status !== undefined ? { status: updates.status } : {}),
    ...(updates.maxAgeSeconds !== undefined ? { maxAgeSeconds: updates.maxAgeSeconds } : {}),
    ...(updates.dispatchPoolId !== undefined ? { dispatchPoolId: updates.dispatchPoolId } : {}),
    ...(updates.dispatchPoolCode !== undefined
      ? { dispatchPoolCode: updates.dispatchPoolCode }
      : {}),
    ...(updates.delaySeconds !== undefined ? { delaySeconds: updates.delaySeconds } : {}),
    ...(updates.sequence !== undefined ? { sequence: updates.sequence } : {}),
    ...(updates.mode !== undefined ? { mode: updates.mode } : {}),
    ...(updates.timeoutSeconds !== undefined ? { timeoutSeconds: updates.timeoutSeconds } : {}),
    ...(updates.maxRetries !== undefined ? { maxRetries: updates.maxRetries } : {}),
    ...(updates.serviceAccountId !== undefined
      ? { serviceAccountId: updates.serviceAccountId }
      : {}),
    ...(updates.dataOnly !== undefined ? { dataOnly: updates.dataOnly } : {}),
  };
}

/**
 * Helper predicates.
 */
export function isPlatformWide(sub: Subscription): boolean {
  return !sub.clientScoped;
}

export function isAllClients(sub: Subscription): boolean {
  return sub.clientScoped && sub.clientId === null;
}

export function isSpecificClient(sub: Subscription): boolean {
  return sub.clientScoped && sub.clientId !== null;
}
