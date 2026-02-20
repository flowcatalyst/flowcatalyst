/**
 * Update Subscription Command
 */

import type { Command } from "@flowcatalyst/application";
import type {
	EventTypeBinding,
	ConfigEntry,
	DispatchMode,
	SubscriptionStatus,
} from "../../../domain/index.js";

export interface UpdateSubscriptionCommand extends Command {
	readonly subscriptionId: string;
	readonly name?: string | undefined;
	readonly description?: string | null | undefined;
	readonly eventTypes?: EventTypeBinding[] | undefined;
	readonly target?: string | undefined;
	readonly queue?: string | null | undefined;
	readonly customConfig?: ConfigEntry[] | undefined;
	readonly status?: SubscriptionStatus | undefined;
	readonly maxAgeSeconds?: number | undefined;
	readonly dispatchPoolId?: string | null | undefined;
	readonly dispatchPoolCode?: string | null | undefined;
	readonly delaySeconds?: number | undefined;
	readonly sequence?: number | undefined;
	readonly mode?: DispatchMode | undefined;
	readonly timeoutSeconds?: number | undefined;
	readonly maxRetries?: number | undefined;
	readonly serviceAccountId?: string | null | undefined;
	readonly dataOnly?: boolean | undefined;
}
