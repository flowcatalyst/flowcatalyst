/**
 * Finalise Schema Command
 */

import type { Command } from '@flowcatalyst/application';

export interface FinaliseSchemaCommand extends Command {
  readonly eventTypeId: string;
  readonly version: string;
}
