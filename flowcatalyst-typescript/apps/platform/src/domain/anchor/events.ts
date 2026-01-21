/**
 * Anchor Domain Events
 *
 * Events emitted when anchor domain state changes occur.
 */

import { BaseDomainEvent, DomainEvent, type ExecutionContext } from '@flowcatalyst/domain-core';

const APP = 'platform';
const DOMAIN = 'admin';
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// AnchorDomainCreated
// -----------------------------------------------------------------------------

export interface AnchorDomainCreatedData {
	readonly anchorDomainId: string;
	readonly domain: string;
	readonly [key: string]: unknown;
}

export class AnchorDomainCreated extends BaseDomainEvent<AnchorDomainCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'anchor-domain', 'created');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: AnchorDomainCreatedData) {
		super(
			{
				eventType: AnchorDomainCreated.EVENT_TYPE,
				specVersion: AnchorDomainCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'anchor-domain', data.anchorDomainId),
				messageGroup: DomainEvent.messageGroup(APP, 'anchor-domain', data.anchorDomainId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// AnchorDomainUpdated
// -----------------------------------------------------------------------------

export interface AnchorDomainUpdatedData {
	readonly anchorDomainId: string;
	readonly domain: string;
	readonly previousDomain: string;
	readonly [key: string]: unknown;
}

export class AnchorDomainUpdated extends BaseDomainEvent<AnchorDomainUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'anchor-domain', 'updated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: AnchorDomainUpdatedData) {
		super(
			{
				eventType: AnchorDomainUpdated.EVENT_TYPE,
				specVersion: AnchorDomainUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'anchor-domain', data.anchorDomainId),
				messageGroup: DomainEvent.messageGroup(APP, 'anchor-domain', data.anchorDomainId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// AnchorDomainDeleted
// -----------------------------------------------------------------------------

export interface AnchorDomainDeletedData {
	readonly anchorDomainId: string;
	readonly domain: string;
	readonly [key: string]: unknown;
}

export class AnchorDomainDeleted extends BaseDomainEvent<AnchorDomainDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'anchor-domain', 'deleted');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: AnchorDomainDeletedData) {
		super(
			{
				eventType: AnchorDomainDeleted.EVENT_TYPE,
				specVersion: AnchorDomainDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'anchor-domain', data.anchorDomainId),
				messageGroup: DomainEvent.messageGroup(APP, 'anchor-domain', data.anchorDomainId),
			},
			ctx,
			data,
		);
	}
}
