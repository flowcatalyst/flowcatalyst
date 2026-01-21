/**
 * Application Status
 *
 * Status values for applications.
 */

/**
 * Application status values.
 */
export type ApplicationStatus = 'ACTIVE' | 'INACTIVE' | 'DEPRECATED';

/**
 * Application status enum for use in code.
 */
export const ApplicationStatus = {
	ACTIVE: 'ACTIVE' as const,
	INACTIVE: 'INACTIVE' as const,
	DEPRECATED: 'DEPRECATED' as const,
} as const;
