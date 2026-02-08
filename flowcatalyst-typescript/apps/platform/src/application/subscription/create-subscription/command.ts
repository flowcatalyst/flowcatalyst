/**
 * Create Subscription Command
 */

import type { Command } from '@flowcatalyst/application';
import type { EventTypeBinding, ConfigEntry, DispatchMode } from '../../../domain/index.js';

export interface CreateSubscriptionCommand extends Command {
	readonly code: string;
	readonly applicationCode?: string | null | undefined;
	readonly name: string;
	readonly description?: string | null | undefined;
	readonly clientId?: string | null | undefined;
	readonly clientScoped?: boolean | undefined;
	readonly eventTypes: EventTypeBinding[];
	readonly target: string;
	readonly queue?: string | null | undefined;
	readonly customConfig?: ConfigEntry[] | undefined;
	readonly source?: string | undefined;
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
