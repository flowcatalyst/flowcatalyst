/**
 * SpecVersion Status
 *
 * - FINALISING: Being prepared, can be modified/deleted
 * - CURRENT: Active, one per major version
 * - DEPRECATED: Superseded but still valid for reading
 */

export type SpecVersionStatus = 'FINALISING' | 'CURRENT' | 'DEPRECATED';

export const SpecVersionStatus = {
  FINALISING: 'FINALISING' as const,
  CURRENT: 'CURRENT' as const,
  DEPRECATED: 'DEPRECATED' as const,
} as const;
