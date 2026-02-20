/**
 * EventType Domain Events
 *
 * Events emitted when event type state changes occur.
 */

import {
	BaseDomainEvent,
	DomainEvent,
	type ExecutionContext,
} from "@flowcatalyst/domain-core";
import type { SchemaType } from "./schema-type.js";

const APP = "platform";
const DOMAIN = "admin";
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// EventTypeCreated
// -----------------------------------------------------------------------------

export interface EventTypeCreatedData {
	readonly eventTypeId: string;
	readonly code: string;
	readonly name: string;
	readonly description: string | null;
	readonly [key: string]: unknown;
}

export class EventTypeCreated extends BaseDomainEvent<EventTypeCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"event-type",
		"created",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: EventTypeCreatedData) {
		super(
			{
				eventType: EventTypeCreated.EVENT_TYPE,
				specVersion: EventTypeCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "event-type", data.eventTypeId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"event-type",
					data.eventTypeId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// EventTypeUpdated
// -----------------------------------------------------------------------------

export interface EventTypeUpdatedData {
	readonly eventTypeId: string;
	readonly name: string;
	readonly description: string | null;
	readonly [key: string]: unknown;
}

export class EventTypeUpdated extends BaseDomainEvent<EventTypeUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"event-type",
		"updated",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: EventTypeUpdatedData) {
		super(
			{
				eventType: EventTypeUpdated.EVENT_TYPE,
				specVersion: EventTypeUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "event-type", data.eventTypeId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"event-type",
					data.eventTypeId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// EventTypeArchived
// -----------------------------------------------------------------------------

export interface EventTypeArchivedData {
	readonly eventTypeId: string;
	readonly code: string;
	readonly [key: string]: unknown;
}

export class EventTypeArchived extends BaseDomainEvent<EventTypeArchivedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"event-type",
		"archived",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: EventTypeArchivedData) {
		super(
			{
				eventType: EventTypeArchived.EVENT_TYPE,
				specVersion: EventTypeArchived.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "event-type", data.eventTypeId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"event-type",
					data.eventTypeId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// EventTypeDeleted
// -----------------------------------------------------------------------------

export interface EventTypeDeletedData {
	readonly eventTypeId: string;
	readonly code: string;
	readonly [key: string]: unknown;
}

export class EventTypeDeleted extends BaseDomainEvent<EventTypeDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"event-type",
		"deleted",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: EventTypeDeletedData) {
		super(
			{
				eventType: EventTypeDeleted.EVENT_TYPE,
				specVersion: EventTypeDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "event-type", data.eventTypeId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"event-type",
					data.eventTypeId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// SchemaAdded
// -----------------------------------------------------------------------------

export interface SchemaAddedData {
	readonly eventTypeId: string;
	readonly version: string;
	readonly mimeType: string;
	readonly schemaType: SchemaType;
	readonly [key: string]: unknown;
}

export class SchemaAdded extends BaseDomainEvent<SchemaAddedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"event-type",
		"schema-added",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: SchemaAddedData) {
		super(
			{
				eventType: SchemaAdded.EVENT_TYPE,
				specVersion: SchemaAdded.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "event-type", data.eventTypeId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"event-type",
					data.eventTypeId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// SchemaFinalised
// -----------------------------------------------------------------------------

export interface SchemaFinalisedData {
	readonly eventTypeId: string;
	readonly version: string;
	readonly deprecatedVersion: string | null;
	readonly [key: string]: unknown;
}

export class SchemaFinalised extends BaseDomainEvent<SchemaFinalisedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"event-type",
		"schema-finalised",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: SchemaFinalisedData) {
		super(
			{
				eventType: SchemaFinalised.EVENT_TYPE,
				specVersion: SchemaFinalised.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "event-type", data.eventTypeId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"event-type",
					data.eventTypeId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// SchemaDeprecated
// -----------------------------------------------------------------------------

export interface SchemaDeprecatedData {
	readonly eventTypeId: string;
	readonly version: string;
	readonly [key: string]: unknown;
}

export class SchemaDeprecated extends BaseDomainEvent<SchemaDeprecatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"event-type",
		"schema-deprecated",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: SchemaDeprecatedData) {
		super(
			{
				eventType: SchemaDeprecated.EVENT_TYPE,
				specVersion: SchemaDeprecated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "event-type", data.eventTypeId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"event-type",
					data.eventTypeId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// EventTypesSynced
// -----------------------------------------------------------------------------

export interface EventTypesSyncedData {
	readonly applicationCode: string;
	readonly eventTypesCreated: number;
	readonly eventTypesUpdated: number;
	readonly eventTypesDeleted: number;
	readonly syncedEventTypeCodes: string[];
	readonly [key: string]: unknown;
}

export class EventTypesSynced extends BaseDomainEvent<EventTypesSyncedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"event-type",
		"synced",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: EventTypesSyncedData) {
		super(
			{
				eventType: EventTypesSynced.EVENT_TYPE,
				specVersion: EventTypesSynced.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "event-type", data.applicationCode),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"event-type",
					data.applicationCode,
				),
			},
			ctx,
			data,
		);
	}
}
