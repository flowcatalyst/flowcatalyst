/**
 * Query Scope Filtering
 *
 * Helpers to scope database queries based on the current principal's
 * access level. These are used by read endpoints (list, search, filter)
 * to ensure principals only see resources they are authorized to access.
 *
 * For single-resource reads (GET by ID), the route handler should fetch
 * the resource then check access - returning 404 for both "not found"
 * and "not authorized" to avoid leaking existence.
 *
 * For list/search reads, these helpers inject scope filters INTO the query
 * for efficiency (don't fetch all then filter in memory).
 */

import { AuditContext, type PrincipalInfo } from '@flowcatalyst/domain-core';

/**
 * Result of a scope check for queries.
 * - `unrestricted`: Principal can see all resources (ANCHOR scope)
 * - `restricted`: Principal can only see resources matching the filter
 * - `denied`: Principal has no access (shouldn't normally happen after auth)
 */
export type QueryScope<T> =
  | { readonly type: 'unrestricted' }
  | { readonly type: 'restricted'; readonly filter: T }
  | { readonly type: 'denied' };

/**
 * Client scope filter - determines which client IDs the principal can access.
 */
export interface ClientScopeFilter {
  /** Client IDs the principal can access */
  readonly clientIds: string[];
}

/**
 * Get the client scope for the current principal.
 *
 * @returns QueryScope with client ID filter
 */
export function getClientQueryScope(): QueryScope<ClientScopeFilter> {
  const principal = AuditContext.getPrincipal();
  if (!principal) {
    return { type: 'denied' };
  }

  return getClientQueryScopeForPrincipal(principal);
}

/**
 * Get the client scope for a specific principal.
 *
 * - ANCHOR: unrestricted (can see all clients)
 * - PARTNER: restricted to home client + granted clients
 * - CLIENT: restricted to home client only
 */
export function getClientQueryScopeForPrincipal(
  principal: PrincipalInfo,
): QueryScope<ClientScopeFilter> {
  switch (principal.scope) {
    case 'ANCHOR':
      return { type: 'unrestricted' };

    case 'PARTNER': {
      const clientIds = principal.clientId ? [principal.clientId] : [];
      // TODO: Add explicitly granted client IDs from client access grants
      return { type: 'restricted', filter: { clientIds } };
    }

    case 'CLIENT': {
      if (!principal.clientId) {
        return { type: 'denied' };
      }
      return { type: 'restricted', filter: { clientIds: [principal.clientId] } };
    }

    default:
      return { type: 'denied' };
  }
}

/**
 * Check if a principal can access a specific resource by client ID.
 * Returns true for unrestricted access, checks filter for restricted.
 * Use for single-resource reads after fetching.
 *
 * @param clientId - The client ID of the resource (null for unscoped resources)
 */
export function canAccessResourceByClient(clientId: string | null): boolean {
  if (!clientId) {
    // Unscoped resource, allow
    return true;
  }

  const scope = getClientQueryScope();

  switch (scope.type) {
    case 'unrestricted':
      return true;
    case 'restricted':
      return scope.filter.clientIds.includes(clientId);
    case 'denied':
      return false;
  }
}

/**
 * Get accessible client IDs for the current principal, or null if unrestricted.
 * Convenience function for injecting into repository queries.
 *
 * @returns Array of accessible client IDs, or null for unrestricted access
 */
export function getAccessibleClientIds(): string[] | null {
  const scope = getClientQueryScope();

  switch (scope.type) {
    case 'unrestricted':
      return null; // No filter needed
    case 'restricted':
      return scope.filter.clientIds;
    case 'denied':
      return []; // Empty array = no results
  }
}
