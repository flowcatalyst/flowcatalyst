/**
 * Application Entity
 *
 * Represents an application or integration in the FlowCatalyst platform ecosystem.
 *
 * Two types are supported:
 * - APPLICATION: User-facing applications (TMS, WMS, etc.) that users log into.
 *   Can be assigned to clients and users.
 * - INTEGRATION: Third-party adapters and connectors (Salesforce, SAP, etc.).
 *   Used for event/subscription scoping but not user access.
 *
 * Each application/integration has a unique code used as the prefix for:
 * - Roles (e.g., "tms:admin", "sf:sync")
 * - Event types (e.g., "tms:shipment-created", "sf:contact-updated")
 * - Permissions
 */

import { generate } from '@flowcatalyst/tsid';

/**
 * Application type.
 */
export type ApplicationType = 'APPLICATION' | 'INTEGRATION';

export const ApplicationType = {
	/** User-facing application that users log into */
	APPLICATION: 'APPLICATION' as const,
	/** Third-party adapter or connector */
	INTEGRATION: 'INTEGRATION' as const,
} as const;

/**
 * Application entity.
 */
export interface Application {
	/** TSID primary key */
	readonly id: string;

	/**
	 * Type of this entity.
	 * APPLICATION = User-facing application
	 * INTEGRATION = Third-party adapter/connector
	 */
	readonly type: ApplicationType;

	/**
	 * Unique application code.
	 * Used as a prefix for resources managed by this application.
	 * Max 50 characters, lowercase, alphanumeric with hyphens.
	 * Example: "crm", "inventory", "messaging", "sf" (for Salesforce)
	 */
	readonly code: string;

	/** Application display name */
	readonly name: string;

	/** Application description */
	readonly description: string | null;

	/** Icon URL for this application */
	readonly iconUrl: string | null;

	/**
	 * Public website URL for this application/integration.
	 * Can be overridden per client via ApplicationClientConfig.
	 */
	readonly website: string | null;

	/**
	 * Embedded logo content (SVG/vector format).
	 * Stored directly in the database.
	 */
	readonly logo: string | null;

	/**
	 * MIME type of the logo content.
	 * Example: "image/svg+xml" for SVG logos.
	 */
	readonly logoMimeType: string | null;

	/**
	 * Default base URL for the application.
	 * Can be overridden per client via ApplicationClientConfig.
	 * Primarily used for APPLICATION type.
	 */
	readonly defaultBaseUrl: string | null;

	/**
	 * Service account ID for this application.
	 * Contains webhook credentials for dispatching.
	 */
	readonly serviceAccountId: string | null;

	/** Whether the application is active */
	readonly active: boolean;

	/** When the application was created */
	readonly createdAt: Date;

	/** When the application was last updated */
	readonly updatedAt: Date;
}

/**
 * Input for creating a new Application.
 */
export type NewApplication = Omit<Application, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new application.
 */
export function createApplication(params: {
	code: string;
	name: string;
	type?: ApplicationType;
	description?: string | null;
	iconUrl?: string | null;
	website?: string | null;
	logo?: string | null;
	logoMimeType?: string | null;
	defaultBaseUrl?: string | null;
}): NewApplication {
	return {
		id: generate('APPLICATION'),
		type: params.type ?? ApplicationType.APPLICATION,
		code: params.code.toLowerCase(),
		name: params.name,
		description: params.description ?? null,
		iconUrl: params.iconUrl ?? null,
		website: params.website ?? null,
		logo: params.logo ?? null,
		logoMimeType: params.logoMimeType ?? null,
		defaultBaseUrl: params.defaultBaseUrl ?? null,
		serviceAccountId: null,
		active: true,
	};
}

/**
 * Update an application.
 */
export function updateApplication(
	application: Application,
	updates: Partial<Pick<Application, 'name' | 'description' | 'iconUrl' | 'website' | 'logo' | 'logoMimeType' | 'defaultBaseUrl'>>,
): Application {
	return {
		...application,
		...updates,
		updatedAt: new Date(),
	};
}

/**
 * Activate an application.
 */
export function activateApplication(application: Application): Application {
	return {
		...application,
		active: true,
		updatedAt: new Date(),
	};
}

/**
 * Deactivate an application.
 */
export function deactivateApplication(application: Application): Application {
	return {
		...application,
		active: false,
		updatedAt: new Date(),
	};
}

/**
 * Check if this is a user-facing application.
 */
export function isApplication(app: Application): boolean {
	return app.type === ApplicationType.APPLICATION;
}

/**
 * Check if this is a third-party integration.
 */
export function isIntegration(app: Application): boolean {
	return app.type === ApplicationType.INTEGRATION;
}
