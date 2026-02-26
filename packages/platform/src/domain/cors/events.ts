/**
 * CORS Domain Events
 */

import {
	BaseDomainEvent,
	DomainEvent,
	type ExecutionContext,
} from "@flowcatalyst/domain";

const APP = "platform";
const DOMAIN = "admin";
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// CorsOriginAdded
// -----------------------------------------------------------------------------

export interface CorsOriginAddedData {
	readonly originId: string;
	readonly origin: string;
	readonly [key: string]: unknown;
}

export class CorsOriginAdded extends BaseDomainEvent<CorsOriginAddedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"cors-origin",
		"added",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: CorsOriginAddedData) {
		super(
			{
				eventType: CorsOriginAdded.EVENT_TYPE,
				specVersion: CorsOriginAdded.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "cors-origin", data.originId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"cors-origin",
					data.originId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// CorsOriginDeleted
// -----------------------------------------------------------------------------

export interface CorsOriginDeletedData {
	readonly originId: string;
	readonly origin: string;
	readonly [key: string]: unknown;
}

export class CorsOriginDeleted extends BaseDomainEvent<CorsOriginDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"cors-origin",
		"deleted",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: CorsOriginDeletedData) {
		super(
			{
				eventType: CorsOriginDeleted.EVENT_TYPE,
				specVersion: CorsOriginDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, "cors-origin", data.originId),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"cors-origin",
					data.originId,
				),
			},
			ctx,
			data,
		);
	}
}
