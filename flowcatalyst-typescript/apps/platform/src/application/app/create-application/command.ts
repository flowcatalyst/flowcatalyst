/**
 * Create Application Command
 */

import type { Command } from '@flowcatalyst/application';
import type { ApplicationType } from '../../../domain/index.js';

/**
 * Command to create a new application.
 */
export interface CreateApplicationCommand extends Command {
	readonly code: string;
	readonly name: string;
	readonly type?: ApplicationType;
	readonly description?: string | null;
	readonly iconUrl?: string | null;
	readonly website?: string | null;
	readonly logo?: string | null;
	readonly logoMimeType?: string | null;
	readonly defaultBaseUrl?: string | null;
}
