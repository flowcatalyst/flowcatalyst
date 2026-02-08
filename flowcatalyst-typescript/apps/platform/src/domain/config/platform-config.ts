/**
 * Platform Config Entity
 *
 * Represents a platform configuration entry with support for
 * global and client-scoped settings.
 */

import { generate } from '@flowcatalyst/tsid';

export type ConfigScope = 'GLOBAL' | 'CLIENT';
export type ConfigValueType = 'PLAIN' | 'SECRET';

/**
 * Platform config entity.
 */
export interface PlatformConfig {
	readonly id: string;
	readonly applicationCode: string;
	readonly section: string;
	readonly property: string;
	readonly scope: ConfigScope;
	readonly clientId: string | null;
	readonly valueType: ConfigValueType;
	readonly value: string;
	readonly description: string | null;
	readonly createdAt: Date;
	readonly updatedAt: Date;
}

export type NewPlatformConfig = Omit<PlatformConfig, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date | undefined;
	updatedAt?: Date | undefined;
};

/**
 * Get the full key of a config entry.
 */
export function getConfigFullKey(config: PlatformConfig): string {
	return `${config.applicationCode}.${config.section}.${config.property}`;
}

/**
 * Create a new platform config entry.
 */
export function createPlatformConfig(params: {
	applicationCode: string;
	section: string;
	property: string;
	scope: ConfigScope;
	clientId: string | null;
	valueType: ConfigValueType;
	value: string;
	description: string | null;
}): NewPlatformConfig {
	return {
		id: generate('PLATFORM_CONFIG'),
		...params,
	};
}
