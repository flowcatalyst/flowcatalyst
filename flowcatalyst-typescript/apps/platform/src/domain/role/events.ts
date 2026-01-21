/**
 * Role Domain Events
 */

import { BaseDomainEvent, DomainEvent, type ExecutionContext } from '@flowcatalyst/domain-core';

import type { RoleSource } from './auth-role.js';

const APP = 'platform';
const DOMAIN = 'admin';
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// RoleCreated
// -----------------------------------------------------------------------------

/**
 * Role created event data.
 */
export interface RoleCreatedData {
	readonly roleId: string;
	readonly name: string;
	readonly displayName: string;
	readonly applicationId: string | null;
	readonly applicationCode: string | null;
	readonly source: RoleSource;
	readonly permissions: readonly string[];
	readonly [key: string]: unknown;
}

/**
 * Role created event.
 */
export class RoleCreated extends BaseDomainEvent<RoleCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'role', 'created');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: RoleCreatedData) {
		super(
			{
				eventType: RoleCreated.EVENT_TYPE,
				specVersion: RoleCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'role', data.roleId),
				messageGroup: DomainEvent.messageGroup(APP, 'role', data.roleId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// RoleUpdated
// -----------------------------------------------------------------------------

/**
 * Role updated event data.
 */
export interface RoleUpdatedData {
	readonly roleId: string;
	readonly displayName: string;
	readonly permissions: readonly string[];
	readonly clientManaged: boolean;
	readonly [key: string]: unknown;
}

/**
 * Role updated event.
 */
export class RoleUpdated extends BaseDomainEvent<RoleUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'role', 'updated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: RoleUpdatedData) {
		super(
			{
				eventType: RoleUpdated.EVENT_TYPE,
				specVersion: RoleUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'role', data.roleId),
				messageGroup: DomainEvent.messageGroup(APP, 'role', data.roleId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// RoleDeleted
// -----------------------------------------------------------------------------

/**
 * Role deleted event data.
 */
export interface RoleDeletedData {
	readonly roleId: string;
	readonly name: string;
	readonly [key: string]: unknown;
}

/**
 * Role deleted event.
 */
export class RoleDeleted extends BaseDomainEvent<RoleDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'role', 'deleted');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: RoleDeletedData) {
		super(
			{
				eventType: RoleDeleted.EVENT_TYPE,
				specVersion: RoleDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'role', data.roleId),
				messageGroup: DomainEvent.messageGroup(APP, 'role', data.roleId),
			},
			ctx,
			data,
		);
	}
}
