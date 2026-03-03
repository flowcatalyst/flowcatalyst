/**
 * Update Connection Command
 */

import type { Command } from "@flowcatalyst/application";
import type { ConnectionStatus } from "../../../domain/index.js";

export interface UpdateConnectionCommand extends Command {
	readonly connectionId: string;
	readonly name?: string | undefined;
	readonly description?: string | null | undefined;
	readonly endpoint?: string | undefined;
	readonly externalId?: string | null | undefined;
	readonly status?: ConnectionStatus | undefined;
	readonly serviceAccountId?: string | undefined;
}
