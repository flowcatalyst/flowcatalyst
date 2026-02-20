/**
 * Sync Roles Command
 */

import type { Command } from "@flowcatalyst/application";

export interface SyncRoleItem {
	readonly name: string;
	readonly displayName?: string;
	readonly description?: string | null;
	readonly permissions?: string[];
	readonly clientManaged?: boolean;
}

export interface SyncRolesCommand extends Command {
	readonly applicationCode: string;
	readonly roles: SyncRoleItem[];
	readonly removeUnlisted: boolean;
}
