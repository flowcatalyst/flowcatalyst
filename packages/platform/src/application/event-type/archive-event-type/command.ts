/**
 * Archive EventType Command
 */

import type { Command } from "@flowcatalyst/application";

export interface ArchiveEventTypeCommand extends Command {
	readonly eventTypeId: string;
}
