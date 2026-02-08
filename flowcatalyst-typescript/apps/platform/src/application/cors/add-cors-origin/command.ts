/**
 * Add CORS Origin Command
 */

import type { Command } from '@flowcatalyst/application';

export interface AddCorsOriginCommand extends Command {
	readonly origin: string;
	readonly description: string | null;
}
