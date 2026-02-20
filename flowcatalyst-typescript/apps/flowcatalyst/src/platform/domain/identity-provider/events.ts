/**
 * Identity Provider Domain Events
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
// IdentityProviderCreated
// -----------------------------------------------------------------------------

export interface IdentityProviderCreatedData {
	readonly identityProviderId: string;
	readonly code: string;
	readonly name: string;
	readonly type: string;
	readonly [key: string]: unknown;
}

export class IdentityProviderCreated extends BaseDomainEvent<IdentityProviderCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"identity-provider",
		"created",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: IdentityProviderCreatedData) {
		super(
			{
				eventType: IdentityProviderCreated.EVENT_TYPE,
				specVersion: IdentityProviderCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"identity-provider",
					data.identityProviderId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"identity-provider",
					data.identityProviderId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// IdentityProviderUpdated
// -----------------------------------------------------------------------------

export interface IdentityProviderUpdatedData {
	readonly identityProviderId: string;
	readonly name: string;
	readonly type: string;
	readonly [key: string]: unknown;
}

export class IdentityProviderUpdated extends BaseDomainEvent<IdentityProviderUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"identity-provider",
		"updated",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: IdentityProviderUpdatedData) {
		super(
			{
				eventType: IdentityProviderUpdated.EVENT_TYPE,
				specVersion: IdentityProviderUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"identity-provider",
					data.identityProviderId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"identity-provider",
					data.identityProviderId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// IdentityProviderDeleted
// -----------------------------------------------------------------------------

export interface IdentityProviderDeletedData {
	readonly identityProviderId: string;
	readonly code: string;
	readonly [key: string]: unknown;
}

export class IdentityProviderDeleted extends BaseDomainEvent<IdentityProviderDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"identity-provider",
		"deleted",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: IdentityProviderDeletedData) {
		super(
			{
				eventType: IdentityProviderDeleted.EVENT_TYPE,
				specVersion: IdentityProviderDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"identity-provider",
					data.identityProviderId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"identity-provider",
					data.identityProviderId,
				),
			},
			ctx,
			data,
		);
	}
}
