/**
 * Auth Config Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";
import { AuthConfigTypeSchema, AuthProviderSchema } from "../shared-schemas.js";

export const AuthConfigCreatedDataSchema = Type.Object({
	authConfigId: Type.String(),
	emailDomain: Type.String(),
	configType: AuthConfigTypeSchema,
	authProvider: AuthProviderSchema,
	primaryClientId: Type.Union([Type.String(), Type.Null()]),
});

export const AuthConfigUpdatedDataSchema = Type.Object({
	authConfigId: Type.String(),
	emailDomain: Type.String(),
	configType: AuthConfigTypeSchema,
	authProvider: AuthProviderSchema,
	changes: Type.Record(Type.String(), Type.Unknown()),
});

export const AuthConfigDeletedDataSchema = Type.Object({
	authConfigId: Type.String(),
	emailDomain: Type.String(),
});
