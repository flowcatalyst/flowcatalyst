/**
 * CORS Allowed Origin Entity
 *
 * Represents an allowed CORS origin for the platform.
 */

import { generate } from "@flowcatalyst/tsid";

/**
 * CORS allowed origin entity.
 */
export interface CorsAllowedOrigin {
	readonly id: string;
	readonly origin: string;
	readonly description: string | null;
	readonly createdBy: string | null;
	readonly createdAt: Date;
	readonly updatedAt: Date;
}

/**
 * Input for creating a new CorsAllowedOrigin.
 */
export type NewCorsAllowedOrigin = Omit<
	CorsAllowedOrigin,
	"createdAt" | "updatedAt"
> & {
	createdAt?: Date | undefined;
	updatedAt?: Date | undefined;
};

/**
 * Create a new CORS allowed origin.
 */
export function createCorsAllowedOrigin(
	origin: string,
	description: string | null,
	createdBy: string | null,
): NewCorsAllowedOrigin {
	return {
		id: generate("CORS_ORIGIN"),
		origin: origin.toLowerCase(),
		description,
		createdBy,
	};
}
