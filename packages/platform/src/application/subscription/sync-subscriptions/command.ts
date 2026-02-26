/**
 * Sync Subscriptions Command
 */

import type { Command } from "@flowcatalyst/application";
import type {
	EventTypeBinding,
	ConfigEntry,
	DispatchMode,
} from "../../../domain/index.js";

export interface SyncSubscriptionItem {
	readonly code: string;
	readonly name: string;
	readonly description?: string | null | undefined;
	readonly clientScoped?: boolean | undefined;
	readonly eventTypes: EventTypeBinding[];
	readonly target: string;
	readonly queue?: string | null | undefined;
	readonly customConfig?: ConfigEntry[] | undefined;
	readonly maxAgeSeconds?: number | undefined;
	readonly dispatchPoolCode?: string | null | undefined;
	readonly delaySeconds?: number | undefined;
	readonly sequence?: number | undefined;
	readonly mode?: DispatchMode | undefined;
	readonly timeoutSeconds?: number | undefined;
	readonly maxRetries?: number | undefined;
	readonly dataOnly?: boolean | undefined;
}

export interface SyncSubscriptionsCommand extends Command {
	readonly applicationCode: string;
	readonly subscriptions: SyncSubscriptionItem[];
	readonly removeUnlisted: boolean;
}
