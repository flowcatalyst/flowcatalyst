/**
 * Email Domain Mapping Domain Aggregate
 *
 * Maps an email domain to an identity provider and access scope.
 * Replaces the legacy AnchorDomain model with richer scope control.
 */

import { generate } from "@flowcatalyst/tsid";
import type { ScopeType } from "./scope-type.js";

export interface EmailDomainMapping {
	readonly id: string;
	readonly emailDomain: string;
	readonly identityProviderId: string;
	readonly scopeType: ScopeType;
	readonly primaryClientId: string | null;
	readonly additionalClientIds: readonly string[];
	readonly grantedClientIds: readonly string[];
	readonly requiredOidcTenantId: string | null;
	readonly allowedRoleIds: readonly string[];
	readonly syncRolesFromIdp: boolean;
	readonly createdAt: Date;
	readonly updatedAt: Date;
}

export type NewEmailDomainMapping = Omit<
	EmailDomainMapping,
	"createdAt" | "updatedAt"
> & {
	createdAt?: Date;
	updatedAt?: Date;
};

/**
 * Create a new email domain mapping.
 */
export function createEmailDomainMapping(params: {
	emailDomain: string;
	identityProviderId: string;
	scopeType: ScopeType;
	primaryClientId?: string | null;
	additionalClientIds?: string[];
	grantedClientIds?: string[];
	requiredOidcTenantId?: string | null;
	allowedRoleIds?: string[];
	syncRolesFromIdp?: boolean;
}): NewEmailDomainMapping {
	return {
		id: generate("EMAIL_DOMAIN_MAPPING"),
		emailDomain: params.emailDomain.toLowerCase(),
		identityProviderId: params.identityProviderId,
		scopeType: params.scopeType,
		primaryClientId: params.primaryClientId ?? null,
		additionalClientIds: params.additionalClientIds ?? [],
		grantedClientIds: params.grantedClientIds ?? [],
		requiredOidcTenantId: params.requiredOidcTenantId ?? null,
		allowedRoleIds: params.allowedRoleIds ?? [],
		syncRolesFromIdp: params.syncRolesFromIdp ?? false,
	};
}

/**
 * Update an email domain mapping.
 * Immutable field (emailDomain) is preserved.
 */
export function updateEmailDomainMapping(
	mapping: EmailDomainMapping,
	updates: {
		identityProviderId?: string | undefined;
		scopeType?: ScopeType | undefined;
		primaryClientId?: string | null | undefined;
		additionalClientIds?: string[] | undefined;
		grantedClientIds?: string[] | undefined;
		requiredOidcTenantId?: string | null | undefined;
		allowedRoleIds?: string[] | undefined;
		syncRolesFromIdp?: boolean | undefined;
	},
): EmailDomainMapping {
	return {
		...mapping,
		...(updates.identityProviderId !== undefined
			? { identityProviderId: updates.identityProviderId }
			: {}),
		...(updates.scopeType !== undefined
			? { scopeType: updates.scopeType }
			: {}),
		...(updates.primaryClientId !== undefined
			? { primaryClientId: updates.primaryClientId }
			: {}),
		...(updates.additionalClientIds !== undefined
			? { additionalClientIds: updates.additionalClientIds }
			: {}),
		...(updates.grantedClientIds !== undefined
			? { grantedClientIds: updates.grantedClientIds }
			: {}),
		...(updates.requiredOidcTenantId !== undefined
			? { requiredOidcTenantId: updates.requiredOidcTenantId }
			: {}),
		...(updates.allowedRoleIds !== undefined
			? { allowedRoleIds: updates.allowedRoleIds }
			: {}),
		...(updates.syncRolesFromIdp !== undefined
			? { syncRolesFromIdp: updates.syncRolesFromIdp }
			: {}),
	};
}

/**
 * Get all accessible client IDs for this mapping.
 */
export function getMappingAccessibleClientIds(
	mapping: EmailDomainMapping,
): string[] {
	switch (mapping.scopeType) {
		case "ANCHOR":
			return []; // Access to all clients
		case "PARTNER":
			return [...mapping.grantedClientIds];
		case "CLIENT": {
			const ids = mapping.primaryClientId ? [mapping.primaryClientId] : [];
			return [...ids, ...mapping.additionalClientIds];
		}
	}
}

/**
 * Check if mapping has role restrictions.
 */
export function hasRoleRestrictions(mapping: EmailDomainMapping): boolean {
	return mapping.allowedRoleIds.length > 0;
}
