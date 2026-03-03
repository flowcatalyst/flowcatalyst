/**
 * Client Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";
import { ClientStatusSchema } from "../shared-schemas.js";

export const ClientCreatedDataSchema = Type.Object({
	clientId: Type.String(),
	name: Type.String(),
	identifier: Type.String(),
});

export const ClientUpdatedDataSchema = Type.Object({
	clientId: Type.String(),
	name: Type.String(),
	previousName: Type.String(),
});

export const ClientStatusChangedDataSchema = Type.Object({
	clientId: Type.String(),
	name: Type.String(),
	previousStatus: ClientStatusSchema,
	newStatus: ClientStatusSchema,
	reason: Type.Union([Type.String(), Type.Null()]),
});

export const ClientDeletedDataSchema = Type.Object({
	clientId: Type.String(),
	name: Type.String(),
	identifier: Type.String(),
});

export const ClientNoteAddedDataSchema = Type.Object({
	clientId: Type.String(),
	category: Type.String(),
	text: Type.String(),
	addedBy: Type.String(),
});
