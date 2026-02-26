/**
 * Delete CORS Origin Command
 */

import type { Command } from "@flowcatalyst/application";

export interface DeleteCorsOriginCommand extends Command {
	readonly originId: string;
}
