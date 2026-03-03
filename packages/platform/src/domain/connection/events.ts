/**
 * Connection Domain Events
 *
 * Events emitted when connection state changes occur.
 */

import {
	BaseDomainEvent,
	DomainEvent,
	type ExecutionContext,
} from "@flowcatalyst/domain";

const APP = "platform";
const DOMAIN = "control-plane";
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// ConnectionCreated
// -----------------------------------------------------------------------------

export interface ConnectionCreatedData {
	readonly connectionId: string;
	readonly code: string;
	readonly name: string;
	readonly endpoint: string;
	readonly externalId: string | null;
	readonly serviceAccountId: string;
	readonly clientId: string | null;
	readonly [key: string]: unknown;
}

export class ConnectionCreated extends BaseDomainEvent<ConnectionCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"connection",
		"created",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: ConnectionCreatedData) {
		super(
			{
				eventType: ConnectionCreated.EVENT_TYPE,
				specVersion: ConnectionCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "connection", data.connectionId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"connection",
					data.clientId ?? "anchor",
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ConnectionUpdated
// -----------------------------------------------------------------------------

export interface ConnectionUpdatedData {
	readonly connectionId: string;
	readonly code: string;
	readonly name: string;
	readonly endpoint: string;
	readonly externalId: string | null;
	readonly status: string;
	readonly [key: string]: unknown;
}

export class ConnectionUpdated extends BaseDomainEvent<ConnectionUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"connection",
		"updated",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: ConnectionUpdatedData) {
		super(
			{
				eventType: ConnectionUpdated.EVENT_TYPE,
				specVersion: ConnectionUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "connection", data.connectionId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"connection",
					data.connectionId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ConnectionDeleted
// -----------------------------------------------------------------------------

export interface ConnectionDeletedData {
	readonly connectionId: string;
	readonly code: string;
	readonly clientId: string | null;
	readonly [key: string]: unknown;
}

export class ConnectionDeleted extends BaseDomainEvent<ConnectionDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"connection",
		"deleted",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: ConnectionDeletedData) {
		super(
			{
				eventType: ConnectionDeleted.EVENT_TYPE,
				specVersion: ConnectionDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "connection", data.connectionId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"connection",
					data.clientId ?? "anchor",
				),
			},
			ctx,
			data,
		);
	}
}
