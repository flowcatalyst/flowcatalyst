/**
 * OAuth Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";
import { OAuthClientTypeSchema } from "../shared-schemas.js";

export const OAuthClientCreatedDataSchema = Type.Object({
	oauthClientId: Type.String(),
	clientId: Type.String(),
	clientName: Type.String(),
	clientType: OAuthClientTypeSchema,
});

export const OAuthClientUpdatedDataSchema = Type.Object({
	oauthClientId: Type.String(),
	clientId: Type.String(),
	changes: Type.Record(Type.String(), Type.Unknown()),
});

export const OAuthClientSecretRegeneratedDataSchema = Type.Object({
	oauthClientId: Type.String(),
	clientId: Type.String(),
});

export const OAuthClientDeletedDataSchema = Type.Object({
	oauthClientId: Type.String(),
	clientId: Type.String(),
});
