/**
 * Delete Anchor Domain Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to delete an anchor domain.
 */
export interface DeleteAnchorDomainCommand extends Command {
	readonly anchorDomainId: string;
}
