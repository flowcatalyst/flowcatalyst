/**
 * Create Service Account Command
 */

import type { Command } from '@flowcatalyst/application';
import type { WebhookAuthType } from '../../../domain/index.js';

/**
 * Command to create a new service account.
 *
 * Atomically creates:
 * - A SERVICE-type Principal with embedded service account data
 * - A CONFIDENTIAL OAuthClient for client_credentials authentication
 * - Encrypted webhook auth token and signing secret
 */
export interface CreateServiceAccountCommand extends Command {
  /** Unique code for the service account (e.g., "my-app-service") */
  readonly code: string;

  /** Display name for the service account */
  readonly name: string;

  /** Optional description */
  readonly description: string | null;

  /** Application ID this service account belongs to (optional) */
  readonly applicationId: string | null;

  /** Client ID this service account is scoped to (optional) */
  readonly clientId: string | null;

  /** Webhook authentication type */
  readonly webhookAuthType?: WebhookAuthType | undefined;
}
