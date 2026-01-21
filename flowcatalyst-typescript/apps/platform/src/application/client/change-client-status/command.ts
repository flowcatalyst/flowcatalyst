/**
 * Change Client Status Command
 */

import type { Command } from '@flowcatalyst/application';
import type { ClientStatus } from '../../../domain/client/client-status.js';

/**
 * Command to change a client's status.
 */
export interface ChangeClientStatusCommand extends Command {
	readonly clientId: string;
	readonly newStatus: ClientStatus;
	readonly reason: string | null;
	readonly note: string | null;
}
