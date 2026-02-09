/**
 * Service Account Data
 *
 * Data stored as JSONB in the Principal record for SERVICE type principals.
 * This is the embedded data model - the full ServiceAccount view is a projection
 * that combines this with the Principal fields.
 */

import type { WebhookAuthType } from './webhook-auth-type.js';
import type { SignatureAlgorithm } from './signature-algorithm.js';

/**
 * Embedded service account data within a Principal.
 */
export interface ServiceAccountData {
  /** Unique code for the service account (e.g., "my-app-service") */
  readonly code: string;

  /** Optional description */
  readonly description: string | null;

  /** Webhook authentication type */
  readonly whAuthType: WebhookAuthType;

  /** Encrypted reference to the webhook auth token */
  readonly whAuthTokenRef: string | null;

  /** Encrypted reference to the webhook signing secret */
  readonly whSigningSecretRef: string | null;

  /** Signing algorithm for webhook signatures */
  readonly whSigningAlgorithm: SignatureAlgorithm;

  /** When webhook credentials were first created */
  readonly whCredentialsCreatedAt: Date | null;

  /** When webhook credentials were last regenerated */
  readonly whCredentialsRegeneratedAt: Date | null;

  /** When the service account was last used for authentication */
  readonly lastUsedAt: Date | null;
}

/**
 * Create initial service account data for a new service account.
 */
export function createServiceAccountData(params: {
  code: string;
  description: string | null;
  whAuthType?: WebhookAuthType | undefined;
  whAuthTokenRef?: string | null | undefined;
  whSigningSecretRef?: string | null | undefined;
  whSigningAlgorithm?: SignatureAlgorithm | undefined;
}): ServiceAccountData {
  const now = new Date();
  return {
    code: params.code,
    description: params.description,
    whAuthType: params.whAuthType ?? 'BEARER_TOKEN',
    whAuthTokenRef: params.whAuthTokenRef ?? null,
    whSigningSecretRef: params.whSigningSecretRef ?? null,
    whSigningAlgorithm: params.whSigningAlgorithm ?? 'HMAC_SHA256',
    whCredentialsCreatedAt: params.whAuthTokenRef ? now : null,
    whCredentialsRegeneratedAt: null,
    lastUsedAt: null,
  };
}
