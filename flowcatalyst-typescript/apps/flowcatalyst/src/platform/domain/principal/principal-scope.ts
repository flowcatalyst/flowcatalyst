/**
 * Defines the access scope for a principal (user or service account).
 *
 * This determines which clients the principal can access:
 * - ANCHOR: Platform admin (typically from the anchor domain). Can access all clients.
 * - PARTNER: Partner principals who can access multiple explicitly assigned clients.
 * - CLIENT: Principals bound to a single client (their home client).
 *
 * The scope can be:
 * 1. Derived from email domain configuration (ClientAuthConfig.clientId)
 * 2. Explicitly set on the principal for override cases
 */
export const PrincipalScope = {
  /**
   * Anchor/platform principals - have access to all clients.
   * Typically users from the anchor domain (e.g., flowcatalyst.local).
   */
  ANCHOR: 'ANCHOR',

  /**
   * Partner principals - have access to multiple explicitly assigned clients.
   * Their accessible clients are stored in client access grants.
   */
  PARTNER: 'PARTNER',

  /**
   * Client principals - bound to a single client (their home client).
   * Their clientId determines their access scope.
   */
  CLIENT: 'CLIENT',
} as const;

export type PrincipalScope = (typeof PrincipalScope)[keyof typeof PrincipalScope];
