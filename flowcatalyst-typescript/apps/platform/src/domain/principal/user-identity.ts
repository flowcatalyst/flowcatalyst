/**
 * User Identity
 *
 * User identity information (embedded in Principal for USER type).
 */

import type { IdpType } from './idp-type.js';

/**
 * User identity information embedded in a Principal.
 */
export interface UserIdentity {
	/** User's email address */
	readonly email: string;

	/** Domain extracted from email (e.g., "acme.com") */
	readonly emailDomain: string;

	/** Identity provider type (INTERNAL or OIDC) */
	readonly idpType: IdpType;

	/** Subject from OIDC token (for OIDC auth) */
	readonly externalIdpId: string | null;

	/** Argon2id password hash (for INTERNAL auth only) */
	readonly passwordHash: string | null;

	/** When the user last logged in */
	readonly lastLoginAt: Date | null;
}

/**
 * Create a new user identity.
 */
export function createUserIdentity(params: {
	email: string;
	idpType: IdpType;
	passwordHash?: string | null;
	externalIdpId?: string | null;
}): UserIdentity {
	const emailDomain = extractEmailDomain(params.email);

	return {
		email: params.email.toLowerCase(),
		emailDomain,
		idpType: params.idpType,
		externalIdpId: params.externalIdpId ?? null,
		passwordHash: params.passwordHash ?? null,
		lastLoginAt: null,
	};
}

/**
 * Extract the domain from an email address.
 */
export function extractEmailDomain(email: string): string {
	const atIndex = email.indexOf('@');
	if (atIndex === -1) {
		throw new Error(`Invalid email address: ${email}`);
	}
	return email.substring(atIndex + 1).toLowerCase();
}
