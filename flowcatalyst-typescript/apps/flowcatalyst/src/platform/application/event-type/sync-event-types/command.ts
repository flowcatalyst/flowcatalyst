/**
 * Sync EventTypes Command
 *
 * Syncs event types from an application SDK.
 */

import type { Command } from "@flowcatalyst/application";

export interface SyncEventTypeItem {
	readonly subdomain: string;
	readonly aggregate: string;
	readonly event: string;
	readonly name: string;
	readonly description?: string | null;
	readonly clientScoped?: boolean;
}

export interface SyncEventTypesCommand extends Command {
	readonly applicationCode: string;
	readonly eventTypes: SyncEventTypeItem[];
	readonly removeUnlisted?: boolean;
}
