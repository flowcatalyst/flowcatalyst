/**
 * Allowed Role Filter
 *
 * Bridges between role IDs (stored in EmailDomainMapping.allowedRoleIds)
 * and role names (used in role assignments).
 *
 * Resolves role IDs to names at the enforcement point,
 * keeping IDs as the source of truth in the database.
 */

import type { EmailDomainMappingRepository } from "../infrastructure/persistence/repositories/email-domain-mapping-repository.js";
import type { RoleRepository } from "../infrastructure/persistence/repositories/role-repository.js";

export interface AllowedRoleFilterDeps {
	emailDomainMappingRepository: EmailDomainMappingRepository;
	roleRepository: RoleRepository;
}

/**
 * Get the set of allowed role names for an email domain.
 *
 * @returns null if no restrictions apply (ANCHOR scope or empty allowedRoleIds);
 *          Set<string> if restrictions apply
 */
export async function getAllowedRoleNames(
	emailDomain: string,
	deps: AllowedRoleFilterDeps,
): Promise<Set<string> | null> {
	if (!emailDomain) {
		return null;
	}

	const mapping = await deps.emailDomainMappingRepository.findByEmailDomain(
		emailDomain.toLowerCase(),
	);
	if (!mapping) {
		return null;
	}

	// ANCHOR scope has no role restrictions
	if (mapping.scopeType === "ANCHOR") {
		return null;
	}

	// No restrictions if allowedRoleIds is empty
	if (mapping.allowedRoleIds.length === 0) {
		return null;
	}

	// Resolve role IDs to names
	const allowedNames = new Set<string>();
	for (const roleId of mapping.allowedRoleIds) {
		const role = await deps.roleRepository.findById(roleId);
		if (role) {
			allowedNames.add(role.name);
		}
	}

	return allowedNames;
}

/**
 * Filter a set of role names to only include those allowed for the given email domain.
 *
 * @returns the filtered set of allowed role names (returns all if no restrictions apply)
 */
export async function filterAllowedRoles(
	requestedRoleNames: Set<string>,
	emailDomain: string,
	deps: AllowedRoleFilterDeps,
): Promise<Set<string>> {
	const allowed = await getAllowedRoleNames(emailDomain, deps);
	if (!allowed) {
		return requestedRoleNames;
	}
	return new Set([...requestedRoleNames].filter((name) => allowed.has(name)));
}
