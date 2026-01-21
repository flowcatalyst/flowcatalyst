/**
 * Application Client Config Entity
 *
 * Represents the configuration for an application enabled for a specific client.
 */

import { generate } from '@flowcatalyst/tsid';

/**
 * Application client configuration entity.
 */
export interface ApplicationClientConfig {
	/** TSID primary key */
	readonly id: string;

	/** Application ID */
	readonly applicationId: string;

	/** Client ID */
	readonly clientId: string;

	/** Whether the application is enabled for this client */
	readonly enabled: boolean;

	/** When the config was created */
	readonly createdAt: Date;

	/** When the config was last updated */
	readonly updatedAt: Date;
}

/**
 * Input for creating a new ApplicationClientConfig.
 */
export type NewApplicationClientConfig = Omit<ApplicationClientConfig, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new application client config.
 */
export function createApplicationClientConfig(params: {
	applicationId: string;
	clientId: string;
	enabled?: boolean;
}): NewApplicationClientConfig {
	return {
		id: generate('APP_CLIENT_CONFIG'),
		applicationId: params.applicationId,
		clientId: params.clientId,
		enabled: params.enabled ?? true,
	};
}

/**
 * Enable/disable an application client config.
 */
export function setApplicationClientConfigEnabled(
	config: ApplicationClientConfig,
	enabled: boolean,
): ApplicationClientConfig {
	return {
		...config,
		enabled,
		updatedAt: new Date(),
	};
}
