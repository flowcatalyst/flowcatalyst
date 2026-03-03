/**
 * Platform Event Schema Registry
 *
 * Maps each platform event type code to its TypeBox schema.
 * TypeBox schemas ARE JSON Schema objects at runtime — no conversion needed.
 */

import type { TObject } from "@sinclair/typebox";

// ─── Principal (IAM > User) ─────────────────────────────────────────────────
import {
	UserCreatedDataSchema,
	UserUpdatedDataSchema,
	UserActivatedDataSchema,
	UserDeactivatedDataSchema,
	UserDeletedDataSchema,
	RolesAssignedDataSchema,
	ApplicationAccessAssignedDataSchema,
	ClientAccessGrantedDataSchema,
	ClientAccessRevokedDataSchema,
	UserLoggedInDataSchema,
	PasswordResetRequestedDataSchema,
	PasswordResetDataSchema,
	PrincipalsSyncedDataSchema,
} from "./principal/schemas.js";

// ─── Service Account (IAM) ──────────────────────────────────────────────────
import {
	ServiceAccountCreatedDataSchema,
	ServiceAccountUpdatedDataSchema,
	AuthTokenRegeneratedDataSchema,
	SigningSecretRegeneratedDataSchema,
	ServiceAccountDeletedDataSchema,
} from "./service-account/schemas.js";

// ─── OAuth (IAM) ────────────────────────────────────────────────────────────
import {
	OAuthClientCreatedDataSchema,
	OAuthClientUpdatedDataSchema,
	OAuthClientSecretRegeneratedDataSchema,
	OAuthClientDeletedDataSchema,
} from "./oauth/schemas.js";

// ─── Identity Provider (IAM) ────────────────────────────────────────────────
import {
	IdentityProviderCreatedDataSchema,
	IdentityProviderUpdatedDataSchema,
	IdentityProviderDeletedDataSchema,
} from "./identity-provider/schemas.js";

// ─── Auth Config (IAM) ─────────────────────────────────────────────────────
import {
	AuthConfigCreatedDataSchema,
	AuthConfigUpdatedDataSchema,
	AuthConfigDeletedDataSchema,
} from "./auth-config/schemas.js";

// ─── Email Domain Mapping (IAM) ─────────────────────────────────────────────
import {
	EmailDomainMappingCreatedDataSchema,
	EmailDomainMappingUpdatedDataSchema,
	EmailDomainMappingDeletedDataSchema,
} from "./email-domain-mapping/schemas.js";

// ─── Client (Admin) ─────────────────────────────────────────────────────────
import {
	ClientCreatedDataSchema,
	ClientUpdatedDataSchema,
	ClientStatusChangedDataSchema,
	ClientDeletedDataSchema,
	ClientNoteAddedDataSchema,
} from "./client/schemas.js";

// ─── Anchor (Admin) ─────────────────────────────────────────────────────────
import {
	AnchorDomainCreatedDataSchema,
	AnchorDomainUpdatedDataSchema,
	AnchorDomainDeletedDataSchema,
} from "./anchor/schemas.js";

// ─── Application (Admin) ────────────────────────────────────────────────────
import {
	ApplicationCreatedDataSchema,
	ApplicationUpdatedDataSchema,
	ApplicationActivatedDataSchema,
	ApplicationDeactivatedDataSchema,
	ApplicationDeletedDataSchema,
	ApplicationEnabledForClientDataSchema,
	ApplicationDisabledForClientDataSchema,
} from "./application/schemas.js";

// ─── Role (Admin) ───────────────────────────────────────────────────────────
import {
	RoleCreatedDataSchema,
	RoleUpdatedDataSchema,
	RoleDeletedDataSchema,
	RolesSyncedDataSchema,
} from "./role/schemas.js";

// ─── CORS (Admin) ───────────────────────────────────────────────────────────
import {
	CorsOriginAddedDataSchema,
	CorsOriginDeletedDataSchema,
} from "./cors/schemas.js";

// ─── Event Type (Admin) ─────────────────────────────────────────────────────
import {
	EventTypeCreatedDataSchema,
	EventTypeUpdatedDataSchema,
	EventTypeArchivedDataSchema,
	EventTypeDeletedDataSchema,
	SchemaAddedDataSchema,
	SchemaFinalisedDataSchema,
	SchemaDeprecatedDataSchema,
	EventTypesSyncedDataSchema,
} from "./event-type/schemas.js";

// ─── Dispatch Pool (Control Plane) ──────────────────────────────────────────
import {
	DispatchPoolCreatedDataSchema,
	DispatchPoolUpdatedDataSchema,
	DispatchPoolDeletedDataSchema,
	DispatchPoolsSyncedDataSchema,
} from "./dispatch-pool/schemas.js";

// ─── Subscription (Control Plane / Messaging) ──────────────────────────────
import {
	SubscriptionCreatedDataSchema,
	SubscriptionUpdatedDataSchema,
	SubscriptionDeletedDataSchema,
	SubscriptionsSyncedDataSchema,
} from "./subscription/schemas.js";

/**
 * Map of event type code → TypeBox JSON Schema for event data payload.
 *
 * Codes follow the pattern: platform:{subdomain}:{aggregate}:{event}
 * Matches PLATFORM_EVENT_DEFINITIONS in platform-event-registry.ts.
 */
// eslint-disable-next-line @typescript-eslint/no-explicit-any
type AnySchema = TObject<any>;

export const PLATFORM_EVENT_SCHEMAS: ReadonlyMap<string, AnySchema> = new Map<string, AnySchema>([
	// ─── IAM > User ──────────────────────────────────────────────────────
	["platform:iam:user:created", UserCreatedDataSchema],
	["platform:iam:user:updated", UserUpdatedDataSchema],
	["platform:iam:user:activated", UserActivatedDataSchema],
	["platform:iam:user:deactivated", UserDeactivatedDataSchema],
	["platform:iam:user:deleted", UserDeletedDataSchema],
	["platform:iam:user:roles-assigned", RolesAssignedDataSchema],
	["platform:iam:user:application-access-assigned", ApplicationAccessAssignedDataSchema],
	["platform:iam:user:client-access-granted", ClientAccessGrantedDataSchema],
	["platform:iam:user:client-access-revoked", ClientAccessRevokedDataSchema],
	["platform:iam:user:logged-in", UserLoggedInDataSchema],
	["platform:iam:user:password-reset-requested", PasswordResetRequestedDataSchema],
	["platform:iam:user:password-reset", PasswordResetDataSchema],
	["platform:iam:user:principals-synced", PrincipalsSyncedDataSchema],

	// ─── IAM > Service Account ───────────────────────────────────────────
	["platform:iam:service-account:created", ServiceAccountCreatedDataSchema],
	["platform:iam:service-account:updated", ServiceAccountUpdatedDataSchema],
	["platform:iam:service-account:auth-token-regenerated", AuthTokenRegeneratedDataSchema],
	["platform:iam:service-account:signing-secret-regenerated", SigningSecretRegeneratedDataSchema],
	["platform:iam:service-account:deleted", ServiceAccountDeletedDataSchema],

	// ─── IAM > OAuth Client ──────────────────────────────────────────────
	["platform:iam:oauth-client:created", OAuthClientCreatedDataSchema],
	["platform:iam:oauth-client:updated", OAuthClientUpdatedDataSchema],
	["platform:iam:oauth-client:secret-regenerated", OAuthClientSecretRegeneratedDataSchema],
	["platform:iam:oauth-client:deleted", OAuthClientDeletedDataSchema],

	// ─── IAM > Identity Provider ─────────────────────────────────────────
	["platform:iam:identity-provider:created", IdentityProviderCreatedDataSchema],
	["platform:iam:identity-provider:updated", IdentityProviderUpdatedDataSchema],
	["platform:iam:identity-provider:deleted", IdentityProviderDeletedDataSchema],

	// ─── IAM > Auth Config ───────────────────────────────────────────────
	["platform:iam:auth-config:created", AuthConfigCreatedDataSchema],
	["platform:iam:auth-config:updated", AuthConfigUpdatedDataSchema],
	["platform:iam:auth-config:deleted", AuthConfigDeletedDataSchema],

	// ─── IAM > Email Domain Mapping ──────────────────────────────────────
	["platform:iam:email-domain-mapping:created", EmailDomainMappingCreatedDataSchema],
	["platform:iam:email-domain-mapping:updated", EmailDomainMappingUpdatedDataSchema],
	["platform:iam:email-domain-mapping:deleted", EmailDomainMappingDeletedDataSchema],

	// ─── Admin > Client ──────────────────────────────────────────────────
	["platform:admin:client:created", ClientCreatedDataSchema],
	["platform:admin:client:updated", ClientUpdatedDataSchema],
	["platform:admin:client:status-changed", ClientStatusChangedDataSchema],
	["platform:admin:client:deleted", ClientDeletedDataSchema],
	["platform:admin:client:note-added", ClientNoteAddedDataSchema],

	// ─── Admin > Anchor Domain ───────────────────────────────────────────
	["platform:admin:anchor-domain:created", AnchorDomainCreatedDataSchema],
	["platform:admin:anchor-domain:updated", AnchorDomainUpdatedDataSchema],
	["platform:admin:anchor-domain:deleted", AnchorDomainDeletedDataSchema],

	// ─── Admin > Application ─────────────────────────────────────────────
	["platform:admin:application:created", ApplicationCreatedDataSchema],
	["platform:admin:application:updated", ApplicationUpdatedDataSchema],
	["platform:admin:application:activated", ApplicationActivatedDataSchema],
	["platform:admin:application:deactivated", ApplicationDeactivatedDataSchema],
	["platform:admin:application:deleted", ApplicationDeletedDataSchema],
	["platform:admin:application:enabled-for-client", ApplicationEnabledForClientDataSchema],
	["platform:admin:application:disabled-for-client", ApplicationDisabledForClientDataSchema],

	// ─── Admin > Role ────────────────────────────────────────────────────
	["platform:admin:role:created", RoleCreatedDataSchema],
	["platform:admin:role:updated", RoleUpdatedDataSchema],
	["platform:admin:role:deleted", RoleDeletedDataSchema],
	["platform:admin:role:synced", RolesSyncedDataSchema],

	// ─── Admin > CORS Origin ─────────────────────────────────────────────
	["platform:admin:cors-origin:added", CorsOriginAddedDataSchema],
	["platform:admin:cors-origin:deleted", CorsOriginDeletedDataSchema],

	// ─── Admin > Event Type ──────────────────────────────────────────────
	["platform:admin:event-type:created", EventTypeCreatedDataSchema],
	["platform:admin:event-type:updated", EventTypeUpdatedDataSchema],
	["platform:admin:event-type:archived", EventTypeArchivedDataSchema],
	["platform:admin:event-type:deleted", EventTypeDeletedDataSchema],
	["platform:admin:event-type:schema-added", SchemaAddedDataSchema],
	["platform:admin:event-type:schema-finalised", SchemaFinalisedDataSchema],
	["platform:admin:event-type:schema-deprecated", SchemaDeprecatedDataSchema],
	["platform:admin:event-type:synced", EventTypesSyncedDataSchema],

	// ─── Control Plane > Dispatch Pool ───────────────────────────────────
	["platform:control-plane:dispatch-pool:created", DispatchPoolCreatedDataSchema],
	["platform:control-plane:dispatch-pool:updated", DispatchPoolUpdatedDataSchema],
	["platform:control-plane:dispatch-pool:deleted", DispatchPoolDeletedDataSchema],
	["platform:control-plane:dispatch-pool:synced", DispatchPoolsSyncedDataSchema],

	// ─── Control Plane > Subscription ────────────────────────────────────
	["platform:control-plane:subscription:created", SubscriptionCreatedDataSchema],
	["platform:control-plane:subscription:updated", SubscriptionUpdatedDataSchema],
	["platform:control-plane:subscription:deleted", SubscriptionDeletedDataSchema],

	// ─── Messaging > Subscription ────────────────────────────────────────
	["platform:messaging:subscription:synced", SubscriptionsSyncedDataSchema],
]);
