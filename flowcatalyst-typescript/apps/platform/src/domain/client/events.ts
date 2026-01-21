/**
 * Client Domain Events
 *
 * Events emitted when client state changes occur.
 */

import { BaseDomainEvent, DomainEvent, type ExecutionContext } from '@flowcatalyst/domain-core';
import type { ClientStatus } from './client-status.js';

const APP = 'platform';
const DOMAIN = 'admin';
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// ClientCreated
// -----------------------------------------------------------------------------

export interface ClientCreatedData {
	readonly clientId: string;
	readonly name: string;
	readonly identifier: string;
	readonly [key: string]: unknown;
}

export class ClientCreated extends BaseDomainEvent<ClientCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'client', 'created');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ClientCreatedData) {
		super(
			{
				eventType: ClientCreated.EVENT_TYPE,
				specVersion: ClientCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'client', data.clientId),
				messageGroup: DomainEvent.messageGroup(APP, 'client', data.clientId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ClientUpdated
// -----------------------------------------------------------------------------

export interface ClientUpdatedData {
	readonly clientId: string;
	readonly name: string;
	readonly previousName: string;
	readonly [key: string]: unknown;
}

export class ClientUpdated extends BaseDomainEvent<ClientUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'client', 'updated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ClientUpdatedData) {
		super(
			{
				eventType: ClientUpdated.EVENT_TYPE,
				specVersion: ClientUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'client', data.clientId),
				messageGroup: DomainEvent.messageGroup(APP, 'client', data.clientId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ClientStatusChanged
// -----------------------------------------------------------------------------

export interface ClientStatusChangedData {
	readonly clientId: string;
	readonly name: string;
	readonly previousStatus: ClientStatus;
	readonly newStatus: ClientStatus;
	readonly reason: string | null;
	readonly [key: string]: unknown;
}

export class ClientStatusChanged extends BaseDomainEvent<ClientStatusChangedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'client', 'status-changed');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ClientStatusChangedData) {
		super(
			{
				eventType: ClientStatusChanged.EVENT_TYPE,
				specVersion: ClientStatusChanged.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'client', data.clientId),
				messageGroup: DomainEvent.messageGroup(APP, 'client', data.clientId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ClientDeleted
// -----------------------------------------------------------------------------

export interface ClientDeletedData {
	readonly clientId: string;
	readonly name: string;
	readonly identifier: string;
	readonly [key: string]: unknown;
}

export class ClientDeleted extends BaseDomainEvent<ClientDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'client', 'deleted');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ClientDeletedData) {
		super(
			{
				eventType: ClientDeleted.EVENT_TYPE,
				specVersion: ClientDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'client', data.clientId),
				messageGroup: DomainEvent.messageGroup(APP, 'client', data.clientId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ClientNoteAdded
// -----------------------------------------------------------------------------

export interface ClientNoteAddedData {
	readonly clientId: string;
	readonly category: string;
	readonly text: string;
	readonly addedBy: string;
	readonly [key: string]: unknown;
}

export class ClientNoteAdded extends BaseDomainEvent<ClientNoteAddedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'client', 'note-added');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ClientNoteAddedData) {
		super(
			{
				eventType: ClientNoteAdded.EVENT_TYPE,
				specVersion: ClientNoteAdded.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'client', data.clientId),
				messageGroup: DomainEvent.messageGroup(APP, 'client', data.clientId),
			},
			ctx,
			data,
		);
	}
}
