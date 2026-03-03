/**
 * Shared TypeBox Schemas
 *
 * Reusable TypeBox definitions for string literal unions and shared
 * types referenced across multiple domain event data schemas.
 */

import { Type } from "@sinclair/typebox";

export const PrincipalScopeSchema = Type.Union([
	Type.Literal("ANCHOR"),
	Type.Literal("PARTNER"),
	Type.Literal("CLIENT"),
]);

export const IdpTypeSchema = Type.Union([
	Type.Literal("INTERNAL"),
	Type.Literal("OIDC"),
]);

export const ClientStatusSchema = Type.Union([
	Type.Literal("ACTIVE"),
	Type.Literal("INACTIVE"),
	Type.Literal("SUSPENDED"),
]);

export const ApplicationTypeSchema = Type.Union([
	Type.Literal("APPLICATION"),
	Type.Literal("INTEGRATION"),
]);

export const RoleSourceSchema = Type.Union([
	Type.Literal("CODE"),
	Type.Literal("DATABASE"),
	Type.Literal("SDK"),
]);

export const AuthConfigTypeSchema = Type.Union([
	Type.Literal("ANCHOR"),
	Type.Literal("PARTNER"),
	Type.Literal("CLIENT"),
]);

export const AuthProviderSchema = Type.Union([
	Type.Literal("INTERNAL"),
	Type.Literal("OIDC"),
]);

export const OAuthClientTypeSchema = Type.Union([
	Type.Literal("PUBLIC"),
	Type.Literal("CONFIDENTIAL"),
]);

export const SchemaTypeSchema = Type.Union([
	Type.Literal("JSON_SCHEMA"),
	Type.Literal("PROTO"),
	Type.Literal("XSD"),
]);

export const EventTypeBindingSchema = Type.Object({
	eventTypeId: Type.Union([Type.String(), Type.Null()]),
	eventTypeCode: Type.String(),
	specVersion: Type.Union([Type.String(), Type.Null()]),
});
