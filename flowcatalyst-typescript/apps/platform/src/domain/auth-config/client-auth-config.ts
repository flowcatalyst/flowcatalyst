/**
 * Client Auth Config Entity
 *
 * Authentication configuration per email domain.
 * Determines whether users from a specific domain authenticate via
 * INTERNAL (password) or OIDC (external IDP).
 */

import { generate } from '@flowcatalyst/tsid';
import type { AuthConfigType } from './auth-config-type.js';
import type { AuthProvider } from './auth-provider.js';

/**
 * Client Auth Config entity.
 */
export interface ClientAuthConfig {
	/** TSID primary key */
	readonly id: string;

	/** The email domain this configuration applies to (e.g., "acmecorp.com") */
	readonly emailDomain: string;

	/** Config type determining user access scope */
	readonly configType: AuthConfigType;

	/** Primary client ID (required for CLIENT type, null for others) */
	readonly primaryClientId: string | null;

	/** Additional client IDs for CLIENT type configurations */
	readonly additionalClientIds: readonly string[];

	/** Granted client IDs for PARTNER type configurations */
	readonly grantedClientIds: readonly string[];

	/** Authentication provider type: INTERNAL or OIDC */
	readonly authProvider: AuthProvider;

	/** OIDC issuer URL (required for OIDC provider) */
	readonly oidcIssuerUrl: string | null;

	/** OIDC client ID (required for OIDC provider) */
	readonly oidcClientId: string | null;

	/** Whether this is a multi-tenant OIDC configuration */
	readonly oidcMultiTenant: boolean;

	/** Pattern for validating multi-tenant issuers */
	readonly oidcIssuerPattern: string | null;

	/** Reference to the OIDC client secret (NOT the plaintext secret) */
	readonly oidcClientSecretRef: string | null;

	/** When the config was created */
	readonly createdAt: Date;

	/** When the config was last updated */
	readonly updatedAt: Date;
}

/**
 * Input for creating a new ClientAuthConfig.
 */
export type NewClientAuthConfig = Omit<ClientAuthConfig, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Input for creating an INTERNAL auth config.
 */
export interface CreateInternalAuthConfigInput {
	emailDomain: string;
	configType: AuthConfigType;
	primaryClientId?: string | null | undefined;
	additionalClientIds?: readonly string[] | undefined;
	grantedClientIds?: readonly string[] | undefined;
}

/**
 * Input for creating an OIDC auth config.
 */
export interface CreateOidcAuthConfigInput {
	emailDomain: string;
	configType: AuthConfigType;
	primaryClientId?: string | null | undefined;
	additionalClientIds?: readonly string[] | undefined;
	grantedClientIds?: readonly string[] | undefined;
	oidcIssuerUrl: string;
	oidcClientId: string;
	oidcClientSecretRef?: string | null | undefined;
	oidcMultiTenant?: boolean | undefined;
	oidcIssuerPattern?: string | null | undefined;
}

/**
 * Create a new INTERNAL auth config.
 */
export function createInternalAuthConfig(input: CreateInternalAuthConfigInput): NewClientAuthConfig {
	return {
		id: generate('CLIENT_AUTH_CONFIG'),
		emailDomain: input.emailDomain.toLowerCase(),
		configType: input.configType,
		primaryClientId: input.primaryClientId ?? null,
		additionalClientIds: input.additionalClientIds ?? [],
		grantedClientIds: input.grantedClientIds ?? [],
		authProvider: 'INTERNAL',
		oidcIssuerUrl: null,
		oidcClientId: null,
		oidcMultiTenant: false,
		oidcIssuerPattern: null,
		oidcClientSecretRef: null,
	};
}

/**
 * Create a new OIDC auth config.
 */
export function createOidcAuthConfig(input: CreateOidcAuthConfigInput): NewClientAuthConfig {
	return {
		id: generate('CLIENT_AUTH_CONFIG'),
		emailDomain: input.emailDomain.toLowerCase(),
		configType: input.configType,
		primaryClientId: input.primaryClientId ?? null,
		additionalClientIds: input.additionalClientIds ?? [],
		grantedClientIds: input.grantedClientIds ?? [],
		authProvider: 'OIDC',
		oidcIssuerUrl: input.oidcIssuerUrl,
		oidcClientId: input.oidcClientId,
		oidcMultiTenant: input.oidcMultiTenant ?? false,
		oidcIssuerPattern: input.oidcIssuerPattern ?? null,
		oidcClientSecretRef: input.oidcClientSecretRef ?? null,
	};
}

/**
 * Validate OIDC configuration.
 */
export function validateOidcConfig(config: ClientAuthConfig): string | null {
	if (config.authProvider === 'OIDC') {
		if (!config.oidcIssuerUrl) {
			return 'OIDC issuer URL is required for OIDC auth provider';
		}
		if (!config.oidcClientId) {
			return 'OIDC client ID is required for OIDC auth provider';
		}
	}
	return null;
}

/**
 * Validate config type constraints.
 */
export function validateConfigTypeConstraints(config: ClientAuthConfig): string | null {
	switch (config.configType) {
		case 'ANCHOR':
			if (config.primaryClientId) {
				return 'ANCHOR config cannot have a primary client';
			}
			if (config.additionalClientIds.length > 0) {
				return 'ANCHOR config cannot have additional clients';
			}
			if (config.grantedClientIds.length > 0) {
				return 'ANCHOR config cannot have granted clients';
			}
			break;
		case 'PARTNER':
			if (config.primaryClientId) {
				return 'PARTNER config cannot have a primary client';
			}
			if (config.additionalClientIds.length > 0) {
				return 'PARTNER config cannot have additional clients';
			}
			// grantedClientIds is allowed
			break;
		case 'CLIENT':
			if (!config.primaryClientId) {
				return 'CLIENT config must have a primary client';
			}
			if (config.grantedClientIds.length > 0) {
				return 'CLIENT config cannot have granted clients';
			}
			// additionalClientIds is allowed
			break;
	}
	return null;
}

/**
 * Get all client IDs this config grants access to.
 */
export function getAllAccessibleClientIds(config: ClientAuthConfig): readonly string[] {
	switch (config.configType) {
		case 'ANCHOR':
			return []; // Access determined by scope, not client list
		case 'PARTNER':
			return config.grantedClientIds;
		case 'CLIENT': {
			const result: string[] = [];
			if (config.primaryClientId) {
				result.push(config.primaryClientId);
			}
			result.push(...config.additionalClientIds);
			return result;
		}
	}
}

/**
 * Check if a token issuer is valid for this configuration.
 */
export function isValidIssuer(config: ClientAuthConfig, tokenIssuer: string): boolean {
	if (!tokenIssuer) {
		return false;
	}

	if (!config.oidcMultiTenant) {
		// Single tenant: exact match
		return tokenIssuer === config.oidcIssuerUrl;
	}

	// Multi-tenant: match against pattern
	const pattern = getEffectiveIssuerPattern(config);
	if (!pattern) {
		return false;
	}

	// Convert pattern to regex: {tenantId} -> [a-zA-Z0-9-]+
	const regex = new RegExp(
		'^' +
			pattern
				.replace(/[.+?^${}()|[\]\\]/g, '\\$&')
				.replace(/\{tenantId\}/g, '[a-zA-Z0-9-]+') +
			'$',
	);

	return regex.test(tokenIssuer);
}

/**
 * Get the effective issuer pattern for multi-tenant validation.
 */
function getEffectiveIssuerPattern(config: ClientAuthConfig): string | null {
	if (config.oidcIssuerPattern) {
		return config.oidcIssuerPattern;
	}
	if (!config.oidcIssuerUrl) {
		return null;
	}
	// Auto-derive pattern for common multi-tenant IDPs
	return config.oidcIssuerUrl
		.replace('/organizations/', '/{tenantId}/')
		.replace('/common/', '/{tenantId}/')
		.replace('/consumers/', '/{tenantId}/');
}
