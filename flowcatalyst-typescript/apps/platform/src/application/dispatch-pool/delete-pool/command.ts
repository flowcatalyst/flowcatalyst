/**
 * Delete Dispatch Pool Command
 */

import type { Command } from '@flowcatalyst/application';

export interface DeleteDispatchPoolCommand extends Command {
  readonly poolId: string;
}
