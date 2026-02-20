/**
 * Service Account Domain Events
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
// ServiceAccountCreated
// -----------------------------------------------------------------------------

export interface ServiceAccountCreatedData {
	readonly serviceAccountId: string;
	readonly principalId: string;
	readonly oauthClientId: string;
	readonly code: string;
	readonly name: string;
	readonly applicationId: string | null;
	readonly [key: string]: unknown;
}

export class ServiceAccountCreated extends BaseDomainEvent<ServiceAccountCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"service-account",
		"created",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: ServiceAccountCreatedData) {
		super(
			{
				eventType: ServiceAccountCreated.EVENT_TYPE,
				specVersion: ServiceAccountCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"service-account",
					data.serviceAccountId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"service-account",
					data.serviceAccountId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ServiceAccountUpdated
// -----------------------------------------------------------------------------

export interface ServiceAccountUpdatedData {
	readonly serviceAccountId: string;
	readonly code: string;
	readonly [key: string]: unknown;
}

export class ServiceAccountUpdated extends BaseDomainEvent<ServiceAccountUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"service-account",
		"updated",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: ServiceAccountUpdatedData) {
		super(
			{
				eventType: ServiceAccountUpdated.EVENT_TYPE,
				specVersion: ServiceAccountUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"service-account",
					data.serviceAccountId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"service-account",
					data.serviceAccountId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// AuthTokenRegenerated
// -----------------------------------------------------------------------------

export interface AuthTokenRegeneratedData {
	readonly serviceAccountId: string;
	readonly code: string;
	readonly [key: string]: unknown;
}

export class AuthTokenRegenerated extends BaseDomainEvent<AuthTokenRegeneratedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"service-account",
		"auth-token-regenerated",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: AuthTokenRegeneratedData) {
		super(
			{
				eventType: AuthTokenRegenerated.EVENT_TYPE,
				specVersion: AuthTokenRegenerated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"service-account",
					data.serviceAccountId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"service-account",
					data.serviceAccountId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// SigningSecretRegenerated
// -----------------------------------------------------------------------------

export interface SigningSecretRegeneratedData {
	readonly serviceAccountId: string;
	readonly code: string;
	readonly [key: string]: unknown;
}

export class SigningSecretRegenerated extends BaseDomainEvent<SigningSecretRegeneratedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"service-account",
		"signing-secret-regenerated",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: SigningSecretRegeneratedData) {
		super(
			{
				eventType: SigningSecretRegenerated.EVENT_TYPE,
				specVersion: SigningSecretRegenerated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"service-account",
					data.serviceAccountId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"service-account",
					data.serviceAccountId,
				),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ServiceAccountDeleted
// -----------------------------------------------------------------------------

export interface ServiceAccountDeletedData {
	readonly serviceAccountId: string;
	readonly code: string;
	readonly [key: string]: unknown;
}

export class ServiceAccountDeleted extends BaseDomainEvent<ServiceAccountDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(
		APP,
		DOMAIN,
		"service-account",
		"deleted",
	);
	static readonly SPEC_VERSION = "1.0";

	constructor(ctx: ExecutionContext, data: ServiceAccountDeletedData) {
		super(
			{
				eventType: ServiceAccountDeleted.EVENT_TYPE,
				specVersion: ServiceAccountDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(
					APP,
					"service-account",
					data.serviceAccountId,
				),
				messageGroup: DomainEvent.messageGroup(
					APP,
					"service-account",
					data.serviceAccountId,
				),
			},
			ctx,
			data,
		);
	}
}
