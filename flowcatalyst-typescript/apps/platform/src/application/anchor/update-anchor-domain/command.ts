/**
 * Update Anchor Domain Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to update an anchor domain.
 */
export interface UpdateAnchorDomainCommand extends Command {
	readonly anchorDomainId: string;
	readonly domain: string;
}
