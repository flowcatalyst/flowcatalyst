/**
 * Connection Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";

export const ConnectionCreatedDataSchema = Type.Object({
	connectionId: Type.String(),
	code: Type.String(),
	name: Type.String(),
	endpoint: Type.String(),
	externalId: Type.Union([Type.String(), Type.Null()]),
	serviceAccountId: Type.String(),
	clientId: Type.Union([Type.String(), Type.Null()]),
});

export const ConnectionUpdatedDataSchema = Type.Object({
	connectionId: Type.String(),
	code: Type.String(),
	name: Type.String(),
	endpoint: Type.String(),
	externalId: Type.Union([Type.String(), Type.Null()]),
	status: Type.String(),
});

export const ConnectionDeletedDataSchema = Type.Object({
	connectionId: Type.String(),
	code: Type.String(),
	clientId: Type.Union([Type.String(), Type.Null()]),
});
