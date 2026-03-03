/**
 * Delete Connection Command
 */

import type { Command } from "@flowcatalyst/application";

export interface DeleteConnectionCommand extends Command {
	readonly connectionId: string;
}
