/**
 * Type of principal in the system.
 */
export const PrincipalType = {
	/** Human user account */
	USER: 'USER',
	/** Service account for machine-to-machine authentication */
	SERVICE: 'SERVICE',
} as const;

export type PrincipalType = (typeof PrincipalType)[keyof typeof PrincipalType];
