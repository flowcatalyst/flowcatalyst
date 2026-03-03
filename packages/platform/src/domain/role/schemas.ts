/**
 * Role Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";
import { RoleSourceSchema } from "../shared-schemas.js";

export const RoleCreatedDataSchema = Type.Object({
	roleId: Type.String(),
	name: Type.String(),
	displayName: Type.String(),
	applicationId: Type.Union([Type.String(), Type.Null()]),
	applicationCode: Type.Union([Type.String(), Type.Null()]),
	source: RoleSourceSchema,
	permissions: Type.Array(Type.String()),
});

export const RoleUpdatedDataSchema = Type.Object({
	roleId: Type.String(),
	displayName: Type.String(),
	permissions: Type.Array(Type.String()),
	clientManaged: Type.Boolean(),
});

export const RoleDeletedDataSchema = Type.Object({
	roleId: Type.String(),
	name: Type.String(),
});

export const RolesSyncedDataSchema = Type.Object({
	applicationCode: Type.String(),
	rolesCreated: Type.Integer(),
	rolesUpdated: Type.Integer(),
	rolesDeleted: Type.Integer(),
	syncedRoleNames: Type.Array(Type.String()),
});
