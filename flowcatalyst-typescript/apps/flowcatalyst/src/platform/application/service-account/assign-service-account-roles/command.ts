/**
 * Assign Service Account Roles Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to assign roles to a service account.
 */
export interface AssignServiceAccountRolesCommand extends Command {
  /** Principal ID of the service account */
  readonly serviceAccountId: string;

  /** Role names to assign (replaces existing roles) */
  readonly roles: readonly string[];
}
