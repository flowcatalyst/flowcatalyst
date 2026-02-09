/**
 * Regenerate Signing Secret Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to regenerate a service account's webhook signing secret.
 * The new plaintext secret is returned once in the event data.
 */
export interface RegenerateSigningSecretCommand extends Command {
  /** Principal ID of the service account */
  readonly serviceAccountId: string;
}
