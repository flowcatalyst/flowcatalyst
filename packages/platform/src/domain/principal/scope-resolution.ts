/**
 * Scope Resolution
 *
 * Resolves the effective scope for a user based on email domain configuration.
 * Priority: EmailDomainMapping > AnchorDomain > fallback.
 */

import type { EmailDomainMapping } from "../email-domain-mapping/email-domain-mapping.js";
import type { PrincipalScope } from "./principal-scope.js";

export interface ResolvedScope {
	readonly scope: PrincipalScope;
	readonly clientId: string | null;
}

/**
 * Resolve the effective scope for a user email.
 *
 * Priority (highest to lowest):
 * 1. EmailDomainMapping.scopeType for the user's email domain
 * 2. AnchorDomain table (legacy fallback) -> ANCHOR
 * 3. fallbackScope/fallbackClientId (from API or default)
 * 4. Default: CLIENT
 */
export function resolveScopeForEmail(params: {
	mapping: EmailDomainMapping | undefined;
	isAnchorDomain: boolean;
	fallbackScope?: PrincipalScope | undefined;
	fallbackClientId?: string | null | undefined;
}): ResolvedScope {
	const { mapping, isAnchorDomain, fallbackScope, fallbackClientId } = params;

	// 1. Email domain mapping takes precedence
	if (mapping) {
		const scope = mapping.scopeType as PrincipalScope;
		const clientId = scope === "CLIENT" ? (mapping.primaryClientId ?? null) : null;
		return { scope, clientId };
	}

	// 2. Legacy anchor domain fallback
	if (isAnchorDomain) {
		return { scope: "ANCHOR" as PrincipalScope, clientId: null };
	}

	// 3. API-provided fallback
	if (fallbackScope !== undefined) {
		return {
			scope: fallbackScope,
			clientId: fallbackClientId ?? null,
		};
	}

	// 4. Default
	return { scope: "CLIENT" as PrincipalScope, clientId: fallbackClientId ?? null };
}
