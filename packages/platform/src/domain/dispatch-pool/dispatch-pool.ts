/**
 * Dispatch Pool Domain Aggregate
 *
 * A dispatch pool controls the rate at which dispatch jobs can be processed.
 */

import { generate } from "@flowcatalyst/tsid";
import type { DispatchPoolStatus } from "./dispatch-pool-status.js";

export interface DispatchPool {
	readonly id: string;
	readonly code: string;
	readonly name: string;
	readonly description: string | null;
	readonly rateLimit: number;
	readonly concurrency: number;
	readonly clientId: string | null;
	readonly clientIdentifier: string | null;
	readonly status: DispatchPoolStatus;
	readonly createdAt: Date;
	readonly updatedAt: Date;
}

export type NewDispatchPool = Omit<DispatchPool, "createdAt" | "updatedAt"> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new dispatch pool with sensible defaults.
 */
export function createDispatchPool(params: {
	code: string;
	name: string;
	description?: string | null;
	rateLimit?: number;
	concurrency?: number;
	clientId?: string | null;
	clientIdentifier?: string | null;
}): NewDispatchPool {
	return {
		id: generate("DISPATCH_POOL"),
		code: params.code,
		name: params.name,
		description: params.description ?? null,
		rateLimit: params.rateLimit ?? 100,
		concurrency: params.concurrency ?? 10,
		clientId: params.clientId ?? null,
		clientIdentifier: params.clientIdentifier ?? null,
		status: "ACTIVE",
	};
}

/**
 * Update a dispatch pool with partial updates.
 */
export function updateDispatchPool(
	pool: DispatchPool,
	updates: {
		name?: string;
		description?: string | null;
		rateLimit?: number;
		concurrency?: number;
		status?: DispatchPoolStatus;
	},
): DispatchPool {
	return {
		...pool,
		...(updates.name !== undefined ? { name: updates.name } : {}),
		...(updates.description !== undefined
			? { description: updates.description }
			: {}),
		...(updates.rateLimit !== undefined
			? { rateLimit: updates.rateLimit }
			: {}),
		...(updates.concurrency !== undefined
			? { concurrency: updates.concurrency }
			: {}),
		...(updates.status !== undefined ? { status: updates.status } : {}),
	};
}

/**
 * Check if the pool is anchor-level (not client-scoped).
 */
export function isAnchorLevel(pool: DispatchPool): boolean {
	return pool.clientId === null;
}
