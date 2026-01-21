/**
 * Account Adapter for oidc-provider
 *
 * Integrates oidc-provider with the Principal repository to provide
 * user account information for token claims.
 */

import type { FindAccount, Account, KoaContextWithOIDC, AccountClaims, ClaimsParameterMember } from 'oidc-provider';
import type { PrincipalRepository } from '../persistence/repositories/principal-repository.js';
import type { Principal } from '../../domain/principal/principal.js';

/**
 * Maps a Principal to OIDC standard claims.
 */
function principalToClaims(principal: Principal): AccountClaims {
	const claims: AccountClaims = {
		sub: principal.id,
		name: principal.name,
		updated_at: Math.floor(principal.updatedAt.getTime() / 1000),
	};

	// Add user identity claims if available (USER type)
	if (principal.userIdentity) {
		claims['email'] = principal.userIdentity.email;
		claims['email_verified'] = true; // We trust our internal verification
	}

	// Add custom FlowCatalyst claims
	claims['flowcatalyst:type'] = principal.type;
	claims['flowcatalyst:scope'] = principal.scope;
	claims['flowcatalyst:client_id'] = principal.clientId;
	claims['flowcatalyst:roles'] = principal.roles.map((r) => r.roleName);

	// For ANCHOR users, clients = ["*"] (all clients)
	// For PARTNER users, clients would be loaded from grants (not in scope here)
	// For CLIENT users, clients = [clientId]
	if (principal.scope === 'ANCHOR') {
		claims['flowcatalyst:clients'] = ['*'];
	} else if (principal.scope === 'CLIENT' && principal.clientId) {
		claims['flowcatalyst:clients'] = [principal.clientId];
	}

	return claims;
}

/**
 * Creates the findAccount function for oidc-provider.
 *
 * This function is called by oidc-provider to load user information
 * when issuing tokens or validating sessions.
 */
export function createFindAccount(principalRepository: PrincipalRepository): FindAccount {
	return async function findAccount(
		_ctx: KoaContextWithOIDC,
		id: string,
		_token?: unknown,
	): Promise<Account | undefined> {
		// Load principal from repository
		const principal = await principalRepository.findById(id);

		if (!principal) {
			return undefined;
		}

		// Only return active principals
		if (!principal.active) {
			return undefined;
		}

		// Return an Account object
		return {
			accountId: principal.id,

			/**
			 * Returns claims for the given scope and claims request.
			 *
			 * @param use - "id_token" | "userinfo"
			 * @param scope - Requested scopes (e.g., "openid profile email")
			 * @param claims - Specific claims requested
			 * @param rejected - Claims that were rejected
			 */
			async claims(
				use: string,
				scope: string,
				claims: { [key: string]: ClaimsParameterMember | null },
				rejected: string[],
			): Promise<AccountClaims> {
				const allClaims = principalToClaims(principal);

				// Filter claims based on requested scopes
				const requestedScopes = scope.split(' ');
				const result: AccountClaims = {
					sub: allClaims.sub, // sub is always included
				};

				// Standard OIDC scopes
				if (requestedScopes.includes('profile')) {
					result['name'] = allClaims['name'];
					result['updated_at'] = allClaims['updated_at'];
				}

				if (requestedScopes.includes('email')) {
					result['email'] = allClaims['email'];
					result['email_verified'] = allClaims['email_verified'];
				}

				// FlowCatalyst custom claims (always included for now)
				// In production, you might want to scope these
				result['flowcatalyst:type'] = allClaims['flowcatalyst:type'];
				result['flowcatalyst:scope'] = allClaims['flowcatalyst:scope'];
				result['flowcatalyst:client_id'] = allClaims['flowcatalyst:client_id'];
				result['flowcatalyst:roles'] = allClaims['flowcatalyst:roles'];
				result['flowcatalyst:clients'] = allClaims['flowcatalyst:clients'];

				// Remove rejected claims
				for (const claim of rejected) {
					delete result[claim];
				}

				return result;
			},
		};
	};
}

/**
 * Creates an account adapter for verifying credentials (login).
 *
 * This is separate from findAccount as it handles password verification.
 */
export interface AccountAdapter {
	/**
	 * Verify credentials and return the account ID if valid.
	 */
	verifyCredentials(email: string, password: string): Promise<string | null>;

	/**
	 * Find account by email.
	 */
	findByEmail(email: string): Promise<Principal | null>;
}

/**
 * Create an account adapter for credential verification.
 */
export function createAccountAdapter(
	principalRepository: PrincipalRepository,
	verifyPassword: (password: string, hash: string) => Promise<boolean>,
): AccountAdapter {
	return {
		async verifyCredentials(email: string, password: string): Promise<string | null> {
			const principal = await principalRepository.findByEmail(email.toLowerCase());

			if (!principal) {
				return null;
			}

			// Must be a USER type
			if (principal.type !== 'USER') {
				return null;
			}

			// Must be active
			if (!principal.active) {
				return null;
			}

			// Must have user identity with password hash (INTERNAL auth)
			if (!principal.userIdentity?.passwordHash) {
				return null;
			}

			// Verify password
			const isValid = await verifyPassword(password, principal.userIdentity.passwordHash);
			if (!isValid) {
				return null;
			}

			return principal.id;
		},

		async findByEmail(email: string): Promise<Principal | null> {
			const principal = await principalRepository.findByEmail(email.toLowerCase());
			return principal ?? null;
		},
	};
}
