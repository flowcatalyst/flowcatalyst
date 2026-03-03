/**
 * Service Account Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";

export const ServiceAccountCreatedDataSchema = Type.Object({
	serviceAccountId: Type.String(),
	principalId: Type.String(),
	oauthClientId: Type.String(),
	code: Type.String(),
	name: Type.String(),
	applicationId: Type.Union([Type.String(), Type.Null()]),
});

export const ServiceAccountUpdatedDataSchema = Type.Object({
	serviceAccountId: Type.String(),
	code: Type.String(),
});

export const AuthTokenRegeneratedDataSchema = Type.Object({
	serviceAccountId: Type.String(),
	code: Type.String(),
});

export const SigningSecretRegeneratedDataSchema = Type.Object({
	serviceAccountId: Type.String(),
	code: Type.String(),
});

export const ServiceAccountDeletedDataSchema = Type.Object({
	serviceAccountId: Type.String(),
	code: Type.String(),
});
