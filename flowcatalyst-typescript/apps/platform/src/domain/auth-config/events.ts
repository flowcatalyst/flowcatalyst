/**
 * Client Auth Config Events
 *
 * Events emitted when client auth config state changes occur.
 */

import { BaseDomainEvent, DomainEvent, type ExecutionContext } from '@flowcatalyst/domain-core';
import type { AuthConfigType } from './auth-config-type.js';
import type { AuthProvider } from './auth-provider.js';

const APP = 'platform';
const DOMAIN = 'iam';
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// AuthConfigCreated
// -----------------------------------------------------------------------------

export interface AuthConfigCreatedData {
	readonly authConfigId: string;
	readonly emailDomain: string;
	readonly configType: AuthConfigType;
	readonly authProvider: AuthProvider;
	readonly primaryClientId: string | null;
	readonly [key: string]: unknown;
}

export class AuthConfigCreated extends BaseDomainEvent<AuthConfigCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'auth-config', 'created');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: AuthConfigCreatedData) {
		super(
			{
				eventType: AuthConfigCreated.EVENT_TYPE,
				specVersion: AuthConfigCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'auth-config', data.authConfigId),
				messageGroup: DomainEvent.messageGroup(APP, 'auth-config', data.authConfigId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// AuthConfigUpdated
// -----------------------------------------------------------------------------

export interface AuthConfigUpdatedData {
	readonly authConfigId: string;
	readonly emailDomain: string;
	readonly configType: AuthConfigType;
	readonly authProvider: AuthProvider;
	readonly changes: Record<string, unknown>;
	readonly [key: string]: unknown;
}

export class AuthConfigUpdated extends BaseDomainEvent<AuthConfigUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'auth-config', 'updated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: AuthConfigUpdatedData) {
		super(
			{
				eventType: AuthConfigUpdated.EVENT_TYPE,
				specVersion: AuthConfigUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'auth-config', data.authConfigId),
				messageGroup: DomainEvent.messageGroup(APP, 'auth-config', data.authConfigId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// AuthConfigDeleted
// -----------------------------------------------------------------------------

export interface AuthConfigDeletedData {
	readonly authConfigId: string;
	readonly emailDomain: string;
	readonly [key: string]: unknown;
}

export class AuthConfigDeleted extends BaseDomainEvent<AuthConfigDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'auth-config', 'deleted');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: AuthConfigDeletedData) {
		super(
			{
				eventType: AuthConfigDeleted.EVENT_TYPE,
				specVersion: AuthConfigDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'auth-config', data.authConfigId),
				messageGroup: DomainEvent.messageGroup(APP, 'auth-config', data.authConfigId),
			},
			ctx,
			data,
		);
	}
}
