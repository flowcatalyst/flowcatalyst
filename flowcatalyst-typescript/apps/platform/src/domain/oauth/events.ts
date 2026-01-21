/**
 * OAuth Client Events
 *
 * Events emitted when OAuth client state changes occur.
 */

import { BaseDomainEvent, DomainEvent, type ExecutionContext } from '@flowcatalyst/domain-core';
import type { OAuthClientType } from './oauth-client-type.js';

const APP = 'platform';
const DOMAIN = 'iam';
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// OAuthClientCreated
// -----------------------------------------------------------------------------

export interface OAuthClientCreatedData {
	readonly oauthClientId: string;
	readonly clientId: string;
	readonly clientName: string;
	readonly clientType: OAuthClientType;
	readonly [key: string]: unknown;
}

export class OAuthClientCreated extends BaseDomainEvent<OAuthClientCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'oauth-client', 'created');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: OAuthClientCreatedData) {
		super(
			{
				eventType: OAuthClientCreated.EVENT_TYPE,
				specVersion: OAuthClientCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'oauth-client', data.oauthClientId),
				messageGroup: DomainEvent.messageGroup(APP, 'oauth-client', data.oauthClientId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// OAuthClientUpdated
// -----------------------------------------------------------------------------

export interface OAuthClientUpdatedData {
	readonly oauthClientId: string;
	readonly clientId: string;
	readonly changes: Record<string, unknown>;
	readonly [key: string]: unknown;
}

export class OAuthClientUpdated extends BaseDomainEvent<OAuthClientUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'oauth-client', 'updated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: OAuthClientUpdatedData) {
		super(
			{
				eventType: OAuthClientUpdated.EVENT_TYPE,
				specVersion: OAuthClientUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'oauth-client', data.oauthClientId),
				messageGroup: DomainEvent.messageGroup(APP, 'oauth-client', data.oauthClientId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// OAuthClientSecretRegenerated
// -----------------------------------------------------------------------------

export interface OAuthClientSecretRegeneratedData {
	readonly oauthClientId: string;
	readonly clientId: string;
	readonly [key: string]: unknown;
}

export class OAuthClientSecretRegenerated extends BaseDomainEvent<OAuthClientSecretRegeneratedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'oauth-client', 'secret-regenerated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: OAuthClientSecretRegeneratedData) {
		super(
			{
				eventType: OAuthClientSecretRegenerated.EVENT_TYPE,
				specVersion: OAuthClientSecretRegenerated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'oauth-client', data.oauthClientId),
				messageGroup: DomainEvent.messageGroup(APP, 'oauth-client', data.oauthClientId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// OAuthClientDeleted
// -----------------------------------------------------------------------------

export interface OAuthClientDeletedData {
	readonly oauthClientId: string;
	readonly clientId: string;
	readonly [key: string]: unknown;
}

export class OAuthClientDeleted extends BaseDomainEvent<OAuthClientDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'oauth-client', 'deleted');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: OAuthClientDeletedData) {
		super(
			{
				eventType: OAuthClientDeleted.EVENT_TYPE,
				specVersion: OAuthClientDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'oauth-client', data.oauthClientId),
				messageGroup: DomainEvent.messageGroup(APP, 'oauth-client', data.oauthClientId),
			},
			ctx,
			data,
		);
	}
}
