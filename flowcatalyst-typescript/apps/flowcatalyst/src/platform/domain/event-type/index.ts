/**
 * EventType Domain
 *
 * Exports for event type entities and related types.
 */

export {
	type EventType,
	type NewEventType,
	parseCodeSegments,
	buildCode,
	createEventType,
	createEventTypeFromApi,
	updateEventType,
	addSpecVersion,
	updateSpecVersion,
	findSpecVersion,
	allVersionsDeprecated,
	allVersionsFinalising,
	archiveEventType,
} from "./event-type.js";
export {
	type SpecVersion,
	type NewSpecVersion,
	createSpecVersion,
	majorVersion,
	minorVersion,
	withStatus,
} from "./spec-version.js";
export {
	type EventTypeStatus,
	EventTypeStatus as EventTypeStatusEnum,
} from "./event-type-status.js";
export {
	type EventTypeSource,
	EventTypeSource as EventTypeSourceEnum,
} from "./event-type-source.js";
export {
	type SchemaType,
	SchemaType as SchemaTypeEnum,
} from "./schema-type.js";
export {
	type SpecVersionStatus,
	SpecVersionStatus as SpecVersionStatusEnum,
} from "./spec-version-status.js";
export {
	type EventTypeCreatedData,
	EventTypeCreated,
	type EventTypeUpdatedData,
	EventTypeUpdated,
	type EventTypeArchivedData,
	EventTypeArchived,
	type EventTypeDeletedData,
	EventTypeDeleted,
	type SchemaAddedData,
	SchemaAdded,
	type SchemaFinalisedData,
	SchemaFinalised,
	type SchemaDeprecatedData,
	SchemaDeprecated,
	type EventTypesSyncedData,
	EventTypesSynced,
} from "./events.js";
