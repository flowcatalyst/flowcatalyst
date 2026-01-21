/**
 * AuthPermission Entity
 *
 * Represents a permission definition stored in the database.
 * Permissions follow a hierarchical naming pattern: {subdomain}:{context}:{aggregate}:{action}
 */

import { generate } from '@flowcatalyst/tsid';

/**
 * AuthPermission entity.
 */
export interface AuthPermission {
	readonly id: string;
	readonly code: string;
	readonly subdomain: string;
	readonly context: string;
	readonly aggregate: string;
	readonly action: string;
	readonly description: string | null;
	readonly createdAt: Date;
	readonly updatedAt: Date;
}

/**
 * Input for creating a new AuthPermission.
 */
export interface CreateAuthPermissionInput {
	readonly subdomain: string;
	readonly context: string;
	readonly aggregate: string;
	readonly action: string;
	readonly description?: string | null;
}

/**
 * Create a new AuthPermission entity.
 */
export function createAuthPermission(input: CreateAuthPermissionInput): AuthPermission {
	const now = new Date();
	const code = `${input.subdomain}:${input.context}:${input.aggregate}:${input.action}`;
	return {
		id: generate('PERMISSION'),
		code,
		subdomain: input.subdomain,
		context: input.context,
		aggregate: input.aggregate,
		action: input.action,
		description: input.description ?? null,
		createdAt: now,
		updatedAt: now,
	};
}

/**
 * Parse a permission code into its components.
 */
export function parsePermissionCode(code: string): {
	subdomain: string;
	context: string;
	aggregate: string;
	action: string;
} | null {
	const parts = code.split(':');
	if (parts.length !== 4) {
		return null;
	}
	const [subdomain, context, aggregate, action] = parts;
	if (!subdomain || !context || !aggregate || !action) {
		return null;
	}
	return { subdomain, context, aggregate, action };
}
