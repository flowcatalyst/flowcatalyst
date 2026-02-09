/**
 * Regenerate Auth Token Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to regenerate a service account's webhook auth token.
 * The new plaintext token is returned once in the event data.
 */
export interface RegenerateAuthTokenCommand extends Command {
  /** Principal ID of the service account */
  readonly serviceAccountId: string;
}
