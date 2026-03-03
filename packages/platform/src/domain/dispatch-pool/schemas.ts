/**
 * Dispatch Pool Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";

export const DispatchPoolCreatedDataSchema = Type.Object({
	poolId: Type.String(),
	code: Type.String(),
	name: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
	rateLimit: Type.Integer(),
	concurrency: Type.Integer(),
	clientId: Type.Union([Type.String(), Type.Null()]),
	clientIdentifier: Type.Union([Type.String(), Type.Null()]),
	status: Type.String(),
});

export const DispatchPoolUpdatedDataSchema = Type.Object({
	poolId: Type.String(),
	code: Type.String(),
	name: Type.String(),
	description: Type.Union([Type.String(), Type.Null()]),
	rateLimit: Type.Integer(),
	concurrency: Type.Integer(),
	status: Type.String(),
});

export const DispatchPoolDeletedDataSchema = Type.Object({
	poolId: Type.String(),
	code: Type.String(),
	clientId: Type.Union([Type.String(), Type.Null()]),
});

export const DispatchPoolsSyncedDataSchema = Type.Object({
	applicationCode: Type.String(),
	poolsCreated: Type.Integer(),
	poolsUpdated: Type.Integer(),
	poolsDeleted: Type.Integer(),
	syncedPoolCodes: Type.Array(Type.String()),
});
