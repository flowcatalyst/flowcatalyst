/**
 * Application Domain Events
 *
 * Events emitted when application state changes occur.
 */

import { BaseDomainEvent, DomainEvent, type ExecutionContext } from '@flowcatalyst/domain-core';
import type { ApplicationType } from './application.js';

const APP = 'platform';
const DOMAIN = 'admin';
const SOURCE = `${APP}:${DOMAIN}`;

// -----------------------------------------------------------------------------
// ApplicationCreated
// -----------------------------------------------------------------------------

export interface ApplicationCreatedData {
	readonly applicationId: string;
	readonly type: ApplicationType;
	readonly code: string;
	readonly name: string;
	readonly [key: string]: unknown;
}

export class ApplicationCreated extends BaseDomainEvent<ApplicationCreatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'application', 'created');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ApplicationCreatedData) {
		super(
			{
				eventType: ApplicationCreated.EVENT_TYPE,
				specVersion: ApplicationCreated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'application', data.applicationId),
				messageGroup: DomainEvent.messageGroup(APP, 'application', data.applicationId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ApplicationUpdated
// -----------------------------------------------------------------------------

export interface ApplicationUpdatedData {
	readonly applicationId: string;
	readonly code: string;
	readonly name: string;
	readonly previousName: string;
	readonly [key: string]: unknown;
}

export class ApplicationUpdated extends BaseDomainEvent<ApplicationUpdatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'application', 'updated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ApplicationUpdatedData) {
		super(
			{
				eventType: ApplicationUpdated.EVENT_TYPE,
				specVersion: ApplicationUpdated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'application', data.applicationId),
				messageGroup: DomainEvent.messageGroup(APP, 'application', data.applicationId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ApplicationActivated
// -----------------------------------------------------------------------------

export interface ApplicationActivatedData {
	readonly applicationId: string;
	readonly code: string;
	readonly [key: string]: unknown;
}

export class ApplicationActivated extends BaseDomainEvent<ApplicationActivatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'application', 'activated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ApplicationActivatedData) {
		super(
			{
				eventType: ApplicationActivated.EVENT_TYPE,
				specVersion: ApplicationActivated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'application', data.applicationId),
				messageGroup: DomainEvent.messageGroup(APP, 'application', data.applicationId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ApplicationDeactivated
// -----------------------------------------------------------------------------

export interface ApplicationDeactivatedData {
	readonly applicationId: string;
	readonly code: string;
	readonly [key: string]: unknown;
}

export class ApplicationDeactivated extends BaseDomainEvent<ApplicationDeactivatedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'application', 'deactivated');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ApplicationDeactivatedData) {
		super(
			{
				eventType: ApplicationDeactivated.EVENT_TYPE,
				specVersion: ApplicationDeactivated.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'application', data.applicationId),
				messageGroup: DomainEvent.messageGroup(APP, 'application', data.applicationId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ApplicationDeleted
// -----------------------------------------------------------------------------

export interface ApplicationDeletedData {
	readonly applicationId: string;
	readonly code: string;
	readonly name: string;
	readonly [key: string]: unknown;
}

export class ApplicationDeleted extends BaseDomainEvent<ApplicationDeletedData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'application', 'deleted');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ApplicationDeletedData) {
		super(
			{
				eventType: ApplicationDeleted.EVENT_TYPE,
				specVersion: ApplicationDeleted.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'application', data.applicationId),
				messageGroup: DomainEvent.messageGroup(APP, 'application', data.applicationId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ApplicationEnabledForClient
// -----------------------------------------------------------------------------

export interface ApplicationEnabledForClientData {
	readonly applicationId: string;
	readonly clientId: string;
	readonly configId: string;
	readonly [key: string]: unknown;
}

export class ApplicationEnabledForClient extends BaseDomainEvent<ApplicationEnabledForClientData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'application', 'enabled-for-client');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ApplicationEnabledForClientData) {
		super(
			{
				eventType: ApplicationEnabledForClient.EVENT_TYPE,
				specVersion: ApplicationEnabledForClient.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'application', data.applicationId),
				messageGroup: DomainEvent.messageGroup(APP, 'application', data.applicationId),
			},
			ctx,
			data,
		);
	}
}

// -----------------------------------------------------------------------------
// ApplicationDisabledForClient
// -----------------------------------------------------------------------------

export interface ApplicationDisabledForClientData {
	readonly applicationId: string;
	readonly clientId: string;
	readonly configId: string;
	readonly [key: string]: unknown;
}

export class ApplicationDisabledForClient extends BaseDomainEvent<ApplicationDisabledForClientData> {
	static readonly EVENT_TYPE = DomainEvent.eventType(APP, DOMAIN, 'application', 'disabled-for-client');
	static readonly SPEC_VERSION = '1.0';

	constructor(ctx: ExecutionContext, data: ApplicationDisabledForClientData) {
		super(
			{
				eventType: ApplicationDisabledForClient.EVENT_TYPE,
				specVersion: ApplicationDisabledForClient.SPEC_VERSION,
				source: SOURCE,
				subject: DomainEvent.subject(APP, 'application', data.applicationId),
				messageGroup: DomainEvent.messageGroup(APP, 'application', data.applicationId),
			},
			ctx,
			data,
		);
	}
}
