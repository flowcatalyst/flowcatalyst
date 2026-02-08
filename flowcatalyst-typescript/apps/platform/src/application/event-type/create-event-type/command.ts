/**
 * Create EventType Command
 */

import type { Command } from '@flowcatalyst/application';

export interface CreateEventTypeCommand extends Command {
	readonly application: string;
	readonly subdomain: string;
	readonly aggregate: string;
	readonly event: string;
	readonly name: string;
	readonly description?: string | null;
	readonly clientScoped?: boolean;
}
