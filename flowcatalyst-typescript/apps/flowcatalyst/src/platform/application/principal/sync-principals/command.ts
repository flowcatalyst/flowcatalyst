/**
 * Sync Principals Command
 */

import type { Command } from '@flowcatalyst/application';

export interface SyncPrincipalItem {
  /** User's email address (unique identifier for matching) */
  readonly email: string;
  /** Display name */
  readonly name: string;
  /** Role short names to assign (will be prefixed with applicationCode) */
  readonly roles?: string[];
  /** Whether the user is active (default: true) */
  readonly active?: boolean;
}

export interface SyncPrincipalsCommand extends Command {
  readonly applicationCode: string;
  readonly principals: SyncPrincipalItem[];
  readonly removeUnlisted: boolean;
}
