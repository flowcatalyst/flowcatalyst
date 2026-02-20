/**
 * Email Domain Mapping Domain Events
 */

import {
	BaseDomainEvent,
	DomainEvent,
	type ExecutionContext,
} from "@flowcatalyst/domain-core";

const APP = "platform";
const DOMAIN = "iam";
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// EmailDomainMappingCreated
// -----------------------------------------------------------------------------

export interface EmailDomainMappingCreatedData {
	readonly emailDomainMappingId: string;
	readonly emailDomain: string;
	readonly identityProviderId: string;
	readonly scopeType: string;
	readonly primaryClientId: string | null;
	readonly additionalClientIds: readonly string[];
	readonly grantedClientIds: readonly string[];
	readonly [key: string]: unknown;
}

export class EmailDomainMappingCreated extends BaseDomainEvent<EmailDomainMappingCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"email-domain-mapping",
		"created",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: EmailDomainMappingCreatedData) {
		super(
			{
				eventType: EmailDomainMappingCreated.EVENT_TYPE,
				specVersion: EmailDomainMappingCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"email-domain-mapping",
					data.emailDomainMappingId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"email-domain-mapping",
					data.emailDomainMappingId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// EmailDomainMappingUpdated
// -----------------------------------------------------------------------------

export interface EmailDomainMappingUpdatedData {
	readonly emailDomainMappingId: string;
	readonly emailDomain: string;
	readonly identityProviderId: string;
	readonly scopeType: string;
	readonly primaryClientId: string | null;
	readonly additionalClientIds: readonly string[];
	readonly grantedClientIds: readonly string[];
	readonly [key: string]: unknown;
}

export class EmailDomainMappingUpdated extends BaseDomainEvent<EmailDomainMappingUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"email-domain-mapping",
		"updated",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: EmailDomainMappingUpdatedData) {
		super(
			{
				eventType: EmailDomainMappingUpdated.EVENT_TYPE,
				specVersion: EmailDomainMappingUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"email-domain-mapping",
					data.emailDomainMappingId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"email-domain-mapping",
					data.emailDomainMappingId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// EmailDomainMappingDeleted
// -----------------------------------------------------------------------------

export interface EmailDomainMappingDeletedData {
	readonly emailDomainMappingId: string;
	readonly emailDomain: string;
	readonly identityProviderId: string;
	readonly [key: string]: unknown;
}

export class EmailDomainMappingDeleted extends BaseDomainEvent<EmailDomainMappingDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"email-domain-mapping",
		"deleted",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: EmailDomainMappingDeletedData) {
		super(
			{
				eventType: EmailDomainMappingDeleted.EVENT_TYPE,
				specVersion: EmailDomainMappingDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"email-domain-mapping",
					data.emailDomainMappingId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"email-domain-mapping",
					data.emailDomainMappingId,
				),
			},
			ctx,
			data,
		);
	}
}
