/**
 * Event Type Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";
import { SchemaTypeSchema } from "../shared-schemas.js";

export const EventTypeCreatedDataSchema = Type.Object({
	eventTypeId: Type.String(),
	code: Type.String(),
	name: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
});

export const EventTypeUpdatedDataSchema = Type.Object({
	eventTypeId: Type.String(),
	name: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
});

export const EventTypeArchivedDataSchema = Type.Object({
	eventTypeId: Type.String(),
	code: Type.String(),
});

export const EventTypeDeletedDataSchema = Type.Object({
	eventTypeId: Type.String(),
	code: Type.String(),
});

export const SchemaAddedDataSchema = Type.Object({
	eventTypeId: Type.String(),
	version: Type.String(),
	mimeType: Type.String(),
	schemaType: SchemaTypeSchema,
});

export const SchemaFinalisedDataSchema = Type.Object({
	eventTypeId: Type.String(),
	version: Type.String(),
	deprecatedVersion: Type.Union([Type.String(), Type.Null()]),
});

export const SchemaDeprecatedDataSchema = Type.Object({
	eventTypeId: Type.String(),
	version: Type.String(),
});

export const EventTypesSyncedDataSchema = Type.Object({
	applicationCode: Type.String(),
	eventTypesCreated: Type.Integer(),
	eventTypesUpdated: Type.Integer(),
	eventTypesDeleted: Type.Integer(),
	syncedEventTypeCodes: Type.Array(Type.String()),
});
