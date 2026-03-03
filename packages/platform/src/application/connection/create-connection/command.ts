/**
 * Create Connection Command
 */

import type { Command } from "@flowcatalyst/application";

export interface CreateConnectionCommand extends Command {
	readonly code: string;
	readonly name: string;
	readonly description?: string | null | undefined;
	readonly endpoint: string;
	readonly externalId?: string | null | undefined;
	readonly serviceAccountId: string;
	readonly clientId?: string | null | undefined;
}
