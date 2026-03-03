/**
 * Email Domain Mapping Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";

export const EmailDomainMappingCreatedDataSchema = Type.Object({
	emailDomainMappingId: Type.String(),
	emailDomain: Type.String(),
	identityProviderId: Type.String(),
	scopeType: Type.String(),
	primaryClientId: Type.Union([Type.String(), Type.Null()]),
	additionalClientIds: Type.Array(Type.String()),
	grantedClientIds: Type.Array(Type.String()),
});

export const EmailDomainMappingUpdatedDataSchema = Type.Object({
	emailDomainMappingId: Type.String(),
	emailDomain: Type.String(),
	identityProviderId: Type.String(),
	scopeType: Type.String(),
	primaryClientId: Type.Union([Type.String(), Type.Null()]),
	additionalClientIds: Type.Array(Type.String()),
	grantedClientIds: Type.Array(Type.String()),
});

export const EmailDomainMappingDeletedDataSchema = Type.Object({
	emailDomainMappingId: Type.String(),
	emailDomain: Type.String(),
	identityProviderId: Type.String(),
});
