/**
 * Dispatch Pool Domain Events
 *
 * Events emitted when dispatch pool state changes occur.
 */

import { BaseDomainEvent, DomainEvent, type ExecutionContext } from '@flowcatalyst/domain-core';

const APP = 'platform';
const DOMAIN = 'control-plane';
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// DispatchPoolCreated
// -----------------------------------------------------------------------------

export interface DispatchPoolCreatedData {
  readonly poolId: string;
  readonly code: string;
  readonly name: string;
  readonly description: string | null;
  readonly rateLimit: number;
  readonly concurrency: number;
  readonly clientId: string | null;
  readonly clientIdentifier: string | null;
  readonly status: string;
  readonly [key: string]: unknown;
}

export class DispatchPoolCreated extends BaseDomainEvent<DispatchPoolCreatedData> {
  static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'dispatch-pool', 'created');
  static readonly SPEC_VERSION = '1.0';

  constructor(ctx: ExecutionContext, data: DispatchPoolCreatedData) {
    super(
      {
        eventType: DispatchPoolCreated.EVENT_TYPE,
        specVersion: DispatchPoolCreated.SPEC_VERSION,
        source: SOURCE,
        subject: DomainEvent.subject(APP, 'dispatch-pool', data.poolId),
        messageGroup: DomainEvent.messageGroup(APP, 'dispatch-pool', data.poolId),
      },
      ctx,
      data,
    );
  }
}

// -----------------------------------------------------------------------------
// DispatchPoolUpdated
// -----------------------------------------------------------------------------

export interface DispatchPoolUpdatedData {
  readonly poolId: string;
  readonly code: string;
  readonly name: string;
  readonly description: string | null;
  readonly rateLimit: number;
  readonly concurrency: number;
  readonly status: string;
  readonly [key: string]: unknown;
}

export class DispatchPoolUpdated extends BaseDomainEvent<DispatchPoolUpdatedData> {
  static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'dispatch-pool', 'updated');
  static readonly SPEC_VERSION = '1.0';

  constructor(ctx: ExecutionContext, data: DispatchPoolUpdatedData) {
    super(
      {
        eventType: DispatchPoolUpdated.EVENT_TYPE,
        specVersion: DispatchPoolUpdated.SPEC_VERSION,
        source: SOURCE,
        subject: DomainEvent.subject(APP, 'dispatch-pool', data.poolId),
        messageGroup: DomainEvent.messageGroup(APP, 'dispatch-pool', data.poolId),
      },
      ctx,
      data,
    );
  }
}

// -----------------------------------------------------------------------------
// DispatchPoolDeleted
// -----------------------------------------------------------------------------

export interface DispatchPoolDeletedData {
  readonly poolId: string;
  readonly code: string;
  readonly clientId: string | null;
  readonly [key: string]: unknown;
}

export class DispatchPoolDeleted extends BaseDomainEvent<DispatchPoolDeletedData> {
  static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'dispatch-pool', 'deleted');
  static readonly SPEC_VERSION = '1.0';

  constructor(ctx: ExecutionContext, data: DispatchPoolDeletedData) {
    super(
      {
        eventType: DispatchPoolDeleted.EVENT_TYPE,
        specVersion: DispatchPoolDeleted.SPEC_VERSION,
        source: SOURCE,
        subject: DomainEvent.subject(APP, 'dispatch-pool', data.poolId),
        messageGroup: DomainEvent.messageGroup(APP, 'dispatch-pool', data.poolId),
      },
      ctx,
      data,
    );
  }
}

// -----------------------------------------------------------------------------
// DispatchPoolsSynced
// -----------------------------------------------------------------------------

export interface DispatchPoolsSyncedData {
  readonly applicationCode: string;
  readonly poolsCreated: number;
  readonly poolsUpdated: number;
  readonly poolsDeleted: number;
  readonly syncedPoolCodes: string[];
  readonly [key: string]: unknown;
}

export class DispatchPoolsSynced extends BaseDomainEvent<DispatchPoolsSyncedData> {
  static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'dispatch-pool', 'synced');
  static readonly SPEC_VERSION = '1.0';

  constructor(ctx: ExecutionContext, data: DispatchPoolsSyncedData) {
    super(
      {
        eventType: DispatchPoolsSynced.EVENT_TYPE,
        specVersion: DispatchPoolsSynced.SPEC_VERSION,
        source: SOURCE,
        subject: DomainEvent.subject(APP, 'dispatch-pool', data.applicationCode),
        messageGroup: DomainEvent.messageGroup(APP, 'dispatch-pool', data.applicationCode),
      },
      ctx,
      data,
    );
  }
}
