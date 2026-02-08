/**
 * Update EventType Command
 */

import type { Command } from '@flowcatalyst/application';

export interface UpdateEventTypeCommand extends Command {
	readonly eventTypeId: string;
	readonly name?: string | undefined;
	readonly description?: string | null | undefined;
}
