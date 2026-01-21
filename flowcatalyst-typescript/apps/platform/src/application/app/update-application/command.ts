/**
 * Update Application Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to update an application.
 */
export interface UpdateApplicationCommand extends Command {
	readonly applicationId: string;
	readonly name: string;
	readonly description?: string | null;
	readonly iconUrl?: string | null;
	readonly website?: string | null;
	readonly logo?: string | null;
	readonly logoMimeType?: string | null;
	readonly defaultBaseUrl?: string | null;
}
