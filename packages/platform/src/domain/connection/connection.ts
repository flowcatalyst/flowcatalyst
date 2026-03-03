/**
 * Connection Domain Aggregate
 *
 * A named endpoint + credentials grouping that sits between
 * ServiceAccount (WHO) and Subscription (WHAT).
 * Provides pause/unpause semantics for delivery control.
 */

import { generate } from "@flowcatalyst/tsid";
import type { ConnectionStatus } from "./connection-status.js";

export interface Connection {
	readonly id: string;
	readonly code: string;
	readonly name: string;
	readonly description: string | null;
	readonly endpoint: string;
	readonly externalId: string | null;
	readonly status: ConnectionStatus;
	readonly serviceAccountId: string;
	readonly clientId: string | null;
	readonly clientIdentifier: string | null;
	readonly createdAt: Date;
	readonly updatedAt: Date;
}

export type NewConnection = Omit<Connection, "createdAt" | "updatedAt"> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new connection with sensible defaults.
 */
export function createConnection(params: {
	code: string;
	name: string;
	description?: string | null;
	endpoint: string;
	externalId?: string | null;
	serviceAccountId: string;
	clientId?: string | null;
	clientIdentifier?: string | null;
}): NewConnection {
	return {
		id: generate("CONNECTION"),
		code: params.code,
		name: params.name,
		description: params.description ?? null,
		endpoint: params.endpoint,
		externalId: params.externalId ?? null,
		status: "ACTIVE",
		serviceAccountId: params.serviceAccountId,
		clientId: params.clientId ?? null,
		clientIdentifier: params.clientIdentifier ?? null,
	};
}

/**
 * Update a connection with partial updates.
 * Immutable fields (code, clientId, clientIdentifier) are preserved.
 */
export function updateConnection(
	conn: Connection,
	updates: {
		name?: string | undefined;
		description?: string | null | undefined;
		endpoint?: string | undefined;
		externalId?: string | null | undefined;
		status?: ConnectionStatus | undefined;
		serviceAccountId?: string | undefined;
	},
): Connection {
	return {
		...conn,
		...(updates.name !== undefined ? { name: updates.name } : {}),
		...(updates.description !== undefined
			? { description: updates.description }
			: {}),
		...(updates.endpoint !== undefined ? { endpoint: updates.endpoint } : {}),
		...(updates.externalId !== undefined
			? { externalId: updates.externalId }
			: {}),
		...(updates.status !== undefined ? { status: updates.status } : {}),
		...(updates.serviceAccountId !== undefined
			? { serviceAccountId: updates.serviceAccountId }
			: {}),
	};
}
