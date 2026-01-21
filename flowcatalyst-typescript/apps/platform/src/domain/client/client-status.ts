/**
 * Status of a client organization.
 */
export const ClientStatus = {
	/** Client is active and operational */
	ACTIVE: 'ACTIVE',
	/** Client is inactive (see statusReason for details) */
	INACTIVE: 'INACTIVE',
	/** Client is suspended (temporarily disabled) */
	SUSPENDED: 'SUSPENDED',
} as const;

export type ClientStatus = (typeof ClientStatus)[keyof typeof ClientStatus];
