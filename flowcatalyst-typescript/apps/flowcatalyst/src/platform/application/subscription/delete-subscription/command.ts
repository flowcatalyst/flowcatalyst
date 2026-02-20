/**
 * Delete Subscription Command
 */

import type { Command } from "@flowcatalyst/application";

export interface DeleteSubscriptionCommand extends Command {
	readonly subscriptionId: string;
}
