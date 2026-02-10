/**
 * Delete EventType Command
 */

import type { Command } from '@flowcatalyst/application';

export interface DeleteEventTypeCommand extends Command {
  readonly eventTypeId: string;
}
