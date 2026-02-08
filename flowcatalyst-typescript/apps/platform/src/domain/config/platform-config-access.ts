/**
 * Platform Config Access Entity
 *
 * Controls which roles can read/write platform configuration.
 */

import { generate } from '@flowcatalyst/tsid';

/**
 * Platform config access grant entity.
 */
export interface PlatformConfigAccess {
	readonly id: string;
	readonly applicationCode: string;
	readonly roleCode: string;
	readonly canRead: boolean;
	readonly canWrite: boolean;
	readonly createdAt: Date;
}

export type NewPlatformConfigAccess = Omit<PlatformConfigAccess, 'createdAt'> & {
	createdAt?: Date | undefined;
};

/**
 * Create a new config access grant.
 */
export function createPlatformConfigAccess(params: {
	applicationCode: string;
	roleCode: string;
	canRead: boolean;
	canWrite: boolean;
}): NewPlatformConfigAccess {
	return {
		id: generate('CONFIG_ACCESS'),
		...params,
	};
}
