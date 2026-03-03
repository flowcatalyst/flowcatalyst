/**
 * Identity Provider Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";

export const IdentityProviderCreatedDataSchema = Type.Object({
	identityProviderId: Type.String(),
	code: Type.String(),
	name: Type.String(),
	type: Type.String(),
});

export const IdentityProviderUpdatedDataSchema = Type.Object({
	identityProviderId: Type.String(),
	name: Type.String(),
	type: Type.String(),
});

export const IdentityProviderDeletedDataSchema = Type.Object({
	identityProviderId: Type.String(),
	code: Type.String(),
});
