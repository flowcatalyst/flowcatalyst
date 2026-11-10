// Compile-time contract, type-checked by subscription-mode.test.ts: the type
// must be EXACTLY the platform's dispatch modes (internal/common DispatchMode:
// IMMEDIATE | NEXT_ON_ERROR | BLOCK_ON_ERROR). A Record over the union fails
// to compile if a member is missing or an extra one is present.
import type { SubscriptionMode } from "../src/sync/definitions.js";

export const MODES: Record<SubscriptionMode, true> = {
	IMMEDIATE: true,
	NEXT_ON_ERROR: true,
	BLOCK_ON_ERROR: true,
};
