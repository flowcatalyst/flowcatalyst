/**
 * Identity Provider Domain Aggregate
 *
 * Represents an authentication provider (internal or external OIDC).
 */

import { generate } from '@flowcatalyst/tsid';
import type { IdentityProviderType } from './identity-provider-type.js';

export interface IdentityProvider {
	readonly id: string;
	readonly code: string;
	readonly name: string;
	readonly type: IdentityProviderType;
	readonly oidcIssuerUrl: string | null;
	readonly oidcClientId: string | null;
	readonly oidcClientSecretRef: string | null;
	readonly oidcMultiTenant: boolean;
	readonly oidcIssuerPattern: string | null;
	readonly allowedEmailDomains: readonly string[];
	readonly createdAt: Date;
	readonly updatedAt: Date;
}

export type NewIdentityProvider = Omit<IdentityProvider, 'createdAt' | 'updatedAt'> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new identity provider.
 */
export function createIdentityProvider(params: {
	code: string;
	name: string;
	type: IdentityProviderType;
	oidcIssuerUrl?: string | null;
	oidcClientId?: string | null;
	oidcClientSecretRef?: string | null;
	oidcMultiTenant?: boolean;
	oidcIssuerPattern?: string | null;
	allowedEmailDomains?: string[];
}): NewIdentityProvider {
	return {
		id: generate('IDENTITY_PROVIDER'),
		code: params.code,
		name: params.name,
		type: params.type,
		oidcIssuerUrl: params.oidcIssuerUrl ?? null,
		oidcClientId: params.oidcClientId ?? null,
		oidcClientSecretRef: params.oidcClientSecretRef ?? null,
		oidcMultiTenant: params.oidcMultiTenant ?? false,
		oidcIssuerPattern: params.oidcIssuerPattern ?? null,
		allowedEmailDomains: params.allowedEmailDomains ?? [],
	};
}

/**
 * Update an identity provider.
 * Immutable fields (code) are preserved.
 */
/**
 * Get the effective issuer pattern for multi-tenant validation.
 * Returns the explicit pattern if set, otherwise derives from oidcIssuerUrl.
 * For Entra ID: replaces /organizations/, /common/, /consumers/ with /{tenantId}/
 */
export function getEffectiveIssuerPattern(idp: IdentityProvider): string | null {
	if (idp.oidcIssuerPattern) {
		return idp.oidcIssuerPattern;
	}
	if (!idp.oidcIssuerUrl) {
		return null;
	}
	return idp.oidcIssuerUrl
		.replace('/organizations/', '/{tenantId}/')
		.replace('/common/', '/{tenantId}/')
		.replace('/consumers/', '/{tenantId}/');
}

/**
 * Validate if a token issuer is valid for this identity provider.
 * Single-tenant: must match oidcIssuerUrl exactly.
 * Multi-tenant: must match the issuer pattern with any tenant ID.
 */
export function isValidIssuer(idp: IdentityProvider, tokenIssuer: string): boolean {
	if (!tokenIssuer) {
		return false;
	}

	if (!idp.oidcMultiTenant) {
		return tokenIssuer === idp.oidcIssuerUrl;
	}

	const pattern = getEffectiveIssuerPattern(idp);
	if (!pattern) {
		return false;
	}

	// Convert pattern to regex: escape dots, replace {tenantId} with [a-zA-Z0-9-]+
	const regex = pattern
		.replace(/\./g, '\\.')
		.replace('{tenantId}', '[a-zA-Z0-9-]+');

	return new RegExp(`^${regex}$`).test(tokenIssuer);
}

/**
 * Check if an email domain is allowed to authenticate through this IDP.
 * If allowedEmailDomains is empty, all domains are allowed.
 */
export function isEmailDomainAllowed(idp: IdentityProvider, emailDomain: string): boolean {
	if (idp.allowedEmailDomains.length === 0) {
		return true;
	}
	return idp.allowedEmailDomains.includes(emailDomain.toLowerCase());
}

/**
 * Validate OIDC configuration is complete.
 */
export function validateOidcConfig(idp: IdentityProvider): boolean {
	if (idp.type !== 'OIDC') return true;
	return !!idp.oidcIssuerUrl && !!idp.oidcClientId;
}

/**
 * Check if this IDP has a client secret configured.
 */
export function hasClientSecret(idp: IdentityProvider): boolean {
	return idp.oidcClientSecretRef !== null && idp.oidcClientSecretRef !== '';
}

export function updateIdentityProvider(
	idp: IdentityProvider,
	updates: {
		name?: string | undefined;
		type?: IdentityProviderType | undefined;
		oidcIssuerUrl?: string | null | undefined;
		oidcClientId?: string | null | undefined;
		oidcClientSecretRef?: string | null | undefined;
		oidcMultiTenant?: boolean | undefined;
		oidcIssuerPattern?: string | null | undefined;
		allowedEmailDomains?: string[] | undefined;
	},
): IdentityProvider {
	return {
		...idp,
		...(updates.name !== undefined ? { name: updates.name } : {}),
		...(updates.type !== undefined ? { type: updates.type } : {}),
		...(updates.oidcIssuerUrl !== undefined ? { oidcIssuerUrl: updates.oidcIssuerUrl } : {}),
		...(updates.oidcClientId !== undefined ? { oidcClientId: updates.oidcClientId } : {}),
		...(updates.oidcClientSecretRef !== undefined ? { oidcClientSecretRef: updates.oidcClientSecretRef } : {}),
		...(updates.oidcMultiTenant !== undefined ? { oidcMultiTenant: updates.oidcMultiTenant } : {}),
		...(updates.oidcIssuerPattern !== undefined ? { oidcIssuerPattern: updates.oidcIssuerPattern } : {}),
		...(updates.allowedEmailDomains !== undefined ? { allowedEmailDomains: updates.allowedEmailDomains } : {}),
	};
}
