/**
 * OIDC Sync Service
 *
 * CRITICAL SECURITY: Service for OIDC user and role synchronization.
 *
 * All state-changing operations go through UnitOfWork to guarantee
 * domain events and audit logs are created atomically.
 *
 * Implements a critical security control: IDP role authorization.
 * Only IDP roles that are explicitly authorized in the idp_role_mappings table
 * are accepted during OIDC login. This prevents partners/customers from
 * injecting unauthorized roles via compromised or misconfigured IDPs.
 *
 * Example attack prevented:
 * - Partner IDP is compromised and grants all users "super-admin" role
 * - This service rejects the role because it's not in idp_role_mappings
 * - Attack is logged and prevented
 */

import type { FastifyBaseLogger } from 'fastify';
import { type ExecutionContext, type UnitOfWork, type Result } from '@flowcatalyst/domain-core';
import {
  createUserPrincipal,
  createUserIdentity,
  extractEmailDomain,
  updatePrincipal,
  type Principal,
  type RoleAssignment,
  type PrincipalScope,
  UserCreated,
  UserUpdated,
  RolesAssigned,
  IdpType,
} from '../../domain/index.js';
import type { PrincipalRepository } from '../persistence/repositories/principal-repository.js';
import type { IdpRoleMappingRepository } from '../persistence/repositories/idp-role-mapping-repository.js';

export interface OidcSyncServiceDeps {
  principalRepository: PrincipalRepository;
  idpRoleMappingRepository: IdpRoleMappingRepository;
  unitOfWork: UnitOfWork;
  log: FastifyBaseLogger;
}

/**
 * Create or update a user principal from OIDC claims.
 * Uses UnitOfWork to ensure domain events and audit logs are created.
 */
export async function createOrUpdateOidcUser(
  params: {
    email: string;
    name: string | null;
    externalIdpId: string;
    clientId: string | null;
    scope: PrincipalScope;
  },
  ctx: ExecutionContext,
  deps: OidcSyncServiceDeps,
): Promise<Result<UserCreated | UserUpdated>> {
  const { email, name, externalIdpId, clientId, scope } = params;

  // Try to find existing user by email
  const existing = await deps.principalRepository.findByEmail(email.toLowerCase());

  if (existing) {
    // Update existing user with latest OIDC info
    const updated = updatePrincipal(existing, {
      name: name ?? existing.name,
      scope,
      clientId,
      userIdentity: existing.userIdentity
        ? {
            ...existing.userIdentity,
            externalIdpId,
            idpType: 'OIDC',
            lastLoginAt: new Date(),
          }
        : createUserIdentity({
            email: email.toLowerCase(),
            idpType: 'OIDC',
            externalIdpId,
          }),
    });

    const event = new UserUpdated(ctx, {
      userId: existing.id,
      name: updated.name,
      previousName: existing.name,
    });

    return deps.unitOfWork.commit(updated, event, { _type: 'OidcUserSync', email, externalIdpId });
  }

  // Create new user principal
  const emailDomain = extractEmailDomain(email.toLowerCase());
  const newPrincipal = createUserPrincipal({
    name: name ?? email,
    scope,
    clientId,
    userIdentity: createUserIdentity({
      email: email.toLowerCase(),
      idpType: 'OIDC',
      externalIdpId,
    }),
  });

  const event = new UserCreated(ctx, {
    userId: newPrincipal.id,
    email: email.toLowerCase(),
    emailDomain,
    name: newPrincipal.name,
    scope,
    clientId,
    idpType: IdpType.OIDC,
    isAnchorUser: scope === 'ANCHOR',
  });

  return deps.unitOfWork.commit(newPrincipal, event, {
    _type: 'OidcUserCreate',
    email,
    externalIdpId,
  });
}

/**
 * CRITICAL SECURITY: Synchronize IDP roles with optional domain-level filter.
 *
 * Uses UnitOfWork to ensure RolesAssigned event and audit log are created.
 *
 * Only IDP roles explicitly authorized in idp_role_mappings are accepted.
 * Any unauthorized role is rejected and logged as a security warning.
 *
 * Flow:
 * 1. For each IDP role name from the token:
 *    a. Look up the role in idp_role_mappings
 *    b. If found: Accept and add the mapped internal role name
 *    c. If NOT found: REJECT and log as security warning
 * 2. Apply domain filter if allowedRoleNames is provided
 * 3. Remove all existing IDP-sourced roles from the principal
 * 4. Assign all authorized internal role names with "IDP_SYNC" source
 * 5. Commit via UnitOfWork (creates RolesAssigned event + audit log)
 */
export async function syncIdpRoles(
  principal: Principal,
  idpRoleNames: string[],
  allowedRoleNames: Set<string> | null,
  ctx: ExecutionContext,
  deps: OidcSyncServiceDeps,
): Promise<Result<RolesAssigned>> {
  const authorizedRoleNames = new Set<string>();

  if (!idpRoleNames || idpRoleNames.length === 0) {
    deps.log.info({ principalId: principal.id }, 'No IDP roles provided for principal');
  } else {
    // SECURITY: Only accept IDP roles that are explicitly authorized in idp_role_mappings
    for (const idpRoleName of idpRoleNames) {
      const mapping = await deps.idpRoleMappingRepository.findByIdpRoleName(idpRoleName);

      if (mapping) {
        authorizedRoleNames.add(mapping.internalRoleName);
        deps.log.debug(
          {
            principalId: principal.id,
            idpRole: idpRoleName,
            internalRole: mapping.internalRoleName,
          },
          'Accepted IDP role',
        );
      } else {
        // SECURITY: Reject unauthorized IDP role
        deps.log.warn(
          {
            principalId: principal.id,
            email: principal.userIdentity?.email,
            idpRole: idpRoleName,
          },
          'SECURITY: REJECTED unauthorized IDP role. Role not found in idp_role_mappings table. Platform administrator must explicitly authorize this IDP role before it can be used.',
        );
      }
    }
  }

  // Apply domain filter if provided
  let finalRoleNames = authorizedRoleNames;
  if (allowedRoleNames && allowedRoleNames.size > 0) {
    finalRoleNames = new Set<string>();
    for (const roleName of authorizedRoleNames) {
      if (allowedRoleNames.has(roleName)) {
        finalRoleNames.add(roleName);
      } else {
        deps.log.warn(
          {
            principalId: principal.id,
            email: principal.userIdentity?.email,
            role: roleName,
          },
          "SECURITY: Domain role filter REMOVED IDP-synced role. Role not in email domain mapping's allowedRoleIds.",
        );
      }
    }

    deps.log.info(
      {
        principalId: principal.id,
        authorized: authorizedRoleNames.size,
        passedFilter: finalRoleNames.size,
      },
      'Domain role filter applied',
    );
  }

  // Build new role list: keep non-IDP roles, add authorized IDP roles
  const nonIdpRoles = principal.roles.filter((r) => r.assignmentSource !== 'IDP_SYNC');
  const idpRoleAssignments: RoleAssignment[] = [...finalRoleNames].map((roleName) => ({
    roleName,
    assignmentSource: 'IDP_SYNC',
    assignedAt: new Date(),
  }));

  const allRoles = [...nonIdpRoles, ...idpRoleAssignments];
  const previousRoles = principal.roles.map((r) => r.roleName);

  // Update principal with new role list
  const updated = updatePrincipal(principal, { roles: allRoles });

  const event = new RolesAssigned(ctx, {
    userId: principal.id,
    email: principal.userIdentity?.email ?? '',
    roles: allRoles.map((r) => r.roleName),
    previousRoles,
  });

  deps.log.info(
    {
      principalId: principal.id,
      email: principal.userIdentity?.email,
      provided: idpRoleNames?.length ?? 0,
      authorized: authorizedRoleNames.size,
      assigned: idpRoleAssignments.length,
    },
    'IDP role sync complete',
  );

  return deps.unitOfWork.commit(updated, event, {
    _type: 'OidcIdpRoleSync',
    principalId: principal.id,
  });
}

/**
 * Extract role names from an ID token payload.
 * Checks common claims: realm_access.roles (Keycloak), roles (generic), groups (Entra ID).
 */
export function extractIdpRoles(idTokenPayload: Record<string, unknown>): string[] {
  const roles: string[] = [];

  // Keycloak: realm_access.roles
  const realmAccess = idTokenPayload['realm_access'] as { roles?: string[] } | undefined;
  if (realmAccess?.roles && Array.isArray(realmAccess.roles)) {
    for (const role of realmAccess.roles) {
      if (typeof role === 'string' && !roles.includes(role)) {
        roles.push(role);
      }
    }
  }

  // Generic: roles
  const rolesArray = idTokenPayload['roles'] as string[] | undefined;
  if (Array.isArray(rolesArray)) {
    for (const role of rolesArray) {
      if (typeof role === 'string' && !roles.includes(role)) {
        roles.push(role);
      }
    }
  }

  // Entra ID: groups
  const groups = idTokenPayload['groups'] as string[] | undefined;
  if (Array.isArray(groups)) {
    for (const group of groups) {
      if (typeof group === 'string' && !roles.includes(group)) {
        roles.push(group);
      }
    }
  }

  return roles;
}
