/**
 * Service Account
 *
 * A projected view that combines a SERVICE-type Principal with its
 * embedded ServiceAccountData. This is the view used by routes and
 * use cases - it flattens the nested structure for convenience.
 */

import type { RoleAssignment } from "../principal/role-assignment.js";
import type { WebhookAuthType } from "./webhook-auth-type.js";
import type { SignatureAlgorithm } from "./signature-algorithm.js";

/**
 * Service Account - projected from Principal + serviceAccount JSONB.
 */
export interface ServiceAccount {
	/** Principal ID */
	readonly id: string;

	/** Unique code (from serviceAccount data) */
	readonly code: string;

	/** Display name (from Principal) */
	readonly name: string;

	/** Description (from serviceAccount data) */
	readonly description: string | null;

	/** Application this service account belongs to (from Principal) */
	readonly applicationId: string | null;

	/** Client IDs this service account can access */
	readonly clientId: string | null;

	/** Whether the service account is active (from Principal) */
	readonly active: boolean;

	/** Webhook auth type */
	readonly whAuthType: WebhookAuthType;

	/** Signing algorithm */
	readonly whSigningAlgorithm: SignatureAlgorithm;

	/** Whether webhook credentials exist */
	readonly hasWebhookCredentials: boolean;

	/** When webhook credentials were created */
	readonly whCredentialsCreatedAt: Date | null;

	/** When webhook credentials were last regenerated */
	readonly whCredentialsRegeneratedAt: Date | null;

	/** When last used for authentication */
	readonly lastUsedAt: Date | null;

	/** Assigned roles */
	readonly roles: readonly RoleAssignment[];

	/** When created */
	readonly createdAt: Date;

	/** When last updated */
	readonly updatedAt: Date;
}
