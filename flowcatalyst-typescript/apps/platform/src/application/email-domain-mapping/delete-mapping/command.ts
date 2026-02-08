/**
 * Delete Email Domain Mapping Command
 */

import type { Command } from '@flowcatalyst/application';

export interface DeleteEmailDomainMappingCommand extends Command {
	readonly emailDomainMappingId: string;
}
