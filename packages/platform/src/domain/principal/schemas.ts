/**
 * Principal Domain – Event Data Schemas
 */

import { Type } from "@sinclair/typebox";
import { PrincipalScopeSchema, IdpTypeSchema } from "../shared-schemas.js";

export const UserCreatedDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
	emailDomain: Type.String(),
	name: Type.String(),
	scope: PrincipalScopeSchema,
	clientId: Type.Union([Type.String(), Type.Null()]),
	idpType: IdpTypeSchema,
	isAnchorUser: Type.Boolean(),
});

export const UserUpdatedDataSchema = Type.Object({
	userId: Type.String(),
	name: Type.String(),
	previousName: Type.String(),
	scope: Type.Union([PrincipalScopeSchema, Type.Null()]),
	previousScope: Type.Union([PrincipalScopeSchema, Type.Null()]),
	clientId: Type.Union([Type.String(), Type.Null()]),
	previousClientId: Type.Union([Type.String(), Type.Null()]),
});

export const UserActivatedDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
});

export const UserDeactivatedDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
});

export const UserDeletedDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
});

export const RolesAssignedDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
	roles: Type.Array(Type.String()),
	previousRoles: Type.Array(Type.String()),
});

export const ApplicationAccessAssignedDataSchema = Type.Object({
	userId: Type.String(),
	applicationIds: Type.Array(Type.String()),
	added: Type.Array(Type.String()),
	removed: Type.Array(Type.String()),
});

export const ClientAccessGrantedDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
	clientId: Type.String(),
});

export const ClientAccessRevokedDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
	clientId: Type.String(),
});

export const UserLoggedInDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
	loginMethod: Type.Union([Type.Literal("INTERNAL"), Type.Literal("OIDC")]),
	identityProviderCode: Type.Union([Type.String(), Type.Null()]),
	flowcatalystClaims: Type.Object({
		email: Type.String(),
		type: Type.String(),
		roles: Type.Array(Type.String()),
		clients: Type.Array(Type.String()),
		applications: Type.Array(Type.String()),
	}),
	federatedClaims: Type.Union([
		Type.Object({
			accessToken: Type.Record(Type.String(), Type.Unknown()),
			idToken: Type.Record(Type.String(), Type.Unknown()),
		}),
		Type.Null(),
	]),
});

export const PasswordResetRequestedDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
});

export const PasswordResetDataSchema = Type.Object({
	userId: Type.String(),
	email: Type.String(),
});

export const PrincipalsSyncedDataSchema = Type.Object({
	applicationCode: Type.String(),
	principalsCreated: Type.Integer(),
	principalsUpdated: Type.Integer(),
	principalsDeactivated: Type.Integer(),
	syncedEmails: Type.Array(Type.String()),
});
