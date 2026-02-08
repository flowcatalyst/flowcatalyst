/**
 * Subscription Domain Events
 *
 * Events emitted when subscription state changes occur.
 */

import { BaseDomainEvent, DomainEvent, type ExecutionContext } from '@flowcatalyst/domain-core';
import type { EventTypeBinding } from './event-type-binding.js';

const APP = 'platform';
const DOMAIN = 'control-plane';
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// SubscriptionCreated
// -----------------------------------------------------------------------------

export interface SubscriptionCreatedData {
	readonly subscriptionId: string;
	readonly code: string;
	readonly applicationCode: string | null;
	readonly name: string;
	readonly clientId: string | null;
	readonly clientScoped: boolean;
	readonly eventTypes: readonly EventTypeBinding[];
	readonly target: string;
	readonly [key: string]: unknown;
}

export class SubscriptionCreated extends BaseDomainEvent<SubscriptionCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'subscription', 'created');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: SubscriptionCreatedData) {
		super(
			{
				eventType: SubscriptionCreated.EVENT_TYPE,
				specVersion: SubscriptionCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'subscription', data.subscriptionId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					'subscription',
					data.clientId ?? 'anchor',
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// SubscriptionUpdated
// -----------------------------------------------------------------------------

export interface SubscriptionUpdatedData {
	readonly subscriptionId: string;
	readonly code: string;
	readonly applicationCode: string | null;
	readonly name: string;
	readonly clientId: string | null;
	readonly eventTypes: readonly EventTypeBinding[];
	readonly target: string;
	readonly [key: string]: unknown;
}

export class SubscriptionUpdated extends BaseDomainEvent<SubscriptionUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'subscription', 'updated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: SubscriptionUpdatedData) {
		super(
			{
				eventType: SubscriptionUpdated.EVENT_TYPE,
				specVersion: SubscriptionUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'subscription', data.subscriptionId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					'subscription',
					data.clientId ?? 'anchor',
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// SubscriptionDeleted
// -----------------------------------------------------------------------------

export interface SubscriptionDeletedData {
	readonly subscriptionId: string;
	readonly code: string;
	readonly applicationCode: string | null;
	readonly clientId: string | null;
	readonly [key: string]: unknown;
}

export class SubscriptionDeleted extends BaseDomainEvent<SubscriptionDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'subscription', 'deleted');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: SubscriptionDeletedData) {
		super(
			{
				eventType: SubscriptionDeleted.EVENT_TYPE,
				specVersion: SubscriptionDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'subscription', data.subscriptionId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					'subscription',
					data.clientId ?? 'anchor',
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// SubscriptionsSynced
// -----------------------------------------------------------------------------

export interface SubscriptionsSyncedData {
	readonly applicationCode: string;
	readonly subscriptionsCreated: number;
	readonly subscriptionsUpdated: number;
	readonly subscriptionsDeleted: number;
	readonly syncedSubscriptionCodes: string[];
	readonly [key: string]: unknown;
}

export class SubscriptionsSynced extends BaseDomainEvent<SubscriptionsSyncedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, 'messaging', 'subscription', 'synced');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: SubscriptionsSyncedData) {
		super(
			{
				eventType: SubscriptionsSynced.EVENT_TYPE,
				specVersion: SubscriptionsSynced.SPEC_VERSION,
				source: `${APP}:messaging`,
				subject: DomainEvent.subject(APP, 'application', `${data.applicationCode}.subscriptions`),
				messageGroup: DomainEvent.messageGroup(APP, 'application', data.applicationCode),
			},
			ctx,
			data,
		);
	}
}
