/**
 * Principal Domain Events
 *
 * Events emitted when principal state changes occur.
 */

import { BaseDomainEvent, DomainEvent, type ExecutionContext } from '@flowcatalyst/domain-core';
import type { IdpType } from './idp-type.js';
import type { UserScope } from './user-scope.js';

const APP = 'platform';
const DOMAIN = 'iam';
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// UserCreated
// -----------------------------------------------------------------------------

export interface UserCreatedData {
	readonly userId: string;
	readonly email: string;
	readonly emailDomain: string;
	readonly name: string;
	readonly scope: UserScope;
	readonly clientId: string | null;
	readonly idpType: IdpType;
	readonly isAnchorUser: boolean;
	readonly [key: string]: unknown;
}

export class UserCreated extends BaseDomainEvent<UserCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'user', 'created');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: UserCreatedData) {
		super(
			{
				eventType: UserCreated.EVENT_TYPE,
				specVersion: UserCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'user', data.userId),
				messageGroup: DomainEvent.messageGroup(APP, 'user', data.userId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// UserUpdated
// -----------------------------------------------------------------------------

export interface UserUpdatedData {
	readonly userId: string;
	readonly name: string;
	readonly previousName: string;
	readonly [key: string]: unknown;
}

export class UserUpdated extends BaseDomainEvent<UserUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'user', 'updated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: UserUpdatedData) {
		super(
			{
				eventType: UserUpdated.EVENT_TYPE,
				specVersion: UserUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'user', data.userId),
				messageGroup: DomainEvent.messageGroup(APP, 'user', data.userId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// UserActivated
// -----------------------------------------------------------------------------

export interface UserActivatedData {
	readonly userId: string;
	readonly email: string;
	readonly [key: string]: unknown;
}

export class UserActivated extends BaseDomainEvent<UserActivatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'user', 'activated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: UserActivatedData) {
		super(
			{
				eventType: UserActivated.EVENT_TYPE,
				specVersion: UserActivated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'user', data.userId),
				messageGroup: DomainEvent.messageGroup(APP, 'user', data.userId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// UserDeactivated
// -----------------------------------------------------------------------------

export interface UserDeactivatedData {
	readonly userId: string;
	readonly email: string;
	readonly [key: string]: unknown;
}

export class UserDeactivated extends BaseDomainEvent<UserDeactivatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'user', 'deactivated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: UserDeactivatedData) {
		super(
			{
				eventType: UserDeactivated.EVENT_TYPE,
				specVersion: UserDeactivated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'user', data.userId),
				messageGroup: DomainEvent.messageGroup(APP, 'user', data.userId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// UserDeleted
// -----------------------------------------------------------------------------

export interface UserDeletedData {
	readonly userId: string;
	readonly email: string;
	readonly [key: string]: unknown;
}

export class UserDeleted extends BaseDomainEvent<UserDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'user', 'deleted');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: UserDeletedData) {
		super(
			{
				eventType: UserDeleted.EVENT_TYPE,
				specVersion: UserDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'user', data.userId),
				messageGroup: DomainEvent.messageGroup(APP, 'user', data.userId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// RolesAssigned
// -----------------------------------------------------------------------------

export interface RolesAssignedData {
	readonly userId: string;
	readonly email: string;
	readonly roles: readonly string[];
	readonly previousRoles: readonly string[];
	readonly [key: string]: unknown;
}

export class RolesAssigned extends BaseDomainEvent<RolesAssignedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'user', 'roles-assigned');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: RolesAssignedData) {
		super(
			{
				eventType: RolesAssigned.EVENT_TYPE,
				specVersion: RolesAssigned.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'user', data.userId),
				messageGroup: DomainEvent.messageGroup(APP, 'user', data.userId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ClientAccessGranted
// -----------------------------------------------------------------------------

export interface ClientAccessGrantedData {
	readonly userId: string;
	readonly email: string;
	readonly clientId: string;
	readonly [key: string]: unknown;
}

export class ClientAccessGranted extends BaseDomainEvent<ClientAccessGrantedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'user', 'client-access-granted');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ClientAccessGrantedData) {
		super(
			{
				eventType: ClientAccessGranted.EVENT_TYPE,
				specVersion: ClientAccessGranted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'user', data.userId),
				messageGroup: DomainEvent.messageGroup(APP, 'user', data.userId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ClientAccessRevoked
// -----------------------------------------------------------------------------

export interface ClientAccessRevokedData {
	readonly userId: string;
	readonly email: string;
	readonly clientId: string;
	readonly [key: string]: unknown;
}

export class ClientAccessRevoked extends BaseDomainEvent<ClientAccessRevokedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'user', 'client-access-revoked');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ClientAccessRevokedData) {
		super(
			{
				eventType: ClientAccessRevoked.EVENT_TYPE,
				specVersion: ClientAccessRevoked.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'user', data.userId),
				messageGroup: DomainEvent.messageGroup(APP, 'user', data.userId),
			},
			ctx,
			data,
		);
	}
}
