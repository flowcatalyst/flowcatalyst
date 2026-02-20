/**
 * Deprecate Schema Command
 */

import type { Command } from "@flowcatalyst/application";

export interface DeprecateSchemaCommand extends Command {
	readonly eventTypeId: string;
	readonly version: string;
}
