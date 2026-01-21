/**
 * Create Anchor Domain Command
 */

import type { Command } from '@flowcatalyst/application';

/**
 * Command to create a new anchor domain.
 */
export interface CreateAnchorDomainCommand extends Command {
	readonly domain: string;
}
