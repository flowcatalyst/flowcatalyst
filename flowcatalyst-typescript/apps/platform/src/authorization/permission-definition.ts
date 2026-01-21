/**
 * Permission Definition
 *
 * Defines the structure for permissions using a hierarchical naming scheme:
 * {subdomain}:{context}:{aggregate}:{action}
 *
 * Example: "platform:iam:user:create"
 */

/**
 * Permission definition interface.
 */
export interface PermissionDefinition {
	/** Subdomain (e.g., "platform") */
	readonly subdomain: string;
	/** Context/bounded context (e.g., "iam") */
	readonly context: string;
	/** Aggregate/resource (e.g., "user") */
	readonly aggregate: string;
	/** Action (e.g., "create", "read", "update", "delete", "manage") */
	readonly action: string;
	/** Human-readable description */
	readonly description: string;
}

/**
 * Convert a permission definition to its string representation.
 *
 * @param permission - Permission definition
 * @returns Permission string (e.g., "platform:iam:user:create")
 */
export function permissionToString(permission: PermissionDefinition): string {
	return `${permission.subdomain}:${permission.context}:${permission.aggregate}:${permission.action}`;
}

/**
 * Parse a permission string into its components.
 *
 * @param permissionString - Permission string (e.g., "platform:iam:user:create")
 * @returns Parsed components or null if invalid
 */
export function parsePermissionString(
	permissionString: string,
): { subdomain: string; context: string; aggregate: string; action: string } | null {
	const parts = permissionString.split(':');
	if (parts.length !== 4) {
		return null;
	}
	const [subdomain, context, aggregate, action] = parts;
	if (!subdomain || !context || !aggregate || !action) {
		return null;
	}
	return { subdomain, context, aggregate, action };
}

/**
 * Create a permission definition.
 *
 * @param subdomain - Subdomain (e.g., "platform")
 * @param context - Context (e.g., "iam")
 * @param aggregate - Aggregate (e.g., "user")
 * @param action - Action (e.g., "create")
 * @param description - Human-readable description
 * @returns Permission definition
 */
export function makePermission(
	subdomain: string,
	context: string,
	aggregate: string,
	action: string,
	description: string,
): PermissionDefinition {
	return {
		subdomain,
		context,
		aggregate,
		action,
		description,
	};
}

/**
 * Check if a permission matches a pattern.
 * Patterns support wildcards (*) at any level.
 *
 * @param permission - Permission string to check
 * @param pattern - Pattern to match against (supports * wildcard)
 * @returns True if permission matches pattern
 *
 * @example
 * matchesPattern("platform:iam:user:create", "platform:iam:user:*") // true
 * matchesPattern("platform:iam:user:create", "platform:*:*:*") // true
 * matchesPattern("platform:iam:user:create", "other:iam:user:create") // false
 */
export function matchesPattern(permission: string, pattern: string): boolean {
	const permParts = permission.split(':');
	const patternParts = pattern.split(':');

	if (permParts.length !== 4 || patternParts.length !== 4) {
		return false;
	}

	for (let i = 0; i < 4; i++) {
		if (patternParts[i] !== '*' && patternParts[i] !== permParts[i]) {
			return false;
		}
	}

	return true;
}
