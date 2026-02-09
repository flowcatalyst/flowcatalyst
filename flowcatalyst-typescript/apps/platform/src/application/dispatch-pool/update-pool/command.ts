/**
 * Update Dispatch Pool Command
 */

import type { Command } from '@flowcatalyst/application';
import type { DispatchPoolStatus } from '../../../domain/index.js';

export interface UpdateDispatchPoolCommand extends Command {
  readonly poolId: string;
  readonly name?: string | undefined;
  readonly description?: string | null | undefined;
  readonly rateLimit?: number | undefined;
  readonly concurrency?: number | undefined;
  readonly status?: DispatchPoolStatus | undefined;
}
