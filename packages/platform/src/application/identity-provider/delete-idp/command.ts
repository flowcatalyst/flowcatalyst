/**
 * Delete Identity Provider Command
 */

import type { Command } from "@flowcatalyst/application";

export interface DeleteIdentityProviderCommand extends Command {
	readonly identityProviderId: string;
}
