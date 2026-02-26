/**
 * EventType Entity
 *
 * Represents an event type definition in the FlowCatalyst platform.
 * Event type codes follow the format: {application}:{subdomain}:{aggregate}:{event}
 *
 * Each event type can have multiple spec versions (schemas) that define
 * the structure of events published under this type.
 */

import { generate } from "@flowcatalyst/tsid";
import type { SpecVersion } from "./spec-version.js";
import type { EventTypeStatus } from "./event-type-status.js";
import type { EventTypeSource } from "./event-type-source.js";

/**
 * EventType entity.
 */
export interface EventType {
	readonly id: string;
	readonly code: string;
	readonly name: string;
	readonly description: string | null;
	readonly specVersions: SpecVersion[];
	readonly status: EventTypeStatus;
	readonly source: EventTypeSource;
	readonly clientScoped: boolean;
	readonly application: string;
	readonly subdomain: string;
	readonly aggregate: string;
	readonly createdAt: Date;
	readonly updatedAt: Date;
}

/**
 * Input for creating a new EventType.
 */
export type NewEventType = Omit<EventType, "createdAt" | "updatedAt"> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Parse code segments from an event type code.
 * Format: {application}:{subdomain}:{aggregate}:{event}
 */
export function parseCodeSegments(code: string): {
	application: string;
	subdomain: string;
	aggregate: string;
	event: string;
} | null {
	const parts = code.split(":");
	if (parts.length !== 4) return null;
	return {
		application: parts[0]!,
		subdomain: parts[1]!,
		aggregate: parts[2]!,
		event: parts[3]!,
	};
}

/**
 * Build an event type code from segments.
 */
export function buildCode(
	application: string,
	subdomain: string,
	aggregate: string,
	event: string,
): string {
	return `${application}:${subdomain}:${aggregate}:${event}`;
}

/**
 * Create a new event type from UI.
 */
export function createEventType(params: {
	application: string;
	subdomain: string;
	aggregate: string;
	event: string;
	name: string;
	description?: string | null;
	clientScoped?: boolean;
}): NewEventType {
	return {
		id: generate("EVENT_TYPE"),
		code: buildCode(
			params.application,
			params.subdomain,
			params.aggregate,
			params.event,
		),
		name: params.name,
		description: params.description ?? null,
		specVersions: [],
		status: "CURRENT",
		source: "UI",
		clientScoped: params.clientScoped ?? false,
		application: params.application,
		subdomain: params.subdomain,
		aggregate: params.aggregate,
	};
}

/**
 * Create a new event type from API/SDK sync.
 */
export function createEventTypeFromApi(params: {
	application: string;
	subdomain: string;
	aggregate: string;
	event: string;
	name: string;
	description?: string | null;
	clientScoped?: boolean;
}): NewEventType {
	return {
		id: generate("EVENT_TYPE"),
		code: buildCode(
			params.application,
			params.subdomain,
			params.aggregate,
			params.event,
		),
		name: params.name,
		description: params.description ?? null,
		specVersions: [],
		status: "CURRENT",
		source: "API",
		clientScoped: params.clientScoped ?? false,
		application: params.application,
		subdomain: params.subdomain,
		aggregate: params.aggregate,
	};
}

/**
 * Update an event type's metadata.
 */
export function updateEventType(
	eventType: EventType,
	updates: Partial<Pick<EventType, "name" | "description">>,
): EventType {
	return {
		...eventType,
		...updates,
		updatedAt: new Date(),
	};
}

/**
 * Add a spec version to an event type.
 */
export function addSpecVersion(
	eventType: EventType,
	specVersion: SpecVersion,
): EventType {
	return {
		...eventType,
		specVersions: [...eventType.specVersions, specVersion],
		updatedAt: new Date(),
	};
}

/**
 * Update a spec version within an event type.
 */
export function updateSpecVersion(
	eventType: EventType,
	version: string,
	updater: (sv: SpecVersion) => SpecVersion,
): EventType {
	return {
		...eventType,
		specVersions: eventType.specVersions.map((sv) =>
			sv.version === version ? updater(sv) : sv,
		),
		updatedAt: new Date(),
	};
}

/**
 * Find a spec version by version string.
 */
export function findSpecVersion(
	eventType: EventType,
	version: string,
): SpecVersion | undefined {
	return eventType.specVersions.find((sv) => sv.version === version);
}

/**
 * Check if all spec versions are deprecated.
 */
export function allVersionsDeprecated(eventType: EventType): boolean {
	return (
		eventType.specVersions.length > 0 &&
		eventType.specVersions.every((sv) => sv.status === "DEPRECATED")
	);
}

/**
 * Check if all spec versions are in FINALISING status.
 */
export function allVersionsFinalising(eventType: EventType): boolean {
	return eventType.specVersions.every((sv) => sv.status === "FINALISING");
}

/**
 * Archive an event type.
 */
export function archiveEventType(eventType: EventType): EventType {
	return {
		...eventType,
		status: "ARCHIVED",
		updatedAt: new Date(),
	};
}
